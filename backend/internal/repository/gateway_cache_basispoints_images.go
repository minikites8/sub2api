package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

var _ service.BasispointsImageFileCache = (*gatewayCache)(nil)

// Each account has one bounded hash and LRU index in the same Redis hash slot.
// Values contain only file IDs and expiry timestamps; image bytes stay in flight.
var basispointsImageGet = redis.NewScript(`
local value = redis.call('HGET', KEYS[1], ARGV[1])
if not value then return false end
local item = cjson.decode(value)
if tonumber(item.expires_at) <= tonumber(ARGV[2]) then
    redis.call('HDEL', KEYS[1], ARGV[1])
    redis.call('ZREM', KEYS[2], ARGV[1])
    return false
end
redis.call('ZADD', KEYS[2], ARGV[2], ARGV[1])
return value
`)

var basispointsImageSet = redis.NewScript(`
redis.call('HSET', KEYS[1], ARGV[1], ARGV[2])
redis.call('ZADD', KEYS[2], ARGV[3], ARGV[1])
local excess = redis.call('ZCARD', KEYS[2]) - 512
if excess > 0 then
    local oldest = redis.call('ZRANGE', KEYS[2], 0, excess - 1)
    for _, key in ipairs(oldest) do
        redis.call('HDEL', KEYS[1], key)
        redis.call('ZREM', KEYS[2], key)
    end
end
local ttl = math.max(tonumber(ARGV[4]), redis.call('PTTL', KEYS[1]))
redis.call('PEXPIRE', KEYS[1], ttl)
redis.call('PEXPIRE', KEYS[2], ttl)
return 1
`)

type basispointsImageRecord struct {
	FileID    string `json:"file_id"`
	ExpiresAt int64  `json:"expires_at"`
}

func basispointsImageKeys(scope string) []string {
	prefix := "basispoints:images:{" + scope + "}"
	return []string{prefix + ":files", prefix + ":lru"}
}

func (c *gatewayCache) GetBasispointsImageFile(ctx context.Context, scope, digest string) (string, time.Time, error) {
	raw, err := basispointsImageGet.Run(ctx, c.rdb, basispointsImageKeys(scope), digest, time.Now().UnixMilli()).Text()
	if errors.Is(err, redis.Nil) {
		return "", time.Time{}, nil
	}
	if err != nil {
		return "", time.Time{}, err
	}
	var item basispointsImageRecord
	if err := json.Unmarshal([]byte(raw), &item); err != nil {
		return "", time.Time{}, err
	}
	return item.FileID, time.UnixMilli(item.ExpiresAt), nil
}

func (c *gatewayCache) SetBasispointsImageFile(ctx context.Context, scope, digest, fileID string, expiresAt time.Time) error {
	now := time.Now()
	ttl := expiresAt.Sub(now).Milliseconds()
	if ttl <= 0 {
		return nil
	}
	raw, err := json.Marshal(basispointsImageRecord{FileID: fileID, ExpiresAt: expiresAt.UnixMilli()})
	if err != nil {
		return err
	}
	return basispointsImageSet.Run(ctx, c.rdb, basispointsImageKeys(scope), digest, string(raw), now.UnixMilli(), ttl).Err()
}
