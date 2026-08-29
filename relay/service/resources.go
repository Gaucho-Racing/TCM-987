package service

import (
	"encoding/binary"
	"errors"
	"fmt"
	"relay/config"
	"relay/model"
	"relay/mqtt"
	"relay/utils"
	"time"
)

var errResourcesUnsupported = errors.New("resource metrics unsupported on this platform")

func InitializeResourceQuery() {
	go func() {
		for {
			metrics, err := QueryResourceMetrics()
			if errors.Is(err, errResourcesUnsupported) {
				utils.SugarLogger.Warnln("Resource metrics unsupported on this platform, disabling")
				return
			}
			if err != nil {
				utils.SugarLogger.Errorf("Error querying resource metrics: %v", err)
			} else {
				utils.SugarLogger.Infof("Resource metrics: %+v", metrics)
				PublishResources(metrics)
			}
			time.Sleep(10 * time.Second)
		}
	}()
}

// resourcePayloadSize is the 0x201 data length:
//
//	[0:12]  4 × (freq u16 LE MHz, util u8 %)
//	[12]    cpu_total_util u8 %
//	[13:15] ram_total  u16 LE MB
//	[15:17] ram_used   u16 LE MB
//	[17]    ram_util   u8 %
//	[18:22] disk_total u32 LE MB
//	[22:26] disk_used  u32 LE MB
//	[26]    disk_util  u8 %
//	[27]    cpu_temp   u8 °C
//	[28]    throttle_flags u8
const resourcePayloadSize = 29

func encodeResourcePayload(m model.ResourceMetrics) []byte {
	data := make([]byte, resourcePayloadSize)

	off := 0
	for i := 0; i < model.ReportedCPUs; i++ {
		binary.LittleEndian.PutUint16(data[off:off+2], clampU16(m.CPUFreq[i]))
		data[off+2] = clampU8(m.CPUUtil[i])
		off += 3
	}

	data[off] = clampU8(m.CPUTotalUtil)
	off++
	binary.LittleEndian.PutUint16(data[off:off+2], clampU16(m.RAMTotal))
	off += 2
	binary.LittleEndian.PutUint16(data[off:off+2], clampU16(m.RAMUsed))
	off += 2
	data[off] = clampU8(m.RAMUtil)
	off++
	binary.LittleEndian.PutUint32(data[off:off+4], clampU32(m.DiskTotal))
	off += 4
	binary.LittleEndian.PutUint32(data[off:off+4], clampU32(m.DiskUsed))
	off += 4
	data[off] = clampU8(m.DiskUtil)
	off++
	data[off] = clampU8(m.CPUTemp)
	off++
	data[off] = clampU8(m.ThrottleFlags)

	return data
}

// Saturate rather than wrap: a bogus reading should pin the field, not
// alias to a plausible-looking small number on the dashboard.
func clampU8(v int) byte {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return byte(v)
}

func clampU16(v int) uint16 {
	if v < 0 {
		return 0
	}
	if v > 65535 {
		return 65535
	}
	return uint16(v)
}

func clampU32(v int) uint32 {
	if v < 0 {
		return 0
	}
	if v > 4294967295 {
		return 4294967295
	}
	return uint32(v)
}

func PublishResources(metrics model.ResourceMetrics) {
	topic := fmt.Sprintf("%s/%s/tcm/0x201", config.TopicRoot, config.VehicleID)
	mqtt.Publish(topic, 0, false, encodePayload(uint64(time.Now().UnixMicro()), encodeResourcePayload(metrics)))
}
