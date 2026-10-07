package middleware

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/requestmodel"
	"github.com/gin-gonic/gin"
)

// RequestModelIntegrity makes model interpretation independent of the upstream
// parser before model permissions, composite routing and billing select a model.
func RequestModelIntegrity(maxNormalizedBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request == nil || (c.Request.Method != http.MethodPost && c.Request.Method != http.MethodPut && c.Request.Method != http.MethodPatch) {
			c.Next()
			return
		}
		body, ok := readAdmissionRequestBody(c)
		if !ok {
			return
		}
		contentType := c.GetHeader("Content-Type")
		if !strings.HasPrefix(strings.ToLower(contentType), "multipart/") {
			// Match the normalization used by text handlers before checking keys.
			normalized, err := httputil.NormalizeLenientJSONRequestBody(body, maxNormalizedBytes)
			if err != nil {
				c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": gin.H{"type": "invalid_request_error", "message": "Request body is too large"}})
				c.Abort()
				return
			}
			// Preserve the original bytes for the protocol handler.
			body = normalized
		}
		if err := requestmodel.ValidateBodyModels(contentType, body); err != nil {
			groupModelAllowlistErrorWriter(c)(c, http.StatusBadRequest, err.Error())
			c.Abort()
			return
		}
		c.Next()
	}
}
