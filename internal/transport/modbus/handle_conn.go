// internal/transport/modbus/handle_conn.go
package modbus

import (
	"io"
	"log"
	"net"

	"MMA2.0/internal/memorycore"
)

// HandleConn handles a single Modbus TCP connection.
func HandleConn(conn net.Conn, store *memorycore.Store, portKey string) {
	defer conn.Close()

	for {
		req, err := ReadRequest(conn, 0)
		if err != nil {
			if err != io.EOF {
				log.Printf("modbus read error: %v", err)
			}
			return
		}

		pdu := DispatchMemory(store, portKey, req)
		if pdu == nil {
			return
		}

		frame := BuildResponse(req, pdu)
		if _, err := conn.Write(frame); err != nil {
			return
		}
	}
}
