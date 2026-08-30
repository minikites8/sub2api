package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type dataResponse struct {
	Code int         `json:"code"`
	Data dataPayload `json:"data"`
}

type dataPayload struct {
	Type           string        `json:"type"`
	Version        int           `json:"version"`
	Proxies        []dataProxy   `json:"proxies"`
	Accounts       []dataAccount `json:"accounts"`
	SkippedShadows int           `json:"skipped_shadows"`
}

type dataProxy struct {
	ProxyKey string `json:"proxy_key"`
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Status   string `json:"status"`
}

type dataAccount struct {
	Name        string         `json:"name"`
	Platform    string         `json:"platform"`
	Type        string         `json:"type"`
	Credentials map[string]any `json:"credentials"`
	Extra       map[string]any `json:"extra"`
	ProxyKey    *string        `json:"proxy_key"`
	Concurrency int            `json:"concurrency"`
	Priority    int            `json:"priority"`
}

func setupAccountDataRouter() (*gin.Engine, *stubAdminService) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	adminSvc := newStubAdminService()

	h := NewAccountHandler(
		adminSvc,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	router.GET("/api/v1/admin/accounts/data", h.ExportData)
	router.POST("/api/v1/admin/accounts/data", h.ImportData)
	return router, adminSvc
}

func TestExportDataIncludesSecrets(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()

	proxyID := int64(11)
	adminSvc.proxies = []service.Proxy{
		{
			ID:       proxyID,
			Name:     "proxy",
			Protocol: "http",
			Host:     "127.0.0.1",
			Port:     8080,
			Username: "user",
			Password: "pass",
			Status:   service.StatusActive,
		},
		{
			ID:       12,
			Name:     "orphan",
			Protocol: "https",
			Host:     "10.0.0.1",
			Port:     443,
			Username: "o",
			Password: "p",
			Status:   service.StatusActive,
		},
	}
	adminSvc.accounts = []service.Account{
		{
			ID:          21,
			Name:        "account",
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeOAuth,
			Credentials: map[string]any{"token": "secret"},
			Extra:       map[string]any{"note": "x"},
			ProxyID:     &proxyID,
			Concurrency: 3,
			Priority:    50,
			Status:      service.StatusDisabled,
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/data", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp dataResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Empty(t, resp.Data.Type)
	require.Equal(t, 0, resp.Data.Version)
	require.Len(t, resp.Data.Proxies, 1)
	require.Equal(t, "pass", resp.Data.Proxies[0].Password)
	require.Len(t, resp.Data.Accounts, 1)
	require.Equal(t, "secret", resp.Data.Accounts[0].Credentials["token"])
}

func TestExportDataWithoutProxies(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()

	proxyID := int64(11)
	adminSvc.proxies = []service.Proxy{
		{
			ID:       proxyID,
			Name:     "proxy",
			Protocol: "http",
			Host:     "127.0.0.1",
			Port:     8080,
			Username: "user",
			Password: "pass",
			Status:   service.StatusActive,
		},
	}
	adminSvc.accounts = []service.Account{
		{
			ID:          21,
			Name:        "account",
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeOAuth,
			Credentials: map[string]any{"token": "secret"},
			ProxyID:     &proxyID,
			Concurrency: 3,
			Priority:    50,
			Status:      service.StatusDisabled,
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/data?include_proxies=false", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp dataResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Len(t, resp.Data.Proxies, 0)
	require.Len(t, resp.Data.Accounts, 1)
	require.Nil(t, resp.Data.Accounts[0].ProxyKey)
}

// TestExportDataExcludesSparkShadow 验证外审第5轮 P1/P2:导出时排除 spark 影子账号
// (影子无凭据、导入侧强制 credentials 非空,混入会产出无法还原的坏备份),并透出跳过计数。
func TestExportDataExcludesSparkShadow(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()

	parentID := int64(21)
	adminSvc.accounts = []service.Account{
		{
			ID:          parentID,
			Name:        "mother",
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeOAuth,
			Credentials: map[string]any{"token": "secret"},
			Status:      service.StatusActive,
		},
		{
			ID:              22,
			Name:            "mother (Spark)",
			Platform:        service.PlatformOpenAI,
			Type:            service.AccountTypeOAuth,
			Credentials:     map[string]any{}, // 影子恒空凭据
			ParentAccountID: &parentID,        // 影子标记
			QuotaDimension:  service.QuotaDimensionSpark,
			Status:          service.StatusActive,
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/data?include_proxies=false", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp dataResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Len(t, resp.Data.Accounts, 1, "影子应被排除,仅导出母账号")
	require.Equal(t, "mother", resp.Data.Accounts[0].Name)
	require.Equal(t, 1, resp.Data.SkippedShadows, "跳过的影子数量应透出")
}

func TestExportDataPassesAccountFiltersAndSort(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()
	adminSvc.accounts = []service.Account{
		{ID: 1, Name: "acc-1", Status: service.StatusActive},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/admin/accounts/data?platform=openai&type=oauth&status=active&group=12&privacy_mode=blocked&search=keyword&sort_by=priority&sort_order=desc",
		nil,
	)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Equal(t, 1, adminSvc.lastListAccounts.calls)
	require.Equal(t, "openai", adminSvc.lastListAccounts.platform)
	require.Equal(t, "oauth", adminSvc.lastListAccounts.accountType)
	require.Equal(t, "active", adminSvc.lastListAccounts.status)
	require.Equal(t, int64(12), adminSvc.lastListAccounts.groupID)
	require.Equal(t, "blocked", adminSvc.lastListAccounts.privacyMode)
	require.Equal(t, "keyword", adminSvc.lastListAccounts.search)
	require.Equal(t, "priority", adminSvc.lastListAccounts.sortBy)
	require.Equal(t, "desc", adminSvc.lastListAccounts.sortOrder)
}

func TestExportDataSelectedIDsOverrideFilters(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/admin/accounts/data?ids=1,2&platform=openai&search=keyword&sort_by=priority&sort_order=desc",
		nil,
	)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp dataResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Len(t, resp.Data.Accounts, 2)
	require.Equal(t, 0, adminSvc.lastListAccounts.calls)
}

func TestImportDataReusesProxyAndSkipsDefaultGroup(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()

	adminSvc.proxies = []service.Proxy{
		{
			ID:       1,
			Name:     "proxy",
			Protocol: "socks5",
			Host:     "1.2.3.4",
			Port:     1080,
			Username: "u",
			Password: "p",
			Status:   service.StatusActive,
		},
	}

	dataPayload := map[string]any{
		"data": map[string]any{
			"type":    dataType,
			"version": dataVersion,
			"proxies": []map[string]any{
				{
					"proxy_key": "socks5|1.2.3.4|1080|u|p",
					"name":      "proxy",
					"protocol":  "socks5",
					"host":      "1.2.3.4",
					"port":      1080,
					"username":  "u",
					"password":  "p",
					"status":    "active",
				},
			},
			"accounts": []map[string]any{
				{
					"name":        "acc",
					"platform":    service.PlatformOpenAI,
					"type":        service.AccountTypeOAuth,
					"credentials": map[string]any{"token": "x"},
					"proxy_key":   "socks5|1.2.3.4|1080|u|p",
					"concurrency": 3,
					"priority":    50,
				},
			},
		},
		"skip_default_group_bind": true,
	}

	body, _ := json.Marshal(dataPayload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Len(t, adminSvc.createdProxies, 0)
	require.Len(t, adminSvc.createdAccounts, 1)
	require.True(t, adminSvc.createdAccounts[0].SkipDefaultGroupBind)
}

func TestImportDataAppliesAccountOptions(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()

	bodyPayload := map[string]any{
		"data": map[string]any{
			"type":    dataType,
			"version": dataVersion,
			"proxies": []map[string]any{},
			"accounts": []map[string]any{{
				"name":        "ye-account",
				"platform":    service.PlatformOpenAI,
				"type":        service.AccountTypeOAuth,
				"credentials": map[string]any{"access_token": "token"},
				"concurrency": 3,
				"priority":    50,
			}},
		},
		"skip_default_group_bind": true,
		"account_options": map[string]any{
			"group_ids":                      []int64{11, 12},
			"concurrency":                    8,
			"priority":                       7,
			"proxy_mode":                     "random",
			"codex_fingerprint_mode":         "session",
			"enable_account_guard":           true,
			"account_guard_interval_minutes": 45,
			"openai_ws_mode":                 "ctx_pool",
		},
	}
	body, err := json.Marshal(bodyPayload)
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, adminSvc.createdAccounts, 1)
	created := adminSvc.createdAccounts[0]
	require.Equal(t, []int64{11, 12}, created.GroupIDs)
	require.Equal(t, 8, created.Concurrency)
	require.Equal(t, 7, created.Priority)
	require.Equal(t, "ctx_pool", created.Extra["openai_oauth_responses_websockets_v2_mode"])
	require.Len(t, adminSvc.createdAccountDefaults, 1)
	require.Equal(t, service.AdminAPIKeyProxyModeRandom, adminSvc.createdAccountDefaults[0].ProxyMode)
	require.Equal(t, service.AdminAPIKeyCodexFingerprintSession, adminSvc.createdAccountDefaults[0].CodexFingerprintMode)
	require.True(t, adminSvc.createdAccountDefaults[0].EnableAccountGuard)
	require.Equal(t, 45, adminSvc.createdAccountDefaults[0].AccountGuardIntervalMinutes)
}

func TestImportDataRejectsKiroAccountManagerJSON(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()

	dataPayload := map[string]any{
		"data": []map[string]any{
			{
				"id":           "source-id-1",
				"email":        "builder@example.com",
				"label":        "Kiro BuilderId 账号",
				"status":       "inactive",
				"addedAt":      "2026/06/15 13:59:19",
				"accessToken":  "access-token",
				"refreshToken": "refresh-token",
				"provider":     "BuilderId",
				"userId":       "d-user-id",
				"authMethod":   "IdC",
				"clientId":     "client-id",
				"clientSecret": "client-secret",
				"region":       "us-east-1",
				"clientIdHash": "client-id-hash",
				"profileArn":   "arn:aws:codewhisperer:us-east-1:123456789012:profile/test",
				"machineId":    "2582956e-cc88-4669-b546-07adbffcb894",
				"enabled":      false,
			},
		},
		"skip_default_group_bind": true,
	}

	body, _ := json.Marshal(dataPayload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "unsupported data format")
	require.Len(t, adminSvc.createdAccounts, 0)
}

func TestReimportCredentialsReplacesOnlyCredentialObject(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()
	adminSvc.getAccountResult = &service.Account{
		ID:          21,
		Name:        "existing",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "old", "refresh_token": "old-refresh"},
		Extra:       map[string]any{"privacy_mode": "private"},
		Status:      service.StatusDisabled,
		Concurrency: 9,
		Priority:    42,
	}

	bodyPayload := map[string]any{
		"data": map[string]any{
			"type":    dataType,
			"version": dataVersion,
			"proxies": []map[string]any{},
			"accounts": []map[string]any{{
				"name":        "new-export-name",
				"platform":    service.PlatformOpenAI,
				"type":        service.AccountTypeOAuth,
				"credentials": map[string]any{"access_token": "new", "refresh_token": "new-refresh", "password": "discard"},
			}},
		},
	}
	body, err := json.Marshal(bodyPayload)
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/21/reimport-credentials", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.POST("/api/v1/admin/accounts/:id/reimport-credentials", NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil).ReimportCredentials)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, adminSvc.lastUpdateAccountInput)
	require.True(t, adminSvc.lastUpdateAccountInput.ReplaceCredentials)
	require.Empty(t, adminSvc.lastUpdateAccountInput.Type)
	require.Nil(t, adminSvc.lastUpdateAccountInput.Extra)
	require.Equal(t, "new", adminSvc.lastUpdateAccountInput.Credentials["access_token"])
	require.Equal(t, "new-refresh", adminSvc.lastUpdateAccountInput.Credentials["refresh_token"])
	require.NotContains(t, adminSvc.lastUpdateAccountInput.Credentials, "password")
}

func TestReimportCredentialsRejectsIdentityMismatch(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()
	adminSvc.getAccountResult = &service.Account{ID: 21, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth}
	bodyPayload := map[string]any{
		"data": map[string]any{
			"proxies":  []map[string]any{},
			"accounts": []map[string]any{{"name": "other", "platform": service.PlatformAnthropic, "type": service.AccountTypeOAuth, "credentials": map[string]any{"access_token": "x"}}},
		},
	}
	body, err := json.Marshal(bodyPayload)
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/21/reimport-credentials", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.POST("/api/v1/admin/accounts/:id/reimport-credentials", NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil).ReimportCredentials)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "ACCOUNT_IDENTITY_MISMATCH")
	require.Nil(t, adminSvc.lastUpdateAccountInput)
}
