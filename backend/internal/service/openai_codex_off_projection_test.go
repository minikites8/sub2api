package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCodexOffResponsesProjection(t *testing.T) {
	svc := &OpenAIGatewayService{}
	for _, namespace := range []string{"stable-account", ""} {
		acct := newTestOAuthAccount(21, nil)
		acct.Credentials = map[string]any{"chatgpt_account_id": namespace}
		for _, transport := range []string{"http", "passthrough", "ws"} {
			t.Run(namespace+"/"+transport, func(t *testing.T) {
				c := newFingerprintStageTestContext(t)
				c.Set("api_key", &APIKey{ID: 14})
				cacheOnly := []byte(`{"model":"gpt-5.1","prompt_cache_key":"shared"}`)
				build := func() http.Header {
					t.Helper()
					switch transport {
					case "ws":
						h, _, err := svc.buildOpenAIWSHeaders(context.Background(), c, acct, "token", OpenAIWSProtocolDecision{}, false, "", c.GetHeader(openAIWSTurnMetadataHeader), "shared", "gpt-5.1", "")
						require.NoError(t, err)
						return h
					case "http":
						r, err := svc.buildUpstreamRequest(context.Background(), c, acct, cacheOnly, "token", true, "shared", false)
						require.NoError(t, err)
						return r.Header
					default:
						r, err := svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, acct, cacheOnly, "token")
						require.NoError(t, err)
						return r.Header
					}
				}
				h := build()
				for _, name := range []string{"session-id", "session_id", "thread-id", "conversation_id"} {
					require.Empty(t, h.Values(name), name)
				}
				in := c.Request.Header
				in.Set("session_id", "S")
				in.Set("thread-id", "P")
				parent := build()
				in.Set("session-id", "S")
				in.Set("session_id", "ignored")
				in.Set("thread-id", "C")
				in.Set(codexParentThreadIDHeader, "P")
				in.Set("x-client-turn-id", "T")
				in.Set(openAIWSTurnMetadataHeader, `{"thread_id":"C","parent_thread_id":"P","turn_id":"T"}`)
				child := build()
				retry := build()
				require.NotEmpty(t, child.Get("session-id"))
				require.Equal(t, parent.Get("session-id"), child.Get("session-id"))
				require.Empty(t, child.Values("session_id"))
				require.NotEqual(t, parent.Get("thread-id"), child.Get("thread-id"))
				require.Equal(t, parent.Get("thread-id"), child.Get(codexParentThreadIDHeader))
				require.Equal(t, child.Get("thread-id"), retry.Get("thread-id"))
				require.Equal(t, "T", child.Get("x-client-turn-id"))
				meta := child.Get(openAIWSTurnMetadataHeader)
				require.Equal(t, "T", gjson.Get(meta, "turn_id").String())
				require.Equal(t, child.Get("thread-id"), gjson.Get(meta, "thread_id").String())
				require.Equal(t, parent.Get("thread-id"), gjson.Get(meta, "parent_thread_id").String())
				c.Set("api_key", &APIKey{ID: 15})
				require.NotEqual(t, child.Get("session-id"), build().Get("session-id"))
				if transport == "ws" {
					require.NotEqual(t, normalizeOpenAIWSHandshakeCompatibility(acct, parent), normalizeOpenAIWSHandshakeCompatibility(acct, child))
				}
			})
		}
	}
}

func TestCodexOffMetadataPreservesTurnReferences(t *testing.T) {
	a := newTestOAuthAccount(21, nil)
	a.Credentials = map[string]any{"chatgpt_account_id": "acct"}
	p := codexResponsesProjection(a, true)
	body := []byte(`{"response":{"client_metadata":{"thread_id":"child","parent_thread_id":"parent","root_thread_id":"parent","turn_id":"turn","parent_turn_id":"previous","x-codex-turn-metadata":"{\"turn_id\":\"turn\",\"parent_thread_id\":\"parent\"}"},"input":[{"turn_id":"untouched"}]}}`)
	out, changed, err := p.applyRaw(body, a, 11)
	require.NoError(t, err)
	require.True(t, changed)
	var v map[string]any
	require.NoError(t, json.Unmarshal(body, &v))
	require.True(t, p.applyMap(v, a, 11))
	mapped, err := json.Marshal(v)
	require.NoError(t, err)
	require.JSONEq(t, string(out), string(mapped))
	cm := gjson.GetBytes(out, "response.client_metadata")
	require.Equal(t, "turn", cm.Get("turn_id").String())
	require.Equal(t, "previous", cm.Get("parent_turn_id").String())
	parent := scopeCodexAccountIdentityValue(a, 11, "thread", "parent")
	require.Equal(t, parent, cm.Get("parent_thread_id").String())
	require.Equal(t, parent, cm.Get("root_thread_id").String())
	require.Equal(t, parent, gjson.Get(cm.Get("x-codex-turn-metadata").String(), "parent_thread_id").String())
	require.Equal(t, "untouched", gjson.GetBytes(out, "response.input.0.turn_id").String())
	require.False(t, gjson.GetBytes(out, "response.prompt_cache_key").Exists())
}

// Reusing a cache key across threads must not make its projection depend on
// whether one client's session happens to have the same literal value.
func TestCodexOffCacheSharingIndependentOfSession(t *testing.T) {
	a := newTestOAuthAccount(21, nil)
	a.Credentials = map[string]any{"chatgpt_account_id": "acct"}
	project := func(session string, key int64) string {
		v := map[string]any{"prompt_cache_key": "shared", "client_metadata": map[string]any{"session_id": session}}
		p := codexResponsesProjection(a, true)
		raw, err := json.Marshal(v)
		require.NoError(t, err)
		out, changed, err := p.applyRaw(raw, a, key)
		require.NoError(t, err)
		require.True(t, changed)
		require.True(t, p.applyMap(v, a, key))
		got := gjson.GetBytes(out, "prompt_cache_key").String()
		require.Equal(t, v["prompt_cache_key"], got)
		require.NotEqual(t, "shared", got)
		return got
	}
	require.Equal(t, project("shared", 11), project("different", 11))
	require.NotEqual(t, project("shared", 11), project("shared", 12))
}
