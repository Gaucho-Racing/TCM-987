package mqtt

import (
	"fmt"
	"relay/config"
	"relay/utils"
	"sync"
	"sync/atomic"
	"time"

	mq "github.com/eclipse/paho.mqtt.golang"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Client is the local broker connection (nil if LOCAL_MQTT_HOST is unset —
// there's no in-car consumer without a local broker, so it's optional).
// CloudClient is the cloud broker connection (nil if CLOUD_MQTT_HOST is unset).
var Client mq.Client
var CloudClient mq.Client

var subscribedTopics = make(map[string]mq.MessageHandler)
var subscribedTopicsMu sync.RWMutex

const (
	connectTimeout       = 15 * time.Second
	connectRetryInterval = 5 * time.Second
)

// pahoLogger adapts paho's Logger interface to zap. Without this every
// connection error inside paho goes to its default NOOPLogger — with
// ConnectRetry enabled that means a wrong host, port, or credential
// retries forever in total silence.
type pahoLogger struct {
	log  func(args ...interface{})
	logf func(template string, args ...interface{})
}

func (l pahoLogger) Println(v ...interface{})               { l.log(v...) }
func (l pahoLogger) Printf(format string, v ...interface{}) { l.logf(format, v...) }

func initPahoLogging() {
	// Drop the stacktrace (it is always the same paho internals) and sample
	// to one line per message per minute. An unreachable broker retries on
	// a 5s interval indefinitely, and a car parked out of coverage would
	// otherwise write that error to the SD card ~17k times a day.
	log := utils.Logger.WithOptions(
		zap.AddStacktrace(zapcore.FatalLevel),
		zap.WrapCore(func(c zapcore.Core) zapcore.Core {
			return zapcore.NewSamplerWithOptions(c, time.Minute, 1, 0)
		}),
	).Sugar()

	mq.ERROR = pahoLogger{log.Errorln, log.Errorf}
	mq.CRITICAL = pahoLogger{log.Errorln, log.Errorf}
	mq.WARN = pahoLogger{log.Warnln, log.Warnf}
}

func InitializeMQTT() {
	initPahoLogging()

	// A configured local broker must be reachable at startup — fatal if
	// not. The container restart policy is our retry mechanism for the
	// local hop.
	if config.LocalMQTTHost != "" {
		Client = newClient(
			"local",
			config.LocalMQTTHost, config.LocalMQTTPort,
			config.LocalMQTTUser, config.LocalMQTTPassword,
			false,
		)
		token := Client.Connect()
		if !token.WaitTimeout(connectTimeout) {
			utils.SugarLogger.Fatalf("[MQ][local] Connect to %s:%s timed out after %s", config.LocalMQTTHost, config.LocalMQTTPort, connectTimeout)
		}
		if err := token.Error(); err != nil {
			utils.SugarLogger.Fatalln("[MQ][local] Failed to connect:", err)
		}
		localQueue = startQueue("local", Client)
	} else {
		utils.SugarLogger.Infoln("[MQ][local] LOCAL_MQTT_HOST unset, local publish disabled")
	}

	// Cloud broker is best-effort — retry forever in background so we don't
	// block startup on cloud reachability.
	if config.CloudMQTTHost != "" {
		CloudClient = newClient(
			"cloud",
			config.CloudMQTTHost, config.CloudMQTTPort,
			config.CloudMQTTUser, config.CloudMQTTPassword,
			true,
		)
		cloudQueue = startQueue("cloud", CloudClient)
		CloudClient.Connect()
	} else {
		utils.SugarLogger.Infoln("[MQ][cloud] CLOUD_MQTT_HOST unset, cloud publish disabled")
	}
}

func newClient(label, host, port, user, password string, connectRetry bool) mq.Client {
	opts := mq.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%s", host, port))
	opts.SetUsername(user)
	opts.SetPassword(password)
	opts.SetAutoReconnect(true)
	opts.SetClientID(fmt.Sprintf("%s-tcm-%s-%06d", config.VehicleID, label, time.Now().UnixNano()%1000000))
	opts.SetOnConnectHandler(onConnectFn(label))
	opts.SetConnectionLostHandler(onConnectionLostFn(label))
	opts.SetReconnectingHandler(onReconnectFn(label))
	opts.SetMaxReconnectInterval(30 * time.Second)
	opts.SetConnectTimeout(connectTimeout)
	opts.SetOrderMatters(false)
	// ConnectRetry retries the initial connection (paho's AutoReconnect only
	// kicks in after a successful first connect).
	if connectRetry {
		opts.SetConnectRetry(true)
		opts.SetConnectRetryInterval(connectRetryInterval)
	}
	return mq.NewClient(opts)
}

// publishQueueDepth bounds the per-broker backlog. paho's Publish blocks
// once its own outbound buffer fills, so a slow-but-connected uplink
// (cellular) would otherwise stall whichever goroutine called it — on the
// CAN read path that means kernel-side frame loss.
const publishQueueDepth = 4096

// dropReportInterval is how often a broker reports the publishes it shed
// while its queue was full. Per-drop logging would itself become the
// bottleneck at full bus rate.
const dropReportInterval = 10 * time.Second

type publishJob struct {
	topic    string
	qos      byte
	retained bool
	payload  []byte
}

// publishQueue serializes publishes to one broker on a single worker.
// Brokers get independent queues so a stalled cloud uplink can't block
// the local broker that the in-car consumers read from.
type publishQueue struct {
	label   string
	client  mq.Client
	jobs    chan publishJob
	dropped atomic.Uint64
	wg      sync.WaitGroup
}

var localQueue *publishQueue
var cloudQueue *publishQueue

func startQueue(label string, client mq.Client) *publishQueue {
	q := &publishQueue{
		label:  label,
		client: client,
		jobs:   make(chan publishJob, publishQueueDepth),
	}
	q.wg.Add(1)
	go q.worker()
	return q
}

func (q *publishQueue) worker() {
	defer q.wg.Done()

	ticker := time.NewTicker(dropReportInterval)
	defer ticker.Stop()

	for {
		select {
		case job, ok := <-q.jobs:
			if !ok {
				q.reportDrops()
				return
			}
			// Skip while disconnected so we don't queue into a paho client
			// that's mid-(re)connect. Durability comes from the local
			// database, not from MQTT — QoS 0 fire-and-forget everywhere.
			if q.client.IsConnected() {
				q.client.Publish(job.topic, job.qos, job.retained, job.payload)
			}
		case <-ticker.C:
			q.reportDrops()
		}
	}
}

func (q *publishQueue) reportDrops() {
	if n := q.dropped.Swap(0); n > 0 {
		utils.SugarLogger.Warnf("[MQ][%s] Queue full, dropped %d publishes", q.label, n)
	}
}

func (q *publishQueue) enqueue(topic string, qos byte, retained bool, payload []byte) {
	select {
	case q.jobs <- publishJob{topic: topic, qos: qos, retained: retained, payload: payload}:
	default:
		q.dropped.Add(1)
	}
}

// LocalEnabled and CloudEnabled report whether a broker is configured, so
// callers can skip building a payload nobody will consume.
func LocalEnabled() bool { return localQueue != nil }

func CloudEnabled() bool { return cloudQueue != nil }

// Publish sends payload to both brokers (best-effort). Use this for
// low-rate telemetry that doesn't need independent per-broker throttling
// (pings, status, resources).
func Publish(topic string, qos byte, retained bool, payload []byte) {
	PublishLocal(topic, qos, retained, payload)
	PublishCloud(topic, qos, retained, payload)
}

func PublishLocal(topic string, qos byte, retained bool, payload []byte) {
	if localQueue == nil {
		return
	}
	localQueue.enqueue(topic, qos, retained, payload)
}

func PublishCloud(topic string, qos byte, retained bool, payload []byte) {
	if cloudQueue == nil {
		return
	}
	cloudQueue.enqueue(topic, qos, retained, payload)
}

// Disconnect drains both publish queues and closes the broker
// connections. Safe to call with either broker unconfigured.
func Disconnect() {
	for _, q := range []*publishQueue{localQueue, cloudQueue} {
		if q == nil {
			continue
		}
		close(q.jobs)
		q.wg.Wait()
		if q.client.IsConnected() {
			q.client.Disconnect(250)
		}
		utils.SugarLogger.Infof("[MQ][%s] Disconnected", q.label)
	}
	localQueue, cloudQueue = nil, nil
}

// Subscribe subscribes on the cloud broker only — there's nothing useful
// for the relay to consume locally. No-op if cloud isn't configured.
func Subscribe(topic string, handler mq.MessageHandler) {
	if CloudClient == nil {
		utils.SugarLogger.Warnf("[MQ][cloud] Cannot subscribe to %s: CLOUD_MQTT_HOST not configured", topic)
		return
	}
	subscribedTopicsMu.Lock()
	subscribedTopics[topic] = handler
	subscribedTopicsMu.Unlock()

	if token := CloudClient.Subscribe(topic, 0, handler); token.Wait() && token.Error() != nil {
		// Not fatal — onConnect will resubscribe once the broker is reachable.
		utils.SugarLogger.Warnf("[MQ][cloud] Failed to subscribe to %s: %v", topic, token.Error())
		return
	}
	utils.SugarLogger.Infof("[MQ][cloud] Subscribed to topic: %s", topic)
}

func onConnectFn(label string) mq.OnConnectHandler {
	return func(client mq.Client) {
		utils.SugarLogger.Infof("[MQ][%s] Connected to broker", label)
		if label != "cloud" {
			return
		}
		// Copy under the lock: Subscribe can be registering a handler from
		// another goroutine while the connection completes, and resubscribing
		// holds the token wait for as long as the broker takes to ack.
		subscribedTopicsMu.RLock()
		topics := make(map[string]mq.MessageHandler, len(subscribedTopics))
		for topic, handler := range subscribedTopics {
			topics[topic] = handler
		}
		subscribedTopicsMu.RUnlock()

		for topic, handler := range topics {
			if token := client.Subscribe(topic, 0, handler); token.Wait() && token.Error() != nil {
				utils.SugarLogger.Errorf("[MQ][cloud] Failed to resubscribe to %s: %v", topic, token.Error())
				continue
			}
			utils.SugarLogger.Infof("[MQ][cloud] Resubscribed to topic: %s", topic)
		}
	}
}

func onConnectionLostFn(label string) mq.ConnectionLostHandler {
	return func(client mq.Client, err error) {
		utils.SugarLogger.Errorf("[MQ][%s] Connection lost: %v", label, err)
	}
}

func onReconnectFn(label string) mq.ReconnectHandler {
	return func(client mq.Client, opts *mq.ClientOptions) {
		utils.SugarLogger.Infof("[MQ][%s] Reconnecting...", label)
	}
}
