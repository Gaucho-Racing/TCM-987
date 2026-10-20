package service

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"relay/config"
	"relay/utils"
	"time"

	"golang.org/x/sys/unix"
)

func StartSeeedCAN() {
	for _, port := range config.SeeedCANPorts {
		go func(port config.CANInterface) {
			for {
				if err := readSeeedCAN(port); err != nil {
					utils.SugarLogger.Errorf("[Seeed:%s] %v, reopening in 5s", port.Name, err)
				}
				time.Sleep(5 * time.Second)
			}
		}(port)
	}
}

func readSeeedCAN(port config.CANInterface) error {
	fd, err := unix.Open(port.Name, unix.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open serial device: %w", err)
	}
	device := os.NewFile(uintptr(fd), port.Name)
	defer device.Close()

	if err := unix.IoctlSetInt(fd, unix.TIOCEXCL, 0); err != nil {
		return fmt.Errorf("claim serial device: %w", err)
	}
	termios, err := unix.IoctlGetTermios(fd, unix.TCGETS2)
	if err != nil {
		return fmt.Errorf("get serial settings: %w", err)
	}
	termios.Iflag, termios.Oflag, termios.Lflag = 0, 0, 0
	termios.Cflag = unix.CS8 | unix.CLOCAL | unix.CREAD | unix.BOTHER
	termios.Ispeed, termios.Ospeed = uint32(config.SeeedSerialBaud), uint32(config.SeeedSerialBaud)
	termios.Cc[unix.VMIN], termios.Cc[unix.VTIME] = 1, 0
	if err := unix.IoctlSetTermios(fd, unix.TCSETS2, termios); err != nil {
		return fmt.Errorf("set serial settings: %w", err)
	}
	if err := unix.IoctlSetInt(fd, unix.TCFLSH, unix.TCIOFLUSH); err != nil {
		return fmt.Errorf("flush serial buffers: %w", err)
	}
	settings := encodeSeeedSettings()
	written, err := device.Write(settings)
	if err != nil {
		return fmt.Errorf("configure adapter: %w", err)
	}
	if written != len(settings) {
		return fmt.Errorf("configure adapter: %w", io.ErrShortWrite)
	}
	utils.SugarLogger.Infof("[Seeed:%s] Reading as bus %q in listen-only mode", port.Name, port.Label)
	reader := bufio.NewReader(device)
	for {
		canID, data, err := readSeeedFrame(reader)
		if err != nil {
			return fmt.Errorf("read serial frame: %w", err)
		}
		PublishData(port.Label, canID, data)
	}
}
