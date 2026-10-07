package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGatewayRoutesRejectAmbiguousModelsWithoutAllowlist(t *testing.T) {
	for _, path := range []string{
		"/v1/responses", "/v1/responses/compact", "/v1/chat/completions", "/v1/messages", "/v1/embeddings",
		"/responses", "/responses/compact", "/chat/completions", "/messages/count_tokens", "/embeddings",
		"/backend-api/codex/responses", "/antigravity/v1/messages",
	} {
		t.Run(path, func(t *testing.T) {
			router := newGatewayRoutesTestRouterWithGroup(allowlistGroup(service.PlatformOpenAI, false))
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"gpt-5.6-luna","model":"gpt-6-astra"}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), "must be unique")
		})
	}
}
