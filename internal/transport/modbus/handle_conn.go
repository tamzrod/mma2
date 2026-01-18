// internal/transport/modbus/handle_conn.go
package modbus

import (
	"io"
	"log"
	"net"

	"MMA2.0/internal/memorycore"
)

// HandleConn handles a single Modbus TCP connection.
func HandleConn(conn net.Conn, store *memorycore.Store) {
	defer conn.Close()

	// Extract local listening port (authoritative)
	localAddr, ok := conn.LocalAddr().(*net.TCPAddr)
	if !ok {
		log.Printf("modbus: failed to get local TCP address")
		return
	}
	port := uint16(localAddr.Port)

	for {


		req, err := ReadRequest(conn, port)
		if err != nil {
			if err != io.EOF {
				log.Printf("modbus read error: %v", err)
			}
			return
		}



		pdu := DispatchMemory(store, req)
		if pdu == nil {
			return
		}

		frame := BuildResponse(req, pdu)
		if _, err := conn.Write(frame); err != nil {
			log.Printf("modbus write error: %v", err)
			return
		}
	}
}
