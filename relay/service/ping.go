package service

import (
	"encoding/binary"
	"fmt"
	"relay/config"
	"relay/database"
	"relay/model"
	"relay/mqtt"
	"relay/utils"
	"time"

	mq "github.com/eclipse/paho.mqtt.golang"
)

func InitializePings() {
	go SubscribePong()
	go func() {
		for {
			PublishPing()
			time.Sleep(config.PingInterval)
		}
	}()
	go func() {
		warnAfter := config.PingInterval * 2
		for {
			lastPing := FindLastSuccessfulPing()
			ageMs := time.Now().UnixMilli() - int64(lastPing.Ping)
			if ageMs > warnAfter.Milliseconds() {
				utils.SugarLogger.Warnf("Last successful ping was %.2fs ago", float64(ageMs)/1000)
			}
			time.Sleep(2345 * time.Millisecond)
		}
	}()
}

func SubscribePong() {
	topic := fmt.Sprintf("%s/%s/tcm/pong", config.TopicRoot, config.VehicleID)
	mqtt.Subscribe(topic, func(client mq.Client, msg mq.Message) {
		if len(msg.Payload()) < 16 {
			return
		}
		ping := binary.BigEndian.Uint64(msg.Payload()[:8])
		pong := binary.BigEndian.Uint64(msg.Payload()[8:])
		now := time.Now()
		uploadLatency := now.UnixMicro() - int64(ping)

		// Cache freshness + latency in the shared state so
		// publishTCMStatus reads without a DB round-trip. Clamp to u16
		// to match the wire field width on TCM Status.
		latencyMs := uploadLatency / 1000
		if latencyMs < 0 {
			latencyMs = 0
		}
		if latencyMs > 65535 {
			latencyMs = 65535
		}
		state.setPong(now, uint16(latencyMs))

		go UpdatePong(int(ping), int(pong), int(uploadLatency))
		utils.SugarLogger.Infof("[MQ] Received pong in %d ms", latencyMs)
	})
}

func PublishPing() {
	topic := fmt.Sprintf("%s/%s/tcm/ping", config.TopicRoot, config.VehicleID)
	micros := time.Now().UnixMicro()
	go CreatePing(int(micros))
	payload := make([]byte, 10)
	binary.BigEndian.PutUint64(payload[0:8], uint64(micros))
	binary.BigEndian.PutUint16(payload[8:10], config.VehicleUploadKey)
	mqtt.Publish(topic, 0, false, payload)
}

func CreatePing(ping int) {
	result := database.DB.Create(&model.Ping{
		VehicleID: config.VehicleID,
		Ping:      ping,
	})
	if result.Error != nil {
		utils.SugarLogger.Errorln("Failed to create ping:", result.Error)
	}
}

func UpdatePong(ping int, pong int, latency int) {
	result := database.DB.Model(&model.Ping{}).Where("ping = ?", ping).Update("pong", pong).Update("latency", latency)
	if result.Error != nil {
		utils.SugarLogger.Errorln("Failed to update pong:", result.Error)
	}
}

func FindLastSuccessfulPing() model.Ping {
	var ping model.Ping
	database.DB.Where("latency > 0").Order("ping DESC").First(&ping)
	return ping
}
