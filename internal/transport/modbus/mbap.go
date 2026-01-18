package modbus

import (
	"encoding/binary"
	"io"
)

type MBAP struct {
	TransactionID uint16
	ProtocolID    uint16
	Length        uint16
	UnitID        uint8
}

func readMBAP(r io.Reader) (MBAP, error) {
	var hdr [7]byte
	_, err := io.ReadFull(r, hdr[:])
	if err != nil {
		return MBAP{}, err
	}

	return MBAP{
		TransactionID: binary.BigEndian.Uint16(hdr[0:2]),
		ProtocolID:    binary.BigEndian.Uint16(hdr[2:4]),
		Length:        binary.BigEndian.Uint16(hdr[4:6]),
		UnitID:        hdr[6],
	}, nil
}

func writeMBAP(w io.Writer, mbap MBAP) error {
	var hdr [7]byte
	binary.BigEndian.PutUint16(hdr[0:2], mbap.TransactionID)
	binary.BigEndian.PutUint16(hdr[2:4], mbap.ProtocolID)
	binary.BigEndian.PutUint16(hdr[4:6], mbap.Length)
	hdr[6] = mbap.UnitID
	_, err := w.Write(hdr[:])
	return err
}
