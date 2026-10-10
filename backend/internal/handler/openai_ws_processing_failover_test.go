//go:build unit

package handler

import (
	"context"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestOpenAIWSProcessingHandlerFailover(t *testing.T) {
	frames := []string{`{"type":"error","error":{"code":"server_error","message":"An error occurred while processing your request. You can retry your request, or contact us through our help center at help.openai.com if the error persists.","type":"api_error"}}`, `{"type":"response.failed","response":{"id":"resp_failed","status":"failed","error":{"code":"server_error","message":"An error occurred while processing your request. You can retry your request, or contact us through our help center at help.openai.com if the error persists.","type":"api_error"}}}`}
	for _, mode := range []string{service.OpenAIWSIngressModePassthrough, service.OpenAIWSIngressModeCtxPool} {
		for i, frame := range frames {
			for _, metadata := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/frame_%d/metadata_%t", mode, i, metadata), func(t *testing.T) { runWSProcessingFixture(t, mode, frame, metadata, false) })
			}
			t.Run(fmt.Sprintf("%s/frame_%d/partial", mode, i), func(t *testing.T) { runWSProcessingFixture(t, mode, frame, true, true) })
		}
	}
}
func runWSProcessingFixture(t *testing.T, mode, frame string, metadata, partial bool) {
	gin.SetMode(gin.TestMode)

	firstHitCh := make(chan []byte, 1)
	secondHitCh := make(chan []byte, 1)
	var firstConnections atomic.Int32
	var secondConnections atomic.Int32

	firstUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstConnections.Add(1)
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()

		readCtx, cancelRead := context.WithTimeout(r.Context(), 3*time.Second)
		_, payload, readErr := conn.Read(readCtx)
		cancelRead()
		if readErr == nil {
			firstHitCh <- payload
		}

		events := []string{}
		if metadata || partial {
			events = append(events, `{"type":"response.created","response":{"id":"resp_failed","model":"gpt-5.6-luna"}}`)
		}
		if partial {
			events = append(events, `{"type":"response.output_text.delta","response_id":"resp_failed","delta":"partial"}`)
		}
		events = append(events, frame)
		for _, event := range events {
			writeCtx, cancelWrite := context.WithTimeout(r.Context(), 3*time.Second)
			err := conn.Write(writeCtx, coderws.MessageText, []byte(event))
			cancelWrite()
			if err != nil {
				return
			}
		}
		readCtx, cancelRead = context.WithTimeout(r.Context(), 3*time.Second)
		_, _, _ = conn.Read(readCtx)
		cancelRead()
	}))
	defer firstUpstream.Close()

	secondUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondConnections.Add(1)
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()

		readCtx, cancelRead := context.WithTimeout(r.Context(), 3*time.Second)
		_, payload, readErr := conn.Read(readCtx)
		cancelRead()
		if readErr == nil {
			secondHitCh <- payload
		}

		for _, event := range []string{
			`{"type":"response.created","response":{"id":"resp_ws_timeout_b","model":"gpt-5.6-luna"}}`,
			`{"type":"response.output_text.delta","response_id":"resp_ws_timeout_b","delta":"recovered"}`,
			`{"type":"response.completed","response":{"id":"resp_ws_timeout_b","model":"gpt-5.6-luna","usage":{"input_tokens":1,"output_tokens":1}}}`,
		} {
			writeCtx, cancelWrite := context.WithTimeout(r.Context(), 3*time.Second)
			writeErr := conn.Write(writeCtx, coderws.MessageText, []byte(event))
			cancelWrite()
			if writeErr != nil {
				return
			}
		}
		readCtx, cancelRead = context.WithTimeout(r.Context(), 3*time.Second)
		_, _, _ = conn.Read(readCtx)
		cancelRead()
	}))
	defer secondUpstream.Close()

	groupID := int64(4212)
	accounts := []service.Account{
		{
			ID:          9912,
			Name:        "openai-ws-first-semantic-timeout",
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeAPIKey,
			Status:      service.StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    1,
			Credentials: map[string]any{"api_key": "sk-first", "base_url": firstUpstream.URL},
			Extra: map[string]any{
				"openai_apikey_responses_websockets_v2_enabled": true,
				"openai_apikey_responses_websockets_v2_mode":    mode,
			},
		},
		{
			ID:          9913,
			Name:        "openai-ws-failover-healthy",
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeAPIKey,
			Status:      service.StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    2,
			Credentials: map[string]any{"api_key": "sk-second", "base_url": secondUpstream.URL},
			Extra: map[string]any{
				"openai_apikey_responses_websockets_v2_enabled": true,
				"openai_apikey_responses_websockets_v2_mode":    mode,
			},
		},
	}

	cfg := &config.Config{}
	cfg.RunMode = config.RunModeSimple
	cfg.Default.RateMultiplier = 1
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.IngressInterTurnIdleTimeoutSeconds = 3
	cfg.Gateway.MaxAccountSwitches = 3

	for i := range accounts {
		accounts[i].GroupIDs = []int64{groupID}
	}
	accountRepo := &openAIWSFailoverHandlerAccountRepoStub{accounts: accounts}
	rateLimitSvc := service.NewRateLimitService(accountRepo, nil, cfg, nil, nil)
	billingCacheSvc := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	gatewaySvc := service.NewOpenAIGatewayService(
		accountRepo, nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil,
		service.NewBillingService(cfg, nil), rateLimitSvc, billingCacheSvc,
		nil, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil,
		nil,
	)
	cache := &concurrencyCacheMock{
		acquireUserSlotFn: func(context.Context, int64, int, string) (bool, error) { return true, nil },
		acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) {
			return true, nil
		},
	}
	h := &OpenAIGatewayHandler{
		gatewayService:      gatewaySvc,
		billingCacheService: billingCacheSvc,
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second),
		maxAccountSwitches:  3,
	}

	apiKey := &service.APIKey{
		ID:      1812,
		GroupID: &groupID,
		User:    &service.User{ID: 1712, Status: service.StatusActive},
		Group:   &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive},
	}
	handlerDone := make(chan struct{})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), apiKey)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.User.ID, Concurrency: 1})
		c.Next()
	})
	router.GET("/openai/v1/responses", func(c *gin.Context) {
		h.ResponsesWebSocket(c)
		close(handlerDone)
	})
	handlerServer := httptest.NewServer(router)
	defer handlerServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(
		dialCtx,
		"ws"+strings.TrimPrefix(handlerServer.URL, "http")+"/openai/v1/responses",
		&coderws.DialOptions{CompressionMode: coderws.CompressionContextTakeover},
	)
	cancelDial()
	require.NoError(t, err)
	defer func() { _ = clientConn.CloseNow() }()

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.6-luna","stream":false}`))
	cancelWrite()
	require.NoError(t, err)

	var eventTypes []string
	readCtx, cancelRead := context.WithTimeout(context.Background(), 6*time.Second)
	for {
		_, event, readErr := clientConn.Read(readCtx)
		require.NoError(t, readErr)
		eventType := gjson.GetBytes(event, "type").String()
		eventTypes = append(eventTypes, eventType)
		if eventType == "error" || eventType == "response.failed" {
			t.Logf("transport=WebSocket model=gpt-5.6-luna mode=%s metadata=%t partial=%t scheduled_connections=[%d,%d] client_failure=%s", mode, metadata, partial, firstConnections.Load(), secondConnections.Load(), event)
			require.True(t, partial, "pre-output processing failure must select the second account")
			break
		}
		if eventType == "response.completed" {
			require.Equal(t, "resp_ws_timeout_b", gjson.GetBytes(event, "response.id").String())
			break
		}
	}
	cancelRead()
	require.Contains(t, eventTypes, "response.output_text.delta")
	if !partial {
		require.NotContains(t, eventTypes, "error")
		require.NotContains(t, eventTypes, "response.failed")
	}
	require.NoError(t, clientConn.Close(coderws.StatusNormalClosure, "done"))

	select {
	case <-handlerDone:
	case <-time.After(3 * time.Second):
		t.Fatal("websocket handler did not finish after healthy failover turn")
	}
	select {
	case <-firstHitCh:
	case <-time.After(3 * time.Second):
		t.Fatal("first upstream did not receive replayable request")
	}
	if !partial {
		select {
		case payload := <-secondHitCh:
			require.Equal(t, "gpt-5.6-luna", gjson.GetBytes(payload, "model").String())
		case <-time.After(3 * time.Second):
			t.Fatal("second upstream did not receive replayed request")
		}
	}
	require.Equal(t, int32(1), firstConnections.Load())
	if partial {
		require.Zero(t, secondConnections.Load())
	} else {
		require.Equal(t, int32(1), secondConnections.Load())
	}
	t.Logf("transport=WebSocket mode=%s metadata=%t partial=%t upstream_attempts=[%d,%d] client_events=%v", mode, metadata, partial, firstConnections.Load(), secondConnections.Load(), eventTypes)
	require.NotContains(t, accountRepo.rateLimitedIDs, int64(9913), "healthy failover account must not be penalized")
}
