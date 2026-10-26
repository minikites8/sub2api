package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"sync"
	"time"
)

const GPTQoSSettingKey = "gpt_qos_scheduling_v1"
const AccountQualityTierKey = "quality_tier"
const AccountResourcePoolKey = "qos_resource_pool"

var qosPoolName = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func (a *Account) QualityTier() string {
	if a != nil {
		if v, _ := a.Extra[AccountQualityTierKey].(string); v == "strong" || v == "basic" {
			return v
		}
	}
	return "unknown"
}
func (a *Account) QoSResourcePool() string {
	if a == nil {
		return ""
	}
	if v, _ := a.Extra[AccountResourcePoolKey].(string); qosPoolName.MatchString(v) {
		return "shared:" + v
	}
	return "account:" + strconv.FormatInt(a.ID, 10)
}

type qualityAuditActorKey struct{}

func WithQualityAuditActor(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, qualityAuditActorKey{}, id)
}

// ValidateQualityExtra also prevents callers from forging the audit actor.
// The database trigger records changes atomically with the account write.
func ValidateQualityExtra(ctx context.Context, extra map[string]any) error {
	if extra == nil {
		return nil
	}
	if v, ok := extra["qos_upstream_kind"]; ok {
		source, valid := v.(string)
		if !valid || (source != "unknown" && source != "vm2api" && source != "direct") {
			return errors.New("invalid qos_upstream_kind")
		}
	}
	if v, exists := extra[AccountQualityTierKey]; exists {
		tier, ok := v.(string)
		if !ok || (tier != "strong" && tier != "basic" && tier != "unknown") {
			return errors.New("quality_tier must be strong, basic or unknown")
		}
		actor, _ := ctx.Value(qualityAuditActorKey{}).(int64)
		extra["quality_tier_actor_id"] = actor
	} else {
		delete(extra, "quality_tier_actor_id")
	}
	if v, exists := extra[AccountResourcePoolKey]; exists {
		name, ok := v.(string)
		if !ok || (name != "" && !qosPoolName.MatchString(name)) {
			return errors.New("qos_resource_pool must be empty or 1–64 letters, digits, underscores or hyphens")
		}
	}
	return nil
}

type GPTQoSGroupConfig struct {
	Tier               string `json:"qos_tier"`
	Weight             int    `json:"weight"`
	AllowBasicFallback bool   `json:"allow_basic_fallback"`
	StrongWaitMs       int    `json:"strong_wait_ms"`
	BasicWaitMs        int    `json:"basic_wait_ms"`
	MaxWaiting         int    `json:"max_waiting"`
	UserMaxWaiting     int    `json:"user_max_waiting"`
	CapacityProtection bool   `json:"capacity_protection"`
	Concurrency        int    `json:"concurrency"`
}

func DefaultGPTQoSGroupConfig(tier string) GPTQoSGroupConfig {
	c := GPTQoSGroupConfig{Tier: tier, MaxWaiting: 200, UserMaxWaiting: 4, BasicWaitMs: 1000}
	switch tier {
	case "premium":
		c.Weight = 6
		c.StrongWaitMs = 5000
		c.CapacityProtection = true
	case "economy":
		c.Weight = 1
		c.StrongWaitMs = 500
		c.AllowBasicFallback = true
	default:
		c.Tier = "standard"
		c.Weight = 3
		c.StrongWaitMs = 2000
	}
	return c
}

type GPTQoSPeakConfig struct {
	Enabled          bool    `json:"enabled"`
	EnterUtilization float64 `json:"enter_utilization"`
	ExitUtilization  float64 `json:"exit_utilization"`
	ProtectionRatio  float64 `json:"protection_ratio"`
	EnterHoldMs      int     `json:"enter_hold_ms"`
	ExitHoldMs       int     `json:"exit_hold_ms"`
	StrictReserve    bool    `json:"strict_reserve"`
}
type GPTQoSResourcePool struct {
	Concurrency int `json:"concurrency"`
}
type GPTQoSConfig struct {
	unavailable           bool
	GroupIDs              []int64                       `json:"group_ids"`
	Version               int64                         `json:"version"`
	QualityRoutingEnabled bool                          `json:"quality_routing_enabled"`
	Enabled               bool                          `json:"enabled"`
	CapacityEnabled       bool                          `json:"capacity_enabled"`
	PremiumWeight         int                           `json:"premium_weight"`
	StandardWeight        int                           `json:"standard_weight"`
	EconomyWeight         int                           `json:"economy_weight"`
	MaxWaiting            int                           `json:"max_waiting"`
	RequestBudgetMs       int                           `json:"request_budget_ms"`
	HighEffortBudgetMs    int                           `json:"high_effort_budget_ms"`
	MaxAttempts           int                           `json:"max_attempts"`
	LogSampleRate         float64                       `json:"log_sample_rate"`
	Peak                  GPTQoSPeakConfig              `json:"peak"`
	Groups                map[string]GPTQoSGroupConfig  `json:"groups"`
	ResourcePools         map[string]GPTQoSResourcePool `json:"resource_pools"`
}

func DefaultGPTQoSConfig() GPTQoSConfig {
	return GPTQoSConfig{PremiumWeight: 6, StandardWeight: 3, EconomyWeight: 1, MaxWaiting: 1000, RequestBudgetMs: 300000, HighEffortBudgetMs: 900000, MaxAttempts: 4, LogSampleRate: 0.1,
		Peak: GPTQoSPeakConfig{EnterUtilization: 0.8, ExitUtilization: 0.65, ProtectionRatio: 0.3, EnterHoldMs: 5000, ExitHoldMs: 15000, StrictReserve: true}, Groups: map[string]GPTQoSGroupConfig{}, ResourcePools: map[string]GPTQoSResourcePool{}}
}
func (c GPTQoSConfig) Group(id *int64) GPTQoSGroupConfig {
	if id != nil {
		if g, ok := c.Groups[strconv.FormatInt(*id, 10)]; ok {
			return g
		}
	}
	// The queue already multiplies by the configured tier weight. Keep the
	// default group's relative weight neutral instead of applying it twice.
	return DefaultGPTQoSGroupConfig("standard")
}
func ValidateGPTQoSConfig(c GPTQoSConfig) error {
	if len(c.GroupIDs) > 1000 {
		return errors.New("at most 1000 rollout groups")
	}
	for _, id := range c.GroupIDs {
		if id <= 0 {
			return errors.New("invalid rollout group ID")
		}
	}
	if c.Enabled && !c.QualityRoutingEnabled {
		return errors.New("QoS requires quality routing")
	}
	if c.Peak.Enabled && (!c.Enabled || !c.CapacityEnabled) {
		return errors.New("peak protection requires QoS and capacity control")
	}
	if c.PremiumWeight < 1 || c.PremiumWeight > 100 || c.StandardWeight < 1 || c.StandardWeight > 100 || c.EconomyWeight < 1 || c.EconomyWeight > 100 || c.MaxWaiting < 1 || c.MaxWaiting > 2000 || c.MaxAttempts < 1 || c.MaxAttempts > 20 {
		return errors.New("invalid weights, queue length or attempt limit")
	}
	if c.RequestBudgetMs < 1000 || c.RequestBudgetMs > 3600000 || c.HighEffortBudgetMs < c.RequestBudgetMs || c.HighEffortBudgetMs > 7200000 {
		return errors.New("invalid request budgets")
	}
	if math.IsNaN(c.LogSampleRate) || c.LogSampleRate < 0 || c.LogSampleRate > 1 {
		return errors.New("invalid log sample rate")
	}
	p := c.Peak
	if math.IsNaN(p.EnterUtilization) || math.IsNaN(p.ExitUtilization) || math.IsNaN(p.ProtectionRatio) || p.ExitUtilization < 0 || p.EnterUtilization > 1 || p.ExitUtilization >= p.EnterUtilization || p.ProtectionRatio < 0 || p.ProtectionRatio > 0.9 || p.EnterHoldMs < 100 || p.EnterHoldMs > 600000 || p.ExitHoldMs < 100 || p.ExitHoldMs > 600000 {
		return errors.New("invalid peak thresholds or hysteresis")
	}
	if len(c.Groups) > 1000 || len(c.ResourcePools) > 1000 {
		return errors.New("at most 1000 groups or resource pools")
	}
	for id, g := range c.Groups {
		n, err := strconv.ParseInt(id, 10, 64)
		if err != nil || n <= 0 {
			return errors.New("invalid QoS group ID")
		}
		if g.Tier != "premium" && g.Tier != "standard" && g.Tier != "economy" {
			return errors.New("invalid qos_tier")
		}
		if g.Weight < 1 || g.Weight > 100 || g.StrongWaitMs < 0 || g.StrongWaitMs > 120000 || g.BasicWaitMs < 0 || g.BasicWaitMs > 120000 || g.MaxWaiting < 1 || g.MaxWaiting > 2000 || g.UserMaxWaiting < 1 || g.UserMaxWaiting > 100 || g.Concurrency < 0 || g.Concurrency > 100000 {
			return fmt.Errorf("invalid QoS limits for group %s", id)
		}
	}
	for id, p := range c.ResourcePools {
		if !qosPoolName.MatchString(id) || p.Concurrency < 1 || p.Concurrency > 100000 {
			return errors.New("invalid shared resource pool")
		}
	}
	return nil
}

type gptQoSConfigCache struct {
	mu              sync.Mutex
	applyMu         sync.Mutex // serialize policy writes/refreshes without blocking request reads
	config          GPTQoSConfig
	expires         time.Time
	capacityExpires time.Time
	refreshing      bool
	revision        uint64
}

func (s *SettingService) GetGPTQoSConfig(ctx context.Context) (GPTQoSConfig, error) {
	c := DefaultGPTQoSConfig()
	raw, err := s.settingRepo.GetValue(ctx, GPTQoSSettingKey)
	if errors.Is(err, ErrSettingNotFound) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err = json.Unmarshal([]byte(raw), &c); err != nil {
		return c, err
	}
	return c, ValidateGPTQoSConfig(c)
}
func (s *SettingService) SaveGPTQoSConfig(ctx context.Context, c GPTQoSConfig) error {
	if err := ValidateGPTQoSConfig(c); err != nil {
		return err
	}
	c.Version = time.Now().UnixNano()
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}
	cache := &s.gptQoSConfig
	cache.applyMu.Lock()
	defer cache.applyMu.Unlock()
	if err = s.settingRepo.Set(ctx, GPTQoSSettingKey, string(raw)); err != nil {
		return err
	}
	if err = s.synchronizeGPTQoSCapacity(ctx, c); err != nil {
		return err
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.revision++
	cache.config = c
	cache.expires = time.Now().Add(time.Second)
	cache.capacityExpires = time.Now().Add(5 * time.Second)
	return nil
}

// No SQL on the request goroutine. Failed refresh retains the last valid policy,
// rather than unexpectedly removing a quality guarantee during a DB outage.
func (s *SettingService) gptQoSRuntimeConfig() GPTQoSConfig {
	if s == nil || s.settingRepo == nil {
		return DefaultGPTQoSConfig()
	}
	cache := &s.gptQoSConfig
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.config.PremiumWeight == 0 {
		return DefaultGPTQoSConfig()
	}
	if !cache.refreshing && time.Now().After(cache.expires) {
		cache.refreshing = true
		revision := cache.revision
		lastVersion := cache.config.Version
		capacityExpired := time.Now().After(cache.capacityExpires)
		go func() {
			cache.applyMu.Lock()
			defer cache.applyMu.Unlock()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			c, err := s.GetGPTQoSConfig(ctx)
			// Cooldown and account expiry change effective Strong capacity without
			// changing the administrator's policy revision. Refresh off the request
			// goroutine, coalesced per instance, while peak protection is enabled.
			refreshCapacity := err == nil && (c.Version != lastVersion || (c.CapacityEnabled && c.Peak.Enabled && capacityExpired))
			if refreshCapacity {
				err = s.synchronizeGPTQoSCapacity(ctx, c)
			}
			cache.mu.Lock()
			defer cache.mu.Unlock()
			cache.refreshing = false
			if cache.revision == revision {
				if err == nil {
					cache.config = c
					if refreshCapacity {
						cache.capacityExpires = time.Now().Add(5 * time.Second)
					}
				}
				cache.expires = time.Now().Add(time.Second)
			}
		}()
	}
	if cache.config.PremiumWeight == 0 {
		return DefaultGPTQoSConfig()
	}
	return cache.config
}

// Account quality filters are propagated without widening the legacy repository interface.
type accountQualityFilterKey struct{}

func WithAccountQualityFilter(ctx context.Context, tier string) context.Context {
	return context.WithValue(ctx, accountQualityFilterKey{}, tier)
}
func AccountQualityFilter(ctx context.Context) string {
	value, _ := ctx.Value(accountQualityFilterKey{}).(string)
	return value
}

func (c GPTQoSConfig) Applies(groupID *int64) bool {
	if len(c.GroupIDs) == 0 {
		return true
	}
	if groupID == nil {
		return false
	}
	for _, id := range c.GroupIDs {
		if id == *groupID {
			return true
		}
	}
	return false
}
