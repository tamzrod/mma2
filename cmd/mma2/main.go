// cmd/mma2/main.go
package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"mma2/internal/accessevents"
	"mma2/internal/authority"
	"mma2/internal/config"
	"mma2/internal/ingress"
	"mma2/internal/memorycore"
	"mma2/internal/notify"
	"mma2/internal/persistence"
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
	if err != nil {
		log.Fatalf("config load failed: %v", err)
	}
	if err := config.Validate(cfg); err != nil {
		log.Fatalf("config validation failed: %v", err)
	}

	identityValues, err := config.BuildDeviceIdentities(cfg)
	if err != nil {
		log.Fatalf("device identity validation failed: %v", err)
	}
	configuredIdentities := make(map[memorycore.MemoryID]modbus.DeviceIdentity, len(identityValues))
	for mid, values := range identityValues {
		identity, err := modbus.NewDeviceIdentity(values.VendorName, values.ProductCode, values.MajorMinorRevision)
		if err != nil {
			log.Fatalf("device identity construction failed for %v: %v", mid, err)
		}
		configuredIdentities[mid] = identity
	}
	identities := modbus.NewDeviceIdentities(configuredIdentities)

	rbeRules, err := config.BuildRBERules(cfg)
	if err != nil {
		log.Fatalf("RBE validation failed: %v", err)
	}
	log.Println("config loaded and validated successfully")
	store, err := config.BuildMemoryStore(cfg)
	if err != nil {
		log.Fatalf("memory build failed: %v", err)
	}

	// Native persistence: build the owner, then restore before any listener
	// accepts connections. A corrupt/incompatible snapshot fails closed.
	persistPlan, err := config.BuildPersistencePlan(cfg)
	if err != nil {
		log.Fatalf("persistence validation failed: %v", err)
	}
	persistMgr, err := persistence.New(persistPlan, config.BuildMemoryAllocations(cfg))
	if err != nil {
		log.Fatalf("persistence init failed: %v", err)
	}
	if persistMgr.Enabled() {
		if err := os.MkdirAll(persistMgr.Directory(), 0o755); err != nil {
			log.Fatalf("persistence: create directory %s: %v", persistMgr.Directory(), err)
		}
		for _, mid := range persistMgr.PersistedIdentities() {
			mem, err := store.MustGet(mid)
			if err != nil {
				log.Fatalf("persistence: memory (port=%d unit=%d) missing: %v", mid.Port, mid.UnitID, err)
			}
			if err := persistMgr.RestoreMemory(mid, mem); err != nil {
				log.Fatalf("persistence restore failed (failing closed): %v", err)
			}
			// Observe committed writes from every transport for this identity.
			persistMgr.AttachMemory(mid, mem)
		}
		log.Printf("persistence ready: %d identities, directory %s", len(persistMgr.PersistedIdentities()), persistMgr.Directory())
	} else {
		log.Println("persistence disabled")
	}
	scheduler := persistence.NewScheduler(persistMgr)
	scheduler.Start(context.Background())
	persistMgr.SetNotifier(scheduler.Notify)
	auth := authority.New()
	policies, err := config.BuildAuthorityPolicies(cfg)
	if err != nil {
		log.Fatalf("policy build failed: %v", err)
	}
	for mid, p := range policies {
		auth.SetMemoryPolicy(mid, p)
	}
	log.Println("authority policies loaded")

	var shutdown []func()
	if persistMgr.Enabled() {
		// Flush pending dirty ranges before listeners close and the process exits.
		shutdown = append(shutdown, scheduler.Close)
	}

	var notifier *notify.Engine
	if cfg.RBE == nil {
		registry, err := config.BuildNotifyRegistry(cfg)
		if err != nil {
			log.Fatalf("notify registry build failed: %v", err)
		}
		if registry != nil {
			adapter := notify.NewStdoutAdapter()
			log.Println("notify engine enabled (stdout adapter)")
			notifier = notify.NewEngine(registry, adapter, 256)
		} else {
			log.Println("notify engine disabled (no rules)")
		}
	}

	var observer *rbe.Engine
	if cfg.RBE != nil {
		var sinks []rbe.Sink
		if cfg.RBE.TCP != nil {
			ln, err := net.Listen("tcp", cfg.RBE.TCP.Listen)
			if err != nil {
				log.Fatalf("RBE TCP bind failed: %v", err)
			}
			publisher, err := rbe.NewTCPPublisher(ln, 256)
			if err != nil {
				_ = ln.Close()
				log.Fatalf("RBE TCP publisher failed: %v", err)
			}
			sinks = append(sinks, publisher)
			shutdown = append(shutdown, func() { _ = publisher.Close() })
			log.Printf("RBE TCP listening on %s", cfg.RBE.TCP.Listen)
		}
		observer, err = rbe.NewEngine(rbeRules, &rbe.MultiSink{Sinks: sinks})
		if err != nil {
			log.Fatalf("RBE engine failed: %v", err)
		}
		log.Printf("RBE engine enabled (%d rules)", len(rbeRules))
	}

	var ae *accessevents.Engine
	if cfg.AccessEvents != nil && cfg.AccessEvents.Enabled {
		ae = accessevents.New(cfg.AccessEvents)
		mux := http.NewServeMux()
		mux.Handle(cfg.AccessEvents.Output.Path, accessevents.NewHandler(ae))
		ln, err := net.Listen("tcp", cfg.AccessEvents.Output.Listen)
		if err != nil {
			log.Fatalf("access events: failed to bind %s: %v", cfg.AccessEvents.Output.Listen, err)
		}
		shutdown = append(shutdown, func() { _ = ln.Close() })
		go func() {
			log.Printf("access events HTTP listening on %s", cfg.AccessEvents.Output.Listen)
			if err := http.Serve(ln, mux); err != nil {
				log.Printf("access events HTTP server stopped: %v", err)
			}
		}()
		log.Println("access events engine started")
	} else {
		log.Println("access events disabled")
	}

	for _, gate := range cfg.Ingress {
		onModbus := func(conn net.Conn) {
			modbus.HandleConnWithIdentities(conn, store, auth, notifier, observer, ae, cfg.Debug, identities)
		}
		onRawIngest := func(conn net.Conn) {
			rawingest.HandleConnWithRBE(conn, store, notifier, observer)
		}
		l := ingress.NewListener(gate)
		shutdown = append(shutdown, func() { _ = l.Close() })
		go func(g *ingress.Listener, gateID string) {
			if err := g.ListenAndServe(onModbus, onRawIngest); err != nil {
				log.Printf("ingress %s stopped: %v", gateID, err)
			}
		}(l, gate.ID)
	}
	log.Println("mma2 ingress started")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	got := <-sig
	log.Printf("mma2 shutting down (%s)", got)
	for i := len(shutdown) - 1; i >= 0; i-- {
		shutdown[i]()
	}
}
