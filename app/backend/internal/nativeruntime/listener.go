package nativeruntime

import (
	"net"
	"sync"
)

// The privileged HTTP relay's connections are bounded independently of the
// unprivileged unit's cgroup; idle connections cannot grow host memory without limit.
type boundedListener struct {
	net.Listener
	slots chan struct{}
}

func (l *boundedListener) Accept() (net.Conn, error) {
	for {
		c, e := l.Listener.Accept()
		if e != nil {
			return nil, e
		}
		select {
		case l.slots <- struct{}{}:
			return &boundedConnection{Conn: c, release: func() { <-l.slots }}, nil
		default:
			_ = c.Close()
		}
	}
}

type boundedConnection struct {
	net.Conn
	once    sync.Once
	release func()
}

func (c *boundedConnection) Close() error { e := c.Conn.Close(); c.once.Do(c.release); return e }
