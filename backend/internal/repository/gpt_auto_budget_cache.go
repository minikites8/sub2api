package repository

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	gptAutoBudgetKeyPrefix   = "gpt_auto:budget:"
	gptAutoCountersKeyPrefix = "gpt_auto:counters:"
	// Keys outlive their UTC day so the dashboard can still show yesterday.
	gptAutoBudgetKeyTTLSeconds = 3 * 24 * 60 * 60
)

// gptAutoBudgetReserveScript admits one classifier call only if the day's
// call count and spend (nano-USD) stay within the limits, and books the
// estimated cost in the same atomic step. A limit of 0 means unlimited.
// Returns {allowed, calls, cost}.
var gptAutoBudgetReserveScript = redis.NewScript(`
local key = KEYS[1]
local callLimit = tonumber(ARGV[1])
local costLimit = tonumber(ARGV[2])
local cost = tonumber(ARGV[3])
local ttl = tonumber(ARGV[4])
local calls = tonumber(redis.call('HGET', key, 'calls') or '0')
local spent = tonumber(redis.call('HGET', key, 'cost') or '0')
if callLimit > 0 and calls + 1 > callLimit then
	return {0, calls, spent}
end
if costLimit > 0 and spent + cost > costLimit then
	return {0, calls, spent}
end
calls = redis.call('HINCRBY', key, 'calls', 1)
spent = redis.call('HINCRBY', key, 'cost', cost)
redis.call('EXPIRE', key, ttl)
return {1, calls, spent}
`)

var gptAutoBudgetAdjustScript = redis.NewScript(`
local key = KEYS[1]
redis.call('HINCRBY', key, 'calls', tonumber(ARGV[1]))
redis.call('HINCRBY', key, 'cost', tonumber(ARGV[2]))
redis.call('EXPIRE', key, tonumber(ARGV[3]))
return 1
`)

// gptAutoBudgetSeedScript initialises a day only if no node has touched it
// yet, so a Redis flush mid-day cannot silently reset the budget to zero.
var gptAutoBudgetSeedScript = redis.NewScript(`
local key = KEYS[1]
if redis.call('EXISTS', key) == 1 then
	return 0
end
redis.call('HSET', key, 'calls', tonumber(ARGV[1]), 'cost', tonumber(ARGV[2]))
redis.call('EXPIRE', key, tonumber(ARGV[3]))
return 1
`)

type gptAutoBudgetStore struct {
	rdb *redis.Client
}

// NewGPTAutoBudgetStore creates the cluster-wide GPT-Auto budget and counter
// store backed by Redis.
func NewGPTAutoBudgetStore(rdb *redis.Client) service.GPTAutoBudgetStore {
	return &gptAutoBudgetStore{rdb: rdb}
}

func (s *gptAutoBudgetStore) Reserve(ctx context.Context, day string, limits service.GPTAutoBudgetLimits, costNanoUSD int64) (service.GPTAutoBudgetUsage, bool, error) {
	values, err := gptAutoBudgetReserveScript.Run(ctx, s.rdb, []string{gptAutoBudgetKeyPrefix + day},
		limits.Calls, limits.CostNanoUSD, costNanoUSD, gptAutoBudgetKeyTTLSeconds).Int64Slice()
	if err != nil {
		return service.GPTAutoBudgetUsage{}, false, fmt.Errorf("reserve gpt-auto budget: %w", err)
	}
	if len(values) != 3 {
		return service.GPTAutoBudgetUsage{}, false, fmt.Errorf("reserve gpt-auto budget: unexpected reply length %d", len(values))
	}
	return service.GPTAutoBudgetUsage{Calls: values[1], CostNanoUSD: values[2]}, values[0] == 1, nil
}

func (s *gptAutoBudgetStore) Adjust(ctx context.Context, day string, callDelta, costDeltaNanoUSD int64) error {
	if callDelta == 0 && costDeltaNanoUSD == 0 {
		return nil
	}
	if err := gptAutoBudgetAdjustScript.Run(ctx, s.rdb, []string{gptAutoBudgetKeyPrefix + day},
		callDelta, costDeltaNanoUSD, gptAutoBudgetKeyTTLSeconds).Err(); err != nil {
		return fmt.Errorf("adjust gpt-auto budget: %w", err)
	}
	return nil
}

func (s *gptAutoBudgetStore) Seed(ctx context.Context, day string, usage service.GPTAutoBudgetUsage) error {
	if err := gptAutoBudgetSeedScript.Run(ctx, s.rdb, []string{gptAutoBudgetKeyPrefix + day},
		usage.Calls, usage.CostNanoUSD, gptAutoBudgetKeyTTLSeconds).Err(); err != nil {
		return fmt.Errorf("seed gpt-auto budget: %w", err)
	}
	return nil
}

func (s *gptAutoBudgetStore) Usage(ctx context.Context, day string) (service.GPTAutoBudgetUsage, bool, error) {
	values, err := s.rdb.HGetAll(ctx, gptAutoBudgetKeyPrefix+day).Result()
	if err != nil {
		return service.GPTAutoBudgetUsage{}, false, fmt.Errorf("read gpt-auto budget: %w", err)
	}
	if len(values) == 0 {
		return service.GPTAutoBudgetUsage{}, false, nil
	}
	calls, _ := strconv.ParseInt(values["calls"], 10, 64)
	cost, _ := strconv.ParseInt(values["cost"], 10, 64)
	return service.GPTAutoBudgetUsage{Calls: calls, CostNanoUSD: cost}, true, nil
}

func (s *gptAutoBudgetStore) AddCounters(ctx context.Context, day string, deltas map[string]int64) error {
	if len(deltas) == 0 {
		return nil
	}
	key := gptAutoCountersKeyPrefix + day
	pipe := s.rdb.TxPipeline()
	for name, delta := range deltas {
		if delta != 0 {
			pipe.HIncrBy(ctx, key, name, delta)
		}
	}
	pipe.Expire(ctx, key, gptAutoBudgetKeyTTLSeconds*time.Second)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("add gpt-auto counters: %w", err)
	}
	return nil
}

func (s *gptAutoBudgetStore) Counters(ctx context.Context, day string) (map[string]int64, error) {
	values, err := s.rdb.HGetAll(ctx, gptAutoCountersKeyPrefix+day).Result()
	if err != nil {
		return nil, fmt.Errorf("read gpt-auto counters: %w", err)
	}
	out := make(map[string]int64, len(values))
	for name, raw := range values {
		if value, err := strconv.ParseInt(raw, 10, 64); err == nil {
			out[name] = value
		}
	}
	return out, nil
}
