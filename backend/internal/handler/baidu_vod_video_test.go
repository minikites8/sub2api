package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

func TestBaiduVODAccountSelectionFailureKeepsRoutingDetailsInternal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	selectionErr := errors.New("no available accounts for Baidu VOD V2 AK/SK model: veo-3.1-lite")

	(&OpenAIGatewayHandler{}).failBaiduVODAccountSelection(c, zap.NewNop(), "veo-3.1-lite", selectionErr)

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Equal(t, "server_error", gjson.Get(recorder.Body.String(), "error.type").String())
	require.Equal(t, baiduVODPublicUnavailableMessage, gjson.Get(recorder.Body.String(), "error.message").String())
	require.NotContains(t, recorder.Body.String(), "Baidu VOD")
	require.NotContains(t, recorder.Body.String(), "V2")
	require.NotContains(t, recorder.Body.String(), "AK/SK")
	require.NotContains(t, recorder.Body.String(), "accounts")
	require.True(t, isOpsRoutingCapacityLimited(c))
	detail, ok := c.Get(service.OpsUpstreamErrorDetailKey)
	require.True(t, ok)
	require.Equal(t, selectionErr.Error(), detail)
}

func TestBaiduVODSubmissionFailureKeepsUpstreamDetailsInternal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	upstreamErr := &service.BaiduVODUpstreamError{
		StatusCode: http.StatusUnauthorized,
		Code:       "INVALID_ACCESS_KEY",
		Message:    "upstream account credential rejected",
	}
	h := &OpenAIGatewayHandler{baiduVODVideoService: &service.BaiduVODVideoService{}}

	h.failBaiduVODSubmission(c, zap.NewNop(), &service.BaiduVODVideoTask{TaskID: "video_public_id"}, upstreamErr)

	body := recorder.Body.String()
	require.Equal(t, http.StatusBadGateway, recorder.Code)
	require.Equal(t, baiduVODPublicUnavailableMessage, gjson.Get(body, "error.message").String())
	require.NotContains(t, body, upstreamErr.Code)
	require.NotContains(t, body, upstreamErr.Message)
	status, ok := c.Get(service.OpsUpstreamStatusCodeKey)
	require.True(t, ok)
	require.Equal(t, http.StatusUnauthorized, status)
	detail, ok := c.Get(service.OpsUpstreamErrorDetailKey)
	require.True(t, ok)
	require.Equal(t, upstreamErr.Error(), detail)
}

func TestWriteBaiduVODVideoHidesInternalProviderAndFailureDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	internalCode := "VEO_ACCOUNT_AUTH_FAILED"
	internalMessage := "Baidu VOD V2 AK/SK credential rejected"
	task := &service.BaiduVODVideoTask{
		TaskID:            "video_public_id",
		Model:             "veo-3.1-lite",
		Provider:          service.BaiduVODProviderVeo,
		Status:            service.BaiduVODTaskStatusFailed,
		Resolution:        "720P",
		Ratio:             "16:9",
		RequestedDuration: 5,
		CreatedAt:         time.Unix(1_800_000_000, 0),
		LastErrorCode:     &internalCode,
		LastErrorMessage:  &internalMessage,
	}

	(&OpenAIGatewayHandler{}).writeBaiduVODVideo(c, task)

	body := recorder.Body.String()
	require.Equal(t, http.StatusOK, recorder.Code)
	require.False(t, gjson.Get(body, "provider").Exists())
	require.Equal(t, baiduVODPublicFailureCode, gjson.Get(body, "error.code").String())
	require.Equal(t, baiduVODPublicFailureMessage, gjson.Get(body, "error.message").String())
	require.NotContains(t, body, internalCode)
	require.NotContains(t, body, internalMessage)
	require.NotContains(t, body, "Baidu VOD")
	require.NotContains(t, body, "AK/SK")
}
