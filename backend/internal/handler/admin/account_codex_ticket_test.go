package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type ticketDisplayAdminStub struct {
	service.AdminService
	account *service.Account
	calls   int
}

func (s *ticketDisplayAdminStub) GetAccount(context.Context, int64) (*service.Account, error) {
	s.calls++
	return s.account, nil
}

type ticketDisplayProviderStub struct {
	logs int
	ids  []int64
}

func (s *ticketDisplayProviderStub) OpenAICodexTicketStatuses(context.Context, *service.Account, time.Time) []service.OpenAICodexTicketStatus {
	return []service.OpenAICodexTicketStatus{{Model: "ticket-model", Ready: true}}
}
func (s *ticketDisplayProviderStub) OpenAICodexTicketStatusBatch(_ context.Context, ids []int64) (map[int64][]service.OpenAICodexTicketStatus, error) {
	s.ids = ids
	return map[int64][]service.OpenAICodexTicketStatus{71: {{Model: "ticket-model", Ready: true}}}, nil
}
func (s *ticketDisplayProviderStub) OpenAICodexTicketLogs(_ context.Context, _ *service.Account, model string, _ time.Time) (*service.OpenAICodexTicketLogs, error) {
	s.logs++
	if model == "unknown" {
		return nil, service.ErrOpenAICodexTicketLogModel
	}
	return &service.OpenAICodexTicketLogs{Model: model, Entries: []service.OpenAICodexTicketLogEntry{}, Limit: 200}, nil
}
func ticketDisplayRouter(t *testing.T) (*gin.Engine, *ticketDisplayAdminStub, *ticketDisplayProviderStub) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	a := &ticketDisplayAdminStub{account: &service.Account{ID: 71, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth}}
	p := &ticketDisplayProviderStub{}
	h := &AccountHandler{adminService: a}
	h.SetCodexTicketProvider(p)
	r := gin.New()
	r.GET("/accounts/:id/codex-ticket-logs", h.GetCodexTicketLogs)
	r.POST("/accounts/codex-tickets/batch", h.GetBatchCodexTickets)
	return r, a, p
}
func TestCodexTicketDisplayLogsValidation(t *testing.T) {
	for _, tc := range []struct {
		path string
		code int
	}{{"71?model=ticket-model", 200}, {"0?model=ticket-model", 400}, {"-1?model=ticket-model", 400}, {"71?", 400}, {"71?model=unknown", 400}} {
		t.Run(tc.path, func(t *testing.T) {
			r, _, p := ticketDisplayRouter(t)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/accounts/"+strings.Replace(tc.path, "?", "/codex-ticket-logs?", 1), nil))
			require.Equal(t, tc.code, rec.Code)
			require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
			if tc.code == 200 {
				require.Equal(t, 1, p.logs)
			}
		})
	}
	r, a, p := ticketDisplayRouter(t)
	a.account.Type = service.AccountTypeAPIKey
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/accounts/71/codex-ticket-logs?model=ticket-model", nil))
	require.Equal(t, 400, rec.Code)
	require.Zero(t, p.logs)
}
func TestCodexTicketDisplayBatchValidationAndDedup(t *testing.T) {
	r, _, p := ticketDisplayRouter(t)
	bodies := []string{`{"account_ids":[71,71]}`, `{"account_ids":[0]}`, `{"account_ids":[-1]}`}
	many, _ := json.Marshal(map[string]any{"account_ids": make([]int, 201)})
	bodies = append(bodies, string(many))
	for i, body := range bodies {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/accounts/codex-tickets/batch", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(rec, req)
		if i == 0 {
			require.Equal(t, 200, rec.Code)
			require.Equal(t, []int64{71}, p.ids)
			require.Contains(t, rec.Body.String(), "ticket-model")
		} else {
			require.Equal(t, 400, rec.Code)
		}
	}
}
