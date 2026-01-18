package modbus

import "io"

type PDU struct {
	Function uint8
	Data     []byte
}

func readPDU(r io.Reader, length uint16) (PDU, error) {
	buf := make([]byte, length-1)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return PDU{}, err
	}

	return PDU{
		Function: buf[0],
		Data:     buf[1:],
	}, nil
}
