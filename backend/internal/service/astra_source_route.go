package service

import (
	"context"
	"net/http"
	"strings"
	"time"
)

type astraSourceCachedRouteKey struct{}

type astraSourceCachedRoute struct {
	accountID int64
	value     string
	expires   time.Time
}

// WithAstraSourceCachedRoute carries the original expiry of a trusted ticket's
// route in process-local context. It contains only the shared routing Cookie.
func WithAstraSourceCachedRoute(ctx context.Context, accountID int64, value string, expires time.Time) context.Context {
	return context.WithValue(ctx, astraSourceCachedRouteKey{}, astraSourceCachedRoute{accountID, value, expires})
}

// AstraSourceCachedRouteFromRequest requires the controlled source-acquisition
// scope, the same account, a live cache entry, and the Cookie actually sent.
func AstraSourceCachedRouteFromRequest(req *http.Request, accountID int64) (string, time.Time) {
	if req == nil || !IsAstraSourceAcquisition(req.Context()) {
		return "", time.Time{}
	}
	seed, ok := req.Context().Value(astraSourceCachedRouteKey{}).(astraSourceCachedRoute)
	if !ok || accountID <= 0 || seed.accountID != accountID || seed.value == "" || !time.Now().Before(seed.expires) {
		return "", time.Time{}
	}
	count := 0
	for _, cookie := range req.Cookies() {
		if cookie.Name == "__oailb" {
			if cookie.Value != seed.value {
				return "", time.Time{}
			}
			count++
		}
	}
	if count != 1 {
		return "", time.Time{}
	}
	return seed.value, seed.expires
}

func withAstraSourceTicketRoute(req *http.Request, account *Account, ticket *openAICodexTicket) *http.Request {
	if req == nil || account == nil || ticket == nil || !IsAstraSourceAcquisition(req.Context()) ||
		ticket.AccountID != account.ID || ticket.Model != "gpt-6-astra" || !codexTicketCookiesFresh(ticket, time.Now()) {
		return req
	}
	for _, pair := range ticket.HarvestCookies {
		name, value, ok := strings.Cut(pair, "=")
		if ok && name == "__oailb" && value != "" {
			return req.WithContext(WithAstraSourceCachedRoute(req.Context(), account.ID, value, codexTicketCookiesExpiry(ticket)))
		}
	}
	return req
}
