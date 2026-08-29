package utils

import (
	"fmt"
	"relay/config"
	"strings"
	"testing"
)

func TestValidTopicSegment(t *testing.T) {
	valid := []string{"cayman", "boxster-987", "gr_987", "987.2", "pcan", "kcan"}
	for _, s := range valid {
		t.Run("valid/"+s, func(t *testing.T) {
			if err := validTopicSegment(s); err != nil {
				t.Errorf("validTopicSegment(%q) = %v, want nil", s, err)
			}
		})
	}

	invalid := map[string]string{
		"empty":          "",
		"slash":          "cayman/987",
		"leading slash":  "/cayman",
		"plus wildcard":  "cay+man",
		"hash wildcard":  "cayman#",
		"inner space":    "cayman 987",
		"trailing space": "cayman ",
		"tab":            "cayman\t987",
		"newline":        "cayman\n",
	}
	for name, s := range invalid {
		t.Run("invalid/"+name, func(t *testing.T) {
			if err := validTopicSegment(s); err == nil {
				t.Errorf("validTopicSegment(%q) = nil, want error", s)
			}
		})
	}
}

// Mapache's ingest splits on "/" and requires exactly 4 segments, reading
// the vehicle from segment 1. Anything that changes the segment count is
// dropped there with no signal back to the relay.
func TestValidVehicleIDKeepsTopicShape(t *testing.T) {
	for _, vehicleID := range []string{"cayman", "boxster-987"} {
		if err := validTopicSegment(vehicleID); err != nil {
			t.Fatalf("validTopicSegment(%q) = %v", vehicleID, err)
		}
		topic := fmt.Sprintf("%s/%s/%s/0x%03x", config.TopicRoot, vehicleID, "pcan", 0x123)
		parts := strings.Split(topic, "/")
		if len(parts) != 4 {
			t.Errorf("topic %q has %d segments, want 4", topic, len(parts))
		}
		if parts[1] != vehicleID {
			t.Errorf("segment 1 = %q, want %q", parts[1], vehicleID)
		}
	}

	// The failure this guards against.
	bad := fmt.Sprintf("%s/%s/%s/0x%03x", config.TopicRoot, "cayman/987", "pcan", 0x123)
	if len(strings.Split(bad, "/")) == 4 {
		t.Error("a slashed vehicle id should break the 4-segment shape — the guard would be pointless otherwise")
	}
}
