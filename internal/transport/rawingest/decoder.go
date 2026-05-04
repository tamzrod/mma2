// internal/transport/rawingest/decoder.go
package rawingest

import (
	"encoding/binary"
	"errors"
	"io"

	"mma2/internal/memorycore"
)

var (
	ErrInvalidMagic   = errors.New("bad magic")
	ErrInvalidVersion = errors.New("bad version")
	ErrInvalidArea    = errors.New("invalid area")
	ErrInvalidCount   = errors.New("count is zero")
	ErrInvalidLength  = errors.New("invalid payload length")
)

const headerLen = 10 // Magic(2) Ver(1) Area(1) UnitID(2) Address(2) Count(2)

func payloadLen(area memorycore.Area, count uint16) (int, error) {
	if count == 0 {
		return 0, ErrInvalidCount
	}
	if area.IsBitArea() {
		return int((count + 7) / 8), nil
	}
	if area.IsRegArea() {
		return int(count) * 2, nil
	}
	return 0, ErrInvalidArea
}

// DecodeOne reads exactly one raw-ingest packet.
func DecodeOne(r io.Reader, port uint16) (*Packet, error) {
	var hdr [headerLen]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}

	if hdr[0] != Magic0 || hdr[1] != Magic1 {
		return nil, ErrInvalidMagic
	}
	if hdr[2] != Version1 {
		return nil, ErrInvalidVersion
	}

	area := memorycore.Area(hdr[3])
	unitID := binary.BigEndian.Uint16(hdr[4:6])
	addr := binary.BigEndian.Uint16(hdr[6:8])
	count := binary.BigEndian.Uint16(hdr[8:10])

	n, err := payloadLen(area, count)
	if err != nil {
		return nil, err
	}

	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, ErrInvalidLength
	}

	return &Packet{
		Port:    port,
		UnitID:  unitID,
		Area:    area,
		Address: addr,
		Count:   count,
		Payload: payload,
	}, nil
}
