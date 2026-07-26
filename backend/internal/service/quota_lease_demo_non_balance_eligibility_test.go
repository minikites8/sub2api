//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
)

// A granted quota lease replaces the balance check only. If the admin-configured
// user × platform spend cap stopped being enforced, nothing else would catch it
// before settlement.
func TestCheckNonBalanceEligibility_EnforcesPlatformQuota(t *testing.T) {
	daily := 5.0
	repo := &fakeQuotaRepo{rec: &UserPlatformQuotaRecord{
		UserID: 1, Platform: "anthropic", DailyLimitUSD: &daily,
	}}
	cache := &fakeFullCache{entry: &UserPlatformQuotaCacheEntry{
		DailyUsageUSD:    5.0,
		DailyLimitUSD:    &daily,
		DailyWindowStart: currentDayStart(),
		SchemaVersion:    UserPlatformQuotaCacheSchemaV1,
	}}
	s := newServiceForPreflight(t, repo, cache)

	err := s.CheckNonBalanceEligibility(context.Background(), &User{ID: 1}, nil, nil, nil, "anthropic")
	if err == nil {
		t.Fatal("expected the exhausted platform quota to be rejected, got nil")
	}
	if !errors.Is(err, ErrUserPlatformDailyQuotaExhausted) {
		t.Fatalf("expected ErrUserPlatformDailyQuotaExhausted, got %v", err)
	}
}

func TestCheckNonBalanceEligibility_AllowsWhenUnderPlatformQuota(t *testing.T) {
	daily := 10.0
	repo := &fakeQuotaRepo{rec: &UserPlatformQuotaRecord{
		UserID: 1, Platform: "anthropic", DailyLimitUSD: &daily,
	}}
	cache := &fakeFullCache{entry: &UserPlatformQuotaCacheEntry{
		DailyUsageUSD:    1.0,
		DailyLimitUSD:    &daily,
		DailyWindowStart: currentDayStart(),
		SchemaVersion:    UserPlatformQuotaCacheSchemaV1,
	}}
	s := newServiceForPreflight(t, repo, cache)

	if err := s.CheckNonBalanceEligibility(context.Background(), &User{ID: 1}, nil, nil, nil, "anthropic"); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}
