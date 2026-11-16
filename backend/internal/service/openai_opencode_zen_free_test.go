//go:build unit

package service

import (
	"bytes"
	"container/list"
	"context"
	"crypto/sha256"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const zenFreeChatURL = "https://opencode.ai/zen/v1/chat/completions"
const zenFreeResponsesURL = "https://opencode.ai/zen/v1/responses"

func TestOpenCodeZenFreeScope(t *testing.T) {
	account := openCodeMappedTestAccount()
	body := []byte(`{"model":"beam-free","input":"hi","stream":false}`)
	for _, target := range []string{
		"https://opencode.ai/zen/go/v1/responses",
		"https://opencode.ai.evil.example/zen/v1/responses",
		"https://api.opencode.ai/zen/v1/responses",
		"http://opencode.ai/zen/v1/responses",
		"https://relay.example/zen/v1/responses",
		"https://opencode.ai/zen/v1/responses/compact",
	} {
		require.False(t, isOpenCodeZenFreeRequest(account, target, body), target)
		require.Equal(t, body, normalizeOpenCodeFreeTierBody(account, target, body))
	}
	require.True(t, isOpenCodeZenFreeRequest(account, zenFreeResponsesURL, body))
	require.False(t, isOpenCodeZenFreeRequest(nil, zenFreeResponsesURL, body))
	account.Type = AccountTypeOAuth
	require.False(t, isOpenCodeZenFreeRequest(account, zenFreeResponsesURL, body))
}

func TestOpenCodeZenFreeBodyPreservesCallerData(t *testing.T) {
	account := openCodeMappedTestAccount()
	for _, target := range []string{zenFreeChatURL, zenFreeResponsesURL} {
		body := []byte(`{"model":"beam-free","input":"hi","stream":false,"metadata":{"number":9007199254740993}}`)
		got := normalizeOpenCodeFreeTierBody(account, target, body)
		require.True(t, gjson.GetBytes(got, "stream").Bool())
		require.Equal(t, "9007199254740993", gjson.GetBytes(got, "metadata.number").Raw)
		require.Equal(t, "none", gjson.GetBytes(got, "tool_choice").String())
		if target == zenFreeResponsesURL {
			require.Equal(t, "bash", gjson.GetBytes(got, "tools.0.name").String())
			require.Equal(t, "read", gjson.GetBytes(got, "tools.1.name").String())
		} else {
			require.Equal(t, "bash", gjson.GetBytes(got, "tools.0.function.name").String())
			require.Equal(t, "read", gjson.GetBytes(got, "tools.1.function.name").String())
			require.True(t, gjson.GetBytes(got, "stream_options.include_usage").Bool())
		}
		require.JSONEq(t, string(got), string(normalizeOpenCodeFreeTierBody(account, target, got)))
	}
	body := []byte(`{"model":"beam-free","input":"hi","tools":[{"type":"function","name":"exec","parameters":{"type":"object","properties":{}}}],"tool_choice":{"type":"function","name":"exec"}}`)
	got := normalizeOpenCodeFreeTierBody(account, zenFreeResponsesURL, body)
	require.Equal(t, gjson.GetBytes(body, "tools").Raw, gjson.GetBytes(got, "tools").Raw)
	require.Equal(t, gjson.GetBytes(body, "tool_choice").Raw, gjson.GetBytes(got, "tool_choice").Raw)
	paid := []byte(`{"model":"claude-sonnet-4","stream":false}`)
	require.Equal(t, paid, normalizeOpenCodeFreeTierBody(account, zenFreeResponsesURL, paid))
}

func TestOpenCodeZenFreeTLSProfileScopeAndOverrides(t *testing.T) {
	account := openCodeMappedTestAccount()
	request := func(target, body string) *http.Request {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, target, strings.NewReader(body))
		require.NoError(t, err)
		return req
	}
	free := request(zenFreeResponsesURL, `{"model":"beam-free"}`)
	require.Equal(t, tlsfingerprint.OpenCodeBunProfileName, openCodeZenTLSProfile(free, account, nil).Name)
	configured := &tlsfingerprint.Profile{Name: "custom"}
	require.Same(t, configured, openCodeZenTLSProfile(free, account, configured))
	require.Nil(t, openCodeZenTLSProfile(request(zenFreeResponsesURL, `{"model":"paid"}`), account, nil))
	require.Nil(t, openCodeZenTLSProfile(request("https://relay.example/v1/responses", `{"model":"beam-free"}`), account, nil))
	require.Nil(t, openCodeZenTLSProfile(nil, account, nil))
	require.Nil(t, openCodeZenTLSProfile(free, nil, nil))
	account.Type = AccountTypeOAuth
	require.Nil(t, openCodeZenTLSProfile(free, account, nil))
	data, err := io.ReadAll(free.Body)
	require.NoError(t, err)
	require.JSONEq(t, `{"model":"beam-free"}`, string(data), "fingerprint detection must not consume the request body")
}

func TestOpenCodeZenFreeSessionStableAcrossTurnsAndProtocols(t *testing.T) {
	account := openCodeMappedTestAccount()
	c := newOpenCodeSessionTestContext(t, "client-session")
	c.Set("api_key", &APIKey{ID: 321})
	body := []byte(`{"model":"beam-free","prompt_cache_key":"body-session","input":"hi"}`)
	headers := http.Header{"originator": {"codex_cli_rs"}, "x-stainless-lang": {"python"}, "x-codex-window-id": {"old"}}
	applyOpenCodeSessionHeader(c, account, zenFreeResponsesURL, headers, body)
	session := headers.Get(openCodeSessionHeader)
	require.Regexp(t, `^ses_[0-9a-f]{12}[0-9A-Za-z]{14}$`, session)
	require.Equal(t, session, headers.Get(openCodeZenSessionIDHeader))
	require.Equal(t, session, headers.Get(openCodeSessionAffinityHeader))
	require.Equal(t, session, headers.Get(openCodeSessionIDHeader))
	require.Regexp(t, `^msg_[0-9a-f]{12}[0-9A-Za-z]{14}$`, headers.Get(openCodeZenRequestHeader))
	require.Empty(t, getHeaderRaw(headers, "originator"))
	require.Empty(t, getHeaderRaw(headers, "x-stainless-lang"))
	require.Empty(t, getHeaderRaw(headers, "x-codex-window-id"))
	later := newOpenCodeSessionTestContext(t, "client-session")
	later.Set("api_key", &APIKey{ID: 321})
	headers2 := make(http.Header)
	applyOpenCodeSessionHeader(later, account, zenFreeChatURL, headers2, []byte(`{"model":"beam-free","messages":[]}`))
	require.Equal(t, session, headers2.Get(openCodeSessionHeader))
	require.NotEqual(t, headers.Get(openCodeZenRequestHeader), headers2.Get(openCodeZenRequestHeader))
	later.Set("api_key", &APIKey{ID: 322})
	headers3 := make(http.Header)
	applyOpenCodeSessionHeader(later, account, zenFreeResponsesURL, headers3, body)
	require.NotEqual(t, session, headers3.Get(openCodeSessionHeader))
}

func TestOpenCodeZenFreeHonorsNativeSessionAndHeaderOverrides(t *testing.T) {
	account := openCodeMappedTestAccount()
	c := newOpenCodeSessionTestContext(t, "")
	session := newOpenCodeZenID("ses_", true)
	c.Request.Header.Set(openCodeZenSessionIDHeader, session)
	requestID := newOpenCodeZenID("msg_", false)
	headers := http.Header{"x-opencode-request": {requestID}, "x-opencode-client": {"desktop"}, "User-Agent": {"custom-agent"}}
	applyOpenCodeSessionHeader(c, account, zenFreeResponsesURL, headers, []byte(`{"model":"beam-free"}`))
	require.Equal(t, session, headers.Get(openCodeSessionHeader))
	require.Equal(t, requestID, headers.Get(openCodeZenRequestHeader))
	require.Equal(t, "desktop", getHeaderRaw(headers, "x-opencode-client"))
	require.Equal(t, "custom-agent", headers.Get("User-Agent"))
	for _, target := range []string{zenFreeResponsesURL, zenFreeChatURL} {
		first, second := make(http.Header), make(http.Header)
		applyOpenCodeSessionHeader(c, account, target, first, []byte(`{"model":"beam-free"}`))
		applyOpenCodeSessionHeader(c, account, target, second, []byte(`{"model":"beam-free"}`))
		require.Equal(t, first.Get(openCodeSessionHeader), second.Get(openCodeSessionHeader))
	}
}

func TestOpenCodeZenIDTimestampAndBoundedSessionCache(t *testing.T) {
	for _, descending := range []bool{true, false} {
		prefix := "msg_"
		if descending {
			prefix = "ses_"
		}
		id := newOpenCodeZenID(prefix, descending)
		require.True(t, regexp.MustCompile(`^[a-z]{3}_[0-9a-f]{12}[0-9A-Za-z]{14}$`).MatchString(id))
		head, err := strconv.ParseUint(id[4:16], 16, 64)
		require.NoError(t, err)
		if descending {
			head = ^head & 0xffffffffffff
		}
		require.InDelta(t, time.Now().UnixMilli()&0xfffffffff, int64(head>>12), 1000)
	}
	cache := openCodeZenSessionCache{entries: make(map[[32]byte]*list.Element), order: list.New()}
	key := sha256.Sum256([]byte("first"))
	first := cache.resolve(key)
	require.Equal(t, first, cache.resolve(key))
	cache.entries[key].Value.(*openCodeZenSessionEntry).lastUsed = time.Now().Add(-2 * openCodeZenSessionTTL)
	require.NotEqual(t, first, cache.resolve(key))
	for i := 0; i < openCodeZenSessionLimit+1; i++ {
		cache.resolve(sha256.Sum256([]byte(strconv.Itoa(i))))
	}
	require.Len(t, cache.entries, openCodeZenSessionLimit)
}

type zenFreeCaptureUpstream struct {
	*httpUpstreamRecorder
	profile *tlsfingerprint.Profile
}

func (u *zenFreeCaptureUpstream) DoWithTLS(req *http.Request, proxy string, id int64, concurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	u.profile = profile
	return u.Do(req, proxy, id, concurrency)
}

func TestOpenCodeZenFreeProbeAndForwardShareClient(t *testing.T) {
	account := openCodeMappedTestAccount()
	for _, responses := range []bool{false, true} {
		var reply *http.Response
		if responses {
			reply = adaptiveCNResponsesTestResponse()
		} else {
			reply = adaptiveCNChatTestResponse()
		}
		probe, base := adaptiveCNAccountTestService(account, reply)
		upstream := &zenFreeCaptureUpstream{httpUpstreamRecorder: base}
		probe.httpUpstream = upstream
		c, _ := newTestContext()
		var err error
		if responses {
			err = probe.testCNProviderAdaptiveResponsesConnection(c, account, "beam-free", "sk-opencode")
		} else {
			err = probe.testOpenAIChatCompletionsConnection(c, account, "beam-free", "hi", "https://opencode.ai/zen/v1", "sk-opencode")
		}
		require.NoError(t, err)
		require.Equal(t, openCodeUpstreamUserAgent, upstream.lastReq.Header.Get("User-Agent"))
		require.NotEmpty(t, upstream.lastReq.Header.Get(openCodeZenSessionIDHeader))
		require.Equal(t, tlsfingerprint.OpenCodeBunProfileName, upstream.profile.Name)
		require.Equal(t, "none", gjson.GetBytes(upstream.lastBody, "tool_choice").String())
	}
	upstream := &zenFreeCaptureUpstream{httpUpstreamRecorder: &httpUpstreamRecorder{resp: adaptiveCNChatTestResponse()}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	c := newOpenCodeSessionTestContext(t, "client-forward")
	reply, err := svc.sendCCUpstreamRequest(context.Background(), c, account, zenFreeChatURL,
		[]byte(`{"model":"beam-free","messages":[{"role":"user","content":"hi"}],"stream":true}`), true, "sk-opencode", "", "")
	require.NoError(t, err)
	require.NoError(t, reply.Body.Close())
	require.Equal(t, tlsfingerprint.OpenCodeBunProfileName, upstream.profile.Name)
	require.Equal(t, "none", gjson.GetBytes(upstream.lastBody, "tool_choice").String())
	require.NotEmpty(t, upstream.lastReq.Header.Get(openCodeZenSessionIDHeader))
}

func TestOpenCodeZenFreeBufferedChatAndLimits(t *testing.T) {
	body := `data: {"id":"chat_1","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}` + "\n\ndata: [DONE]\n\n"
	upstream := &zenFreeCaptureUpstream{httpUpstreamRecorder: &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body)),
	}}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	request := []byte(`{"model":"deepseek-v4.1-flash-free","stream":false,"messages":[{"role":"user","content":"hi"}]}`)
	c, recorder := newTestContext()
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(request))
	result, err := svc.ForwardAsChatCompletions(context.Background(), c, openCodeMappedTestAccount(), request, "", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())
	require.Equal(t, "ok", gjson.GetBytes(recorder.Body.Bytes(), "choices.0.message.content").String())
	require.Equal(t, int64(3), gjson.GetBytes(recorder.Body.Bytes(), "usage.total_tokens").Int())

	limitBody := `{"type":"error","error":{"type":"FreeUsageLimitError","message":"Free usage limit reached"}}`
	upstream.httpUpstreamRecorder = &httpUpstreamRecorder{resp: newJSONResponse(http.StatusTooManyRequests, limitBody)}
	reply, err := svc.sendCCUpstreamRequest(context.Background(), newOpenCodeSessionTestContext(t, ""), openCodeMappedTestAccount(), zenFreeChatURL,
		request, false, "sk-opencode", "", "")
	require.NoError(t, err)
	require.Equal(t, http.StatusTooManyRequests, reply.StatusCode)
	data, err := io.ReadAll(reply.Body)
	require.NoError(t, err)
	require.NoError(t, reply.Body.Close())
	require.JSONEq(t, limitBody, string(data))
	require.Len(t, upstream.requests, 1)
}
