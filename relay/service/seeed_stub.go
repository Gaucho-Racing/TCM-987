//go:build !linux

package service

import (
	"relay/config"
	"relay/utils"
)

func StartSeeedCAN() {
	if len(config.SeeedCANPorts) > 0 {
		utils.SugarLogger.Warnln("Seeed USB-CAN requires linux; ignoring SEEED_CAN_PORTS")
	}
}
