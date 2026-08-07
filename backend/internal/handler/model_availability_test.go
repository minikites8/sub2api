package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func TestModelAvailabilityMiddlewareRecordsCompletedPost(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tracker := service.NewModelAvailabilityTracker()
	router := gin.New()
	router.Use(ModelAvailabilityMiddleware(tracker))
	router.POST("/invoke", func(c *gin.Context) {
		setOpsRequestContext(c, "gpt-image-2", false)
		c.Status(http.StatusOK)
	})

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/invoke", nil))
	observation, ok := tracker.Snapshot("gpt-image-2")
	if !ok || observation.TotalCalls != 1 || observation.SuccessfulCalls != 1 {
		t.Fatalf("observation = %#v, ok=%v", observation, ok)
	}
}

func TestModelAvailabilityMiddlewareRecordsFailureAndSkipsGet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tracker := service.NewModelAvailabilityTracker()
	router := gin.New()
	router.Use(ModelAvailabilityMiddleware(tracker))
	handler := func(c *gin.Context) {
		setOpsRequestContext(c, "veo-3", false)
		c.Status(http.StatusBadGateway)
	}
	router.POST("/invoke", handler)
	router.GET("/invoke", handler)

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/invoke", nil))
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/invoke", nil))
	observation, ok := tracker.Snapshot("veo-3")
	if !ok || observation.TotalCalls != 1 || observation.SuccessfulCalls != 0 {
		t.Fatalf("observation = %#v, ok=%v", observation, ok)
	}
}

func TestModelAvailabilityMiddlewareCountsInBandStreamFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tracker := service.NewModelAvailabilityTracker()
	router := gin.New()
	router.Use(ModelAvailabilityMiddleware(tracker))
	router.POST("/invoke", func(c *gin.Context) {
		setOpsRequestContext(c, "grok-imagine-video-1.5", true)
		service.MarkOpsStreamError(c, "upstream_error", "upstream failed", http.StatusBadGateway)
		c.Status(http.StatusOK)
	})

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/invoke", nil))
	observation, ok := tracker.Snapshot("grok-imagine-video-1.5")
	if !ok || observation.TotalCalls != 1 || observation.SuccessfulCalls != 0 {
		t.Fatalf("observation = %#v, ok=%v", observation, ok)
	}
}
