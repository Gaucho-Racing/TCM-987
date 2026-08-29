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

// A batch survives ~1.5s of retries before it is dropped; the queue keeps
// buffering behind it for as long as DB_QUEUE_SIZE allows.
const (
	writeRetries    = 3
	writeRetryDelay = 250 * time.Millisecond
)

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
	msg := model.P987Message{
		Timestamp:  timestamp,
		VehicleID:  vehicleID,
		Topic:      topic,
		Data:       data,
		Synced:     0,
		SourceNode: sourceNode,
		TargetNode: targetNode,
	}

	// Held across the send: StopDBQueue closes the channel under the write
	// lock, so no sender can be mid-send when it does.
	dbQueue.mu.RLock()
	defer dbQueue.mu.RUnlock()

	if dbQueue.stopped {
		return
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

// writeBatch retries transient failures before giving up on the batch.
// `database is locked` is expected in normal operation — shelter holds the
// same SQLite file — and dropping thousands of frames for it would put
// holes in the telemetry capture. The insert is a single transaction, so a
// failed attempt wrote nothing and retrying cannot duplicate rows.
func (q *DBQueue) writeBatch(batch []model.P987Message) {
	if len(batch) == 0 {
		return
	}

	start := time.Now()
	for attempt := 1; ; attempt++ {
		result := database.DB.CreateInBatches(&batch, len(batch))
		if result.Error == nil {
			utils.SugarLogger.Infof("[DB] Inserted %d messages in %v", len(batch), time.Since(start))
			return
		}
		if attempt > writeRetries {
			utils.SugarLogger.Errorf("[DB] Dropping %d messages after %d failed inserts: %v", len(batch), attempt, result.Error)
			return
		}
		utils.SugarLogger.Warnf("[DB] Insert of %d messages failed (attempt %d/%d): %v", len(batch), attempt, writeRetries, result.Error)
		time.Sleep(time.Duration(attempt) * writeRetryDelay)
	}
}

// StopDBQueue closes the queue and blocks until the worker has flushed
// everything still buffered.
func StopDBQueue() {
	if dbQueue == nil {
		return
	}

	dbQueue.mu.Lock()
	if dbQueue.stopped {
		dbQueue.mu.Unlock()
		return
	}
	dbQueue.stopped = true
	close(dbQueue.messages)
	dbQueue.mu.Unlock()

	dbQueue.wg.Wait()

	utils.SugarLogger.Infof("[DB] Stopped")
}
