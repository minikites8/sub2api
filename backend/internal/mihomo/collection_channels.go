package mihomo

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Collection channels share a configuration epoch. Each live lease exclusively
// owns its selectors, while management reloads wait for the last lease to close.
func (m *Manager) lockCollectionSetup(ctx context.Context) (func(), error) {
	m.mu.Lock()
	if m.collectionGate == nil {
		m.collectionGate = make(chan struct{}, 1)
		m.collectionChanged = make(chan struct{})
	}
	gate := m.collectionGate
	m.mu.Unlock()
	select {
	case gate <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		<-gate
		return nil, err
	}
	return func() { <-gate }, nil
}

func (m *Manager) reserveCollectionChannels(ctx context.Context, count int) ([]int, []int, func() error, error) {
	if count < 1 {
		return nil, nil, nil, errors.New("collection requires at least one lane")
	}
	for {
		unlock, err := m.lockCollectionSetup(ctx)
		if err != nil {
			return nil, nil, nil, err
		}
		if len(m.collectionPorts) == 0 {
			if err = m.acquire(ctx); err != nil {
				unlock()
				return nil, nil, nil, err
			}
			m.mu.Lock()
			snapshot := m.saved
			ready := !m.closed && m.state.Running
			m.mu.Unlock()
			if !ready {
				m.release()
				unlock()
				return nil, nil, nil, errors.New("managed Mihomo is not running")
			}
			capacity := max(count, defaultCollectLanes)
			if count >= defaultCollectLanes {
				// Keep room for target pinning and another account while a
				// full-size parallel task owns its requested worker channels.
				capacity += defaultCollectLanes
			}
			provision := &Collection{m: m, ports: make([]int, capacity)}
			for i := range provision.ports {
				provision.ports[i] = collectPort + i
			}
			if capacity > defaultCollectLanes {
				err = provision.extend(ctx, snapshot)
				if err != nil {
					if provision.extended {
						err = errors.Join(err, m.restoreCollectionConfig())
					}
					m.release()
					unlock()
					return nil, nil, nil, err
				}
			}
			m.collectionPorts = provision.ports
			m.collectionBusy = make([]bool, len(provision.ports))
			m.collectionExtended = provision.extended
		}
		lanes := make([]int, 0, count)
		ports := make([]int, 0, count)
		for lane, busy := range m.collectionBusy {
			if !busy {
				lanes = append(lanes, lane)
				ports = append(ports, m.collectionPorts[lane])
				if len(lanes) == count {
					break
				}
			}
		}
		if len(lanes) == 0 {
			changed := m.collectionChanged
			unlock()
			select {
			case <-ctx.Done():
				return nil, nil, nil, ctx.Err()
			case <-changed:
			}
			continue
		}
		for _, lane := range lanes {
			m.collectionBusy[lane] = true
		}
		unlock()
		var once sync.Once
		var closeErr error
		release := func() error {
			once.Do(func() {
				done, _ := m.lockCollectionSetup(context.Background())
				defer done()
				for _, lane := range lanes {
					m.collectionBusy[lane] = false
				}
				busy := false
				for _, used := range m.collectionBusy {
					busy = busy || used
				}
				if !busy {
					m.mu.Lock()
					dirty := m.saved.UseOnce
					m.mu.Unlock()
					if dirty || m.collectionExtended {
						closeErr = m.restoreCollectionConfig()
					}
					m.collectionPorts = nil
					m.collectionBusy = nil
					m.collectionExtended = false
					m.release()
				}
				close(m.collectionChanged)
				m.collectionChanged = make(chan struct{})
			})
			return closeErr
		}
		return lanes, ports, release, nil
	}
}

func (m *Manager) restoreCollectionConfig() error {
	m.mu.Lock()
	snapshot := m.saved
	m.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	config, err := m.config(snapshot)
	if err == nil {
		err = m.reload(ctx, config, snapshot.Secret)
	}
	if err != nil {
		m.stop()
		return errors.New("cannot restore collection configuration; managed proxy stopped")
	}
	return nil
}
