// tools/raw_ingest_send.go
package main

import (
	"encoding/binary"
	"fmt"
	"net"
)

func main() {
	conn, err := net.Dial("tcp", "127.0.0.1:4667")
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	/*
		Raw Ingest Packet
		Magic:   'R''I'
		Version: 0x01
		Area:    3 (Holding Registers)
		UnitID:  1
		Addr:    10
		Count:   1
		Value:   0x1234
	*/

	pkt := make([]byte, 12)

	pkt[0] = 'R'
	pkt[1] = 'I'
	pkt[2] = 0x01
	pkt[3] = 0x03 // holding registers

	binary.BigEndian.PutUint16(pkt[4:6], 1)   // unit id
	binary.BigEndian.PutUint16(pkt[6:8], 10)  // address
	binary.BigEndian.PutUint16(pkt[8:10], 1)  // count
	binary.BigEndian.PutUint16(pkt[10:12], 0x1234)

	if _, err := conn.Write(pkt); err != nil {
		panic(err)
	}

	resp := make([]byte, 1)
	if _, err := conn.Read(resp); err != nil {
		panic(err)
	}

	fmt.Printf("raw ingest response = %d\n", resp[0])
}
