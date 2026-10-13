// Package keyedlock provides cancellable, reference-counted resource queues.
package keyedlock

import (
	"context"
	"sort"
	"sync"
)

type entry struct {
	token chan struct{}
	refs  int
}

// Pool is ready for use with its zero value. Idle keys are removed on release.
type Pool struct {
	mu      sync.Mutex
	entries map[string]*entry
}

func (p *Pool) Acquire(ctx context.Context, key string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p.mu.Lock()
	if p.entries == nil {
		p.entries = make(map[string]*entry)
	}
	e := p.entries[key]
	if e == nil {
		e = &entry{token: make(chan struct{}, 1)}
		e.token <- struct{}{}
		p.entries[key] = e
	}
	e.refs++
	p.mu.Unlock()
	drop := func() {
		p.mu.Lock()
		e.refs--
		if e.refs == 0 {
			delete(p.entries, key)
		}
		p.mu.Unlock()
	}
	select {
	case <-ctx.Done():
		drop()
		return nil, ctx.Err()
	case <-e.token:
	}
	if err := ctx.Err(); err != nil {
		e.token <- struct{}{}
		drop()
		return nil, err
	}
	var once sync.Once
	return func() { once.Do(func() { e.token <- struct{}{}; drop() }) }, nil
}

// AcquireMany uses stable key order to avoid cycles in overlapping model sets.
func (p *Pool) AcquireMany(ctx context.Context, keys []string) (func(), error) {
	keys = append([]string(nil), keys...)
	sort.Strings(keys)
	releases := make([]func(), 0, len(keys))
	var once sync.Once
	release := func() {
		once.Do(func() {
			for i := len(releases) - 1; i >= 0; i-- {
				releases[i]()
			}
		})
	}
	for i, key := range keys {
		if i > 0 && key == keys[i-1] {
			continue
		}
		unlock, err := p.Acquire(ctx, key)
		if err != nil {
			release()
			return nil, err
		}
		releases = append(releases, unlock)
	}
	return release, nil
}
