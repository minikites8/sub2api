package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestBasispointsImageRedisCache(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := &gatewayCache{rdb: client}
	ctx := context.Background()
	expires := time.Now().Add(24 * time.Hour).Truncate(time.Millisecond)
	require.NoError(t, cache.SetBasispointsImageFile(ctx, "account-a", "image", "file-one", expires))
	// A new repository instance shares IDs across service restarts/replicas.
	restarted := &gatewayCache{rdb: client}
	id, actualExpiry, err := restarted.GetBasispointsImageFile(ctx, "account-a", "image")
	require.NoError(t, err)
	require.Equal(t, "file-one", id)
	require.True(t, expires.Equal(actualExpiry))
	id, _, err = cache.GetBasispointsImageFile(ctx, "account-b", "image")
	require.NoError(t, err)
	require.Empty(t, id)
	server.FastForward(25 * time.Hour)
	id, _, err = cache.GetBasispointsImageFile(ctx, "account-a", "image")
	require.NoError(t, err)
	require.Empty(t, id)
}

func TestBasispointsImageRedisCapacityAndAbsoluteExpiry(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := &gatewayCache{rdb: client}
	ctx := context.Background()
	keys := basispointsImageKeys("scope")
	for i := 0; i < 520; i++ {
		require.NoError(t, cache.SetBasispointsImageFile(ctx, "scope", fmt.Sprint(i), fmt.Sprintf("file-%d", i), time.Now().Add(time.Hour)))
	}
	require.EqualValues(t, 512, client.HLen(ctx, keys[0]).Val())
	require.EqualValues(t, 512, client.ZCard(ctx, keys[1]).Val())
	require.Greater(t, client.PTTL(ctx, keys[0]).Val(), time.Duration(0))
	require.NoError(t, client.HSet(ctx, keys[0], "expired", ` {"file_id":"file-old","expires_at":1}`).Err())
	require.NoError(t, client.ZAdd(ctx, keys[1], redis.Z{Score: 1, Member: "expired"}).Err())
	id, _, err := cache.GetBasispointsImageFile(ctx, "scope", "expired")
	require.NoError(t, err)
	require.Empty(t, id)
	require.False(t, client.HExists(ctx, keys[0], "expired").Val())
}
