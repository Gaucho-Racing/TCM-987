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

type cpuSample struct {
	total uint64
	idle  uint64
}

var cpuSampleMu sync.Mutex
var prevCPUSamples map[string]cpuSample

// QueryResourceMetrics reads Pi resource stats straight from /proc and
// /sys. CPU utilization is the delta since the previous call (the 10s poll
// cadence is the smoothing window), so the first call reports 0%.
func QueryResourceMetrics() (model.ResourceMetrics, error) {
	var m model.ResourceMetrics

	utils, total, err := readCPUUtilization()
	if err != nil {
		return m, fmt.Errorf("cpu utilization: %w", err)
	}
	m.CPUTotalUtil = total
	for i := 0; i < model.ReportedCPUs; i++ {
		if i < len(utils) {
			m.CPUUtil[i] = utils[i]
		}
		m.CPUFreq[i] = readCPUFreqMHz(i)
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
	m.ThrottleFlags = readThrottleFlags()

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

// Firmware get_throttled bit positions (raspberrypi.com/documentation
// "vcgencmd get_throttled"). The low nibble is live state, bits 16+ are
// sticky since boot.
const (
	fwUndervoltage      = 1 << 0
	fwThrottled         = 1 << 2
	fwUndervoltageSince = 1 << 16
	fwThrottledSince    = 1 << 18
)

const (
	throttledPath = "/sys/devices/platform/soc/soc:firmware/get_throttled"
	hwmonGlob     = "/sys/class/hwmon/hwmon*/in0_lcrit_alarm"
)

// readThrottleFlags reports under-voltage and thermal throttling. On the
// Pi both are handled in firmware rather than by the kernel thermal
// governor, so the firmware's get_throttled word is the only place the
// live bits are visible. That attribute is deprecated upstream, so fall
// back to the rpi_volt hwmon alarm — which exposes only the sticky
// under-voltage bit, nothing thermal and nothing live.
func readThrottleFlags() int {
	raw, err := readThrottledWord()
	if err != nil {
		return readUndervoltageAlarm()
	}

	var flags int
	if raw&fwUndervoltage != 0 {
		flags |= model.ThrottleUndervoltage
	}
	if raw&fwUndervoltageSince != 0 {
		flags |= model.ThrottleUndervoltageSince
	}
	if raw&fwThrottled != 0 {
		flags |= model.ThrottleThermal
	}
	if raw&fwThrottledSince != 0 {
		flags |= model.ThrottleThermalSince
	}
	return flags
}

func readThrottledWord() (uint64, error) {
	data, err := os.ReadFile(throttledPath)
	if err != nil {
		return 0, err
	}
	// The attribute is printed as bare hex; vcgencmd renders the same word
	// with an 0x prefix, so tolerate both.
	text := strings.TrimPrefix(strings.TrimSpace(string(data)), "0x")
	return strconv.ParseUint(text, 16, 64)
}

func readUndervoltageAlarm() int {
	matches, err := filepath.Glob(hwmonGlob)
	if err != nil {
		return 0
	}
	for _, path := range matches {
		name, err := os.ReadFile(filepath.Join(filepath.Dir(path), "name"))
		if err != nil || strings.TrimSpace(string(name)) != "rpi_volt" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(data)) == "1" {
			return model.ThrottleUndervoltageSince
		}
		return 0
	}
	return 0
}
