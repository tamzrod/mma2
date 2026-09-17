package rbe

import (
	"io"
	"net"
	"testing"
	"time"
)

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
