package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpsServiceRecordErrorBatch_SanitizesAndBatches(t *testing.T) {
	t.Parallel()

	var captured []*OpsInsertErrorLogInput
	repo := &opsRepoMock{
		BatchInsertErrorLogsFn: func(ctx context.Context, inputs []*OpsInsertErrorLogInput) (int64, error) {
			captured = append(captured, inputs...)
			return int64(len(inputs)), nil
		},
	}
	svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	msg := " upstream failed: https://example.com?access_token=secret-value "
	detail := `{"authorization":"Bearer secret-token"}`
	entries := []*OpsInsertErrorLogInput{
		{
			ErrorBody:            `{"error":"bad","access_token":"secret"}`,
			UpstreamStatusCode:   intPtr(-10),
			UpstreamErrorMessage: strPtr(msg),
			UpstreamErrorDetail:  strPtr(detail),
			UpstreamErrors: []*OpsUpstreamErrorEvent{
				{
					AccountID:          -2,
					UpstreamStatusCode: 429,
					Message:            " token leaked ",
					Detail:             `{"refresh_token":"secret"}`,
				},
			},
		},
		{
			ErrorPhase: "upstream",
			ErrorType:  "upstream_error",
			CreatedAt:  time.Now().UTC(),
		},
	}

	require.NoError(t, svc.RecordErrorBatch(context.Background(), entries))
	require.Len(t, captured, 2)

	first := captured[0]
	require.Equal(t, "internal", first.ErrorPhase)
	require.Equal(t, "api_error", first.ErrorType)
	require.Nil(t, first.UpstreamStatusCode)
	require.NotNil(t, first.UpstreamErrorMessage)
	require.NotContains(t, *first.UpstreamErrorMessage, "secret-value")
	require.Contains(t, *first.UpstreamErrorMessage, "access_token=***")
	require.NotNil(t, first.UpstreamErrorDetail)
	require.NotContains(t, *first.UpstreamErrorDetail, "secret-token")
	require.NotContains(t, first.ErrorBody, "secret")
	require.Nil(t, first.UpstreamErrors)
	require.NotNil(t, first.UpstreamErrorsJSON)
	require.NotContains(t, *first.UpstreamErrorsJSON, "secret")
	require.Contains(t, *first.UpstreamErrorsJSON, "[REDACTED]")

	second := captured[1]
	require.Equal(t, "upstream", second.ErrorPhase)
	require.Equal(t, "upstream_error", second.ErrorType)
	require.False(t, second.CreatedAt.IsZero())
}

func TestOpsServiceRecordErrorBatch_FallsBackToSingleInsert(t *testing.T) {
	t.Parallel()

	var (
		batchCalls  int
		singleCalls int
	)
	repo := &opsRepoMock{
		BatchInsertErrorLogsFn: func(ctx context.Context, inputs []*OpsInsertErrorLogInput) (int64, error) {
			batchCalls++
			return 0, errors.New("batch failed")
		},
		InsertErrorLogFn: func(ctx context.Context, input *OpsInsertErrorLogInput) (int64, error) {
			singleCalls++
			return int64(singleCalls), nil
		},
	}
	svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	err := svc.RecordErrorBatch(context.Background(), []*OpsInsertErrorLogInput{
		{ErrorMessage: "first"},
		{ErrorMessage: "second"},
	})
	require.NoError(t, err)
	require.Equal(t, 1, batchCalls)
	require.Equal(t, 2, singleCalls)
}

func TestOpsServiceRecordErrorBatch_QuotaLeaseDemoNodeForwardsWithoutLocalInsert(t *testing.T) {
	globalQuotaLeaseDemo.mu.Lock()
	previous := globalQuotaLeaseDemo.svc
	globalQuotaLeaseDemo.svc = nil
	globalQuotaLeaseDemo.mu.Unlock()
	t.Cleanup(func() {
		globalQuotaLeaseDemo.mu.Lock()
		globalQuotaLeaseDemo.svc = previous
		globalQuotaLeaseDemo.mu.Unlock()
	})

	cfg := &config.Config{
		DeploymentRole: config.DeploymentRoleNode,
		Ops: config.OpsConfig{
			Enabled: true,
		},
		Gateway: config.GatewayConfig{
			QuotaLeaseDemo: config.GatewayQuotaLeaseDemoConfig{
				Enabled:             true,
				NodeID:              "node-us",
				ControlPlaneBaseURL: "http://127.0.0.1:1",
				ControlPlaneKey:     "control-secret",
			},
		},
	}
	var localInsertCalls int
	repo := &opsRepoMock{
		InsertErrorLogFn: func(ctx context.Context, input *OpsInsertErrorLogInput) (int64, error) {
			localInsertCalls++
			return 1, nil
		},
		BatchInsertErrorLogsFn: func(ctx context.Context, inputs []*OpsInsertErrorLogInput) (int64, error) {
			localInsertCalls++
			return int64(len(inputs)), nil
		},
	}
	svc := NewOpsService(repo, nil, cfg, nil, nil, nil, nil, nil, nil, nil, nil)
	userID := int64(10)
	apiKeyID := int64(20)
	upstreamStatus := 503

	require.NoError(t, svc.RecordErrorBatch(context.Background(), []*OpsInsertErrorLogInput{{
		RequestID:          "ops-forward-req-1",
		UserID:             &userID,
		APIKeyID:           &apiKeyID,
		Platform:           PlatformOpenAI,
		Model:              "gpt-5",
		ErrorPhase:         "upstream",
		ErrorType:          "upstream_error",
		StatusCode:         503,
		ErrorMessage:       "upstream failed",
		ErrorBody:          `{"access_token":"secret"}`,
		UpstreamStatusCode: &upstreamStatus,
	}}))

	require.Zero(t, localInsertCalls)
	leaseSvc := GetQuotaLeaseDemoService(cfg)
	pending := leaseSvc.pendingOpsErrorLogSnapshots()
	require.Len(t, pending, 1)
	require.Equal(t, "node-us", pending[0].NodeID)
	require.Equal(t, "ops-forward-req-1", pending[0].RequestID)
	require.NotContains(t, pending[0].ErrorBody, "secret")
}

func TestOpsServiceRecordErrorPersistsExplicitAccountAuthStatusZero(t *testing.T) {
	t.Parallel()

	var captured *OpsInsertErrorLogInput
	repo := &opsRepoMock{
		InsertErrorLogFn: func(_ context.Context, input *OpsInsertErrorLogInput) (int64, error) {
			captured = input
			return 1, nil
		},
	}
	svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	staleStatus := 403
	staleMessage := "stale inference message"
	staleDetail := "stale inference detail"

	err := svc.RecordError(context.Background(), &OpsInsertErrorLogInput{
		ErrorPhase:           "upstream",
		ErrorType:            "upstream_error",
		ErrorOwner:           "provider",
		ErrorSource:          "upstream_http",
		UpstreamStatusCode:   &staleStatus,
		UpstreamErrorMessage: &staleMessage,
		UpstreamErrorDetail:  &staleDetail,
		UpstreamErrors: []*OpsUpstreamErrorEvent{
			{Stage: string(GatewayFailureStageInference), UpstreamStatusCode: 403, Message: staleMessage, Detail: staleDetail},
			{
				Stage: string(GatewayFailureStageAccountAuth), Scope: string(GatewayFailureScopeAccount),
				Reason: string(GrokCredentialReasonRevoked), Message: "Grok OAuth credentials require account action",
			},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, captured)
	require.Equal(t, "account_auth", captured.ErrorPhase)
	require.Equal(t, "provider", captured.ErrorOwner)
	require.Equal(t, "gateway", captured.ErrorSource)
	require.NotNil(t, captured.UpstreamStatusCode)
	require.Zero(t, *captured.UpstreamStatusCode)
	require.NotNil(t, captured.UpstreamErrorMessage)
	require.Equal(t, "Grok OAuth credentials require account action", *captured.UpstreamErrorMessage)
	require.Nil(t, captured.UpstreamErrorDetail)
	require.Nil(t, captured.UpstreamErrors)
	require.NotNil(t, captured.UpstreamErrorsJSON)
	require.Contains(t, *captured.UpstreamErrorsJSON, `"upstream_status_code":403`)
	require.Contains(t, *captured.UpstreamErrorsJSON, `"stage":"account_auth"`)
}

func strPtr(v string) *string {
	return &v
}
