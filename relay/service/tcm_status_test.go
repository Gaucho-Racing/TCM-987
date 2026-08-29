package service

import (
	"encoding/binary"
	"testing"
)

func TestStatusBits(t *testing.T) {
	tests := []struct {
		name                           string
		inet, mqttOK, mapacheOK, clock bool
		want                           byte
	}{
		{"all down", false, false, false, false, 0},
		{"all up", true, true, true, true, 0b1111},
		{"internet only", true, false, false, false, tcmStatusConnectionOK},
		{"mqtt only", false, true, false, false, tcmStatusMQTTOK},
		{"mapache only", false, false, true, false, tcmStatusMapacheOK},
		{"clock only", false, false, false, true, tcmStatusClockOK},
		{"link up, mapache silent", true, true, false, true, tcmStatusConnectionOK | tcmStatusMQTTOK | tcmStatusClockOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := statusBits(tt.inet, tt.mqttOK, tt.mapacheOK, tt.clock); got != tt.want {
				t.Errorf("statusBits = %08b, want %08b", got, tt.want)
			}
		})
	}
}

func TestEncodeTCMStatus(t *testing.T) {
	got := encodeTCMStatus(0b1011, 1234)

	if len(got) != 8 {
		t.Fatalf("payload length = %d, want 8", len(got))
	}
	if got[0] != 0b1011 {
		t.Errorf("status bits = %08b, want %08b", got[0], 0b1011)
	}
	if ping := binary.LittleEndian.Uint16(got[1:3]); ping != 1234 {
		t.Errorf("mapache ping = %d, want 1234", ping)
	}
	for i, b := range got[3:] {
		if b != 0 {
			t.Errorf("reserved byte %d = %d, want 0", i+3, b)
		}
	}
}
