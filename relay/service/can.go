package service

import (
	"encoding/binary"
	"fmt"
	"net"
	"relay/config"
	"relay/mqtt"
	"relay/utils"
	"strconv"
	"time"

	cmap "github.com/orcaman/concurrent-map/v2"
)

// PublishData persists a frame and fans it out to both brokers.
//
// Topic format: p987/{vehicle_id}/{bus}/0x{can_id_hex}. The bus label
// replaces GR26's node segment — on stock Porsche CAN the sender ECU is a
// pure function of the arbitration ID, so the only routing info the ID
// can't carry is which physical bus it came from.
//
// MQTT payload: [0:8] timestamp u64 BE µs | [8:10] upload key u16 BE |
// [10:] raw CAN data. QoS 0, retain=false — durability comes from the
// database write, which happens before any throttle check so every frame
// is persisted regardless of connectivity.
func PublishData(busLabel string, canID uint32, data []byte) {
	topic := fmt.Sprintf("%s/%s/%s/0x%03x", config.TopicRoot, config.VehicleID, busLabel, canID)
	timestamp := uint64(time.Now().UnixMicro())

	QueueDBWrite(int(timestamp), config.VehicleID, topic, data, busLabel, "")

	buf := make([]byte, 10+len(data))
	binary.BigEndian.PutUint64(buf[0:8], timestamp)
	binary.BigEndian.PutUint16(buf[8:10], config.VehicleUploadKey)
	copy(buf[10:], data)

	// Throttle key includes the bus label — the same 11-bit ID can exist
	// on two physical buses with independent ID spaces.
	throttleKey := fmt.Sprintf("%s/%d", busLabel, canID)
	if shouldPublishOn(throttleKey, timestamp, config.LastLocalPublish, config.LocalPublishIntervalInt) {
		mqtt.PublishLocal(topic, 0, false, buf)
	}
	if shouldPublishOn(throttleKey, timestamp, config.LastCloudPublish, config.CloudPublishIntervalInt) {
		mqtt.PublishCloud(topic, 0, false, buf)
	}
}

func shouldPublishOn(
	key string,
	ts uint64,
	last cmap.ConcurrentMap[string, uint64],
	intervalMs int,
) bool {
	lastTs, ok := last.Get(key)
	if ok && ts-lastTs <= uint64(intervalMs*1000) {
		return false
	}
	// Update before publishing so a slow MQTT call can't push us past the
	// next interval window.
	last.Set(key, ts)
	return true
}

// ListenVirtualCAN binds a UDP port and dispatches synthetic CAN frames
// from on-tcm software services (e.g. shelter heartbeats) through
// PublishData under the "tcm" bus label. Wire format is TCM-26's 72-byte
// struct:
//
//	[0:4]   CAN ID  u32 LE
//	[4]     bus     u8 (ignored — virtual frames are always bus "tcm")
//	[5]     length  u8 (actual byte count, 0-64)
//	[6:70]  data
//	[70:72] alignment padding
func ListenVirtualCAN(port string) {
	shouldLog := config.Env == "DEV"

	portInt, err := strconv.Atoi(port)
	if err != nil {
		utils.SugarLogger.Fatalf("[VCAN:%s] Failed to convert port to int: %v", port, err)
	}
	addr := net.UDPAddr{
		Port: portInt,
		IP:   net.ParseIP("0.0.0.0"),
	}
	conn, err := net.ListenUDP("udp", &addr)
	if err != nil {
		utils.SugarLogger.Fatalf("[VCAN:%s] Failed to create UDP connection: %v", port, err)
	}
	defer conn.Close()
	utils.SugarLogger.Infof("[VCAN:%s] listening", port)

	for {
		buffer := make([]byte, 1024)
		n, remoteAddr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			utils.SugarLogger.Errorf("[VCAN:%s] Error reading from UDP: %v", port, err)
			continue
		}
		if shouldLog {
			utils.SugarLogger.Infof("[VCAN:%s] Received %d bytes from %s", port, n, remoteAddr.String())
		}

		if n < 72 {
			utils.SugarLogger.Infof("[VCAN:%s] Invalid packet size: expected at least 72 bytes, got %d", port, n)
			continue
		}

		canID := binary.LittleEndian.Uint32(buffer[0:4])
		length := int(buffer[5])
		if length > 64 || length+6 > n {
			utils.SugarLogger.Infof("[VCAN:%s] Payload length %d exceeds packet size %d, skipping", port, length, n)
			continue
		}

		payload := make([]byte, length)
		copy(payload, buffer[6:6+length])

		if shouldLog {
			utils.SugarLogger.Infof("[VCAN:%s] CAN ID: 0x%03x, Length: %d", port, canID, length)
		}

		go PublishData(config.VirtualBusLabel, canID, payload)
	}
}
