package config

import (
	"os"
	"strings"
	"time"

	cmap "github.com/orcaman/concurrent-map/v2"
)

var Version = "0.1.0"
var Env = os.Getenv("ENV")

// TopicRoot is the vehicle-generation namespace for all MQTT topics,
// matching the Mapache p987 ingest service's subscription filter.
const TopicRoot = "p987"

// VirtualBusLabel is the topic bus segment for frames that never touched a
// physical CAN bus — TCM housekeeping (0x200/0x201) and anything arriving
// on a virtual CAN port (e.g. shelter's 0x210/0x211). Keeping these in
// their own namespace means their CAN IDs can never collide with the
// car's own arbitration IDs.
const VirtualBusLabel = "tcm"

var VehicleID = os.Getenv("VEHICLE_ID")
var VehicleUploadKeyString = os.Getenv("VEHICLE_UPLOAD_KEY")
var VehicleUploadKey uint16

var DatabasePath = os.Getenv("DATABASE_PATH")

var LocalMQTTHost = os.Getenv("LOCAL_MQTT_HOST")
var LocalMQTTPort = os.Getenv("LOCAL_MQTT_PORT")
var LocalMQTTUser = os.Getenv("LOCAL_MQTT_USER")
var LocalMQTTPassword = os.Getenv("LOCAL_MQTT_PASSWORD")

var CloudMQTTHost = os.Getenv("CLOUD_MQTT_HOST")
var CloudMQTTPort = os.Getenv("CLOUD_MQTT_PORT")
var CloudMQTTUser = os.Getenv("CLOUD_MQTT_USER")
var CloudMQTTPassword = os.Getenv("CLOUD_MQTT_PASSWORD")

// CANInterface maps a socketcan interface to the bus label used as the
// third MQTT topic segment. The label distinguishes physical buses whose
// 11-bit ID spaces are independent (pcan vs kcan), so it participates in
// topics, throttle keys, and the Mapache-side decoder registry.
type CANInterface struct {
	Name  string
	Label string
}

// CANInterfaces parses CAN_INTERFACES, a comma-separated list of
// iface:label pairs ("can0:pcan,can1:kcan"). A bare interface name uses
// itself as the label.
var CANInterfaces = parseInterfaceList(os.Getenv("CAN_INTERFACES"))

func parseInterfaceList(s string) []CANInterface {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]CANInterface, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		name, label, found := strings.Cut(p, ":")
		if !found || label == "" {
			label = name
		}
		out = append(out, CANInterface{Name: name, Label: label})
	}
	return out
}

// VirtualCANPorts is a comma-separated list of UDP ports to listen on for
// synthetic CAN frames produced by on-tcm software services (e.g.
// shelter). Frames arrive in the TCM-26 72-byte wire format and publish
// under VirtualBusLabel.
var VirtualCANPorts = parsePortList(os.Getenv("VIRTUAL_CAN_PORTS"))

func parsePortList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Per-bus-and-CAN-ID publish throttles, in milliseconds. Local and cloud
// are tracked independently so an in-car consumer can run high-rate while
// the cellular uplink stays modest.
var LocalPublishInterval = os.Getenv("LOCAL_PUBLISH_INTERVAL")
var LocalPublishIntervalInt int
var CloudPublishInterval = os.Getenv("CLOUD_PUBLISH_INTERVAL")
var CloudPublishIntervalInt int

var LastLocalPublish = cmap.ConcurrentMap[string, uint64]{}
var LastCloudPublish = cmap.ConcurrentMap[string, uint64]{}

var PingIntervalRaw = os.Getenv("PING_INTERVAL")
var PingInterval time.Duration

// DB queue sizing. The channel buffer is preallocated at startup
// (elemsize × cap), so keep DB_QUEUE_SIZE modest on small boards — the
// default 50k slots ≈ 5 MB and still buffers ~40s of full-bus traffic
// against the 1s flush cadence.
var DBQueueSizeRaw = os.Getenv("DB_QUEUE_SIZE")
var DBQueueSize int
var DBBatchSizeRaw = os.Getenv("DB_BATCH_SIZE")
var DBBatchSize int

// RetentionHours bounds how long synced messages (and old pings) stay in
// the local database before the hourly purge deletes them. <= 0 disables
// purging entirely.
var RetentionHoursRaw = os.Getenv("RETENTION_HOURS")
var RetentionHours int
