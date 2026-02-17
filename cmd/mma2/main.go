// cmd/mma2/main.go
package main

import (
	"log"
	"net"
	"os"

	"MMA2.0/internal/authority"
	"MMA2.0/internal/config"
	"MMA2.0/internal/ingress"
	"MMA2.0/internal/notify"
	"MMA2.0/internal/transport/modbus"
	"MMA2.0/internal/transport/rawingest"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: mma2 <config.yaml>")
	}

	cfgPath := os.Args[1]

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("config load failed: %v", err)
	}

	if err := config.Validate(cfg); err != nil {
		log.Fatalf("config validation failed: %v", err)
	}

	log.Println("config loaded and validated successfully")

	store, err := config.BuildMemoryStore(cfg)
	if err != nil {
		log.Fatalf("memory build failed: %v", err)
	}

	auth := authority.New()

	policies, err := config.BuildAuthorityPolicies(cfg)
	if err != nil {
		log.Fatalf("policy build failed: %v", err)
	}

	for mid, p := range policies {
		auth.SetMemoryPolicy(mid, p)
	}

	log.Println("authority policies loaded")

	// --------------------
	// Notify (stdout debug adapter)
	// --------------------

	var notifier *notify.Engine

	registry, err := config.BuildNotifyRegistry(cfg)
	if err != nil {
		log.Fatalf("notify registry build failed: %v", err)
	}

	if registry != nil {
		adapter := notify.NewStdoutAdapter()
		notifier = notify.NewEngine(registry, adapter, 256)
		log.Println("notify engine enabled (stdout adapter)")
	} else {
		log.Println("notify engine disabled (no rules)")
	}

	// --------------------
	// Start ingress
	// --------------------

	for _, gate := range cfg.Ingress {

		onModbus := func(conn net.Conn) {
			modbus.HandleConn(conn, store, auth, notifier)
		}

		onRawIngest := func(conn net.Conn) {
			rawingest.HandleConn(conn, store, notifier)
		}

		l := ingress.NewListener(gate)

		go func(g ingress.Listener) {
			if err := g.ListenAndServe(onModbus, onRawIngest); err != nil {
				log.Fatalf("ingress %s failed: %v", gate.ID, err)
			}
		}(*l)
	}

	log.Println("mma2 ingress started")

	select {}
}