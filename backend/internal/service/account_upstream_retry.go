package service

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	credKeyOpenAIUpstreamRetryAccount = "openai_upstream_retry_account"
	credKeyOpenAIUpstreamMaxRetries   = "openai_upstream_max_retries"
	openAIUpstreamRetryAccountHeader  = "X-Sub2API-Retry-Account"
	openAIUpstreamMaxRetriesHeader    = "X-Sub2API-Max-Retries"
	maxOpenAIUpstreamRetries          = math.MaxInt32
)

// NormalizeOpenAIUpstreamRetryCredentials validates both full saves and JSONB
// merge patches. Explicit null limits are retained so bulk edits can clear them.
func NormalizeOpenAIUpstreamRetryCredentials(credentials map[string]any) error {
	if value, ok := credentials[credKeyOpenAIUpstreamRetryAccount]; ok && value != nil {
		if _, valid := value.(bool); !valid {
			return infraerrors.New(http.StatusBadRequest, "INVALID_UPSTREAM_RETRY", "openai_upstream_retry_account must be a boolean")
		}
	}
	if value, ok := credentials[credKeyOpenAIUpstreamMaxRetries]; ok && value != nil {
		limit, valid := openAIUpstreamRetryLimit(value)
		if !valid {
			return infraerrors.New(http.StatusBadRequest, "INVALID_UPSTREAM_RETRY", "openai_upstream_max_retries must be an integer between 0 and 2147483647")
		}
		credentials[credKeyOpenAIUpstreamMaxRetries] = limit
	}
	return nil
}

func openAIUpstreamRetryLimit(value any) (int, bool) {
	var number float64
	switch v := value.(type) {
	case int:
		return v, v >= 0 && v <= maxOpenAIUpstreamRetries
	case int64:
		return int(v), v >= 0 && v <= maxOpenAIUpstreamRetries
	case float64:
		number = v
	case json.Number:
		var err error
		number, err = v.Float64()
		if err != nil {
			return 0, false
		}
	default:
		return 0, false
	}
	if number < 0 || number > maxOpenAIUpstreamRetries || math.IsNaN(number) || math.Trunc(number) != number {
		return 0, false
	}
	return int(number), true
}

// applyOpenAIUpstreamRetryHeaders runs after generic header overrides. The
// dedicated panel owns these two headers while enabled; existing custom header
// configurations retain their behavior when the panel is disabled or unset.
func (a *Account) applyOpenAIUpstreamRetryHeaders(headers http.Header) {
	if a == nil || headers == nil || a.Platform != PlatformOpenAI || a.Type != AccountTypeAPIKey {
		return
	}
	if enabled, _ := a.Credentials[credKeyOpenAIUpstreamRetryAccount].(bool); !enabled {
		return
	}
	for name := range headers {
		if strings.EqualFold(name, openAIUpstreamRetryAccountHeader) || strings.EqualFold(name, openAIUpstreamMaxRetriesHeader) {
			delete(headers, name)
		}
	}
	headers.Set(openAIUpstreamRetryAccountHeader, "True")
	if limit, valid := openAIUpstreamRetryLimit(a.Credentials[credKeyOpenAIUpstreamMaxRetries]); valid {
		headers.Set(openAIUpstreamMaxRetriesHeader, strconv.Itoa(limit))
	}
}
