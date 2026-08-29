package service

import (
	"bytes"
	"encoding/binary"
	"relay/config"
	"testing"

	cmap "github.com/orcaman/concurrent-map/v2"
)

func TestEncodePayload(t *testing.T) {
	config.VehicleUploadKey = 0xBEEF
	data := []byte{0x01, 0x02, 0x03}

	got := encodePayload(0x0011223344556677, data)

	if len(got) != 10+len(data) {
		t.Fatalf("payload length = %d, want %d", len(got), 10+len(data))
	}
	if ts := binary.BigEndian.Uint64(got[0:8]); ts != 0x0011223344556677 {
		t.Errorf("timestamp = %#x, want %#x", ts, uint64(0x0011223344556677))
	}
	if key := binary.BigEndian.Uint16(got[8:10]); key != 0xBEEF {
		t.Errorf("upload key = %#x, want %#x", key, 0xBEEF)
	}
	if !bytes.Equal(got[10:], data) {
		t.Errorf("data = %v, want %v", got[10:], data)
	}
}

func TestEncodePayloadNoData(t *testing.T) {
	config.VehicleUploadKey = 1
	if got := encodePayload(42, nil); len(got) != 10 {
		t.Errorf("header-only payload length = %d, want 10", len(got))
	}
}

func TestParseVirtualCANFrame(t *testing.T) {
	frame := func(canID uint32, length byte, data []byte) []byte {
		p := make([]byte, virtualCANFrameSize)
		binary.LittleEndian.PutUint32(p[0:4], canID)
		p[5] = length
		copy(p[6:], data)
		return p
	}

	t.Run("valid frame", func(t *testing.T) {
		canID, data, err := parseVirtualCANFrame(frame(0x210, 4, []byte{0xDE, 0xAD, 0xBE, 0xEF}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if canID != 0x210 {
			t.Errorf("canID = %#x, want %#x", canID, 0x210)
		}
		if !bytes.Equal(data, []byte{0xDE, 0xAD, 0xBE, 0xEF}) {
			t.Errorf("data = %v, want DEADBEEF", data)
		}
	})

	t.Run("zero length", func(t *testing.T) {
		_, data, err := parseVirtualCANFrame(frame(0x211, 0, nil))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(data) != 0 {
			t.Errorf("data length = %d, want 0", len(data))
		}
	})

	t.Run("data is copied out of the buffer", func(t *testing.T) {
		packet := frame(0x210, 2, []byte{0xAA, 0xBB})
		_, data, err := parseVirtualCANFrame(packet)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// The listener reuses its read buffer, so a retained slice would
		// see the next frame's bytes.
		packet[6], packet[7] = 0x00, 0x00
		if !bytes.Equal(data, []byte{0xAA, 0xBB}) {
			t.Errorf("data = %v after buffer reuse, want AABB", data)
		}
	})

	t.Run("short packet", func(t *testing.T) {
		if _, _, err := parseVirtualCANFrame(make([]byte, virtualCANFrameSize-1)); err == nil {
			t.Error("expected error for undersized packet")
		}
	})

	t.Run("length beyond CAN FD max", func(t *testing.T) {
		if _, _, err := parseVirtualCANFrame(frame(0x210, 65, nil)); err == nil {
			t.Error("expected error for length 65")
		}
	})

	t.Run("max length payload fits", func(t *testing.T) {
		if _, data, err := parseVirtualCANFrame(frame(0x210, 64, nil)); err != nil {
			t.Errorf("64-byte payload in a 72-byte packet should be valid: %v", err)
		} else if len(data) != 64 {
			t.Errorf("data length = %d, want 64", len(data))
		}
	})
}

func TestShouldPublishOn(t *testing.T) {
	last := cmap.New[uint64]()
	const intervalMs = 20
	const key = "pcan/512"

	if !shouldPublishOn(key, 1_000_000, last, intervalMs) {
		t.Fatal("first publish for a key should always pass")
	}
	if shouldPublishOn(key, 1_010_000, last, intervalMs) {
		t.Error("publish 10ms later should be throttled")
	}
	if shouldPublishOn(key, 1_020_000, last, intervalMs) {
		t.Error("publish exactly at the interval should be throttled")
	}
	if !shouldPublishOn(key, 1_020_001, last, intervalMs) {
		t.Error("publish past the interval should pass")
	}
}

func TestShouldPublishOnKeysAreIndependent(t *testing.T) {
	last := cmap.New[uint64]()

	if !shouldPublishOn("pcan/512", 1_000_000, last, 20) {
		t.Fatal("first publish on pcan should pass")
	}
	// Same CAN ID on another bus is a different signal entirely.
	if !shouldPublishOn("kcan/512", 1_000_000, last, 20) {
		t.Error("first publish on kcan should not be throttled by pcan")
	}
}
