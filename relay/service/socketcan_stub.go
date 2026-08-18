//go:build !linux

package service

import (
	"relay/config"
	"relay/utils"
)

func RunSocketCAN() {
	if len(config.CANInterfaces) > 0 {
		utils.SugarLogger.Warnf("socketcan requires linux; ignoring CAN_INTERFACES (%d configured)", len(config.CANInterfaces))
	}
	select {}
}
