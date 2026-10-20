package utils

import (
	"errors"
	"fmt"
	"path/filepath"
	"relay/config"
	"strconv"
	"strings"
	"time"
)

func VerifyConfig() {
	if config.VehicleID == "" {
		SugarLogger.Fatalln("VEHICLE_ID is not set")
	}
	if err := validTopicSegment(config.VehicleID); err != nil {
		SugarLogger.Fatalf("VEHICLE_ID is not usable in a topic: %v", err)
	}
	key, err := strconv.Atoi(config.VehicleUploadKeyString)
	if err != nil {
		SugarLogger.Fatalln("VEHICLE_UPLOAD_KEY is not a number")
	}
	if key < 0 || key > 65535 {
		SugarLogger.Fatalln("VEHICLE_UPLOAD_KEY is not a valid unsigned 16-bit integer")
	}
	config.VehicleUploadKey = uint16(key)

	if config.DatabasePath == "" {
		config.DatabasePath = "relay.db"
	}

	config.LocalPublishIntervalInt = parseIntWithFallback(config.LocalPublishInterval, 20, "LOCAL_PUBLISH_INTERVAL")
	config.CloudPublishIntervalInt = parseIntWithFallback(config.CloudPublishInterval, 100, "CLOUD_PUBLISH_INTERVAL")
	pingMs := parseIntWithFallback(config.PingIntervalRaw, 5000, "PING_INTERVAL")
	config.PingInterval = time.Duration(pingMs) * time.Millisecond

	config.DBQueueSize = parseIntWithFallback(config.DBQueueSizeRaw, 50000, "DB_QUEUE_SIZE")
	config.DBBatchSize = parseIntWithFallback(config.DBBatchSizeRaw, 5000, "DB_BATCH_SIZE")
	config.RetentionHours = parseIntWithFallback(config.RetentionHoursRaw, 72, "RETENTION_HOURS")

	for _, iface := range config.CANInterfaces {
		if err := validTopicSegment(iface.Label); err != nil {
			SugarLogger.Fatalf("CAN_INTERFACES bus label %q is not usable in a topic: %v", iface.Label, err)
		}
	}
	config.SeeedCANBitrate = parseIntWithFallback(config.SeeedCANBitrateRaw, 500000, "SEEED_CAN_BITRATE")
	config.SeeedSerialBaud = parseIntWithFallback(config.SeeedSerialBaudRaw, 2000000, "SEEED_SERIAL_BAUD")
	if config.SeeedFrameType == "" {
		config.SeeedFrameType = "STD"
	}
	if len(config.SeeedCANPorts) > 0 {
		if _, supported := config.SeeedSpeedCodes[config.SeeedCANBitrate]; !supported {
			SugarLogger.Fatalf("Unsupported SEEED_CAN_BITRATE: %d", config.SeeedCANBitrate)
		}
		if config.SeeedSerialBaud <= 0 || config.SeeedSerialBaud > 3000000 {
			SugarLogger.Fatalln("SEEED_SERIAL_BAUD must be between 1 and 3000000")
		}
		if config.SeeedFrameType != "STD" && config.SeeedFrameType != "EXT" {
			SugarLogger.Fatalln("SEEED_FRAME_TYPE must be STD or EXT")
		}
	}
	seenPorts := make(map[string]bool)
	for _, port := range config.SeeedCANPorts {
		if !filepath.IsAbs(port.Name) || seenPorts[port.Name] {
			SugarLogger.Fatalf("SEEED_CAN_PORTS requires unique absolute device paths: %q", port.Name)
		}
		seenPorts[port.Name] = true
		if err := validTopicSegment(port.Label); err != nil {
			SugarLogger.Fatalf("SEEED_CAN_PORTS bus label %q is not usable in a topic: %v", port.Label, err)
		}
	}

	if config.LocalMQTTHost != "" && config.LocalMQTTPort == "" {
		SugarLogger.Fatalln("LOCAL_MQTT_HOST is set but LOCAL_MQTT_PORT is not")
	}
	if config.CloudMQTTHost != "" && config.CloudMQTTPort == "" {
		SugarLogger.Fatalln("CLOUD_MQTT_HOST is set but CLOUD_MQTT_PORT is not")
	}

	if len(config.CANInterfaces) == 0 && len(config.SeeedCANPorts) == 0 && len(config.VirtualCANPorts) == 0 {
		SugarLogger.Warnln("No CAN_INTERFACES, SEEED_CAN_PORTS or VIRTUAL_CAN_PORTS configured — relay has no frame sources")
	}

	SugarLogger.Infof("Vehicle ID: %s", config.VehicleID)
	SugarLogger.Infof("Vehicle Upload Key: %d", config.VehicleUploadKey)
	// Resolved, not raw: the default is relative and lands wherever the
	// workdir points (inside /data in the image), which is not obvious
	// from the configured value alone.
	SugarLogger.Infof("Database Path: %s", resolvedPath(config.DatabasePath))
	for _, iface := range config.CANInterfaces {
		SugarLogger.Infof("CAN Interface: %s (bus %s)", iface.Name, iface.Label)
	}
	for _, port := range config.SeeedCANPorts {
		SugarLogger.Infof("Seeed CAN: %s (bus %s, CAN %d bps, serial %d baud, %s, listen-only)", port.Name, port.Label, config.SeeedCANBitrate, config.SeeedSerialBaud, config.SeeedFrameType)
	}
	SugarLogger.Infof("Local Broker: %s", brokerEndpoint(config.LocalMQTTHost, config.LocalMQTTPort))
	SugarLogger.Infof("Cloud Broker: %s", brokerEndpoint(config.CloudMQTTHost, config.CloudMQTTPort))
	SugarLogger.Infof("Local Publish Interval: %dms", config.LocalPublishIntervalInt)
	SugarLogger.Infof("Cloud Publish Interval: %dms", config.CloudPublishIntervalInt)
	SugarLogger.Infof("Ping Interval: %s", config.PingInterval)
}

// validTopicSegment rejects values that would change the shape of a
// published topic. Mapache's ingest requires exactly four segments and
// reads the vehicle from segment 1, so a "/" here shifts every field and
// the ingest drops the message — silently, since we publish at QoS 0 and
// never learn it was rejected. MQTT also forbids wildcards in a topic
// being published to.
func validTopicSegment(s string) error {
	if s == "" {
		return errors.New("must not be empty")
	}
	for _, bad := range []string{"/", "+", "#"} {
		if strings.Contains(s, bad) {
			return fmt.Errorf("must not contain %q", bad)
		}
	}
	if strings.TrimSpace(s) != s || strings.ContainsAny(s, " \t\r\n") {
		return errors.New("must not contain whitespace")
	}
	return nil
}

func resolvedPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

func brokerEndpoint(host, port string) string {
	if host == "" {
		return "disabled"
	}
	return host + ":" + port
}

func parseIntWithFallback(raw string, fallback int, name string) int {
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		SugarLogger.Errorf("%s is not a number, using %d: %v", name, fallback, err)
		return fallback
	}
	return v
}
