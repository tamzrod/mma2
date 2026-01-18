// internal/transport/raw/header_v1.go
// PURPOSE: Raw Ingest v1 header definition and parsing.
// ALLOWED: binary parsing, structural validation
// FORBIDDEN: memory access, semantics, retries, logging

package raw

import (
	"encoding/binary"
)

// =========================
// Raw Ingest v1 Constants
// =========================

const (
	RawMagic uint16 = 0xA55A
	RawVerV1 uint8  = 0x01
)

// Header size (fixed, bytes)
const RawHeaderV1Size = 14

// =========================
// Raw Ingest v1 Header
// =========================
//
// Layout (14 bytes total):
// [ Magic(2) ][ Ver(1) ][ Flags(1) ]
// [ Area(1) ][ Rsv(1) ]
// [ UnitID(2) ]
// [ Port(2) ]
// [ Address(2) ][ Count(2) ]
//
type v1Header struct {
	Magic   uint16
	Version uint8
	Flags   uint8

	Area MemoryArea
	Rsv  uint8

	UnitID uint16
	Port   uint16

	Address uint16
	Count   uint16
}

// =========================
// Header Parsing
// =========================

// parseV1Header parses and validates a Raw Ingest v1 header.
// It performs ONLY structural validation.
func parseV1Header(buf []byte) (v1Header, error) {
	if len(buf) != RawHeaderV1Size {
		return v1Header{}, ErrRejected
	}

	h := v1Header{
		Magic:   binary.BigEndian.Uint16(buf[0:2]),
		Version: buf[2],
		Flags:   buf[3],

		Area: MemoryArea(buf[4]),
		Rsv:  buf[5],

		UnitID: binary.BigEndian.Uint16(buf[6:8]),
		Port:   binary.BigEndian.Uint16(buf[8:10]),

		Address: binary.BigEndian.Uint16(buf[10:12]),
		Count:   binary.BigEndian.Uint16(buf[12:14]),
	}

	// Structural validation only
	if h.Magic != RawMagic {
		return v1Header{}, ErrRejected
	}

	if h.Version != RawVerV1 {
		return v1Header{}, ErrRejected
	}

	switch h.Area {
	case AreaCoils,
	AreaDiscreteInputs,
	AreaHoldingRegs,
	AreaInputRegs:

		
	default:
		return v1Header{}, ErrRejected
	}

	if h.Count == 0 {
		return v1Header{}, ErrRejected
	}

	return h, nil
}
