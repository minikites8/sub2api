package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenCodeZenEnvironmentPreservesChatContent(t *testing.T) {
	for _, body := range []string{
		`{"messages":[{"role":"user","content":"hello","vendor":{"number":9007199254740993}}],"seed":9007199254740993}`,
		`{"messages":[{"role":"system","content":"Keep this instruction."},{"role":"user","content":"hello","vendor":{"number":9007199254740993}}],"seed":9007199254740993}`,
		`{"messages":[{"role":"developer","content":[{"type":"text","text":"Keep this instruction.","cache_control":{"type":"ephemeral"}}]},{"role":"user","content":"hello","vendor":{"number":9007199254740993}}],"seed":9007199254740993}`,
	} {
		got := ensureOpenCodeZenEnvironment([]byte(body), false)
		require.Equal(t, "9007199254740993", gjson.GetBytes(got, "seed").Raw)
		messages := gjson.GetBytes(got, "messages").Array()
		require.Equal(t, "hello", messages[len(messages)-1].Get("content").String())
		require.Equal(t, "9007199254740993", messages[len(messages)-1].Get("vendor.number").Raw)
		require.True(t, openCodeZenHasEnvironment(messages[0].Get("content")))
		require.Equal(t, got, ensureOpenCodeZenEnvironment(got, false), "retries must not duplicate context")
		if original := gjson.Get(body, "messages.0.content"); original.IsArray() {
			require.Equal(t, original.Array()[0].Raw, messages[0].Get("content.0").Raw)
		} else if gjson.Get(body, "messages.0.role").String() == "system" {
			require.Contains(t, messages[0].Get("content").String(), original.String())
		}
	}
}

func TestOpenCodeZenEnvironmentPreservesResponsesInstructions(t *testing.T) {
	body := []byte(`{"instructions":"Keep these rules.","input":[{"role":"user","content":[{"type":"input_text","text":"hi"}]}],"metadata":{"number":9007199254740993}}`)
	got := ensureOpenCodeZenEnvironment(body, true)
	require.Contains(t, gjson.GetBytes(got, "instructions").String(), "Keep these rules.\n\n<env>")
	require.Equal(t, gjson.GetBytes(body, "input").Raw, gjson.GetBytes(got, "input").Raw)
	require.Equal(t, "9007199254740993", gjson.GetBytes(got, "metadata.number").Raw)
	require.Equal(t, got, ensureOpenCodeZenEnvironment(got, true))
	got = ensureOpenCodeZenEnvironment([]byte(`{"input":"hi"}`), true)
	require.Equal(t, openCodeZenGatewayEnvironment, gjson.GetBytes(got, "instructions").String())
}

func TestOpenCodeZenEnvironmentHonorsCallerContext(t *testing.T) {
	for _, test := range []struct {
		body      string
		responses bool
	}{
		{`{"messages":[{"role":"system","content":"<env>actual client workspace</env>"}]}`, false},
		{`{"messages":[{"role":"developer","content":[{"type":"text","text":"<env>actual client workspace</env>"}]}]}`, false},
		{`{"instructions":"<env>actual client workspace</env>","input":"hi"}`, true},
		{`{"input":[{"role":"developer","content":[{"type":"input_text","text":"<env>actual client workspace</env>"}]}]}`, true},
		{`{"instructions":{"invalid":"type"},"input":"hi"}`, true},
		{`{"messages":"invalid"}`, false},
		{`not json`, false},
	} {
		require.Equal(t, test.body, string(ensureOpenCodeZenEnvironment([]byte(test.body), test.responses)))
	}
	// A user quoting an environment is not a system/developer instruction.
	got := ensureOpenCodeZenEnvironment([]byte(`{"messages":[{"role":"user","content":"<env>quoted</env>"}]}`), false)
	require.Equal(t, int64(2), gjson.GetBytes(got, "messages.#").Int())
}
