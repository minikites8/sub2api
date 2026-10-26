package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type qosAccountUpdateStub struct {
	service.AdminService
	account *service.Account
	updated map[string]any
}

func (s *qosAccountUpdateStub) GetAccount(context.Context, int64) (*service.Account, error) {
	return s.account, nil
}
func (s *qosAccountUpdateStub) UpdateAccount(_ context.Context, _ int64, in *service.UpdateAccountInput) (*service.Account, error) {
	s.updated = in.Extra
	s.account.Extra = in.Extra
	return s.account, nil
}

func TestGPTQoSIndependentAccountFieldUpdatePreservesExtra(t *testing.T) {
	for _, body := range []string{`{"qos_resource_pool":"shared-new"}`, `{"quality_tier":"basic"}`} {
		t.Run(body, func(t *testing.T) {
			svc := &qosAccountUpdateStub{account: &service.Account{ID: 41, Name: "fixture", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Extra: map[string]any{service.AccountQualityTierKey: "strong", "openai_passthrough": true, "base_rpm": 100}}}
			handler := &AccountHandler{adminService: svc}
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.PUT("/accounts/:id", handler.Update)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut, "/accounts/41", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			require.Equal(t, 200, w.Code, w.Body.String())
			require.Equal(t, true, svc.updated["openai_passthrough"])
			require.Equal(t, 100, svc.updated["base_rpm"])
			if strings.Contains(body, "qos_resource_pool") {
				require.Equal(t, "strong", svc.updated[service.AccountQualityTierKey])
				require.Equal(t, "shared-new", svc.updated[service.AccountResourcePoolKey])
			} else {
				require.Equal(t, "basic", svc.updated[service.AccountQualityTierKey])
			}
		})
	}
}
