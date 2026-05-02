// internal/accessevents/engine.go
package accessevents

import (
	"log"
	"sync"
	"time"
)

// Engine aggregates access control decisions and fans out events to subscribers.
// All public methods are safe for concurrent use. Record must never block.
type Engine struct {
	cfg       *AccessEventsConfig
	mu        sync.Mutex
	state     map[Key]windowState
	broadcast chan Event

	subMu sync.Mutex
	subs  []chan Event
}

// New creates and starts a running Engine from the given configuration.
// It launches the broadcast and cleanup goroutines.
func New(cfg *AccessEventsConfig) *Engine {
	e := &Engine{
		cfg:       cfg,
		state:     make(map[Key]windowState),
		broadcast: make(chan Event, 1024),
	}
	go e.broadcastLoop()
	go e.runCleanupLoop()
	return e
}

// Record observes one access control decision. It is non-blocking: events are
// dropped silently when the system is at capacity or the broadcast channel is full.
func (e *Engine) Record(srcIP string, port uint16, unit uint8, fc uint8, allowed bool) {
	action := fcToAction(fc)
	if action == "" {
		// Unknown function code: ignored per design contract.
		return
	}

	status := "denied"
	if allowed {
		status = "allowed"
	}

	k := Key{
		SrcIP:        srcIP,
		FunctionCode: fc,
		Action:       action,
		Status:       status,
		Port:         port,
		Unit:         uint16(unit),
	}

	now := time.Now().UnixNano()
	windowNanos := int64(e.cfg.Window) * int64(time.Second)

	var toEmit []Event

	e.mu.Lock()
	s, exists := e.state[k]
	switch {
	case !exists:
		// Step 1: first event, no active window.
		if len(e.state) >= e.cfg.Limits.MaxKeys {
			// Overflow: drop the new key; existing keys continue.
			e.mu.Unlock()
			return
		}
		e.state[k] = windowState{windowStart: now}
		toEmit = append(toEmit, buildEvent(k, now))

	case now < s.windowStart+windowNanos:
		// Step 2: within the active window; suppress and count.
		s.suppressedCount++
		e.state[k] = s

	default:
		// Step 3+4: window has expired.
		// Emit a summary for the closed window if anything was suppressed,
		// then open a new window for the arriving event.
		if s.suppressedCount > 0 {
			toEmit = append(toEmit, e.buildSummary(k, s, now))
		}
		e.state[k] = windowState{windowStart: now}
		toEmit = append(toEmit, buildEvent(k, now))
	}
	e.mu.Unlock()

	for _, evt := range toEmit {
		select {
		case e.broadcast <- evt:
		default:
		}
	}
}

// Subscribe returns a buffered per-client channel that receives events.
// The caller must call Unsubscribe when the client disconnects.
func (e *Engine) Subscribe() chan Event {
	ch := make(chan Event, 64)
	e.subMu.Lock()
	e.subs = append(e.subs, ch)
	e.subMu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel and closes it.
// Sending to the channel stops after this call returns.
func (e *Engine) Unsubscribe(ch chan Event) {
	e.subMu.Lock()
	defer e.subMu.Unlock()
	for i, s := range e.subs {
		if s == ch {
			e.subs = append(e.subs[:i], e.subs[i+1:]...)
			close(ch)
			return
		}
	}
}

// broadcastLoop reads from the internal broadcast channel and fans out each
// event to all subscriber channels. Each per-client send is non-blocking:
// slow consumers have events dropped rather than stalling the loop.
func (e *Engine) broadcastLoop() {
	for evt := range e.broadcast {
		e.subMu.Lock()
		for _, sub := range e.subs {
			select {
			case sub <- evt:
			default:
			}
		}
		e.subMu.Unlock()
	}
}

// runCleanupLoop runs TTL-based cleanup at twice the window interval.
// It recovers from panics and restarts itself automatically.
func (e *Engine) runCleanupLoop() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("accessevents: cleanup panic recovered: %v; restarting", r)
			go e.runCleanupLoop()
		}
	}()

	interval := time.Duration(e.cfg.Window*2) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		e.runCleanup()
	}
}

// runCleanup iterates the state map and removes keys whose TTL has elapsed.
// Summary events are emitted for any removed key with a non-zero suppressed count.
// This function never emits events for keys that have zero suppressed count.
func (e *Engine) runCleanup() {
	now := time.Now().UnixNano()
	ttlNanos := int64(e.cfg.Limits.TTL) * int64(time.Second)

	e.mu.Lock()
	var toEmit []Event
	for k, s := range e.state {
		if now >= s.windowStart+ttlNanos {
			if s.suppressedCount > 0 {
				toEmit = append(toEmit, e.buildSummary(k, s, now))
			}
			delete(e.state, k)
		}
	}
	e.mu.Unlock()

	for _, evt := range toEmit {
		select {
		case e.broadcast <- evt:
		default:
		}
	}
}

// buildEvent creates an individual (non-summary) event for a key.
func buildEvent(k Key, ts int64) Event {
	return Event{
		Ts:           ts,
		Port:         k.Port,
		Unit:         k.Unit,
		FunctionCode: k.FunctionCode,
		Action:       k.Action,
		Status:       k.Status,
		SrcIP:        k.SrcIP,
	}
}

// buildSummary creates a summary event for a closed aggregation window.
// Count and WindowSec are only included when IncludeCounter is configured.
func (e *Engine) buildSummary(k Key, s windowState, ts int64) Event {
	evt := buildEvent(k, ts)
	if e.cfg.IncludeCounter {
		evt.Count = s.suppressedCount
		evt.WindowSec = e.cfg.Window
	}
	return evt
}

// fcToAction maps a Modbus function code to its action string.
// Returns "" for function codes that are not tracked.
func fcToAction(fc uint8) string {
	switch fc {
	case 1, 2, 3, 4:
		return "read"
	case 5, 6, 15, 16:
		return "write"
	default:
		return ""
	}
}
