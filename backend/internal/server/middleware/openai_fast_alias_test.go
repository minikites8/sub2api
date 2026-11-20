package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"io"
	"net/http"
	"testing"
)

func TestOpenAIFastAliasHTTPAdmission(t *testing.T) {
	for _, route := range []string{"/v1/responses", "/v1/chat/completions", "/backend-api/codex/responses", "/responses", "/chat/completions"} {
		t.Run(route, func(t *testing.T) {
			key := allowlistAPIKey(true, "gpt-5.5")
			r := gin.New()
			r.Use(func(c *gin.Context) { c.Set(string(ContextKeyAPIKey), key) }, RequestModelIntegrity(1024), GroupModelAllowlist())
			r.POST(route, func(c *gin.Context) {
				b, err := io.ReadAll(c.Request.Body)
				require.NoError(t, err)
				c.Data(200, "application/json", b)
			})
			w := doJSON(t, r, http.MethodPost, route, `{"model":"gpt-5.5-fast","input":[{"model":"literal-fast"}]}`)
			require.Equal(t, 200, w.Code)
			require.Equal(t, "priority", gjson.Get(w.Body.String(), "service_tier").String())
			require.Equal(t, "gpt-5.5", gjson.Get(w.Body.String(), "model").String())
			require.Equal(t, "literal-fast", gjson.Get(w.Body.String(), "input.0.model").String())
			require.Equal(t, 404, doJSON(t, r, http.MethodPost, route, `{"model":"gpt-4.1-fast"}`).Code)
		})
	}
}
