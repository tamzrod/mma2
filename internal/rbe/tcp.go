package rbe

import (
	"errors"
	"net"
	"sync"
	"time"
)

// acceptRetryDelay bounds how fast the accept loop spins on transient errors.
const acceptRetryDelay = 5 * time.Millisecond

// TCPPublisher publishes exactly one RuleID byte per event. MMA owns the
// listening socket; subscribers connect and read bytes from a persistent TCP
// connection. Neither Publish nor a memory write waits for network I/O.
//
// Events are ephemeral: there is no replay or snapshot on connection. A
// subscriber MUST reconcile state over Modbus after every connection.
// If its bounded queue fills, the subscriber is disconnected so it can
// reconnect and reconcile rather than silently continue with stale state.
type TCPPublisher struct {
	listener net.Listener
	queueSize int

	mu      sync.Mutex
	clients map[net.Conn]chan byte
	closed  bool
	done    chan struct{}
}

// NewTCPPublisher takes ownership of an already-bound listener, allowing
// callers to fail startup if the RBE port cannot be bound.
func NewTCPPublisher(listener net.Listener, queueSize int) (*TCPPublisher, error) {
	if listener == nil {
		return nil, errors.New("rbe: TCP listener is nil")
	}
	if queueSize < 1 {
		return nil, errors.New("rbe: TCP queue size must be positive")
	}
	p := &TCPPublisher{
		listener:  listener,
		queueSize: queueSize,
		clients:   make(map[net.Conn]chan byte),
		done:      make(chan struct{}),
	}
	go p.acceptLoop()
	return p, nil
}

// Publish is nonblocking and never performs network I/O. Zero is reserved and
// is not a valid rule ID. Each connected client has an independent queue.
func (p *TCPPublisher) Publish(id uint8) {
	if p == nil || id == 0 {
		return
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	var dropped []net.Conn
	for conn, queue := range p.clients {
		select {
		case queue <- id:
		default:
			// A gap cannot be signalled by this one-byte protocol. Force
			// a reconnect, upon which the PPC must read its Modbus state.
			delete(p.clients, conn)
			close(queue)
			dropped = append(dropped, conn)
		}
	}
	p.mu.Unlock()
	for _, conn := range dropped {
		_ = conn.Close()
	}
}

func (p *TCPPublisher) acceptLoop() {
	for {
		conn, err := p.listener.Accept()
		if err != nil {
			p.mu.Lock()
			closed := p.closed
			p.mu.Unlock()
			if closed {
				return
			}
			// Transient accept failures (for example fd exhaustion) must not
			// permanently disable RBE delivery. Back off briefly and retry;
			// only a permanent listener error is terminal.
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				select {
				case <-p.done:
					return
				case <-time.After(acceptRetryDelay):
				}
				continue
			}
			_ = p.Close()
			return
		}
		if tcp, ok := conn.(*net.TCPConn); ok {
			_ = tcp.SetNoDelay(true)
		}
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			_ = conn.Close()
			return
		}
		queue := make(chan byte, p.queueSize)
		p.clients[conn] = queue
		p.mu.Unlock()
		go p.writeLoop(conn, queue)
	}
}

func (p *TCPPublisher) writeLoop(conn net.Conn, queue <-chan byte) {
	defer func() {
		p.mu.Lock()
		delete(p.clients, conn)
		p.mu.Unlock()
		_ = conn.Close()
	}()
	for {
		select {
		case id, ok := <-queue:
			if !ok {
				return
			}
			// net.Conn.Write is allowed to return a short write.
			data := [1]byte{id}
			n, err := conn.Write(data[:])
			if err != nil || n != 1 {
				return
			}
		case <-p.done:
			return
		}
	}
}

// Close closes the listener and all client connections. It is idempotent.
func (p *TCPPublisher) Close() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	close(p.done)
	err := p.listener.Close()
	for conn, queue := range p.clients {
		close(queue)
		_ = conn.Close()
		delete(p.clients, conn)
	}
	return err
}
