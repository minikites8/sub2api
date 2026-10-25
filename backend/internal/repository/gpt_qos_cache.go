package repository

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

//go:embed lua/gpt_qos.lua
var gptQoSScriptSource string
var gptQoSScript = redis.NewScript(gptQoSScriptSource)

func (c *concurrencyCache) runGPTQoS(ctx context.Context, op, id string, entry any) (string, error) {
	data, err := json.Marshal(entry)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return gptQoSScript.Run(ctx, c.rdb, []string{"gptqos:{v1}:requests"}, op, id, c.slotTTLSeconds, string(data)).Text()
}
func (c *concurrencyCache) GPTQoSPoll(ctx context.Context, e service.GPTQoSEntry) (service.GPTQoSGrant, error) {
	if e.ID == "" || e.UserID <= 0 || len(e.Candidates) == 0 || len(e.Candidates) > 256 || e.MaxWaiting < 1 || e.MaxWaiting > 2000 || e.Weight < 1 || e.WaitMs < 1 || e.WaitMs > 240001 {
		return service.GPTQoSGrant{}, errors.New("invalid QoS entry")
	}
	for _, candidate := range e.Candidates {
		if candidate.Concurrency < 1 || candidate.PoolCapacity < 1 {
			return service.GPTQoSGrant{}, errors.New("unbounded QoS capacity")
		}
	}
	raw, err := c.runGPTQoS(ctx, "poll", e.ID, e)
	if err != nil {
		return service.GPTQoSGrant{}, err
	}
	var grant service.GPTQoSGrant
	err = json.Unmarshal([]byte(raw), &grant)
	return grant, err
}
func (c *concurrencyCache) GPTQoSClose(ctx context.Context, id string) error {
	_, err := c.runGPTQoS(ctx, "close", id, nil)
	return err
}
func (c *concurrencyCache) GPTQoSRefresh(ctx context.Context, id string) (bool, error) {
	raw, err := c.runGPTQoS(ctx, "refresh", id, nil)
	return raw == "true", err
}
func (c *concurrencyCache) GPTQoSState(ctx context.Context) (map[string]any, error) {
	raw, err := c.runGPTQoS(ctx, "state", "", nil)
	if err != nil {
		return nil, err
	}
	var state map[string]any
	err = json.Unmarshal([]byte(raw), &state)
	return state, err
}
func (c *concurrencyCache) GPTQoSRecord(ctx context.Context, event service.GPTQoSMetric) error {
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = c.rdb.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.LPush(ctx, "gptqos:{v1}:metrics", raw)
		p.LTrim(ctx, "gptqos:{v1}:metrics", 0, 9999)
		p.Expire(ctx, "gptqos:{v1}:metrics", 24*time.Hour)
		return nil
	})
	return err
}
func (c *concurrencyCache) GPTQoSMetrics(ctx context.Context, filter service.GPTQoSMetricFilter) ([]service.GPTQoSMetric, error) {
	rows, err := c.rdb.LRange(ctx, "gptqos:{v1}:metrics", 0, 9999).Result()
	if err != nil {
		return nil, err
	}
	out := make([]service.GPTQoSMetric, 0, len(rows))
	oldest := time.Now().Add(-24 * time.Hour).UnixMilli()
	for _, raw := range rows {
		var event service.GPTQoSMetric
		if json.Unmarshal([]byte(raw), &event) == nil && event.At >= oldest && (filter.GroupID == 0 || event.GroupID == filter.GroupID) && (filter.Model == "" || event.Model == filter.Model) && (filter.Effort == "" || event.Effort == filter.Effort) {
			out = append(out, event)
		}
	}
	return out, nil
}

var _ service.GPTQoSAdmissionStore = (*concurrencyCache)(nil)
