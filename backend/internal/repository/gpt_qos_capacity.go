package repository

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

var configureGPTQoSCapacity = redis.NewScript(`
redis.replicate_commands()
local cfg=cjson.decode(ARGV[1])
local tm=redis.call('TIME');local now=tonumber(tm[1])*1000+math.floor(tonumber(tm[2])/1000)
local prefix=string.sub(KEYS[1],1,#KEYS[1]-#'capacity_policy')
local namespace=string.sub(prefix,1,#prefix-#'gptqos:{v1}:')
local previous=redis.call('HGET',KEYS[1],'version')
-- UnixNano revisions exceed Lua's exact integer range. Keep decimal strings:
-- tostring(number) in Redis Lua rounds to 14 significant digits and can make
-- a policy appear newer than itself, discarding every later account mutation.
if previous then
 if string.match(previous,'^%d+$') then
  if #previous>#cfg.version or (#previous==#cfg.version and previous>cfg.version) then return 0 end
 else
  -- One-time compatibility with the earlier scientific-notation registry.
  -- Its formatting error is below 100us at present UnixNano magnitudes.
  if tonumber(previous)>tonumber(cfg.version)+100000 then return 0 end
 end
end
redis.call('DEL',prefix..'account_pools')
redis.call('DEL',prefix..'pool_limits',prefix..'account_quality',prefix..'strong_limits')
for _,a in ipairs(cfg.accounts) do
 redis.call('HSET',prefix..'account_pools',tostring(a.id),a.pool)
 redis.call('HSET',prefix..'account_pools',tostring(a.slot_id),a.pool)
 redis.call('HSET',prefix..'pool_limits',a.pool,a.capacity)
 redis.call('HSET',prefix..'account_quality',tostring(a.id),a.quality)
 redis.call('HSET',prefix..'account_quality',tostring(a.slot_id),a.quality)
 redis.call('HSET',prefix..'strong_limits',a.pool,cfg.strong_limits[a.pool] or 0)
 -- Bootstrap running ordinary slots before enabling the shared cap. This
 -- includes disabled accounts still draining an earlier request.
 local slots=redis.call('ZRANGE',namespace..'concurrency:account:'..a.slot_id,0,-1,'WITHSCORES')
 for i=1,#slots,2 do
  local expires=(tonumber(slots[i+1])+tonumber(ARGV[2]))*1000
  if expires>now then
   redis.call('ZADD',prefix..'pool:'..a.pool,expires,slots[i])
   redis.call('SET',prefix..'slot_pool:'..slots[i],a.pool,'EX',math.ceil((expires-now)/1000)+180)
   if a.quality=='strong' then redis.call('ZADD',prefix..'strong:'..a.pool,expires,slots[i]);redis.call('ZADD',prefix..'all_strong',expires,slots[i]) end
  end
 end
end
redis.call('HSET',KEYS[1],'enabled',cfg.enabled and '1' or '0','version',cfg.version,'peak_enabled',cfg.peak_enabled and '1' or '0','strong_capacity',cfg.strong_capacity)
if not cfg.peak_enabled then redis.call('DEL',prefix..'global_peak') end
return 1
`)
var releaseGPTQoSAccountSlot = redis.NewScript(`
local id=ARGV[1]
local aid=string.match(KEYS[1],'([^:]+)$')
local ns=string.sub(KEYS[1],1,#KEYS[1]-#aid-#'concurrency:account:')
local p=ns..'gptqos:{v1}:'
redis.call('ZREM',KEYS[1],id)
redis.call('ZREM',p..'all_strong',id)
local pool=redis.call('GET',p..'slot_pool:'..id)
if pool then redis.call('ZREM',p..'pool:'..pool,id);redis.call('ZREM',p..'strong:'..pool,id);redis.call('DEL',p..'slot_pool:'..id) end
return 1
`)

func (c *concurrencyCache) GPTQoSConfigureCapacity(ctx context.Context, cfg service.GPTQoSConfig, accounts []service.Account) error {
	type binding struct {
		ID       int64  `json:"id,string"`
		SlotID   int64  `json:"slot_id,string"`
		Pool     string `json:"pool"`
		Capacity int    `json:"capacity"`
		Quality  string `json:"quality"`
	}
	data := struct {
		Version        string         `json:"version"`
		Enabled        bool           `json:"enabled"`
		PeakEnabled    bool           `json:"peak_enabled"`
		Accounts       []binding      `json:"accounts"`
		StrongLimits   map[string]int `json:"strong_limits"`
		StrongCapacity int            `json:"strong_capacity"`
	}{Version: strconv.FormatInt(cfg.Version, 10), Enabled: cfg.CapacityEnabled, PeakEnabled: cfg.Peak.Enabled, Accounts: []binding{}, StrongLimits: map[string]int{}}
	active := map[string]int{}
	strong := map[string]int{}
	for _, a := range accounts {
		if len(a.ProxyPool) > 0 || a.ProxyPoolConfigured {
			continue
		}
		if a.IsSchedulable() {
			active[a.QoSResourcePool()] += max(0, a.Concurrency)
			if a.QualityTier() == "strong" {
				strong[a.QoSResourcePool()] += max(0, a.Concurrency)
			}
		}
	}
	for _, a := range accounts {
		if len(a.ProxyPool) > 0 || a.ProxyPoolConfigured {
			continue
		}
		pool := a.QoSResourcePool()
		p, shared := cfg.ResourcePools[strings.TrimPrefix(pool, "shared:")]
		if !strings.HasPrefix(pool, "shared:") {
			if !cfg.Peak.Enabled || a.QualityTier() != "strong" {
				continue
			}
			p.Concurrency = a.Concurrency
		} else if !shared {
			p.Concurrency = 0
		}
		data.Accounts = append(data.Accounts, binding{a.ID, service.GPTQoSAccountSlotID(&a), pool, min(p.Concurrency, active[pool]), a.QualityTier()})
		data.StrongLimits[pool] = min(p.Concurrency, strong[pool])
	}
	for _, capacity := range data.StrongLimits {
		data.StrongCapacity += capacity
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return configureGPTQoSCapacity.Run(ctx, c.rdb, []string{"gptqos:{v1}:capacity_policy"}, string(raw), c.slotTTLSeconds).Err()
}

// Used by Live admission to avoid bypassing shared caps through a protocol with
// a different lease-transfer lifecycle. Ordinary text/HTTP/SSE/WS remain supported.
func (c *concurrencyCache) gptQoSSharedLiveBlocked(ctx context.Context, accountID int64) (bool, error) {
	if c == nil || c.rdb == nil {
		return false, nil
	}
	enabled, err := c.rdb.HGet(ctx, "gptqos:{v1}:capacity_policy", "enabled").Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if enabled != "1" {
		return false, nil
	}
	return c.rdb.HExists(ctx, "gptqos:{v1}:account_pools", strconv.FormatInt(accountID, 10)).Result()
}

func (c *concurrencyCache) GPTQoSValidatePool(ctx context.Context, id int64, pool string) (bool, error) {
	actual, err := c.rdb.HGet(ctx, "gptqos:{v1}:account_pools", strconv.FormatInt(id, 10)).Result()
	return actual == pool, err
}
