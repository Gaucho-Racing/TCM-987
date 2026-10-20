package service

import (
	"bufio"
	"encoding/binary"
	"relay/config"
)

func encodeSeeedSettings() []byte {
	settings := make([]byte, 20)
	settings[0], settings[1], settings[2] = 0xaa, 0x55, 0x12
	settings[3] = config.SeeedSpeedCodes[config.SeeedCANBitrate]
	settings[4] = 0x01
	if config.SeeedFrameType == "EXT" {
		settings[4] = 0x02
	}
	settings[13] = 0x02
	settings[14] = 0x01
	settings[19] = seeedChecksum(settings[2:19])
	return settings
}

func seeedChecksum(data []byte) byte {
	var checksum byte
	for _, value := range data {
		checksum += value
	}
	return checksum
}

func readSeeedFrame(reader *bufio.Reader) (uint32, []byte, error) {
	for {
		header, err := reader.Peek(2)
		if err != nil {
			return 0, nil, err
		}
		if header[0] != 0xaa {
			reader.Discard(1)
			continue
		}
		if header[1] == 0x55 {
			response, err := reader.Peek(20)
			if err != nil {
				return 0, nil, err
			}
			if seeedChecksum(response[2:19]) != response[19] {
				reader.Discard(1)
				continue
			}
			reader.Discard(20)
			continue
		}
		frameType := header[1]
		length := int(frameType & 0x0f)
		if frameType&0xc0 != 0xc0 || length > 8 {
			reader.Discard(1)
			continue
		}
		idSize := 2
		if frameType&0x20 != 0 {
			idSize = 4
		}
		frameSize := 3 + idSize + length
		frame, err := reader.Peek(frameSize)
		if err != nil {
			return 0, nil, err
		}
		if frame[frameSize-1] != 0x55 {
			reader.Discard(1)
			continue
		}
		canID := uint32(binary.LittleEndian.Uint16(frame[2:4]))
		maxID := uint32(0x7ff)
		if idSize == 4 {
			canID = binary.LittleEndian.Uint32(frame[2:6])
			maxID = 0x1fffffff
		}
		if canID > maxID {
			reader.Discard(1)
			continue
		}
		if frameType&0x10 != 0 {
			reader.Discard(frameSize)
			continue
		}
		data := make([]byte, length)
		copy(data, frame[2+idSize:frameSize-1])
		reader.Discard(frameSize)
		return canID, data, nil
	}
}
