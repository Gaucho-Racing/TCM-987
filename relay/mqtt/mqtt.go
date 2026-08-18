package mqtt

import (
	"fmt"
	"relay/config"
	"relay/utils"
	"time"

	mq "github.com/eclipse/paho.mqtt.golang"
)

// Client is the local broker connection (nil if LOCAL_MQTT_HOST is unset —
// there's no in-car consumer without a local broker, so it's optional).
// CloudClient is the cloud broker connection (nil if CLOUD_MQTT_HOST is unset).
var Client mq.Client
var CloudClient mq.Client

var subscribedTopics = make(map[string]mq.MessageHandler)

const (
	connectTimeout       = 15 * time.Second
	connectRetryInterval = 5 * time.Second
)

func InitializeMQTT() {
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

// Publish sends payload to both brokers (best-effort). Use this for
// low-rate telemetry that doesn't need independent per-broker throttling
// (pings, status, resources).
func Publish(topic string, qos byte, retained bool, payload []byte) {
	PublishLocal(topic, qos, retained, payload)
	PublishCloud(topic, qos, retained, payload)
}

func PublishLocal(topic string, qos byte, retained bool, payload []byte) {
	if Client == nil {
		return
	}
	publishOne(Client, topic, qos, retained, payload)
}

func PublishCloud(topic string, qos byte, retained bool, payload []byte) {
	if CloudClient == nil {
		return
	}
	publishOne(CloudClient, topic, qos, retained, payload)
}

func publishOne(client mq.Client, topic string, qos byte, retained bool, payload []byte) {
	// Skip while disconnected so we don't queue into a paho client that's
	// mid-(re)connect. Durability comes from the local database + shelter,
	// not from MQTT — QoS 0 fire-and-forget everywhere.
	if !client.IsConnected() {
		return
	}
	client.Publish(topic, qos, retained, payload)
}

// Subscribe subscribes on the cloud broker only — there's nothing useful
// for the relay to consume locally. No-op if cloud isn't configured.
func Subscribe(topic string, handler mq.MessageHandler) {
	if CloudClient == nil {
		utils.SugarLogger.Warnf("[MQ][cloud] Cannot subscribe to %s: CLOUD_MQTT_HOST not configured", topic)
		return
	}
	subscribedTopics[topic] = handler
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
		for topic, handler := range subscribedTopics {
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
