//go:build linux

package service

import (
	"bytes"
	"encoding/binary"
	"testing"

	"golang.org/x/sys/unix"
)

func canFrame(rawID uint32, dlc byte, data []byte) []byte {
	frame := make([]byte, canFrameSize)
	binary.LittleEndian.PutUint32(frame[0:4], rawID)
	frame[4] = dlc
	copy(frame[8:], data)
	return frame
}

func TestParseCANFrame(t *testing.T) {
	t.Run("standard id", func(t *testing.T) {
		canID, data, ok := parseCANFrame(canFrame(0x123, 8, []byte{1, 2, 3, 4, 5, 6, 7, 8}))
		if !ok {
			t.Fatal("expected frame to parse")
		}
		if canID != 0x123 {
			t.Errorf("canID = %#x, want 0x123", canID)
		}
		if !bytes.Equal(data, []byte{1, 2, 3, 4, 5, 6, 7, 8}) {
			t.Errorf("data = %v", data)
		}
	})

	t.Run("extended id", func(t *testing.T) {
		canID, _, ok := parseCANFrame(canFrame(0x18DAF110|unix.CAN_EFF_FLAG, 0, nil))
		if !ok {
			t.Fatal("expected frame to parse")
		}
		if canID != 0x18DAF110 {
			t.Errorf("canID = %#x, want 0x18DAF110", canID)
		}
	})

	t.Run("standard id masks off stray high bits", func(t *testing.T) {
		canID, _, ok := parseCANFrame(canFrame(0xFFFFF123&^uint32(unix.CAN_EFF_FLAG|unix.CAN_RTR_FLAG|unix.CAN_ERR_FLAG), 0, nil))
		if !ok {
			t.Fatal("expected frame to parse")
		}
		if canID != 0x123 {
			t.Errorf("canID = %#x, want 0x123", canID)
		}
	})

	t.Run("rtr frames are skipped", func(t *testing.T) {
		if _, _, ok := parseCANFrame(canFrame(0x123|unix.CAN_RTR_FLAG, 0, nil)); ok {
			t.Error("RTR frame should not parse — it carries no telemetry")
		}
	})

	t.Run("error frames are skipped", func(t *testing.T) {
		if _, _, ok := parseCANFrame(canFrame(0x123|unix.CAN_ERR_FLAG, 0, nil)); ok {
			t.Error("error frame should not parse")
		}
	})

	t.Run("short read is skipped", func(t *testing.T) {
		if _, _, ok := parseCANFrame(make([]byte, canFrameSize-1)); ok {
			t.Error("undersized frame should not parse")
		}
	})

	t.Run("dlc is clamped to the classic payload size", func(t *testing.T) {
		// A CAN FD DLC on a classic socket would otherwise read past the
		// 8 data bytes the frame actually carries.
		_, data, ok := parseCANFrame(canFrame(0x123, 15, nil))
		if !ok {
			t.Fatal("expected frame to parse")
		}
		if len(data) != 8 {
			t.Errorf("data length = %d, want 8", len(data))
		}
	})

	t.Run("data is copied out of the read buffer", func(t *testing.T) {
		frame := canFrame(0x123, 2, []byte{0xAA, 0xBB})
		_, data, ok := parseCANFrame(frame)
		if !ok {
			t.Fatal("expected frame to parse")
		}
		frame[8], frame[9] = 0, 0
		if !bytes.Equal(data, []byte{0xAA, 0xBB}) {
			t.Errorf("data = %v after buffer reuse, want AABB", data)
		}
	})
}
