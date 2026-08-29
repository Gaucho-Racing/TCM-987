package service

import (
	"encoding/binary"
	"relay/model"
	"testing"
)

func TestEncodeResourcePayload(t *testing.T) {
	m := model.ResourceMetrics{
		CPUFreq:       [model.ReportedCPUs]int{1000, 1001, 1002, 1003},
		CPUUtil:       [model.ReportedCPUs]int{10, 20, 30, 40},
		CPUTotalUtil:  25,
		RAMTotal:      512,
		RAMUsed:       128,
		RAMUtil:       25,
		DiskTotal:     30000,
		DiskUsed:      12000,
		DiskUtil:      40,
		CPUTemp:       55,
		ThrottleFlags: model.ThrottleUndervoltageSince | model.ThrottleThermal,
	}

	got := encodeResourcePayload(m)

	if len(got) != resourcePayloadSize {
		t.Fatalf("payload length = %d, want %d", len(got), resourcePayloadSize)
	}

	for i := 0; i < model.ReportedCPUs; i++ {
		off := i * 3
		if freq := binary.LittleEndian.Uint16(got[off : off+2]); int(freq) != m.CPUFreq[i] {
			t.Errorf("cpu%d freq = %d, want %d", i, freq, m.CPUFreq[i])
		}
		if util := got[off+2]; int(util) != m.CPUUtil[i] {
			t.Errorf("cpu%d util = %d, want %d", i, util, m.CPUUtil[i])
		}
	}

	if got[12] != 25 {
		t.Errorf("cpu_total_util = %d, want 25", got[12])
	}
	if v := binary.LittleEndian.Uint16(got[13:15]); v != 512 {
		t.Errorf("ram_total = %d, want 512", v)
	}
	if v := binary.LittleEndian.Uint16(got[15:17]); v != 128 {
		t.Errorf("ram_used = %d, want 128", v)
	}
	if got[17] != 25 {
		t.Errorf("ram_util = %d, want 25", got[17])
	}
	if v := binary.LittleEndian.Uint32(got[18:22]); v != 30000 {
		t.Errorf("disk_total = %d, want 30000", v)
	}
	if v := binary.LittleEndian.Uint32(got[22:26]); v != 12000 {
		t.Errorf("disk_used = %d, want 12000", v)
	}
	if got[26] != 40 {
		t.Errorf("disk_util = %d, want 40", got[26])
	}
	if got[27] != 55 {
		t.Errorf("cpu_temp = %d, want 55", got[27])
	}
	if got[28] != model.ThrottleUndervoltageSince|model.ThrottleThermal {
		t.Errorf("throttle_flags = %#04b, want %#04b", got[28], model.ThrottleUndervoltageSince|model.ThrottleThermal)
	}
}

func TestEncodeResourcePayloadSaturates(t *testing.T) {
	// A Pi with no readable sensor reports 0; a bogus reading must pin the
	// field rather than wrap into a plausible small number.
	m := model.ResourceMetrics{
		CPUFreq:   [model.ReportedCPUs]int{70000, -1, 0, 0},
		CPUUtil:   [model.ReportedCPUs]int{300, -5, 0, 0},
		DiskTotal: 5_000_000_000,
	}

	got := encodeResourcePayload(m)

	if v := binary.LittleEndian.Uint16(got[0:2]); v != 65535 {
		t.Errorf("cpu0 freq = %d, want 65535", v)
	}
	if got[2] != 255 {
		t.Errorf("cpu0 util = %d, want 255", got[2])
	}
	if v := binary.LittleEndian.Uint16(got[3:5]); v != 0 {
		t.Errorf("cpu1 freq = %d, want 0", v)
	}
	if got[5] != 0 {
		t.Errorf("cpu1 util = %d, want 0", got[5])
	}
	if v := binary.LittleEndian.Uint32(got[18:22]); v != 4294967295 {
		t.Errorf("disk_total = %d, want 4294967295", v)
	}
}
