package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type baiduVODAccountTestRepo struct {
	AccountRepository
	account *Account
}

func (r *baiduVODAccountTestRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	if r.account != nil && r.account.ID == id {
		return r.account, nil
	}
	return nil, context.Canceled
}

func runBaiduVODAccountTest(t *testing.T, account *Account, response *http.Response) (*httpUpstreamRecorder, *httptest.ResponseRecorder, error) {
	return runBaiduVODAccountTestModel(t, account, "", response)
}

func runBaiduVODAccountTestModel(t *testing.T, account *Account, model string, response *http.Response) (*httpUpstreamRecorder, *httptest.ResponseRecorder, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{resp: response}
	service := &AccountTestService{
		accountRepo:  &baiduVODAccountTestRepo{account: account},
		httpUpstream: upstream,
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/1/test", nil)
	err := service.TestAccountConnection(c, account.ID, model, "", AccountTestModeDefault)
	return upstream, recorder, err
}

func TestAccountTestServiceBaiduVODSeedanceUsesSeedanceProbe(t *testing.T) {
	account := &Account{ID: 4, Platform: PlatformBaiduVOD, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{
		"auth_mode": BaiduVODAuthModeAPIKey,
		"api_key":   "seedance-key",
	}}
	upstream, recorder, err := runBaiduVODAccountTestModel(t, account, "doubao-seedance-2-0-260128", &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(strings.NewReader(`{"error":{"code":"NotFound","message":"task does not exist"}}`)),
	})

	require.NoError(t, err)
	require.Equal(t, "https://vod.bj.baidubce.com/v3/aigc/seedance"+BaiduVODSeedanceTaskPath+baiduVODConnectivityTaskID, upstream.lastReq.URL.String())
	require.Contains(t, recorder.Body.String(), `"type":"test_complete"`)
}

func TestAccountTestServiceBaiduVODVeoUsesDirectProbe(t *testing.T) {
	account := &Account{ID: 5, Platform: PlatformBaiduVOD, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{
		"auth_mode": BaiduVODAuthModeAKSK, "access_key_id": "veo-ak", "secret_access_key": "veo-sk",
	}}
	upstream, recorder, err := runBaiduVODAccountTestModel(t, account, "veo-3.1", &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(strings.NewReader(`{"code":"MediaTaskNotFound","message":"task does not exist"}`)),
	})

	require.NoError(t, err)
	require.Equal(t, "https://vod.bj.baidubce.com"+BaiduVODVeoTaskPath+baiduVODConnectivityTaskID, upstream.lastReq.URL.String())
	require.Regexp(t, `^bce-auth-v1/veo-ak/`, upstream.lastReq.Header.Get("Authorization"))
	require.Contains(t, recorder.Body.String(), `"type":"test_complete"`)
}

func TestAccountTestServiceBaiduVODAPIKeyAcceptsMissingProbeTask(t *testing.T) {
	account := &Account{ID: 1, Platform: PlatformBaiduVOD, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{
		"auth_mode": BaiduVODAuthModeAPIKey,
		"api_key":   "vod-api-key",
		"base_url":  BaiduVODDefaultBaseURL,
	}}
	upstream, recorder, err := runBaiduVODAccountTest(t, account, &http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       io.NopCloser(strings.NewReader(`{"code":"InvalidParameter","message":"task does not exist"}`)),
	})

	require.NoError(t, err)
	require.Equal(t, "https://vod.bj.baidubce.com/v3/aigc/bailian/api/v1/tasks/sub2api-connectivity-check", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer vod-api-key", upstream.lastReq.Header.Get("Authorization"))
	require.Contains(t, recorder.Body.String(), `"type":"test_complete"`)
}

func TestAccountTestServiceBaiduVODAKSKSignsProbe(t *testing.T) {
	account := &Account{ID: 2, Platform: PlatformBaiduVOD, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{
		"auth_mode":         BaiduVODAuthModeAKSK,
		"access_key_id":     "vod-ak",
		"secret_access_key": "vod-sk",
	}}
	upstream, _, err := runBaiduVODAccountTest(t, account, &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(strings.NewReader(`{"code":"TaskNotFound","message":"missing"}`)),
	})

	require.NoError(t, err)
	require.Equal(t, "https://vod.bj.baidubce.com/v2/aigc/bailian/api/v1/tasks/sub2api-connectivity-check", upstream.lastReq.URL.String())
	require.Regexp(t, `^bce-auth-v1/vod-ak/`, upstream.lastReq.Header.Get("Authorization"))
	require.NotEmpty(t, upstream.lastReq.Header.Get("x-bce-date"))
}

func TestAccountTestServiceBaiduVODRejectsAuthenticationFailure(t *testing.T) {
	account := &Account{ID: 3, Platform: PlatformBaiduVOD, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{
		"auth_mode": BaiduVODAuthModeAPIKey,
		"api_key":   "expired-key",
	}}
	_, recorder, err := runBaiduVODAccountTest(t, account, &http.Response{
		StatusCode: http.StatusForbidden,
		Body:       io.NopCloser(strings.NewReader(`{"code":"AccessDenied","message":"invalid token"}`)),
	})

	require.Error(t, err)
	require.Contains(t, recorder.Body.String(), `"type":"error"`)
	require.Contains(t, recorder.Body.String(), "authentication failed")
}
