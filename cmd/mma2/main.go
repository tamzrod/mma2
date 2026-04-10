// cmd/mma2/main.go
package main

import (
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"

	"mma2/internal/authority"
	"mma2/internal/config"
	"mma2/internal/ingress"
	"mma2/internal/notify"
	"mma2/internal/transport/modbus"
	"mma2/internal/transport/rawingest"
	"mma2/internal/version"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatalf("usage: mma2 <config.yaml>")
	}

	cfgPath := os.Args[1]

	ext := strings.ToLower(filepath.Ext(cfgPath))
	if ext != ".yaml" && ext != ".yml" {
		log.Fatalf("config path must end in .yaml or .yml, got: %s", cfgPath)
	}

	log.Printf("mma2 v%s starting", version.Version)
	log.Printf("config path: %s", cfgPath)

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
	// Notify
	// --------------------

	var notifier *notify.Engine

	registry, err := config.BuildNotifyRegistry(cfg)
	if err != nil {
		log.Fatalf("notify registry build failed: %v", err)
	}

	if registry != nil {

		var adapter notify.Adapter

		// Influx adapter (optional)
		if cfg.Notify != nil && cfg.Notify.Influx != nil {

			influxCfg := cfg.Notify.Influx

			adapter = notify.NewInfluxAdapter(
				influxCfg.URL,
				influxCfg.Org,
				influxCfg.Bucket,
				influxCfg.Token,
				influxCfg.Measurement,
			)

			log.Println("notify engine enabled (influx adapter)")
		} else {
			adapter = notify.NewStdoutAdapter()
			log.Println("notify engine enabled (stdout adapter)")
		}

		notifier = notify.NewEngine(registry, adapter, 256)

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