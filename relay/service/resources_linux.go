//go:build linux

package service

import (
	"fmt"
	"os"
	"path/filepath"
	"relay/config"
	"relay/model"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

const maxReportedCPUs = 6

type cpuSample struct {
	total uint64
	idle  uint64
}

var cpuSampleMu sync.Mutex
var prevCPUSamples map[string]cpuSample

// QueryResourceMetrics reads Pi resource stats straight from /proc and
// /sys — no jtop equivalent needed. CPU utilization is the delta since
// the previous call (the 10s poll cadence is the smoothing window), so
// the first call reports 0%.
func QueryResourceMetrics() (model.ResourceMetrics, error) {
	var m model.ResourceMetrics

	utils, total, err := readCPUUtilization()
	if err != nil {
		return m, fmt.Errorf("cpu utilization: %w", err)
	}
	m.CPUTotalUtil = total
	perCore := [maxReportedCPUs]*int{&m.CPU0Util, &m.CPU1Util, &m.CPU2Util, &m.CPU3Util, &m.CPU4Util, &m.CPU5Util}
	perFreq := [maxReportedCPUs]*int{&m.CPU0Freq, &m.CPU1Freq, &m.CPU2Freq, &m.CPU3Freq, &m.CPU4Freq, &m.CPU5Freq}
	for i := 0; i < maxReportedCPUs; i++ {
		if i < len(utils) {
			*perCore[i] = utils[i]
		}
		*perFreq[i] = readCPUFreqMHz(i)
	}

	ramTotal, ramUsed, err := readMemInfo()
	if err != nil {
		return m, fmt.Errorf("meminfo: %w", err)
	}
	m.RAMTotal = ramTotal
	m.RAMUsed = ramUsed
	if ramTotal > 0 {
		m.RAMUtil = ramUsed * 100 / ramTotal
	}

	diskTotal, diskUsed := readDiskUsage(filepath.Dir(config.DatabasePath))
	m.DiskTotal = diskTotal
	m.DiskUsed = diskUsed
	if diskTotal > 0 {
		m.DiskUtil = diskUsed * 100 / diskTotal
	}

	m.CPUTemp = readCPUTemp()

	return m, nil
}

// readCPUUtilization parses /proc/stat and computes per-core + aggregate
// busy percentages against the previous snapshot.
func readCPUUtilization() (perCore []int, total int, err error) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return nil, 0, err
	}

	current := make(map[string]cpuSample)
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || !strings.HasPrefix(fields[0], "cpu") {
			continue
		}
		var sample cpuSample
		for i, f := range fields[1:] {
			v, parseErr := strconv.ParseUint(f, 10, 64)
			if parseErr != nil {
				continue
			}
			sample.total += v
			// idle (field 4) + iowait (field 5)
			if i == 3 || i == 4 {
				sample.idle += v
			}
		}
		current[fields[0]] = sample
	}

	cpuSampleMu.Lock()
	prev := prevCPUSamples
	prevCPUSamples = current
	cpuSampleMu.Unlock()

	utilOf := func(key string) int {
		cur, ok := current[key]
		if !ok {
			return 0
		}
		p, ok := prev[key]
		if !ok {
			return 0
		}
		dTotal := cur.total - p.total
		dIdle := cur.idle - p.idle
		if dTotal == 0 {
			return 0
		}
		return int(100 * (dTotal - dIdle) / dTotal)
	}

	total = utilOf("cpu")
	for i := 0; ; i++ {
		key := fmt.Sprintf("cpu%d", i)
		if _, ok := current[key]; !ok {
			break
		}
		perCore = append(perCore, utilOf(key))
	}
	return perCore, total, nil
}

func readCPUFreqMHz(core int) int {
	data, err := os.ReadFile(fmt.Sprintf("/sys/devices/system/cpu/cpu%d/cpufreq/scaling_cur_freq", core))
	if err != nil {
		return 0
	}
	khz, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return khz / 1000
}

func readMemInfo() (totalMB, usedMB int, err error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0, err
	}
	var totalKB, availKB int
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		v, parseErr := strconv.Atoi(fields[1])
		if parseErr != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			totalKB = v
		case "MemAvailable:":
			availKB = v
		}
	}
	return totalKB / 1024, (totalKB - availKB) / 1024, nil
}

func readDiskUsage(path string) (totalMB, usedMB int) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, 0
	}
	blockSize := uint64(stat.Bsize)
	total := stat.Blocks * blockSize
	avail := stat.Bavail * blockSize
	return int(total / (1024 * 1024)), int((total - avail) / (1024 * 1024))
}

func readCPUTemp() int {
	data, err := os.ReadFile("/sys/class/thermal/thermal_zone0/temp")
	if err != nil {
		return 0
	}
	milli, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return milli / 1000
}
