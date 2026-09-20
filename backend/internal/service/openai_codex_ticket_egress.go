package service

import (
	"context"
	"net/netip"
	"strings"
)

type openAICodexTicketEgressObserverKey struct{}

// OpenAICodexTicketEgressError contains only allowlisted diagnostics. HTTPStatus
// belongs to the anonymous IP trace response, never the ticket response.
type OpenAICodexTicketEgressError struct {
	Reason     string `json:"reason"`
	HTTPStatus int    `json:"http_status,omitempty"`
}

type OpenAICodexTicketEgressResult struct {
	IP          string
	CountryCode string
	Error       *OpenAICodexTicketEgressError
}

// WithOpenAICodexTicketEgressObserver opts a harvest request into observing the
// public IP of its own connection. The transport calls observe before returning
// from Do. A result contains either a same-connection confirmed address or the
// specific diagnostic failure; it never contains raw upstream errors.
func WithOpenAICodexTicketEgressObserver(ctx context.Context, observe func(OpenAICodexTicketEgressResult)) context.Context {
	return context.WithValue(ctx, openAICodexTicketEgressObserverKey{}, observe)
}

func OpenAICodexTicketEgressObserverFromContext(ctx context.Context) func(OpenAICodexTicketEgressResult) {
	observe, _ := ctx.Value(openAICodexTicketEgressObserverKey{}).(func(OpenAICodexTicketEgressResult))
	return observe
}

func normalizeOpenAICodexTicketEgressResult(result OpenAICodexTicketEgressResult) OpenAICodexTicketEgressResult {
	if result.IP != "" {
		if ip := normalizeOpenAICodexTicketEgressIP(result.IP); ip != "" {
			return OpenAICodexTicketEgressResult{IP: ip, CountryCode: normalizeCodexTicketCountry(result.CountryCode)}
		}
		return OpenAICodexTicketEgressResult{Error: &OpenAICodexTicketEgressError{Reason: "invalid_ip"}}
	}
	diagnostic := OpenAICodexTicketEgressError{Reason: "unknown"}
	if result.Error != nil {
		switch result.Error.Reason {
		case "timeout", "canceled", "network_error", "http_error", "connection_closed", "unsupported_protocol",
			"response_too_large", "response_read_error", "missing_ip", "invalid_ip", "ambiguous_ip", "connection_changed",
			"ticket_connection_unavailable", "insufficient_time", "unsupported_transport", "request_error", "unknown":
			diagnostic.Reason = result.Error.Reason
		}
		if diagnostic.Reason == "http_error" && result.Error.HTTPStatus >= 100 && result.Error.HTTPStatus <= 599 {
			diagnostic.HTTPStatus = result.Error.HTTPStatus
		}
	}
	return OpenAICodexTicketEgressResult{Error: &diagnostic}
}

func normalizeOpenAICodexTicketEgressIP(value string) string {
	ip, err := netip.ParseAddr(value)
	if err != nil || ip.Zone() != "" {
		return ""
	}
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return ""
	}
	return ip.String()
}
func normalizeCodexTicketCountry(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if len(value) != 2 || value[0] < 'A' || value[0] > 'Z' || value[1] < 'A' || value[1] > 'Z' {
		return ""
	}
	return value
}
