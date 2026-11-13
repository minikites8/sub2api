package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/requestretry"
	"github.com/gin-gonic/gin"
)

func requestPrefersAccountRetry(c *gin.Context) bool {
	return c != nil && requestretry.Enabled(c.Request)
}

func requestAccountSwitchLimit(c *gin.Context, serverLimit int) int {
	// The initial account counts toward the request's five-account budget.
	// Same-account retries do not consume another account, and client limits
	// may still reduce the budget further.
	const maxAccountSwitches = 4
	if serverLimit > maxAccountSwitches {
		serverLimit = maxAccountSwitches
	}
	if c == nil {
		return serverLimit
	}
	return requestretry.Limit(c.Request, serverLimit)
}

func newRequestFailoverState(c *gin.Context, serverLimit int, hasBoundSession bool) *FailoverState {
	state := NewFailoverState(requestAccountSwitchLimit(c, serverLimit), hasBoundSession)
	state.preferNextAccount = requestPrefersAccountRetry(c)
	return state
}
