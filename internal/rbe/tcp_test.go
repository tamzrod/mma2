package rbe

import (
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// flakyListener returns one transient Temporary() error before delegating to a
// real listener, modelling fd exhaustion or a similar recoverable condition.
type flakyListener struct {
	real net.Listener
	once sync.Once
}

type temporaryError struct{}

func (temporaryError) Error() string   { return "transient accept failure" }
func (temporaryError) Timeout() bool   { return false }
func (temporaryError) Temporary() bool { return true }

func (l *flakyListener) Accept() (net.Conn, error) {
	var err error
	l.once.Do(func() { err = temporaryError{} })
	if err != nil {
		return nil, err
	}
	return l.real.Accept()
}

func (l *flakyListener) Close() error   { return l.real.Close() }
func (l *flakyListener) Addr() net.Addr { return l.real.Addr() }

func TestTCPPublisherOneByteAndPersistentConnection(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewTCPPublisher(listener, 16)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	deadline := time.Now().Add(time.Second)
	for {
		p.mu.Lock()
		count := len(p.clients)
		p.mu.Unlock()
		if count == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("subscriber was not registered")
		}
		time.Sleep(time.Millisecond)
	}

	p.Publish(0) // reserved: must not put a byte on the stream
	p.Publish(1)
	p.Publish(2)
	p.Publish(255)
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 3)
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatal(err)
	}
	for i, want := range []byte{1, 2, 255} {
		if got[i] != want {
			t.Fatalf("byte %d = %d; want %d", i, got[i], want)
		}
	}
	if err := conn.SetReadDeadline(time.Now().Add(15 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	var extra [1]byte
	if n, err := conn.Read(extra[:]); n != 0 || err == nil {
		t.Fatalf("unexpected extra packet: n=%d err=%v", n, err)
	}
}

func TestTCPPublisherCloseAndValidation(t *testing.T) {
	if _, err := NewTCPPublisher(nil, 1); err == nil {
		t.Fatal("nil listener accepted")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewTCPPublisher(listener, 0); err == nil {
		t.Fatal("zero queue accepted")
	}
	p, err := NewTCPPublisher(listener, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	p.Publish(1) // must not panic after Close
}

func TestTCPPublisherOverflowDisconnectsSlowSubscriber(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewTCPPublisher(listener, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	deadline := time.Now().Add(time.Second)
	for {
		p.mu.Lock()
		count := len(p.clients)
		p.mu.Unlock()
		if count == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("subscriber was not registered")
		}
		time.Sleep(time.Millisecond)
	}

	// Do not read: fill the one-slot queue, then overflow must drop the client.
	p.Publish(1)
	p.Publish(2)
	p.Publish(3)

	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 8)
	_, err = io.ReadFull(conn, buf[:1])
	// The subscriber may receive the in-flight byte, but the socket must close
	// instead of delivering an unbounded stream after the queue overflow.
	deadline = time.Now().Add(time.Second)
	for {
		p.mu.Lock()
		count := len(p.clients)
		p.mu.Unlock()
		if count == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("overflow left %d subscribers connected", count)
		}
		time.Sleep(time.Millisecond)
	}

	_ = conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	for {
		n, readErr := conn.Read(buf)
		if n == 0 || readErr != nil {
			if readErr == nil {
				t.Fatal("overflow disconnect returned n=0 with no error")
			}
			break
		}
	}
}

func TestTCPPublisherIndependentSubscriberQueues(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewTCPPublisher(listener, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	c1, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c1.Close()
	c2, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()

	deadline := time.Now().Add(time.Second)
	for {
		p.mu.Lock()
		count := len(p.clients)
		p.mu.Unlock()
		if count == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("both subscribers were not registered")
		}
		time.Sleep(time.Millisecond)
	}

	p.Publish(7)
	for i, conn := range []net.Conn{c1, c2} {
		if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		var b [1]byte
		if _, err := io.ReadFull(conn, b[:]); err != nil {
			t.Fatalf("subscriber %d: %v", i, err)
		}
		if b[0] != 7 {
			t.Fatalf("subscriber %d got %d", i, b[0])
		}
	}
}

// A single transient accept error must not permanently disable RBE delivery.
func TestTCPPublisherTransientAcceptErrorIsNotFatal(t *testing.T) {
	real, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewTCPPublisher(&flakyListener{real: real}, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	// Give the accept loop time to consume the injected transient error.
	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("tcp", real.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	deadline := time.Now().Add(time.Second)
	for {
		p.mu.Lock()
		n := len(p.clients)
		p.mu.Unlock()
		if n == 1 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("publisher stopped accepting after transient error (clients=%d)", n)
		}
		time.Sleep(time.Millisecond)
	}
}

func waitClients(t *testing.T, p *TCPPublisher, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		p.mu.Lock()
		n := len(p.clients)
		p.mu.Unlock()
		if n == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("clients=%d want %d", n, want)
		}
		time.Sleep(time.Millisecond)
	}
}

func closeWithTimeout(t *testing.T, p *TCPPublisher) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- p.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close blocked (likely mutex held across queue or socket close)")
	}
}

func TestTCPPublisherCloseWithLiveSubscriberDoesNotDeadlock(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewTCPPublisher(listener, 8)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	waitClients(t, p, 1)
	closeWithTimeout(t, p)
	closeWithTimeout(t, p)
}

func TestTCPPublisherOverflowThenCloseDoesNotDeadlock(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewTCPPublisher(listener, 1)
	if err != nil {
		t.Fatal(err)
	}
	slow, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer slow.Close()
	fast, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer fast.Close()
	waitClients(t, p, 2)

	stop := make(chan struct{})
	var drain sync.WaitGroup
	drain.Add(1)
	go func() {
		defer drain.Done()
		buf := make([]byte, 1)
		for {
			_ = fast.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
			select {
			case <-stop:
				return
			default:
			}
			_, err := fast.Read(buf)
			if err != nil {
				select {
				case <-stop:
					return
				default:
				}
			}
		}
	}()

	pubDone := make(chan struct{})
	go func() {
		for i := 0; i < 8; i++ {
			p.Publish(3)
		}
		close(pubDone)
	}()
	select {
	case <-pubDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked during overflow")
	}
	close(stop)
	drain.Wait()
	closeWithTimeout(t, p)
}
