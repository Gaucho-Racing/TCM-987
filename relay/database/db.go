package database

import (
	"fmt"
	"relay/config"
	"relay/model"
	"relay/utils"

	"github.com/glebarez/sqlite"
	cmap "github.com/orcaman/concurrent-map/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func InitializeDB() {
	// WAL + NORMAL is the SD-card-friendly durability point: crash-safe,
	// no fsync per commit. busy_timeout covers cross-process access from
	// shelter (relay and shelter share this file).
	dsn := fmt.Sprintf(
		"file:%s?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)",
		config.DatabasePath,
	)

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: gormLogger(),
	})
	if err != nil {
		utils.SugarLogger.Fatalf("[DB] Failed to open database at %s: %v", config.DatabasePath, err)
	}

	utils.SugarLogger.Infoln("[DB] Connected to database")

	if err := db.AutoMigrate(&model.P987Message{}, &model.Ping{}); err != nil {
		utils.SugarLogger.Fatalln("[DB] AutoMigration failed:", err)
	}

	utils.SugarLogger.Infoln("[DB] AutoMigration complete")
	DB = db
}

func gormLogger() logger.Interface {
	if config.Env == "DEV" {
		return logger.Default.LogMode(logger.Warn)
	}
	return logger.Default.LogMode(logger.Error)
}

func InitializeMap() {
	config.LastLocalPublish = cmap.New[uint64]()
	config.LastCloudPublish = cmap.New[uint64]()
}
