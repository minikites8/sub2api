package keyedlock

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPoolScopesCancellationAndCleanup(t *testing.T) {
	var p Pool
	release, err := p.Acquire(t.Context(), "account:1:model:a")
	if err != nil {
		t.Fatal(err)
	}
	other, err := p.Acquire(t.Context(), "account:2:model:a")
	if err != nil {
		t.Fatal(err)
	}
	other()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	_, err = p.AcquireMany(ctx, []string{"account:0:model:a", "account:1:model:a"})
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	release()
	release()
	if len(p.entries) != 0 {
		t.Fatalf("retained keys: %d", len(p.entries))
	}
	release, err = p.AcquireMany(t.Context(), []string{"b", "a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	release()
	if len(p.entries) != 0 {
		t.Fatal("duplicate key leaked")
	}
}

func TestPoolOrdersConflictingModelSets(t *testing.T) {
	var p Pool
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	done := make(chan error, 2)
	for _, keys := range [][]string{{"b", "a"}, {"a", "b"}} {
		go func(keys []string) {
			release, err := p.AcquireMany(ctx, keys)
			if err == nil {
				release()
			}
			done <- err
		}(keys)
	}
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if len(p.entries) != 0 {
		t.Fatal("idle keys retained")
	}
}
