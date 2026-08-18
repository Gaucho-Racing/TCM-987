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

// PublishResources keeps TCM-26's 44-byte 0x201 layout so the
// Mapache-side decoder can be shared across both TCMs.
func PublishResources(metrics model.ResourceMetrics) {
	topic := fmt.Sprintf("%s/%s/tcm/0x201", config.TopicRoot, config.VehicleID)

	dataPayload := make([]byte, 44)
	binary.LittleEndian.PutUint16(dataPayload[:2], uint16(metrics.CPU0Freq))
	dataPayload[2] = byte(metrics.CPU0Util)
	binary.LittleEndian.PutUint16(dataPayload[3:5], uint16(metrics.CPU1Freq))
	dataPayload[5] = byte(metrics.CPU1Util)
	binary.LittleEndian.PutUint16(dataPayload[6:8], uint16(metrics.CPU2Freq))
	dataPayload[8] = byte(metrics.CPU2Util)
	binary.LittleEndian.PutUint16(dataPayload[9:11], uint16(metrics.CPU3Freq))
	dataPayload[11] = byte(metrics.CPU3Util)
	binary.LittleEndian.PutUint16(dataPayload[12:14], uint16(metrics.CPU4Freq))
	dataPayload[14] = byte(metrics.CPU4Util)
	binary.LittleEndian.PutUint16(dataPayload[15:17], uint16(metrics.CPU5Freq))
	dataPayload[17] = byte(metrics.CPU5Util)
	dataPayload[18] = byte(metrics.CPUTotalUtil)
	binary.LittleEndian.PutUint16(dataPayload[19:21], uint16(metrics.RAMTotal))
	binary.LittleEndian.PutUint16(dataPayload[21:23], uint16(metrics.RAMUsed))
	dataPayload[23] = byte(metrics.RAMUtil)
	dataPayload[24] = byte(metrics.GPUUtil)
	binary.LittleEndian.PutUint16(dataPayload[25:27], uint16(metrics.GPUFreq))
	binary.LittleEndian.PutUint32(dataPayload[27:31], uint32(metrics.DiskTotal))
	binary.LittleEndian.PutUint32(dataPayload[31:35], uint32(metrics.DiskUsed))
	dataPayload[35] = byte(metrics.DiskUtil)
	dataPayload[36] = byte(metrics.CPUTemp)
	dataPayload[37] = byte(metrics.GPUTemp)
	binary.LittleEndian.PutUint16(dataPayload[38:40], uint16(metrics.VoltageDraw))
	binary.LittleEndian.PutUint16(dataPayload[40:42], uint16(metrics.CurrentDraw))
	binary.LittleEndian.PutUint16(dataPayload[42:44], uint16(metrics.PowerDraw))

	payload := make([]byte, 10, 54)
	binary.BigEndian.PutUint64(payload[0:8], uint64(time.Now().UnixMicro()))
	binary.BigEndian.PutUint16(payload[8:10], config.VehicleUploadKey)
	payload = append(payload, dataPayload...)

	mqtt.Publish(topic, 0, false, payload)
}
