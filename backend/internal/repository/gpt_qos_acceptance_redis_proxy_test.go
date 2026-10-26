//go:build integration

package repository

import (
	"io"
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// A disposable TCP relay lets the two real services lose Redis connections
// without touching the Docker daemon or any Redis outside this test fixture.
type acceptanceRedisProxy struct {
	mu       sync.Mutex
	disabled bool
	conns    map[net.Conn]bool
	listener net.Listener
	target   string
}

func newAcceptanceRedisProxy(t *testing.T, target string) *acceptanceRedisProxy {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	p := &acceptanceRedisProxy{listener: l, target: target, conns: map[net.Conn]bool{}}
	t.Cleanup(func() { p.block(true); _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go p.relay(c)
		}
	}()
	return p
}
func (p *acceptanceRedisProxy) relay(client net.Conn) {
	p.mu.Lock()
	if p.disabled {
		p.mu.Unlock()
		client.Close()
		return
	}
	p.conns[client] = true
	p.mu.Unlock()
	defer func() { client.Close(); p.mu.Lock(); delete(p.conns, client); p.mu.Unlock() }()
	upstream, err := net.Dial("tcp", p.target)
	if err != nil {
		return
	}
	defer upstream.Close()
	p.mu.Lock()
	if p.disabled {
		p.mu.Unlock()
		return
	}
	p.conns[upstream] = true
	p.mu.Unlock()
	defer func() { p.mu.Lock(); delete(p.conns, upstream); p.mu.Unlock() }()
	done := make(chan struct{})
	go func() { _, _ = io.Copy(upstream, client); upstream.Close(); close(done) }()
	_, _ = io.Copy(client, upstream)
	client.Close()
	<-done
}
func (p *acceptanceRedisProxy) block(v bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.disabled = v
	if v {
		for c := range p.conns {
			_ = c.Close()
		}
	}
}
