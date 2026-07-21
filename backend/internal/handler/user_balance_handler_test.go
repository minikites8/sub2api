package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUserHandlerGetAPIKeyBalanceReturnsFlatPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{
		ID:          42,
		Status:      service.StatusActive,
		Balance:     12.34,
		Concurrency: 2,
	}
	apiKey := &service.APIKey{
		ID:     7,
		UserID: user.ID,
		Name:   "balance-key",
		Status: service.StatusActive,
		User:   user,
		Quota:  20,
	}
	h := NewUserHandler(nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.GET("/user/balance", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), apiKey)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: user.ID, Concurrency: user.Concurrency})
		h.GetAPIKeyBalance(c)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/user/balance", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, true, body["is_valid"])
	require.Equal(t, true, body["is_active"])
	require.Equal(t, true, body["can_request"])
	require.InDelta(t, 12.34, body["balance"], 0.000001)
	require.InDelta(t, 12.34, body["remaining"], 0.000001)
	require.Equal(t, "USD", body["unit"])
	require.Equal(t, "USD", body["currency"])
	require.InDelta(t, 42, body["user_id"], 0.000001)
	require.InDelta(t, 7, body["api_key_id"], 0.000001)
	require.Equal(t, "balance-key", body["api_key_name"])
	require.InDelta(t, 20, body["quota"], 0.000001)
	require.InDelta(t, 20, body["quota_remaining"], 0.000001)
}
