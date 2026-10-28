package nativeruntime

import (
	"net"
	"testing"
	"time"
)

func TestNativeConnectionLimitReleasesExactlyOnceAndRefusesOverflow(t *testing.T) {
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	l := &boundedListener{Listener: listener, slots: make(chan struct{}, 1)}
	defer l.Close()
	first, e := net.Dial("tcp", listener.Addr().String())
	if e != nil {
		t.Fatal(e)
	}
	defer first.Close()
	owned, e := l.Accept()
	if e != nil {
		t.Fatal(e)
	}
	result := make(chan net.Conn, 1)
	go func() {
		c, e := l.Accept()
		if e == nil {
			result <- c
		}
	}()
	overflow, e := net.Dial("tcp", listener.Addr().String())
	if e != nil {
		t.Fatal(e)
	}
	defer overflow.Close()
	_ = overflow.SetReadDeadline(time.Now().Add(time.Second))
	var buf [1]byte
	if _, e = overflow.Read(buf[:]); e == nil {
		t.Fatal("overflow connection survived")
	}
	_ = owned.Close()
	_ = owned.Close()
	if len(l.slots) != 0 {
		t.Fatal("duplicate close leaked a slot")
	}
	next, e := net.Dial("tcp", listener.Addr().String())
	if e != nil {
		t.Fatal(e)
	}
	defer next.Close()
	select {
	case c := <-result:
		_ = c.Close()
	case <-time.After(time.Second):
		t.Fatal("released connection slot not reusable")
	}
}
