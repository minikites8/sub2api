package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

func TestGroupUnavailableTimeBoundaries(t *testing.T) {
	for _, tc := range []struct {
		start, end, current string
		blocked             bool
	}{
		{"08:00", "22:00", "07:59:59", false},
		{"08:00", "22:00", "08:00:00", true},
		{"08:00", "22:00", "21:59:59", true},
		{"08:00", "22:00", "22:00:00", false},
		{"22:00", "08:00", "21:59:59", false},
		{"22:00", "08:00", "22:00:00", true},
		{"22:00", "08:00", "00:00:00", true},
		{"22:00", "08:00", "07:59:59", true},
		{"22:00", "08:00", "08:00:00", false},
		{"23:00", "00:00", "23:59:59", true},
		{"23:00", "00:00", "00:00:00", false},
	} {
		t.Run(tc.start+"-"+tc.end+"/"+tc.current, func(t *testing.T) {
			local, err := time.ParseInLocation("15:04:05", tc.current, timezone.Location())
			require.NoError(t, err)
			group := &Group{SubscriptionType: SubscriptionTypeSubscription, UnavailableTimeEnabled: true, UnavailableStart: tc.start, UnavailableEnd: tc.end}
			require.Equal(t, tc.blocked, group.IsUnavailableAt(local.In(time.FixedZone("client", -7*3600))), "use system timezone, irrespective of input location")
		})
	}
}

func TestGroupUnavailableTimeValidation(t *testing.T) {
	for _, tc := range []struct {
		name, kind, start, end string
		enabled, valid         bool
	}{
		{"night card", "subscription", "08:00", "22:00", true, true},
		{"cross midnight", "subscription", "22:00", "08:00", true, true},
		{"midnight end", "subscription", "23:00", "00:00", true, true},
		{"equal", "subscription", "08:00", "08:00", true, false},
		{"missing", "subscription", "", "08:00", true, false},
		{"bad hour", "subscription", "24:00", "08:00", true, false},
		{"bad minute", "subscription", "08:60", "22:00", true, false},
		{"not padded", "subscription", "8:00", "22:00", true, false},
		{"wrong type", "standard", "08:00", "22:00", true, false},
		{"disabled", "standard", "", "", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateUnavailableTimeConfig(tc.kind, tc.enabled, tc.start, tc.end)
			require.Equal(t, tc.valid, err == nil)
		})
	}
	enabled, start, end := NormalizeUnavailableTimeConfig("standard", true, "08:00", "22:00")
	require.False(t, enabled)
	require.Empty(t, start)
	require.Empty(t, end)
	enabled, start, end = NormalizeUnavailableTimeConfig("subscription", false, "08:00", "22:00")
	require.False(t, enabled)
	require.Equal(t, "08:00", start)
	require.Equal(t, "22:00", end)
	var group *Group
	require.False(t, group.IsUnavailableAt(time.Now()))
	group = &Group{SubscriptionType: "subscription", UnavailableTimeEnabled: true}
	require.True(t, group.IsUnavailableAt(time.Now()), "invalid enabled policy must not silently allow access")
	group.UnavailableTimeEnabled = false
	require.False(t, group.IsUnavailableAt(time.Now()))
}

func TestGroupUnavailableTimeAuthSnapshot(t *testing.T) {
	id := int64(1)
	key := &APIKey{ID: 1, UserID: 1, GroupID: &id, User: &User{ID: 1}, Group: &Group{ID: id, SubscriptionType: "subscription", UnavailableTimeEnabled: true, UnavailableStart: "08:00", UnavailableEnd: "22:00"}}
	svc := &APIKeyService{}
	payload, err := json.Marshal(&APIKeyAuthCacheEntry{Snapshot: svc.snapshotFromAPIKey(context.Background(), key)})
	require.NoError(t, err)
	var entry APIKeyAuthCacheEntry
	require.NoError(t, json.Unmarshal(payload, &entry))
	restored, used, err := svc.applyAuthCacheEntry(key.Key, &entry)
	require.NoError(t, err)
	require.True(t, used)
	require.True(t, restored.Group.UnavailableTimeEnabled)
	require.Equal(t, "08:00", restored.Group.UnavailableStart)
	require.Equal(t, "22:00", restored.Group.UnavailableEnd)
	entry.Snapshot.Version = 30
	_, used, err = svc.applyAuthCacheEntry(key.Key, &entry)
	require.NoError(t, err)
	require.False(t, used)
}

func TestGroupUnavailableTimeBillingCheck(t *testing.T) {
	svc := &BillingCacheService{}
	now := time.Now().In(timezone.Location())
	group := &Group{SubscriptionType: "subscription", UnavailableTimeEnabled: true, UnavailableStart: now.Add(-time.Hour).Format("15:04"), UnavailableEnd: now.Add(time.Hour).Format("15:04")}
	svc.cfg = &config.Config{RunMode: config.RunModeSimple}
	require.ErrorIs(t, svc.CheckBillingEligibility(context.Background(), nil, nil, group, nil, ""), ErrGroupUnavailableTime)
}
