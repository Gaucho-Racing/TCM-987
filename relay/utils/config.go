package utils

import (
	"relay/config"
	"strconv"
	"time"
)

func VerifyConfig() {
	if config.VehicleID == "" {
		SugarLogger.Fatalln("VEHICLE_ID is not set")
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

	if len(config.CANInterfaces) == 0 && len(config.VirtualCANPorts) == 0 {
		SugarLogger.Warnln("No CAN_INTERFACES or VIRTUAL_CAN_PORTS configured — relay has no frame sources")
	}

	SugarLogger.Infof("Vehicle ID: %s", config.VehicleID)
	SugarLogger.Infof("Vehicle Upload Key: %d", config.VehicleUploadKey)
	SugarLogger.Infof("Database Path: %s", config.DatabasePath)
	for _, iface := range config.CANInterfaces {
		SugarLogger.Infof("CAN Interface: %s (bus %s)", iface.Name, iface.Label)
	}
	SugarLogger.Infof("Local Publish Interval: %dms", config.LocalPublishIntervalInt)
	SugarLogger.Infof("Cloud Publish Interval: %dms", config.CloudPublishIntervalInt)
	SugarLogger.Infof("Ping Interval: %s", config.PingInterval)
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
