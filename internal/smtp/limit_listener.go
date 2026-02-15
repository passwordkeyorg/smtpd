package smtp

import (
	"net"
	"sync"
)

// limitListener caps the number of simultaneously open connections accepted
// from an underlying listener.
//
// It is intentionally simple and predictable: a fixed-size semaphore is
// acquired on Accept and released when the connection is closed.
//
// Note: this controls concurrent connections, not message throughput.
type limitListener struct {
	base net.Listener
	sem  chan struct{}
}

func newLimitListener(base net.Listener, maxConns int) net.Listener {
	if maxConns <= 0 {
		return base
	}
	return &limitListener{base: base, sem: make(chan struct{}, maxConns)}
}

func (l *limitListener) Accept() (net.Conn, error) {
	l.sem <- struct{}{}
	c, err := l.base.Accept()
	if err != nil {
		<-l.sem
		return nil, err
	}
	return &limitConn{Conn: c, once: sync.Once{}, release: func() { <-l.sem }}, nil
}

func (l *limitListener) Close() error   { return l.base.Close() }
func (l *limitListener) Addr() net.Addr { return l.base.Addr() }

type limitConn struct {
	net.Conn
	once    sync.Once
	release func()
}

func (c *limitConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(c.release)
	return err
}
