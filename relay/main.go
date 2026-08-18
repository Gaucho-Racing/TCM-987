package main

import (
	"relay/config"
	"relay/database"
	"relay/mqtt"
	"relay/service"
	"relay/utils"
)

func main() {
	config.PrintStartupBanner()
	utils.InitializeLogger()
	defer utils.Logger.Sync()

	utils.VerifyConfig()
	database.InitializeDB()
	database.InitializeMap()
	service.InitDBQueue()
	service.InitializeRetention()
	mqtt.InitializeMQTT()

	service.InitializePings()
	service.InitializeResourceQuery()
	// State watchers must start before the publisher so the first
	// publish has live readings rather than zero defaults.
	service.InitializeTCMState()
	service.InitializeTCMStatus()

	for _, port := range config.VirtualCANPorts {
		go service.ListenVirtualCAN(port)
	}
	service.RunSocketCAN()
}
