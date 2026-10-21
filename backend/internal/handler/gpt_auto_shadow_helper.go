package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// observeGPTAutoShadow hands a request to the GPT-Auto Phase 1 shadow
// observer. The observer only reads body, never retains it and never blocks;
// the request continues unchanged regardless of what happens here.
//
// Call sites run after auth, body parsing and model resolution and before any
// slot acquisition, account selection or upstream call, so the observed body
// is exactly what the customer sent (before prompt-rule injection rewrites
// it into a new buffer).
func observeGPTAutoShadow(c *gin.Context, shadow *service.GPTAutoShadowService, apiKey *service.APIKey, endpoint string, body []byte) {
	if shadow == nil || c == nil || c.Request == nil || apiKey == nil {
		return
	}
	ctx := c.Request.Context()
	// RequestID is left empty: Observe derives the usage-log request id from
	// ctx only for sampled requests, keeping the disabled path allocation-free.
	input := service.GPTAutoObserveInput{
		Endpoint: endpoint,
		Body:     body,
		APIKeyID: apiKey.ID,
		GroupID:  apiKey.GroupID,
	}
	if apiKey.User != nil {
		input.UserID = apiKey.User.ID
	} else {
		input.UserID = apiKey.UserID
	}
	shadow.Observe(ctx, input)
}
