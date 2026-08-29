package service

import (
	"encoding/binary"
	"fmt"
	"relay/config"
	"relay/mqtt"
	"relay/utils"
	"time"
)

const (
	tcmStatusConnectionOK = 1 << 0 // generic internet (DNS reachable)
	tcmStatusMQTTOK       = 1 << 1 // cloud broker connected
	tcmStatusMapacheOK    = 1 << 2 // cloud Mapache responding (fresh pong)
	tcmStatusClockOK      = 1 << 3 // local clock is plausible (RTC/NTP synced)
)

// mapachePongFreshness derives from PING_INTERVAL: 2× allows a single
// missed ping, +5s slack covers jitter and RTT variance.
func mapachePongFreshness() time.Duration {
	return config.PingInterval*2 + 5*time.Second
}

// InitializeTCMStatus publishes a TCM Status (0x200) message every 5s
// summarizing connectivity. The publish path does no I/O — every bit
// comes from the shared tcmState.
func InitializeTCMStatus() {
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			publishTCMStatus()
		}
	}()
}

func publishTCMStatus() {
	inet, mqttOK, clock, lastPongAt, lastPongRTT := state.snapshot()
	mapacheOK := !lastPongAt.IsZero() && time.Since(lastPongAt) < mapachePongFreshness()

	bits := statusBits(inet, mqttOK, mapacheOK, clock)

	topic := fmt.Sprintf("%s/%s/tcm/0x200", config.TopicRoot, config.VehicleID)
	mqtt.Publish(topic, 0, false, encodePayload(uint64(time.Now().UnixMicro()), encodeTCMStatus(bits, lastPongRTT)))
	utils.SugarLogger.Debugf("[TCM] published status: bits=%08b latency=%dms", bits, lastPongRTT)
}

func statusBits(inet, mqttOK, mapacheOK, clock bool) byte {
	var bits byte
	if inet {
		bits |= tcmStatusConnectionOK
	}
	if mqttOK {
		bits |= tcmStatusMQTTOK
	}
	if mapacheOK {
		bits |= tcmStatusMapacheOK
	}
	if clock {
		bits |= tcmStatusClockOK
	}
	return bits
}

// TCM Status data layout (8 bytes):
//
//	[0]    status_bits
//	[1:3]  mapache_ping (u16, ms, little-endian)
//	[3:8]  reserved
func encodeTCMStatus(bits byte, pingMs uint16) []byte {
	data := make([]byte, 8)
	data[0] = bits
	binary.LittleEndian.PutUint16(data[1:3], pingMs)
	return data
}
