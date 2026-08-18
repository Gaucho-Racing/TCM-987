//go:build linux

package service

import (
	"encoding/binary"
	"fmt"
	"net"
	"relay/config"
	"relay/utils"
	"time"

	"golang.org/x/sys/unix"
)

// classic can_frame: [0:4] ID+flags u32 host-order, [4] DLC, [5:8] pad,
// [8:16] data. Our deploy targets (arm64/amd64) are all little-endian.
const canFrameSize = 16

const reopenBackoff = 5 * time.Second

// RunSocketCAN starts one reader per configured interface and blocks
// forever so main has something to park on.
func RunSocketCAN() {
	for _, iface := range config.CANInterfaces {
		go runReader(iface)
	}
	select {}
}

// runReader reads frames until an error, then reopens after a backoff —
// the interface bounces across ignition cycles and `ip link` restarts,
// and the relay has to ride through both.
func runReader(iface config.CANInterface) {
	for {
		if err := readFrames(iface); err != nil {
			utils.SugarLogger.Errorf("[CAN:%s] %v, reopening in %s", iface.Name, err, reopenBackoff)
		}
		time.Sleep(reopenBackoff)
	}
}

func readFrames(iface config.CANInterface) error {
	netIface, err := net.InterfaceByName(iface.Name)
	if err != nil {
		return fmt.Errorf("interface lookup failed: %w", err)
	}

	fd, err := unix.Socket(unix.AF_CAN, unix.SOCK_RAW, unix.CAN_RAW)
	if err != nil {
		return fmt.Errorf("socket failed: %w", err)
	}
	defer unix.Close(fd)

	if err := unix.Bind(fd, &unix.SockaddrCAN{Ifindex: netIface.Index}); err != nil {
		return fmt.Errorf("bind failed: %w", err)
	}

	utils.SugarLogger.Infof("[CAN:%s] reading as bus %q", iface.Name, iface.Label)

	frame := make([]byte, canFrameSize)
	for {
		n, err := unix.Read(fd, frame)
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			return fmt.Errorf("read failed: %w", err)
		}
		if n < canFrameSize {
			continue
		}

		rawID := binary.LittleEndian.Uint32(frame[0:4])
		if rawID&(unix.CAN_RTR_FLAG|unix.CAN_ERR_FLAG) != 0 {
			continue
		}
		canID := rawID & unix.CAN_SFF_MASK
		if rawID&unix.CAN_EFF_FLAG != 0 {
			canID = rawID & unix.CAN_EFF_MASK
		}

		length := int(frame[4])
		if length > 8 {
			length = 8
		}
		data := make([]byte, length)
		copy(data, frame[8:8+length])

		go PublishData(iface.Label, canID, data)
	}
}
