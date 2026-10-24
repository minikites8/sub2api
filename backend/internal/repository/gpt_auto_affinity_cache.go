package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

// gptAutoAffinityKeyPrefix namespaces GPT-Auto session affinity. Keys are
// SHA-256 digests already scoped to one API key; values hold only the model,
// effort and account id, never content.
const gptAutoAffinityKeyPrefix = "gpt_auto:affinity:"

type gptAutoAffinityStore struct {
	rdb *redis.Client
}

// NewGPTAutoAffinityStore returns the Redis-backed session-affinity store.
func NewGPTAutoAffinityStore(rdb *redis.Client) service.GPTAutoAffinityStore {
	return &gptAutoAffinityStore{rdb: rdb}
}

func (s *gptAutoAffinityStore) Get(ctx context.Context, key string) (*service.GPTAutoAffinity, error) {
	raw, err := s.rdb.Get(ctx, gptAutoAffinityKeyPrefix+key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var value service.GPTAutoAffinity
	if err := json.Unmarshal(raw, &value); err != nil {
		// A corrupt entry is a miss, not an outage.
		return nil, nil
	}
	return &value, nil
}

func (s *gptAutoAffinityStore) Set(ctx context.Context, key string, value service.GPTAutoAffinity, ttl time.Duration) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, gptAutoAffinityKeyPrefix+key, raw, ttl).Err()
}

// Atomic optimistic update: overlapping turns cannot advance the same streak.
func (s *gptAutoAffinityStore) CompareAndSet(ctx context.Context, key string, expected int64, value service.GPTAutoAffinity, ttl time.Duration) (bool, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return false, err
	}
	n, err := s.rdb.Eval(ctx, `
 local old=redis.call('GET',KEYS[1])
 local seq=0
 if old then local ok,v=pcall(cjson.decode,old); if not ok then return 0 end; seq=v.sequence or 0 end
 if seq~=tonumber(ARGV[1]) then return 0 end
 redis.call('SET',KEYS[1],ARGV[2],'PX',ARGV[3]); return 1
 `, []string{gptAutoAffinityKeyPrefix + key}, expected, raw, ttl.Milliseconds()).Int()
	return n == 1, err
}
