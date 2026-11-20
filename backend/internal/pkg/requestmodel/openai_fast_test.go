package requestmodel

import (
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"testing"
)

func TestOpenAIFastAlias(t *testing.T) {
	for _, tc := range []struct {
		name, body, model, tier string
		ws, changed, bad        bool
	}{
		{"gpt", `{"model":"gpt-5.5-fast","input":"unchanged"}`, "gpt-5.5", "priority", false, true, false},
		{"explicit", `{"model":"o3-mini-fast","service_tier":"flex"}`, "o3-mini", "flex", false, true, false},
		{"escaped", `{"\u006dodel":"gpt-5.5-\u0066ast"}`, "gpt-5.5", "priority", false, true, false},
		{"ordinary", `{ "model" : "gpt-5.5" }`, "gpt-5.5", "", false, false, false},
		{"literal vendor model", `{"model":"grok-4-fast"}`, "grok-4-fast", "", false, false, false},
		{"nested data", `{"input":{"model":"gpt-5.5-fast"}}`, "", "", false, false, false},
		{"ws control", `{"type":"response.cancel","model":"gpt-5.5-fast"}`, "gpt-5.5-fast", "", true, false, false},
		{"duplicate model", `{"model":"gpt-5.5-fast","model":"other"}`, "", "", false, false, true},
		{"ambiguous tier", `{"model":"gpt-5.5-fast","Service_Tier":"auto"}`, "", "", false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte(tc.body)
			out, changed, err := ExpandOpenAIFast(src, tc.ws)
			if tc.bad {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.changed, changed)
			require.Equal(t, tc.model, gjson.GetBytes(out, "model").String())
			require.Equal(t, tc.tier, gjson.GetBytes(out, "service_tier").String())
			require.NoError(t, ValidateJSONModels(out))
			if !changed {
				require.Equal(t, src, out)
			}
			again, changed, err := ExpandOpenAIFast(out, tc.ws)
			require.NoError(t, err)
			require.False(t, changed)
			require.Equal(t, out, again)
		})
	}
}
