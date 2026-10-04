package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Prism's browser transport supplies terminal text without measured usage.
// Count the client-visible request and terminal output with the same local
// tokenizer used by responses/input_tokens and label the billing estimate.
func prismBrowserUsage(request, response []byte, model string, stream bool) ([]byte, OpenAIUsage, string, error) {
	terminal := response
	if stream {
		for _, line := range bytes.Split(response, []byte("\n")) {
			if bytes.HasPrefix(line, []byte("data:")) {
				data := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
				if gjson.GetBytes(data, "type").String() == "response.completed" {
					terminal = []byte(gjson.GetBytes(data, "response").Raw)
				}
			}
		}
	}
	usage, measured := prismBrowserMeasuredUsage(gjson.GetBytes(terminal, "usage"))
	source := "upstream"
	if !measured {
		source = "estimated"
		var err error
		usage.InputTokens, err = prismBrowserCountInput(request, model)
		if err != nil {
			return nil, OpenAIUsage{}, "", err
		}
		codec, err := openAIInputTokensCodecForModel(model)
		if err != nil {
			return nil, OpenAIUsage{}, "", err
		}
		for _, item := range gjson.GetBytes(terminal, "output").Array() {
			texts := []string{item.Get("name").String(), item.Get("arguments").String(), item.Get("input").String()}
			for _, part := range item.Get("content").Array() {
				texts = append(texts, part.Get("text").String())
			}
			for _, text := range texts {
				n, err := codec.Count(text)
				if err != nil {
					return nil, OpenAIUsage{}, "", err
				}
				usage.OutputTokens += n
			}
		}
	}
	patch := func(raw []byte, prefix string, completed bool) ([]byte, error) {
		var err error
		if completed && !measured {
			raw, err = sjson.SetBytes(raw, prefix+"usage", map[string]any{
				"input_tokens": usage.InputTokens, "output_tokens": usage.OutputTokens,
				"total_tokens":          usage.InputTokens + usage.OutputTokens,
				"input_tokens_details":  map[string]int{"cached_tokens": 0},
				"output_tokens_details": map[string]int{"reasoning_tokens": 0},
			})
			if err != nil {
				return nil, err
			}
		}
		return sjson.SetBytes(raw, prefix+"metadata.prism_usage_source", source)
	}
	if !stream {
		raw, err := patch(response, "", true)
		return raw, usage, source, err
	}
	lines := bytes.Split(response, []byte("\n"))
	for index, line := range lines {
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		data := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		kind := gjson.GetBytes(data, "type").String()
		if kind != "response.completed" && kind != "response.created" && kind != "response.in_progress" {
			continue
		}
		if !gjson.GetBytes(data, "response").IsObject() {
			continue
		}
		raw, err := patch(data, "response.", kind == "response.completed")
		if err != nil {
			return nil, OpenAIUsage{}, "", err
		}
		lines[index] = append([]byte("data: "), raw...)
	}
	return bytes.Join(lines, []byte("\n")), usage, source, nil
}

func prismBrowserMeasuredUsage(raw gjson.Result) (OpenAIUsage, bool) {
	count := func(value gjson.Result) (int, bool) {
		if value.Type != gjson.Number || value.Float() < 0 || value.Float() >= float64(math.MaxInt) || value.Float() != float64(value.Int()) {
			return 0, false
		}
		return int(value.Int()), true
	}
	input, inputOK := count(raw.Get("input_tokens"))
	output, outputOK := count(raw.Get("output_tokens"))
	if !raw.IsObject() || !inputOK || !outputOK || input > math.MaxInt-output {
		return OpenAIUsage{}, false
	}
	cached := 0
	if value := raw.Get("input_tokens_details.cached_tokens"); value.Exists() {
		var ok bool
		cached, ok = count(value)
		if !ok || cached > input {
			return OpenAIUsage{}, false
		}
	}
	return OpenAIUsage{InputTokens: input, OutputTokens: output, CacheReadInputTokens: cached}, true
}

func prismBrowserCountInput(body []byte, model string) (int, error) {
	codec, err := openAIInputTokensCodecForModel(model)
	if err != nil {
		return 0, err
	}
	total, err := estimateOpenAIInputTokensForInput(codec, json.RawMessage(gjson.GetBytes(body, "input").Raw))
	if err != nil {
		return 0, fmt.Errorf("count Prism input: %w", err)
	}
	// Keep custom tool grammar and namespace declarations intact: the typed
	// input_tokens request has fewer tool fields than this bridge accepts.
	for _, key := range []string{"instructions", "tools", "additional_tools", "tool_choice"} {
		value := gjson.GetBytes(body, key)
		text := prismBrowserUsageText(value)
		n, err := codec.Count(text)
		if err != nil {
			return 0, err
		}
		total += n
	}
	for _, item := range gjson.GetBytes(body, "input").Array() {
		texts := []string{}
		if item.Get("type").String() == "custom_tool_call" {
			texts = append(texts, item.Get("input").String())
		}
		for _, part := range item.Get("summary").Array() {
			texts = append(texts, part.Get("text").String())
		}
		if item.Get("type").String() == "additional_tools" {
			texts = append(texts, prismBrowserUsageText(item.Get("tools")))
		}
		for _, text := range texts {
			n, err := codec.Count(text)
			if err != nil {
				return 0, err
			}
			total += n
		}
	}
	return total, nil
}

func prismBrowserUsageText(value gjson.Result) string {
	if value.Type == gjson.String {
		return value.String()
	}
	if value.IsObject() || value.IsArray() {
		// Cached requests compare parsed semantic objects. Canonicalize these
		// fields so whitespace and key order also preserve the billed counters.
		var parsed any
		if json.Unmarshal([]byte(value.Raw), &parsed) == nil {
			if raw, err := json.Marshal(parsed); err == nil {
				return string(raw)
			}
		}
	}
	return value.Raw
}
