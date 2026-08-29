package service

import (
	"net"
	"relay/mqtt"
	"relay/utils"
	"sync"
	"time"
)

// tcmState holds the latest reading from each connectivity watcher.
// publishTCMStatus takes a single snapshot every 5s with no I/O; each
// watcher updates its slot on its own cadence so a slow check (inet's
// 2s dial timeout in the worst case) can't stall the publish hot path.
type tcmState struct {
	mu          sync.RWMutex
	inet        bool
	mqtt        bool
	clock       bool
	lastPongAt  time.Time
	lastPongRTT uint16
}

var state tcmState

func (s *tcmState) setInet(v bool) {
	s.mu.Lock()
	s.inet = v
	s.mu.Unlock()
}

func (s *tcmState) setMqtt(v bool) {
	s.mu.Lock()
	s.mqtt = v
	s.mu.Unlock()
}

func (s *tcmState) setClock(v bool) {
	s.mu.Lock()
	s.clock = v
	s.mu.Unlock()
}

func (s *tcmState) setPong(at time.Time, rttMs uint16) {
	s.mu.Lock()
	s.lastPongAt = at
	s.lastPongRTT = rttMs
	s.mu.Unlock()
}

func (s *tcmState) snapshot() (inet, mqtt, clock bool, lastPongAt time.Time, lastPongRTT uint16) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.inet, s.mqtt, s.clock, s.lastPongAt, s.lastPongRTT
}

const (
	inetCheckInterval = 10 * time.Second
	mqttCheckInterval = 1 * time.Second
)

// internetCheckTarget is dialed to verify general internet connectivity.
// 8.8.8.8:53 is Google Public DNS — opening the socket proves the link is
// up, no DNS query needed.
const internetCheckTarget = "8.8.8.8:53"

func runInetWatcher() {
	state.setInet(checkInet())
	ticker := time.NewTicker(inetCheckInterval)
	defer ticker.Stop()
	for range ticker.C {
		state.setInet(checkInet())
	}
}

func checkInet() bool {
	conn, err := net.DialTimeout("tcp", internetCheckTarget, 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func runMqttWatcher() {
	state.setMqtt(checkMqtt())
	ticker := time.NewTicker(mqttCheckInterval)
	defer ticker.Stop()
	for range ticker.C {
		state.setMqtt(checkMqtt())
	}
}

func checkMqtt() bool {
	return mqtt.CloudClient != nil && mqtt.CloudClient.IsConnected()
}

func runClockWatcher() {
	state.setClock(ClockPlausible())
	ticker := time.NewTicker(clockCheckInterval)
	defer ticker.Stop()
	wasPlausible := true
	for range ticker.C {
		ok := ClockPlausible()
		state.setClock(ok)
		if !ok && wasPlausible {
			utils.SugarLogger.Errorf("[CLK] system clock implausible: now=%s floor=%s (RTC not set / no NTP sync?)",
				time.Now().Format(time.RFC3339), minValidTime.Format(time.RFC3339))
		} else if ok && !wasPlausible {
			utils.SugarLogger.Infof("[CLK] system clock recovered: now=%s", time.Now().Format(time.RFC3339))
		}
		wasPlausible = ok
	}
}

func InitializeTCMState() {
	go runInetWatcher()
	go runMqttWatcher()
	go runClockWatcher()
}
