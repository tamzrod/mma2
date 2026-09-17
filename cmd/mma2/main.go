// cmd/mma2/main.go
package main

import (
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"mma2/internal/accessevents"
	"mma2/internal/authority"
	"mma2/internal/config"
	"mma2/internal/ingress"
	"mma2/internal/notify"
	"mma2/internal/rbe"
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
	if err != nil { log.Fatalf("config load failed: %v", err) }
	if err := config.Validate(cfg); err != nil { log.Fatalf("config validation failed: %v", err) }

	// RBE rules, global IDs, ranges and the dedicated output port must be
	// validated before any listener is started.
	rbeRules, err := config.BuildRBERules(cfg)
	if err != nil { log.Fatalf("RBE validation failed: %v", err) }
	log.Println("config loaded and validated successfully")

	store, err := config.BuildMemoryStore(cfg)
	if err != nil { log.Fatalf("memory build failed: %v", err) }
	auth := authority.New()
	policies, err := config.BuildAuthorityPolicies(cfg)
	if err != nil { log.Fatalf("policy build failed: %v", err) }
	for mid, p := range policies { auth.SetMemoryPolicy(mid, p) }
	log.Println("authority policies loaded")

	var notifier *notify.Engine
	if cfg.RBE == nil {
		registry, err := config.BuildNotifyRegistry(cfg)
		if err != nil { log.Fatalf("notify registry build failed: %v", err) }
		if registry != nil {
			var adapter notify.Adapter
			if cfg.Notify != nil && cfg.Notify.Influx != nil {
				influxCfg := cfg.Notify.Influx
				adapter = notify.NewInfluxAdapter(influxCfg.URL, influxCfg.Org, influxCfg.Bucket, influxCfg.Token, influxCfg.Measurement)
				log.Println("notify engine enabled (influx adapter)")
			} else {
				adapter = notify.NewStdoutAdapter()
				log.Println("notify engine enabled (stdout adapter)")
			}
			notifier = notify.NewEngine(registry, adapter, 256)
		} else {
			log.Println("notify engine disabled (no rules)")
		}
	}

	var observer *rbe.Engine
	if cfg.RBE != nil {
		// Bind before beginning ingress; port conflicts fail startup.
		ln, err := net.Listen("tcp", cfg.RBE.TCP.Listen)
		if err != nil { log.Fatalf("RBE TCP bind failed: %v", err) }
		publisher, err := rbe.NewTCPPublisher(ln, 256)
		if err != nil {
			_ = ln.Close()
			log.Fatalf("RBE TCP publisher failed: %v", err)
		}
		observer, err = rbe.NewEngine(rbeRules, publisher)
		if err != nil {
			_ = publisher.Close()
			log.Fatalf("RBE engine failed: %v", err)
		}
		log.Printf("RBE TCP listening on %s (%d rules)", cfg.RBE.TCP.Listen, len(rbeRules))
	}

	var ae *accessevents.Engine
	if cfg.AccessEvents != nil && cfg.AccessEvents.Enabled {
		ae = accessevents.New(cfg.AccessEvents)
		mux := http.NewServeMux()
		mux.Handle(cfg.AccessEvents.Output.Path, accessevents.NewHandler(ae))
		ln, err := net.Listen("tcp", cfg.AccessEvents.Output.Listen)
		if err != nil { log.Fatalf("access events: failed to bind %s: %v", cfg.AccessEvents.Output.Listen, err) }
		go func() {
			log.Printf("access events HTTP listening on %s", cfg.AccessEvents.Output.Listen)
			if err := http.Serve(ln, mux); err != nil { log.Fatalf("access events HTTP server failed: %v", err) }
		}()
		log.Println("access events engine started")
	} else {
		log.Println("access events disabled")
	}

	for _, gate := range cfg.Ingress {
		onModbus := func(conn net.Conn) {
			modbus.HandleConnWithRBE(conn, store, auth, notifier, observer, ae, cfg.Debug)
		}
		onRawIngest := func(conn net.Conn) {
			rawingest.HandleConnWithRBE(conn, store, notifier, observer)
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
