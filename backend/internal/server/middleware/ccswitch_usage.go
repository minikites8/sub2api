package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func isCCSwitchUsageRequest(c *gin.Context) bool {
	return c.Request != nil && c.Request.URL != nil && c.Request.URL.Path == "/api/usage" &&
		(c.Request.Method == http.MethodGet || c.Request.Method == http.MethodPost)
}
