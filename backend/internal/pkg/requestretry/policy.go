// Package requestretry defines the client's opt-in account failover policy.
package requestretry

import (
	"net/http"
	"strconv"
	"strings"
)

const (
	AccountHeader    = "X-Sub2API-Retry-Account"
	MaxRetriesHeader = "X-Sub2API-Max-Retries"
)

// Enabled deliberately accepts only a single true value. Missing, false,
// malformed and conflicting values preserve the server's existing policy.
func Enabled(r *http.Request) bool {
	if r == nil {
		return false
	}
	values := r.Header.Values(AccountHeader)
	return len(values) == 1 && strings.EqualFold(strings.TrimSpace(values[0]), "true")
}

// Limit counts account switches after the initial attempt. A client may lower
// the server's budget, including to zero, but cannot raise its configured cap.
// Invalid values and a limit without the opt-in header retain the default.
func Limit(r *http.Request, serverLimit int) int {
	if !Enabled(r) {
		return serverLimit
	}
	values := r.Header.Values(MaxRetriesHeader)
	if len(values) != 1 {
		return serverLimit
	}
	value := strings.TrimSpace(values[0])
	if value == "" || strings.IndexFunc(value, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return serverLimit
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit >= serverLimit {
		return serverLimit
	}
	return limit
}
