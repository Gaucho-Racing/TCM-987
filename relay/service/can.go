package service

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"relay/config"
	"relay/mqtt"
	"relay/utils"
	"strconv"
	"time"

	cmap "github.com/orcaman/concurrent-map/v2"
)

// encodePayload builds the Mapache wire format shared by every message the
// relay publishes: [0:8] timestamp u64 BE µs | [8:10] upload key u16 BE |
// [10:] message data.
func encodePayload(timestamp uint64, data []byte) []byte {
	buf := make([]byte, 10+len(data))
	binary.BigEndian.PutUint64(buf[0:8], timestamp)
	binary.BigEndian.PutUint16(buf[8:10], config.VehicleUploadKey)
	copy(buf[10:], data)
	return buf
}

// PublishData persists a frame and fans it out to both brokers.
//
// Topic format: p987/{vehicle_id}/{bus}/0x{can_id_hex}. The bus label
// replaces GR26's node segment — on stock Porsche CAN the sender ECU is a
// pure function of the arbitration ID, so the only routing info the ID
// can't carry is which physical bus it came from.
//
// Runs inline on the caller's goroutine: every step is a channel send or a
// map operation, and both the database write and each broker publish are
// bounded queues drained by their own workers. The database write happens
// before any throttle check so every frame is persisted regardless of
// connectivity.
func PublishData(busLabel string, canID uint32, data []byte) {
	topic := fmt.Sprintf("%s/%s/%s/0x%03x", config.TopicRoot, config.VehicleID, busLabel, canID)
	timestamp := uint64(time.Now().UnixMicro())

	QueueDBWrite(int(timestamp), config.VehicleID, topic, data, busLabel, "")

	// Throttle key includes the bus label — the same 11-bit ID can exist
	// on two physical buses with independent ID spaces.
	throttleKey := fmt.Sprintf("%s/%d", busLabel, canID)
	toLocal := mqtt.LocalEnabled() &&
		shouldPublishOn(throttleKey, timestamp, config.LastLocalPublish, config.LocalPublishIntervalInt)
	toCloud := mqtt.CloudEnabled() &&
		shouldPublishOn(throttleKey, timestamp, config.LastCloudPublish, config.CloudPublishIntervalInt)

	// Most frames are throttled out, and an unconfigured broker consumes
	// none at all — don't pay for the payload until someone wants it.
	if !toLocal && !toCloud {
		return
	}

	buf := encodePayload(timestamp, data)
	if toLocal {
		mqtt.PublishLocal(topic, 0, false, buf)
	}
	if toCloud {
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

// virtualCANFrameSize is TCM-26's UDP wire format for synthetic frames:
//
//	[0:4]   CAN ID  u32 LE
//	[4]     bus     u8 (ignored — virtual frames are always bus "tcm")
//	[5]     length  u8 (actual byte count, 0-64)
//	[6:70]  data
//	[70:72] alignment padding
const virtualCANFrameSize = 72

func parseVirtualCANFrame(packet []byte) (canID uint32, data []byte, err error) {
	if len(packet) < virtualCANFrameSize {
		return 0, nil, fmt.Errorf("invalid packet size: expected at least %d bytes, got %d", virtualCANFrameSize, len(packet))
	}

	canID = binary.LittleEndian.Uint32(packet[0:4])
	length := int(packet[5])
	if length > 64 || 6+length > len(packet) {
		return 0, nil, fmt.Errorf("payload length %d exceeds packet size %d", length, len(packet))
	}

	data = make([]byte, length)
	copy(data, packet[6:6+length])
	return canID, data, nil
}

// ListenVirtualCAN binds a UDP port and dispatches synthetic CAN frames
// from on-tcm software services (e.g. shelter heartbeats) through
// PublishData under the "tcm" bus label.
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

	buffer := make([]byte, 1024)
	for {
		n, remoteAddr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			utils.SugarLogger.Errorf("[VCAN:%s] Error reading from UDP: %v", port, err)
			continue
		}
		if shouldLog {
			utils.SugarLogger.Infof("[VCAN:%s] Received %d bytes from %s", port, n, remoteAddr.String())
		}

		canID, data, err := parseVirtualCANFrame(buffer[:n])
		if err != nil {
			utils.SugarLogger.Infof("[VCAN:%s] %v, skipping", port, err)
			continue
		}

		if shouldLog {
			utils.SugarLogger.Infof("[VCAN:%s] CAN ID: 0x%03x, Length: %d", port, canID, len(data))
		}

		PublishData(config.VirtualBusLabel, canID, data)
	}
}
