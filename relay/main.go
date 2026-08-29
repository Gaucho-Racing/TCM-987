package main

import (
	"os"
	"os/signal"
	"relay/config"
	"relay/database"
	"relay/mqtt"
	"relay/service"
	"relay/utils"
	"syscall"
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
	service.StartSocketCAN()

	// Park until the container is stopped, then flush the write queue
	// before exiting — a full batch is DB_BATCH_SIZE frames that would
	// otherwise never reach the disk.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	sig := <-stop
	utils.SugarLogger.Infof("Received %s, shutting down", sig)

	service.StopDBQueue()
	mqtt.Disconnect()
	database.Close()
}
