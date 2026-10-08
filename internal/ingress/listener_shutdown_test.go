package ingress

import (
    "net"
    "testing"
    "time"

    "mma2/internal/config"
)

// Closing ingress must wait for an accepted handler to finish, before
// the caller performs the final persistence flush.
func TestCloseWaitsForActiveHandler(t *testing.T) {
    l:=NewListener(config.IngressGate{ID:"shutdown"})
    client,server:=net.Pipe()
    defer client.Close()

    entered:=make(chan struct{})
    release:=make(chan struct{})
    completed:=make(chan struct{})
    l.mu.Lock()
    l.active[server]=struct{}{}
    l.wg.Add(1)
    l.mu.Unlock()
    go l.handleConn(server,func(net.Conn){
        close(entered)
        <-release
    },func(net.Conn){
        close(entered)
        <-release
    })
    if _,err:=client.Write([]byte{0,1,2});err!=nil {t.Fatal(err)}
    select {
    case <-entered:
    case <-time.After(time.Second): t.Fatal("handler did not start")
    }
    go func(){_ = l.Close(); close(completed)}()
    select {
    case <-completed: t.Fatal("Close returned while handler was active")
    case <-time.After(20*time.Millisecond):
    }
    close(release)
    select {
    case <-completed:
    case <-time.After(time.Second): t.Fatal("Close did not wait for handler")
    }
}

func TestCloseConcurrentCallsAreIdempotent(t *testing.T) {
    l:=NewListener(config.IngressGate{ID:"concurrent-close"})
    results:=make(chan error,8)
    for i:=0;i<8;i++ {
        go func(){results<-l.Close()}()
    }
    for i:=0;i<8;i++ {
        select {
        case err:=<-results:
            if err!=nil {t.Fatal(err)}
        case <-time.After(2*time.Second):
            t.Fatal("concurrent Close blocked")
        }
    }
}
