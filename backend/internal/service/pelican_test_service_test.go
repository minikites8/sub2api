package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestPelicanPayloadsDoNotChangeDefaultAccountTestPayloads(t *testing.T) {
	defaultClaude, err := createTestPayload("claude-sonnet-4-6")
	require.NoError(t, err)
	defaultMessages, ok := defaultClaude["messages"].([]map[string]any)
	require.True(t, ok)
	defaultContent, ok := defaultMessages[0]["content"].([]map[string]any)
	require.True(t, ok)
	require.Equal(t, "hi", defaultContent[0]["text"])

	defaultOpenAI := createOpenAITestPayload("gpt-6-astra", true)
	defaultInput, ok := defaultOpenAI["input"].([]map[string]any)
	require.True(t, ok)
	defaultOpenAIContent, ok := defaultInput[0]["content"].([]map[string]any)
	require.True(t, ok)
	require.Equal(t, "hi", defaultOpenAIContent[0]["text"])
	require.NotContains(t, defaultOpenAI, "reasoning")

	pelicanClaude, err := createPelicanClaudePayload("claude-sonnet-4-6", "draw the pelican animation")
	require.NoError(t, err)
	pelicanMessages, ok := pelicanClaude["messages"].([]map[string]any)
	require.True(t, ok)
	pelicanContent, ok := pelicanMessages[0]["content"].([]map[string]any)
	require.True(t, ok)
	require.Equal(t, "draw the pelican animation", pelicanContent[0]["text"])

	pelicanOpenAI := createPelicanOpenAIPayload("gpt-6-astra", true, "draw the pelican animation", "medium")
	pelicanInput, ok := pelicanOpenAI["input"].([]map[string]any)
	require.True(t, ok)
	pelicanOpenAIContent, ok := pelicanInput[0]["content"].([]map[string]any)
	require.True(t, ok)
	require.Equal(t, "draw the pelican animation", pelicanOpenAIContent[0]["text"])
	require.Equal(t, map[string]any{"effort": "medium"}, pelicanOpenAI["reasoning"])
}

type pelicanTicketAccountRepo struct {
	AccountRepository
	account *Account
}

func (r *pelicanTicketAccountRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}

func (r *pelicanTicketAccountRepo) UpdateExtra(context.Context, int64, map[string]any) error {
	return nil
}

func pelicanReadyTicketAccount() (*Account, *openAICodexTicket) {
	account := ticketTestAccount(4242)
	now := time.Now().Truncate(time.Second)
	ticket := &openAICodexTicket{
		AccountID: account.ID, Model: "gpt-6-astra", State: mint780State(now), Length: 780,
		IssuedAt: now, CapturedAt: now, ExpiresAt: now.Add(240 * time.Second),
		Transport: "sse", Gateway: "unified-88", HarvestCookiesAt: now,
		HarvestCookies: mint780Pair(now.Add(time.Hour), "unified-88"),
	}
	account.Extra = map[string]any{openAICodexTicketExtraKey(ticket.Model): ticket}
	return account, ticket
}

func TestPelicanOpenAIQuestionUsesHarvestedTicket(t *testing.T) {
	account, ticket := pelicanReadyTicketAccount()
	account.Credentials["model_mapping"] = map[string]any{"quality-alias": "gpt-6-astra", "gpt-6-astra": "gpt-6-sol"}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/4242/pelican-test", nil)
	c.Request.Header.Set("Authorization", "Bearer administrator-token")
	gateway := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true, TargetLength: 780, FailClosed: true, TTLSeconds: 240, Models: []string{ticket.Model}}, nil)
	requests := 0
	gateway.httpUpstream = &harvestProxyUpstream{do: func(req *http.Request, proxy string) (*http.Response, error) {
		requests++
		require.Equal(t, "chatgpt.com", req.URL.Host)
		require.Equal(t, "Bearer tok", req.Header.Get("Authorization"))
		require.Equal(t, ticket.State, req.Header.Get(openAICodexTurnStateHeader))
		require.Equal(t, strings.Join(ticket.HarvestCookies, "; "), req.Header.Get("Cookie"))
		require.Equal(t, HTTPUpstreamProfileOpenAIHarvest, HTTPUpstreamProfileFromContext(req.Context()))
		require.Empty(t, req.Header.Get(responsesLiteHeaderKey))
		require.Empty(t, proxy)
		require.True(t, gateway.codexTicketChatHeld(account.ID))
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.Equal(t, ticket.Model, gjson.GetBytes(body, "model").String(), "the requested alias is mapped once")
		require.Equal(t, "high", gjson.GetBytes(body, "reasoning.effort").String())
		require.Contains(t, string(body), "智商题测试")
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"42\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"model\":\"gpt-6-astra\"}}\n\n"))}, nil
	}}
	svc := ProvideAccountTestService(&pelicanTicketAccountRepo{account: account}, nil, nil, nil, nil, nil, gateway.httpUpstream, gateway.cfg, nil, gateway)
	require.NoError(t, svc.TestPelicanAccountConnection(c, account.ID, "quality-alias", "智商题测试", "high"))
	require.Equal(t, 1, requests)
	require.Contains(t, recorder.Body.String(), `"text":"42"`)
	require.Contains(t, recorder.Body.String(), `"type":"test_complete","success":true`)
	require.NotContains(t, recorder.Body.String(), ticket.State)
	require.False(t, gateway.codexTicketChatHeld(account.ID))
	require.Empty(t, c.Request.Header.Get("Session-Id"))
	require.Equal(t, "Bearer administrator-token", c.Request.Header.Get("Authorization"))
}

func TestPelicanOpenAIQuestionRejectsUnavailableTicket(t *testing.T) {
	for _, reason := range []string{"missing", "expired", "other model", "expired cookie", "wrong transport"} {
		t.Run(reason, func(t *testing.T) {
			account, ticket := pelicanReadyTicketAccount()
			switch reason {
			case "missing":
				account.Extra = nil
			case "expired":
				ticket.ExpiresAt = time.Now().Add(-time.Minute)
			case "other model":
				account.Extra = map[string]any{openAICodexTicketExtraKey("gpt-6-sol"): ticket}
			case "expired cookie":
				ticket.HarvestCookies = mint780Pair(time.Now().Add(-time.Minute), "unified-88")
			case "wrong transport":
				ticket.Transport = "websocket"
			}
			upstream := &harvestProxyUpstream{do: func(*http.Request, string) (*http.Response, error) {
				t.Fatal("an unavailable ticket must be rejected before any upstream request")
				return nil, nil
			}}
			gateway := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true, TargetLength: 780, FailClosed: true, Models: []string{ticket.Model}}, upstream)
			svc := &AccountTestService{accountRepo: &pelicanTicketAccountRepo{account: account}, openaiGatewayService: gateway}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/4242/pelican-test", nil)
			err := svc.TestPelicanAccountConnection(c, account.ID, ticket.Model, "Reply OK", "high")
			require.ErrorContains(t, err, "门票暂不可用")
			require.NotContains(t, recorder.Body.String(), `"type":"test_complete"`)
		})
	}
}

func TestPelicanOpenAIStreamForwardsFragmentedOutputImmediately(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/pelican-test", nil)
	w := &pelicanOpenAIStreamRecorder{ResponseRecorder: httptest.NewRecorder(), service: &AccountTestService{}, client: c}
	_, err := w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"答"))
	require.NoError(t, err)
	require.Empty(t, recorder.Body.String())
	_, err = w.WriteString("案\"}\n\n")
	require.NoError(t, err)
	require.Contains(t, recorder.Body.String(), `"text":"答案"`)
	require.False(t, w.done)
	_, err = w.WriteString("data: {\"type\":\"response.completed\"}\n\n")
	require.NoError(t, err)
	require.True(t, w.done)
	require.Empty(t, w.errorMsg)
	require.NotContains(t, recorder.Body.String(), "test_complete", "completion is emitted after the gateway returns successfully")
}

func TestPelicanOpenAIConnectivityKeepsItsDirectProbe(t *testing.T) {
	account, ticket := pelicanReadyTicketAccount()
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\"}\n\n")),
	}}
	gateway := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true, TargetLength: 780, FailClosed: true, Models: []string{ticket.Model}}, upstream)
	svc := ProvideAccountTestService(&pelicanTicketAccountRepo{account: account}, nil, nil, nil, nil, nil, upstream, gateway.cfg, nil, gateway)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/4242/test", nil)
	require.NoError(t, svc.TestAccountConnection(c, account.ID, ticket.Model, "", AccountTestModeDefault))
	require.Len(t, upstream.requests, 1)
	require.Empty(t, upstream.lastReq.Header.Get(openAICodexTurnStateHeader))
	require.Empty(t, upstream.lastReq.Header.Get("Cookie"))
	require.Equal(t, "hi", gjson.GetBytes(upstream.lastBody, "input.0.content.0.text").String())
}

func TestPelicanOpenAIStreamReportsFailedAndIncompleteResponses(t *testing.T) {
	for _, event := range []string{"response.failed", "response.incomplete"} {
		t.Run(event, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/pelican-test", nil)
			w := &pelicanOpenAIStreamRecorder{ResponseRecorder: httptest.NewRecorder(), service: &AccountTestService{}, client: c}
			_, err := w.WriteString("data: {\"type\":\"" + event + "\",\"response\":{\"error\":{\"message\":\"generation interrupted\"}}}\n\n")
			require.NoError(t, err)
			require.True(t, w.done)
			require.Equal(t, "generation interrupted", w.errorMsg)
		})
	}
}
