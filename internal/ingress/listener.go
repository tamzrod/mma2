// internal/ingress/listener.go
package ingress

import (
	"bufio"
	"log"
	"net"
	"sync"
	"time"

	"mma2/internal/config"
)

const acceptRetryDelay = 5 * time.Millisecond

// bufferedConn ensures all reads flow through a bufio.Reader that already peeked.
type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	return c.r.Read(p)
}

// Listener represents a TCP ingress gate.
type Listener struct {
	cfg config.IngressGate

	mu     sync.Mutex
	ln     net.Listener
	closed bool
	active map[net.Conn]struct{}
	wg sync.WaitGroup
	closeOnce sync.Once
	closeDone chan struct{}
	closeErr error
}

// NewListener creates a new ingress listener.
func NewListener(cfg config.IngressGate) *Listener {
	return &Listener{cfg: cfg, active: make(map[net.Conn]struct{}), closeDone: make(chan struct{})}
}

// ListenAndServe starts the TCP listener and dispatches connections.
// It returns nil after Close, and a non-nil error only for a failed bind
// or a permanent accept error while the listener is still open.
func (l *Listener) ListenAndServe(
	onModbus func(net.Conn),
	onRawIngest func(net.Conn),
) error {
	ln, err := net.Listen("tcp", l.cfg.Listen)
	if err != nil {
		return err
	}

	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		_ = ln.Close()
		return nil
	}
	l.ln = ln
	l.mu.Unlock()

	log.Printf("ingress %s listening on %s", l.cfg.ID, l.cfg.Listen)

	for {
		conn, err := ln.Accept()
		if err != nil {
			l.mu.Lock()
			closed := l.closed
			l.mu.Unlock()
			if closed {
				return nil
			}
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				time.Sleep(acceptRetryDelay)
				continue
			}
			return err
		}
		l.mu.Lock()
		if l.closed {
			l.mu.Unlock()
			_ = conn.Close()
			return nil
		}
		l.active[conn] = struct{}{}
		l.wg.Add(1)
		l.mu.Unlock()
		go l.handleConn(conn, onModbus, onRawIngest)
	}
}

// Close stops Accept, closes active sockets, and waits for all handlers to exit.
// The caller can safely perform a final persistence flush after it returns.
func (l *Listener) Close() error {
	if l == nil {
		return nil
	}
	l.closeOnce.Do(func() {
		l.mu.Lock()
		l.closed = true
		ln := l.ln
		l.ln = nil
		connections := make([]net.Conn, 0, len(l.active))
		for conn := range l.active {
			connections = append(connections, conn)
		}
		l.mu.Unlock()

		// Never invoke socket Close while holding the listener mutex.
		if ln != nil {
			l.closeErr = ln.Close()
		}
		for _, conn := range connections {
			_ = conn.Close()
		}
		// Every accepted handler is registered while holding l.mu before
		// shutdown sets closed, so no new WaitGroup Add can follow.
		l.wg.Wait()
		close(l.closeDone)
	})
	<-l.closeDone
	return l.closeErr
}

func (l *Listener) handleConn(
	conn net.Conn,
	onModbus func(net.Conn),
	onRawIngest func(net.Conn),
) {
	defer func() {
		_ = conn.Close()
		l.mu.Lock()
		delete(l.active, conn)
		l.mu.Unlock()
		l.wg.Done()
	}()
	proto, reader, err := Classify(conn)
	if err != nil {
		conn.Close()
		return
	}

	// Important: after Peek(), all subsequent reads must use reader.
	bc := &bufferedConn{Conn: conn, r: reader}

	switch proto {
	case ProtocolModbus:
		onModbus(bc)
		return

	case ProtocolRawIngest:
		onRawIngest(bc)
		return

	default:
		conn.Close()
	}
}
