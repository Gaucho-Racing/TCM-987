package database

import (
	"fmt"
	"log"
	"os"
	"relay/config"
	"relay/model"
	"relay/utils"
	"time"

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

// Close releases the SQLite handle so WAL checkpointing completes before
// the process exits.
func Close() {
	if DB == nil {
		return
	}
	sqlDB, err := DB.DB()
	if err != nil {
		utils.SugarLogger.Errorf("[DB] Failed to get underlying handle: %v", err)
		return
	}
	if err := sqlDB.Close(); err != nil {
		utils.SugarLogger.Errorf("[DB] Failed to close: %v", err)
		return
	}
	utils.SugarLogger.Infoln("[DB] Closed")
}

// gormLogger silences "record not found", which is a normal result here —
// the ping watchdog polls for a successful pong every few seconds and gets
// none until the car first reaches Mapache. Left at the default it writes
// a colorized SQL dump to the SD card on every poll, forever, while the
// car is offline.
func gormLogger() logger.Interface {
	level := logger.Error
	if config.Env == "DEV" {
		level = logger.Warn
	}
	return logger.New(log.New(os.Stdout, "", log.LstdFlags), logger.Config{
		SlowThreshold:             200 * time.Millisecond,
		LogLevel:                  level,
		IgnoreRecordNotFoundError: true,
		Colorful:                  config.Env == "DEV",
	})
}

func InitializeMap() {
	config.LastLocalPublish = cmap.New[uint64]()
	config.LastCloudPublish = cmap.New[uint64]()
}
