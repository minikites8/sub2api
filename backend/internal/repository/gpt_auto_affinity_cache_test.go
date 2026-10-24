package repository

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGPTAutoAffinityConcurrentCAS(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	defer client.Close()
	store := &gptAutoAffinityStore{rdb: client}
	ctx := context.Background()
	require.NoError(t, store.Set(ctx, "session", service.GPTAutoAffinity{Model: service.GPTAutoModelGPT61Sol, Sequence: 1}, time.Hour))
	var successes atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := store.CompareAndSet(ctx, "session", 1, service.GPTAutoAffinity{Sequence: 2, DowngradeCount: 1}, time.Hour)
			if err == nil && ok {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	require.Equal(t, int64(1), successes.Load())
	v, err := store.Get(ctx, "session")
	require.NoError(t, err)
	require.Equal(t, 1, v.DowngradeCount)
	srv.FastForward(2 * time.Hour)
	ok, err := store.CompareAndSet(ctx, "session", 2, service.GPTAutoAffinity{Sequence: 3}, time.Hour)
	require.NoError(t, err)
	require.False(t, ok)
}
