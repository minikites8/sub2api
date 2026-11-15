package service

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/tidwall/gjson"
)

// Account header overrides and harvested credentials can enable Lite after
// ingress normalization. Enforce its parallel-call contract using the final
// outbound headers, without changing the source request used for failover.
func normalizeOpenAIResponsesLiteEgressRequest(req *http.Request, account *Account) error {
	if req == nil || account == nil || !account.IsOpenAI() || !isOpenAIResponsesLiteHeader(getHeaderRaw(req.Header, responsesLiteHeaderKey)) {
		return nil
	}
	body, err := snapshotHTTPRequestBody(req)
	if err != nil {
		return fmt.Errorf("read outbound responses Lite request: %w", err)
	}
	normalized, changed, err := normalizeOpenAIResponsesLiteParallelToolCallsPayload(body)
	if err != nil {
		return err
	}
	if changed || req.GetBody == nil {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		replaceHTTPRequestJSONBody(req, normalized)
	}
	return nil
}

func normalizeOpenAIResponsesLiteWebSocketEgress(body []byte, headers http.Header) ([]byte, error) {
	if !isOpenAIResponsesLiteHeader(getHeaderRaw(headers, responsesLiteHeaderKey)) && !isOpenAIResponsesLiteWebSocketPayload(body) {
		return body, nil
	}
	if eventType := gjson.GetBytes(body, "type").String(); eventType != "" && eventType != "response.create" {
		return body, nil
	}
	normalized, _, err := normalizeOpenAIResponsesLiteParallelToolCallsPayload(body)
	return normalized, err
}

func normalizeOpenAIResponsesLiteWebSocketEgressValue(value any, headers http.Header) (any, error) {
	if !isOpenAIResponsesLiteHeader(getHeaderRaw(headers, responsesLiteHeaderKey)) {
		return value, nil
	}
	var body []byte
	var err error
	switch payload := value.(type) {
	case openAIWSPreparedJSON:
		body = payload
	case json.RawMessage:
		body = payload
	default:
		body, err = json.Marshal(value)
	}
	if err != nil {
		return nil, err
	}
	normalized, err := normalizeOpenAIResponsesLiteWebSocketEgress(body, headers)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(normalized), nil
}
