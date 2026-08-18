package service

import (
	"relay/config"
	"relay/database"
	"relay/model"
	"relay/utils"
	"sync"
	"time"
)

type DBQueue struct {
	messages  chan model.P987Message
	batchSize int
	flushTime time.Duration
	mu        sync.RWMutex
	stopped   bool
	wg        sync.WaitGroup
}

var dbQueue *DBQueue

func InitDBQueue() {
	dbQueue = &DBQueue{
		messages:  make(chan model.P987Message, config.DBQueueSize),
		batchSize: config.DBBatchSize,
		flushTime: 1000 * time.Millisecond,
	}

	dbQueue.wg.Add(1)
	go dbQueue.worker()

	utils.SugarLogger.Infof("[DB] Initialized queue with buffer %d, batch size %d", config.DBQueueSize, dbQueue.batchSize)
}

func QueueDBWrite(timestamp int, vehicleID, topic string, data []byte, sourceNode string, targetNode string) {
	dbQueue.mu.RLock()
	stopped := dbQueue.stopped
	dbQueue.mu.RUnlock()

	if stopped {
		return
	}

	msg := model.P987Message{
		Timestamp:  timestamp,
		VehicleID:  vehicleID,
		Topic:      topic,
		Data:       data,
		Synced:     0,
		SourceNode: sourceNode,
		TargetNode: targetNode,
	}

	select {
	case dbQueue.messages <- msg:
	default:
		utils.SugarLogger.Warnf("[DB] Queue full, dropping message")
	}
}

func (q *DBQueue) worker() {
	defer q.wg.Done()

	batch := make([]model.P987Message, 0, q.batchSize)
	ticker := time.NewTicker(q.flushTime)
	defer ticker.Stop()

	for {
		select {
		case msg, ok := <-q.messages:
			if !ok {
				if len(batch) > 0 {
					q.writeBatch(batch)
				}
				return
			}

			batch = append(batch, msg)

			if len(batch) >= q.batchSize {
				q.writeBatch(batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				q.writeBatch(batch)
				batch = batch[:0]
			}
		}
	}
}

func (q *DBQueue) writeBatch(batch []model.P987Message) {
	if len(batch) == 0 {
		return
	}

	start := time.Now()
	result := database.DB.CreateInBatches(&batch, len(batch))
	duration := time.Since(start)

	if result.Error != nil {
		utils.SugarLogger.Errorf("[DB] Failed to batch insert %d messages: %v", len(batch), result.Error)
	} else {
		utils.SugarLogger.Infof("[DB] Inserted %d messages in %v", len(batch), duration)
	}
}

func StopDBQueue() {
	if dbQueue == nil {
		return
	}

	dbQueue.mu.Lock()
	dbQueue.stopped = true
	dbQueue.mu.Unlock()

	close(dbQueue.messages)
	dbQueue.wg.Wait()

	utils.SugarLogger.Infof("[DB] Stopped")
}
