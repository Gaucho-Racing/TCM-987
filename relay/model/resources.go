package model

// ReportedCPUs is the core count carried in the 0x201 layout. The Pi Zero
// 2 W's BCM2710A1 is quad-core.
const ReportedCPUs = 4

// Throttle flags, published in the 0x201 throttle byte. Sourced from the
// Pi firmware's get_throttled word — thermal management on the Pi happens
// in firmware, not in the kernel thermal governor, so this is the only
// place the state is observable.
const (
	ThrottleUndervoltage      = 1 << 0 // under-voltage right now
	ThrottleUndervoltageSince = 1 << 1 // under-voltage has occurred since boot
	ThrottleThermal           = 1 << 2 // frequency throttled right now
	ThrottleThermalSince      = 1 << 3 // throttling has occurred since boot
)

// ResourceMetrics is what a Pi Zero 2 W can actually report. The board has
// no discrete GPU counters and no power-rail sensors, so unlike TCM-26's
// Jetson layout there are no GPU or voltage/current/power fields — the
// undervoltage flags are the useful signal in their place.
type ResourceMetrics struct {
	CPUFreq       [ReportedCPUs]int `json:"cpu_freq"`       // 2 bytes each, MHz
	CPUUtil       [ReportedCPUs]int `json:"cpu_util"`       // 1 byte each, %
	CPUTotalUtil  int               `json:"cpu_total_util"` // 1 byte, %
	RAMTotal      int               `json:"ram_total"`      // 2 bytes, MB
	RAMUsed       int               `json:"ram_used"`       // 2 bytes, MB
	RAMUtil       int               `json:"ram_util"`       // 1 byte, %
	DiskTotal     int               `json:"disk_total"`     // 4 bytes, MB
	DiskUsed      int               `json:"disk_used"`      // 4 bytes, MB
	DiskUtil      int               `json:"disk_util"`      // 1 byte, %
	CPUTemp       int               `json:"cpu_temp"`       // 1 byte, °C
	ThrottleFlags int               `json:"throttle_flags"` // 1 byte
}
