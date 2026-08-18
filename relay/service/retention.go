package service

import (
	"relay/config"
	"relay/database"
	"relay/model"
	"relay/utils"
	"time"
)

const retentionCheckInterval = 1 * time.Hour

// InitializeRetention purges rows shelter has already uploaded (synced != 0)
// once they age past RETENTION_HOURS, plus old ping rows. Without this the
// database grows unbounded — a road car accumulates data indefinitely,
// unlike a race weekend.
func InitializeRetention() {
	if config.RetentionHours <= 0 {
		utils.SugarLogger.Infoln("[RET] RETENTION_HOURS <= 0, purge disabled")
		return
	}
	utils.SugarLogger.Infof("[RET] Purging synced rows older than %dh", config.RetentionHours)
	go func() {
		for {
			purgeExpired()
			time.Sleep(retentionCheckInterval)
		}
	}()
}

func purgeExpired() {
	cutoff := time.Now().Add(-time.Duration(config.RetentionHours) * time.Hour).UnixMicro()

	result := database.DB.Where("synced != 0 AND timestamp < ?", cutoff).Delete(&model.P987Message{})
	if result.Error != nil {
		utils.SugarLogger.Errorf("[RET] Failed to purge messages: %v", result.Error)
	} else if result.RowsAffected > 0 {
		utils.SugarLogger.Infof("[RET] Purged %d synced messages", result.RowsAffected)
	}

	result = database.DB.Where("ping < ?", cutoff).Delete(&model.Ping{})
	if result.Error != nil {
		utils.SugarLogger.Errorf("[RET] Failed to purge pings: %v", result.Error)
	} else if result.RowsAffected > 0 {
		utils.SugarLogger.Infof("[RET] Purged %d pings", result.RowsAffected)
	}
}
