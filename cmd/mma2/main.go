// cmd/mma2/main.go
package main

import (
	"log"
	"net"
	"os"

	"MMA2.0/internal/config"
	"MMA2.0/internal/ingress"
	"MMA2.0/internal/transport/modbus"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: mma2 <config.yaml>")
	}

	cfgPath := os.Args[1]

	// --------------------
	// Load + validate config
	// --------------------

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("config load failed: %v", err)
	}

	if err := config.Validate(cfg); err != nil {
		log.Fatalf("config validation failed: %v", err)
	}

	log.Println("config loaded and validated successfully")

	// --------------------
	// Build memory store (authoritative)
	// --------------------

	store, err := config.BuildMemoryStore(cfg)
	if err != nil {
		log.Fatalf("memory build failed: %v", err)
	}

	// --------------------
	// Start ingress listeners
	// --------------------

	for _, gate := range cfg.Ingress {
		portKey := gate.ID

		onModbus := func(conn net.Conn) {
			modbus.HandleConn(conn, store, portKey)
		}

		onRawIngest := func(conn net.Conn) {
			conn.Close()
		}

		l := ingress.NewListener(gate)

		go func(g ingress.Listener) {
			if err := g.ListenAndServe(onModbus, onRawIngest); err != nil {
				log.Fatalf("ingress %s failed: %v", gate.ID, err)
			}
		}(*l)
	}

	log.Println("mma2 ingress started")

	// --------------------
	// Block forever
	// --------------------

	select {}
}
