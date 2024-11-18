package hacox

import (
	"context"
	"io"
	"log"
	"maps"
	"math/rand"
	"net"
	"slices"
	"sync"
	"time"
)

type Proxy struct {
	listenAddrs []string
	backends    []string
	conns       map[string]map[net.Conn]struct{}
	connsCount  map[string]int
	lock        sync.RWMutex
	dialer      *net.Dialer
}

func NewProxy(listenAddrs []string, backends ...string) *Proxy {
	return &Proxy{
		listenAddrs: listenAddrs,
		backends:    backends,
		conns:       make(map[string]map[net.Conn]struct{}),
		connsCount:  make(map[string]int),
		dialer: &net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 5 * time.Second,
		},
	}
}

func (p *Proxy) GetBackendsClientsCount() map[string]int {
	p.lock.RLock()
	defer p.lock.RUnlock()

	counts := maps.Clone(p.connsCount)
	for _, backend := range p.backends {
		if _, ok := counts[backend]; !ok {
			counts[backend] = 0
		}
	}

	return counts
}

func (p *Proxy) UpdateBackends(backends []string) {
	var oldBackends []string

	p.lock.Lock()
	if slices.Equal(p.backends, backends) {
		p.lock.Unlock()
		return
	}
	oldBackends = slices.Clone(p.backends)
	p.backends = slices.Clone(backends)
	p.lock.Unlock()

	var removed []string

	for _, it := range oldBackends {
		if !slices.Contains(backends, it) {
			removed = append(removed, it)
		}
	}

	for _, it := range removed {
		p.delBackend(it)
	}
}

func (p *Proxy) OnNotify(backend string, healthy bool) {
	if healthy {
		p.addBackend(backend)
	} else {
		p.delBackend(backend)
	}
}

func (p *Proxy) addBackend(backend string) {
	p.lock.Lock()
	defer p.lock.Unlock()

	if slices.Contains(p.backends, backend) {
		return
	}

	p.backends = append(p.backends, backend)
}

func (p *Proxy) delBackend(backend string) {
	p.lock.Lock()
	defer p.lock.Unlock()

	if conns, ok := p.conns[backend]; ok {
		for conn := range conns {
			conn.Close()
		}
	}

	delete(p.conns, backend)
	delete(p.connsCount, backend)

	if idx := slices.Index(p.backends, backend); idx != -1 {
		p.backends = slices.Delete(p.backends, idx, idx+1)
	}
}

func (p *Proxy) Start(ctx context.Context) error {
	lc := net.ListenConfig{
		KeepAlive: 5 * time.Second,
	}

	for _, listenAddr := range p.listenAddrs {
		listener, err := lc.Listen(context.Background(), "tcp", listenAddr)
		if err != nil {
			return err
		}
		defer listener.Close()

		go func(addr string, listener net.Listener, ctx context.Context) {
			for {
				select {
				case <-ctx.Done():
					return
				default:
					conn, err := listener.Accept()
					if err != nil {
						log.Printf("accept connection from %s error: %v", addr, err)
						continue
					}
					go p.connect(conn)
				}
			}
		}(listenAddr, listener, ctx)
	}

	<-ctx.Done()
	return nil
}

func (p *Proxy) getBackend() string {
	p.lock.RLock()
	defer p.lock.RUnlock()

	if len(p.backends) == 0 {
		return ""
	}

	return p.backends[rand.Intn(len(p.backends))]
}

func (p *Proxy) addConn(backend string, conn net.Conn) {
	p.lock.Lock()
	defer p.lock.Unlock()

	if _, ok := p.conns[backend]; !ok {
		p.conns[backend] = make(map[net.Conn]struct{})
	}

	p.conns[backend][conn] = struct{}{}
}

func (p *Proxy) incCount(backend string) {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.connsCount[backend]++
}

func (p *Proxy) decCount(backend string) {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.connsCount[backend]--
}

func (p *Proxy) delConn(backend string, conn net.Conn) {
	conn.Close()

	p.lock.Lock()
	defer p.lock.Unlock()

	if conns, ok := p.conns[backend]; ok {
		delete(conns, conn)
	}
}

func (p *Proxy) connect(conn net.Conn) {
	backend := p.getBackend()
	if backend == "" {
		log.Println("no backend available")
		conn.Close()
		return
	}

	defer p.delConn(backend, conn)

	backConn, err := p.dialer.Dial("tcp", backend)
	if err != nil {
		return
	}

	defer p.delConn(backend, backConn)
	p.addConn(backend, backConn)

	p.addConn(backend, conn)
	p.incCount(backend)
	defer p.decCount(backend)

	go io.Copy(backConn, conn)
	io.Copy(conn, backConn)
}
