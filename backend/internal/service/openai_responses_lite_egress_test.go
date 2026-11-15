//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIGatewayServiceForward_LateResponsesLiteHeaderPinsParallelToolCalls(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, source := range []string{"harvest_ticket", "account_override"} {
		for _, passthrough := range []bool{false, true} {
			for _, field := range []string{"", `,"parallel_tool_calls":true`, `,"parallel_tool_calls":false`} {
				t.Run(source+"/"+map[bool]string{false: "managed", true: "passthrough"}[passthrough]+"/"+field, func(t *testing.T) {
					upstream := &httpUpstreamRecorder{resp: &http.Response{
						StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}},
						Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_lite\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n")),
					}}
					svc := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true, TargetLength: 292, Models: []string{"gpt-6-astra"}}, upstream)
					account := ticketTestAccount(41)
					account.Extra = map[string]any{"openai_passthrough": passthrough}
					if source == "harvest_ticket" {
						require.NoError(t, svc.storeOpenAICodexTicket(context.Background(), account, &openAICodexTicket{
							Model: "gpt-6-astra", State: fakeCodexTicketState(292), Length: 292,
							HarvestLite: true, HarvestSessionID: "harvest-lite-session",
							CapturedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
						}))
					} else {
						account.Type = AccountTypeAPIKey
						account.Credentials = map[string]any{"api_key": "sk-test", credKeyHeaderOverrideEnabled: true,
							credKeyHeaderOverrides: map[string]any{responsesLiteHeaderKey: "true"}}
					}
					body := []byte(`{"model":"gpt-6-astra","stream":true,"instructions":"test","input":"hello","tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}]` + field + `}`)
					original := append([]byte(nil), body...)
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
					c.Request.Header.Set("User-Agent", "codex_cli_rs/0.144.1")

					result, err := svc.Forward(context.Background(), c, account, body)

					require.NoError(t, err)
					require.NotNil(t, result)
					require.Equal(t, "true", getHeaderRaw(upstream.lastReq.Header, responsesLiteHeaderKey))
					require.Equal(t, gjson.False, gjson.GetBytes(upstream.lastBody, "parallel_tool_calls").Type, string(upstream.lastBody))
					require.True(t, gjson.GetBytes(upstream.lastBody, "tools").IsArray())
					replay, err := upstream.lastReq.GetBody()
					require.NoError(t, err)
					defer func() { _ = replay.Close() }()
					replayBody, err := io.ReadAll(replay)
					require.NoError(t, err)
					require.Equal(t, upstream.lastBody, replayBody)
					require.Empty(t, c.GetHeader(responsesLiteHeader), "late egress policy must not change ingress headers for account failover")
					require.Equal(t, original, body)
				})
			}
		}
	}
}

func TestOpenAIWSConnLease_LiteHandshakePinsEveryCreateAndPrewarm(t *testing.T) {
	for _, writeMode := range []string{"timeout", "context_timeout", "context"} {
		t.Run(writeMode, func(t *testing.T) {
			upstream := &openAIWSCaptureConn{}
			conn := newOpenAIWSConn("lite", 41, upstream, nil)
			conn.agentRecoveryHeaders = http.Header{}
			conn.agentRecoveryHeaders.Set(responsesLiteHeader, "true")
			lease := &openAIWSConnLease{conn: conn, accountID: 41}
			for _, payload := range []any{
				map[string]any{"type": "response.create", "parallel_tool_calls": true, "generate": false},
				json.RawMessage(`{"type":"response.create","input":"first","sequence":900719925474099312345}`),
				openAIWSPreparedJSON(`{"type":"response.create","previous_response_id":"resp_first","parallel_tool_calls":true}`),
			} {
				original, err := json.Marshal(payload)
				require.NoError(t, err)
				switch writeMode {
				case "timeout":
					err = lease.WriteJSON(payload, time.Second)
				case "context_timeout":
					err = lease.WriteJSONWithContextTimeout(context.Background(), payload, time.Second)
				default:
					err = lease.WriteJSONContext(context.Background(), payload)
				}
				require.NoError(t, err)
				unchanged, err := json.Marshal(payload)
				require.NoError(t, err)
				require.Equal(t, original, unchanged, "outbound correction must not mutate replay or prewarm source payloads")
			}
			require.Len(t, upstream.writes, 3)
			for _, payload := range upstream.writes {
				require.Equal(t, false, payload["parallel_tool_calls"])
			}
			require.Equal(t, json.Number("900719925474099312345"), upstream.writes[1]["sequence"])
		})
	}
}

func TestNormalizeOpenAIResponsesLiteEgressRequest_BodyReplayAndValidation(t *testing.T) {
	account := ticketTestAccount(41)
	for _, field := range []string{``, `,"parallel_tool_calls":true`, `,"parallel_tool_calls":false`} {
		t.Run(field, func(t *testing.T) {
			body := `{"model":"gpt-6-astra","sequence":900719925474099312345` + field + `}`
			req, err := http.NewRequest(http.MethodPost, chatgptCodexURL, io.NopCloser(strings.NewReader(body)))
			require.NoError(t, err)
			req.Header[responsesLiteHeaderKey] = []string{"true"}
			require.NoError(t, normalizeOpenAIResponsesLiteEgressRequest(req, account))
			got, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.Equal(t, gjson.False, gjson.GetBytes(got, "parallel_tool_calls").Type)
			require.Equal(t, "900719925474099312345", gjson.GetBytes(got, "sequence").Raw)
			require.EqualValues(t, len(got), req.ContentLength)
			replay, err := req.GetBody()
			require.NoError(t, err)
			defer func() { _ = replay.Close() }()
			replayed, err := io.ReadAll(replay)
			require.NoError(t, err)
			require.Equal(t, got, replayed)
		})
	}
	for _, body := range []string{`null`, `[]`, `{"parallel_tool_calls":"false"}`} {
		req, err := http.NewRequest(http.MethodPost, chatgptCodexURL, strings.NewReader(body))
		require.NoError(t, err)
		req.Header.Set(responsesLiteHeader, "true")
		require.Error(t, normalizeOpenAIResponsesLiteEgressRequest(req, account))
	}
}

func TestResponsesLiteEgress_PreservesNonLiteRequestsAndControlFrames(t *testing.T) {
	body := []byte(`{"type":"response.create","parallel_tool_calls":true,"input":"hello"}`)
	for _, enabled := range []string{"", "false"} {
		req, err := http.NewRequest(http.MethodPost, chatgptCodexURL, bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set(responsesLiteHeader, enabled)
		require.NoError(t, normalizeOpenAIResponsesLiteEgressRequest(req, ticketTestAccount(41)))
		got, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.Equal(t, body, got)
		value := json.RawMessage(body)
		unchanged, err := normalizeOpenAIResponsesLiteWebSocketEgressValue(value, req.Header)
		require.NoError(t, err)
		require.Equal(t, value, unchanged)
	}
	headers := http.Header{}
	headers.Set(responsesLiteHeader, "true")
	for _, body := range []string{`{"type":"response.cancel","response_id":"resp_first"}`, `{"type":"session.update","session":{"model":"gpt-6-astra"}}`} {
		got, err := normalizeOpenAIResponsesLiteWebSocketEgress([]byte(body), headers)
		require.NoError(t, err)
		require.Equal(t, body, string(got))
	}
}

func TestPassthroughLifecycle_LiteHandshakePinsParallelAcrossTurns(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancel := context.WithCancelCause(context.Background())
	defer cancel(context.Canceled)
	upstream := newStagedPassthroughConn()
	account := passthroughLifecycleAccount()
	account.Credentials[credKeyHeaderOverrideEnabled] = true
	account.Credentials[credKeyHeaderOverrides] = map[string]any{responsesLiteHeaderKey: "true"}
	server, serverErr := startPassthroughLifecycleServer(t, controlCtx, newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream), account)
	defer server.Close()
	client := dialPassthroughLifecycleClientWithPayload(t, server, `{"type":"response.create","model":"gpt-5.1","parallel_tool_calls":true,"input":"first"}`)
	defer func() { _ = client.CloseNow() }()
	for _, responseID := range []string{"resp_first", "resp_second"} {
		payload := requirePassthroughUpstreamWrite(t, upstream, 3*time.Second)
		require.Equal(t, gjson.False, gjson.GetBytes(payload, "parallel_tool_calls").Type, string(payload))
		upstream.Send(`{"type":"response.completed","response":{"id":"` + responseID + `","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)
		_, err := readPassthroughLifecycleFrame(t, client, 3*time.Second)
		require.NoError(t, err)
		if responseID == "resp_first" {
			ctx, stop := context.WithTimeout(context.Background(), 3*time.Second)
			err = client.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","previous_response_id":"resp_first","input":"second"}`))
			stop()
			require.NoError(t, err)
		}
	}
	require.NoError(t, client.Close(coderws.StatusNormalClosure, "done"))
	select {
	case <-serverErr:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Lite passthrough to close")
	}
}
