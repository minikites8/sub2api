package requestmodel

import (
	"errors"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// OpenAIFastBase interprets the suffix only on OpenAI text model families.
// Other providers have literal model names ending in -fast (e.g. Grok).
func OpenAIFastBase(model string) (string, bool) {
	base, ok := strings.CutSuffix(model, "-fast")
	if !ok || strings.HasSuffix(base, "-fast") {
		return model, false
	}
	if (strings.HasPrefix(base, "gpt-") && len(base) > 4) || base == "codex-mini-latest" || (len(base) >= 2 && base[0] == 'o' && base[1] >= '0' && base[1] <= '9' && (len(base) == 2 || base[2] == '-')) {
		return base, true
	}
	return model, false
}

// ExpandOpenAIFast translates a client shorthand before admission and routing.
// No-op requests retain their original bytes. An explicit tier is authoritative.
func ExpandOpenAIFast(body []byte, ws bool) ([]byte, bool, error) {
	model := gjson.GetBytes(body, "model")
	if model.Type != gjson.String {
		return body, false, nil
	}
	base, ok := OpenAIFastBase(model.String())
	if !ok {
		return body, false, nil
	}
	if ws && gjson.GetBytes(body, "type").String() != "response.create" {
		return body, false, nil
	}
	if !gjson.ValidBytes(body) {
		return body, false, errors.New("invalid JSON request")
	}
	if err := ValidateJSONModels(body); err != nil {
		return body, false, err
	}
	// Reject ambiguous tier fields instead of letting a later JSON decoder choose.
	found, ambiguous := false, false
	gjson.ParseBytes(body).ForEach(func(k, v gjson.Result) bool {
		if strings.EqualFold(k.String(), "service_tier") {
			ambiguous = found || k.String() != "service_tier"
			found = true
		}
		return !ambiguous
	})
	if ambiguous {
		return body, false, errors.New("service_tier must be unique and use its canonical name")
	}
	next, err := sjson.SetBytes(body, "model", base)
	if err == nil && !found {
		next, err = sjson.SetBytes(next, "service_tier", "priority")
	}
	if err != nil {
		return body, false, err
	}
	return next, true, nil
}
