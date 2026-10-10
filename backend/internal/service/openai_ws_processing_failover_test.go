//go:build unit

package service

import (
	"context"
	coderws "github.com/coder/websocket"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

func TestOpenAIWSProcessingClassifier(t *testing.T) {
	gin.SetMode(gin.TestMode)
	payload := []byte(`{"type":"error","error":{"code":"server_error","type":"api_error","message":"An error occurred while processing your request. You can retry your request, or contact us through our help center at help.openai.com if the error persists."}}`)
	require.True(t, openAIWSProcessingFailure(payload))
	for _, typ := range []string{AccountTypeOAuth, AccountTypeAPIKey} {
		for _, pool := range []bool{false, true} {
			c := &gin.Context{}
			account := &Account{ID: 9912, Platform: PlatformOpenAI, Type: typ, Credentials: map[string]any{"pool_mode": pool, "pool_mode_retry_count": 10}}
			svc := &OpenAIGatewayService{}
			err := svc.newOpenAIWSProcessingFailoverError(c, account, payload, "gpt-5.6-luna", http.Header{})
			require.True(t, err.ShouldRetryNextAccount())
			require.False(t, err.RetryableOnSameAccount)
			require.Zero(t, err.SameAccountRetryMax)
		}
	}
	for _, p := range []string{
		`{"type":"response.completed"}`,
		`{"type":"error","error":{"code":"invalid_request_error","message":"invalid request"}}`,
		`{"type":"error","error":{"code":"context_length_exceeded","message":"An error occurred while processing your request"}}`,
		`{"type":"response.failed","response":{"error":{"code":"cyber_policy","message":"An error occurred while processing your request"}}}`,
	} {
		require.False(t, openAIWSProcessingFailure([]byte(p)), p)
	}
}
func TestOpenAIWSProcessingMetadataStaging(t *testing.T) {
	metadata := []byte(`{"type":"response.created","response":{"id":"failed_attempt"}}`)
	delta := []byte(`{"type":"response.output_text.delta","delta":"hello"}`)
	failure := []byte(`{"type":"response.failed","response":{"error":{"code":"server_error","message":"An error occurred while processing your request"}}}`)
	var stage openAIWSFirstOutputStage
	require.Empty(t, stage.messages(metadata))
	require.Equal(t, [][]byte{metadata, delta}, stage.messages(delta))
	require.True(t, stage.committed)
	require.Equal(t, [][]byte{failure}, stage.messages(failure))
	var failed openAIWSFirstOutputStage
	require.Empty(t, failed.messages(metadata))
	require.Equal(t, [][]byte{failure}, failed.messages(failure))
	require.False(t, failed.committed)
	require.Empty(t, failed.pending)
	var bounded openAIWSFirstOutputStage
	require.Empty(t, bounded.messages(metadata))
	large := make([]byte, openAIFirstOutputStageMaxBytes+1)
	require.Len(t, bounded.messages(large), 2)
	require.True(t, bounded.committed)
	require.Empty(t, bounded.pending)
}

func TestOpenAIWSProcessingLaterTurnRecoveryGuard(t *testing.T) {
	for _, mode := range []string{OpenAIWSIngressModePassthrough, OpenAIWSIngressModeCtxPool} {
		t.Run(mode, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			controlCtx, cancel := context.WithCancelCause(context.Background())
			defer cancel(context.Canceled)
			upstream := newStagedPassthroughConn()
			upstream.Send(`{"type":"response.completed","response":{"id":"resp_first","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)
			cfg := passthroughLifecycleConfig()
			svc := newPassthroughLifecycleService(cfg, upstream)
			account := passthroughLifecycleAccount()
			account.Extra["openai_apikey_responses_websockets_v2_mode"] = mode
			if mode == OpenAIWSIngressModeCtxPool {
				cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
				cfg.Gateway.OpenAIWS.IngressModeDefault = OpenAIWSIngressModeCtxPool
				pool := newOpenAIWSConnPool(cfg)
				pool.setClientDialerForTest(&stagedPassthroughDialer{conn: &ingressDrainTestConn{upstream}})
				svc.openaiWSPool = pool
			}
			server, serverErr := startPassthroughLifecycleServer(t, controlCtx, svc, account)
			defer server.Close()
			client := dialPassthroughLifecycleClient(t, server)
			defer client.CloseNow()
			completed, err := readPassthroughLifecycleFrame(t, client, 3*time.Second)
			require.NoError(t, err)
			require.Contains(t, string(completed), "resp_first")
			requirePassthroughUpstreamWrite(t, upstream, time.Second)
			writeCtx, writeCancel := context.WithTimeout(context.Background(), time.Second)
			require.NoError(t, client.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","previous_response_id":"resp_first","input":[{"role":"user","content":"second"}]}`)))
			writeCancel()
			requirePassthroughUpstreamWrite(t, upstream, time.Second)
			upstream.Send(`{"type":"error","error":{"code":"server_error","type":"api_error","message":"An error occurred while processing your request. You can retry your request, or contact us through our help center at help.openai.com if the error persists."}}`)
			if mode == OpenAIWSIngressModePassthrough {
				_, readErr := readPassthroughLifecycleFrame(t, client, 3*time.Second)
				var closeErr coderws.CloseError
				require.ErrorAs(t, readErr, &closeErr)
				require.Equal(t, coderws.StatusTryAgainLater, closeErr.Code)
			}
			select {
			case err := <-serverErr:
				if mode == OpenAIWSIngressModeCtxPool {
					var failover *UpstreamFailoverError
					require.ErrorAs(t, err, &failover)
					payload, currentTurn := OpenAIWSCurrentTurnRetryPayload(err)
					require.True(t, currentTurn, "retain the current-turn guard through ingress error unwrapping")
					require.Empty(t, payload, "account-bound continuation stays protected from initial-turn replay")
				} else {
					var closeErr *OpenAIWSClientCloseError
					require.ErrorAs(t, err, &closeErr)
					require.Equal(t, coderws.StatusTryAgainLater, closeErr.StatusCode())
				}
			case <-time.After(3 * time.Second):
				t.Fatal("later-turn recovery did not settle")
			}
		})
	}
}
