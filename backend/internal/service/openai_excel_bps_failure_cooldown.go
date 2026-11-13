package service

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// Connection and model keys are distinct from the legacy account-ID keys in
// excelBPSCooldownUntil. A BPS rejection must not park a parent's native grant.
type excelBPSConnectionCooldownKey struct {
	accountID    int64
	connectionID string
	model        string
}

func (s *OpenAIGatewayService) coolDownExcelBPSFailure(ctx context.Context, account *Account, status int, headers http.Header) {
	if s == nil || account == nil || isQualityObservation(ctx) {
		return
	}
	var duration time.Duration
	switch {
	case status == http.StatusForbidden:
		duration = time.Minute
	case status >= 500 && status <= 599:
		duration = 5 * time.Second
	default:
		return
	}
	// Honor a bounded retry hint without invoking the independent 429 policies.
	if hinted, ok := excelBPSRetryAfter(headers.Get("Retry-After"), time.Now()); ok && hinted > duration {
		duration = hinted
	}
	if duration > time.Minute {
		duration = time.Minute
	}
	var key any = account.ID
	if account.AuthConnectionID != "" {
		key = excelBPSConnectionCooldownKey{accountID: account.ID, connectionID: account.AuthConnectionID}
	}
	s.storeExcelBPSFailureCooldown(key, time.Now().Add(duration))
}

func (s *OpenAIGatewayService) storeExcelBPSFailureCooldown(key any, until time.Time) {
	// Prune expired entries and bound connection/model keys. Legacy route keys
	// retain their existing lifetime and cannot evict a connection's newer state.
	now := time.Now()
	var oldestKey any
	var oldest time.Time
	count := 0
	s.excelBPSCooldownUntil.Range(func(candidate, value any) bool {
		expiry, ok := value.(time.Time)
		if !ok || !now.Before(expiry) {
			s.excelBPSCooldownUntil.CompareAndDelete(candidate, value)
			return true
		}
		if _, connectionKey := candidate.(excelBPSConnectionCooldownKey); connectionKey {
			count++
			if oldest.IsZero() || expiry.Before(oldest) {
				oldest, oldestKey = expiry, candidate
			}
		}
		return true
	})
	if _, connectionKey := key.(excelBPSConnectionCooldownKey); connectionKey {
		if _, exists := s.excelBPSCooldownUntil.Load(key); !exists && count >= excelBPSModelPermissionMaxEntries && oldestKey != nil {
			s.excelBPSCooldownUntil.Delete(oldestKey)
		}
	}
	for {
		previous, loaded := s.excelBPSCooldownUntil.LoadOrStore(key, until)
		if !loaded {
			return
		}
		if current, ok := previous.(time.Time); ok && !until.After(current) {
			return
		}
		if s.excelBPSCooldownUntil.CompareAndSwap(key, previous, until) {
			return
		}
	}
}

func (s *OpenAIGatewayService) excelBPSFailureCooldownActive(key any) bool {
	value, ok := s.excelBPSCooldownUntil.Load(key)
	if !ok {
		return false
	}
	if until, valid := value.(time.Time); valid && time.Now().Before(until) {
		return true
	}
	s.excelBPSCooldownUntil.CompareAndDelete(key, value)
	return false
}

func (s *OpenAIGatewayService) excelBPSConnectionCoolingDown(accountID int64, connection *OpenAIAuthConnection, model string) bool {
	if s == nil || connection == nil || connection.Kind != "bps" {
		return false
	}
	key := excelBPSConnectionCooldownKey{accountID: accountID, connectionID: connection.ID}
	if s.excelBPSFailureCooldownActive(key) {
		return true
	}
	key.model = model
	return model != "" && s.excelBPSFailureCooldownActive(key)
}

// Ignore only the rejected connection. Another healthy connection on the same
// parent remains eligible, including a native connection for the same model.
func (s *OpenAIGatewayService) managedExcelBPSRequestCoolingDown(account *Account, model string, requestContext ...context.Context) bool {
	if s == nil || account == nil || !account.HasManagedOpenAIConnections() {
		return false
	}
	if account.AuthConnectionID != "" {
		return s.excelBPSConnectionCoolingDown(account.ID, &OpenAIAuthConnection{ID: account.AuthConnectionID, Kind: account.GetExtraString("openai_connection_kind")}, account.GetMappedModel(model))
	}
	raw, err := json.Marshal(account.Extra["openai_auth_connections"])
	if err != nil {
		return false
	}
	var connections []*OpenAIAuthConnection
	if json.Unmarshal(raw, &connections) != nil {
		return false
	}
	capability := "responses"
	allowBPSChatBridge := false
	if len(requestContext) > 0 && requestContext[0] != nil {
		if required, ok := openAIConnectionCapabilityFromContext(requestContext[0]); ok {
			capability = required
		}
		allowBPSChatBridge, _ = requestContext[0].Value(openAIBPSChatBridgeEnabledContextKey{}).(bool)
	}
	blocked := false
	for _, connection := range connections {
		if connection == nil || connection.Legacy || !connection.Supports(openAIConnectionRequestedCapability(capability, connection, allowBPSChatBridge)) {
			continue
		}
		if s.excelBPSConnectionCoolingDown(account.ID, connection, account.GetMappedModel(model)) {
			blocked = true
			continue
		}
		return false
	}
	return blocked
}
