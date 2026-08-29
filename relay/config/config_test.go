package config

import "testing"

func TestParseInterfaceList(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []CANInterface
	}{
		{"empty", "", nil},
		{"single pair", "can0:pcan", []CANInterface{{"can0", "pcan"}}},
		{"multiple pairs", "can0:pcan,can1:kcan", []CANInterface{{"can0", "pcan"}, {"can1", "kcan"}}},
		{"bare name labels itself", "can0", []CANInterface{{"can0", "can0"}}},
		{"empty label falls back to name", "can0:", []CANInterface{{"can0", "can0"}}},
		{"surrounding whitespace", " can0:pcan , can1:kcan ", []CANInterface{{"can0", "pcan"}, {"can1", "kcan"}}},
		{"empty entries skipped", "can0:pcan,,can1:kcan", []CANInterface{{"can0", "pcan"}, {"can1", "kcan"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseInterfaceList(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("parseInterfaceList(%q) = %v, want %v", tt.in, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("interface %d = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParsePortList(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"single", "8100", []string{"8100"}},
		{"multiple with whitespace", "8100, 8101", []string{"8100", "8101"}},
		{"empty entries skipped", "8100,,8101", []string{"8100", "8101"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePortList(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("parsePortList(%q) = %v, want %v", tt.in, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("port %d = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
