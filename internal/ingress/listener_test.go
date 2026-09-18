package ingress

import (
	"net"
	"testing"
	"time"

	"mma2/internal/config"
)

func TestListenerCloseUnblocksAccept(t *testing.T) {
	l := NewListener(config.IngressGate{ID: "lab", Listen: "127.0.0.1:0"})
	errc := make(chan error, 1)
	go func() {
		errc <- l.ListenAndServe(func(net.Conn) {}, func(net.Conn) {})
	}()

	deadline := time.Now().Add(time.Second)
	for {
		l.mu.Lock()
		ln := l.ln
		l.mu.Unlock()
		if ln != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("listener did not bind")
		}
		time.Sleep(time.Millisecond)
	}

	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("ListenAndServe after Close: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ListenAndServe did not return after Close")
	}
	if err := l.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}
