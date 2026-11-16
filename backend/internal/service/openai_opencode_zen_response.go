package service

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/tidwall/gjson"
)

// Zen free requests use SSE upstream even when the downstream caller expects
// JSON. Aggregate all choices, reasoning, tool-call fragments and usage without
// reducing the request to the last user message or dropping raw metadata.
func aggregateOpenCodeZenChatSSE(body []byte) ([]byte, error) {
	type choiceState struct {
		fields  map[string]json.RawMessage
		message map[string]json.RawMessage
		tools   map[int]map[string]json.RawMessage
	}
	root := make(map[string]json.RawMessage)
	choices := make(map[int]*choiceState)
	done := false
	var streamErr error
	consume := func(eventType string, data []byte) {
		if streamErr != nil || done {
			return
		}
		if string(data) == "[DONE]" {
			done = true
			return
		}
		if eventType == "error" || gjson.GetBytes(data, "error").Exists() {
			streamErr = fmt.Errorf("OpenCode Zen stream error: %s", extractUpstreamErrorMessage(data))
			return
		}
		var chunk map[string]json.RawMessage
		if err := json.Unmarshal(data, &chunk); err != nil {
			streamErr = fmt.Errorf("decode OpenCode Zen stream: %w", err)
			return
		}
		for key, value := range chunk {
			if key != "choices" && key != "object" && string(value) != "null" {
				root[key] = value
			}
		}
		for _, rawChoice := range gjson.GetBytes(data, "choices").Array() {
			index := int(rawChoice.Get("index").Int())
			state := choices[index]
			if state == nil {
				state = &choiceState{
					fields:  make(map[string]json.RawMessage),
					message: map[string]json.RawMessage{"role": json.RawMessage(`"assistant"`), "content": json.RawMessage(`null`)},
					tools:   make(map[int]map[string]json.RawMessage),
				}
				choices[index] = state
			}
			var fields map[string]json.RawMessage
			_ = json.Unmarshal([]byte(rawChoice.Raw), &fields)
			for key, value := range fields {
				if key != "delta" && string(value) != "null" {
					state.fields[key] = mergeOpenCodeZenDelta(state.fields[key], value, key)
				}
			}
			var delta map[string]json.RawMessage
			_ = json.Unmarshal(fields["delta"], &delta)
			for key, value := range delta {
				if key != "tool_calls" {
					state.message[key] = mergeOpenCodeZenDelta(state.message[key], value, key)
				}
			}
			for _, tool := range rawChoice.Get("delta.tool_calls").Array() {
				toolIndex := int(tool.Get("index").Int())
				if state.tools[toolIndex] == nil {
					state.tools[toolIndex] = make(map[string]json.RawMessage)
				}
				var parts map[string]json.RawMessage
				_ = json.Unmarshal([]byte(tool.Raw), &parts)
				for key, value := range parts {
					if key != "index" {
						state.tools[toolIndex][key] = mergeOpenCodeZenDelta(state.tools[toolIndex][key], value, key)
					}
				}
			}
		}
	}
	var parser openAICompatSSEFrameParser
	emit := func(frame openAICompatSSEFrame, ok bool) {
		if ok {
			consume(frame.EventType, []byte(strings.TrimSpace(frame.Data)))
		}
	}
	for _, line := range strings.Split(string(body), "\n") {
		emit(parser.AddLine(strings.TrimRight(line, "\r")))
	}
	emit(parser.Finish())
	if streamErr != nil {
		return nil, streamErr
	}
	if !done || len(choices) == 0 {
		return nil, fmt.Errorf("OpenCode Zen chat stream ended before completion")
	}
	indexes := make([]int, 0, len(choices))
	for index := range choices {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	output := make([]map[string]json.RawMessage, 0, len(indexes))
	for _, index := range indexes {
		state := choices[index]
		if len(state.fields["finish_reason"]) == 0 {
			return nil, fmt.Errorf("OpenCode Zen chat choice %d has no finish reason", index)
		}
		if len(state.tools) > 0 {
			toolIndexes := make([]int, 0, len(state.tools))
			for toolIndex := range state.tools {
				toolIndexes = append(toolIndexes, toolIndex)
			}
			sort.Ints(toolIndexes)
			tools := make([]map[string]json.RawMessage, 0, len(toolIndexes))
			for _, toolIndex := range toolIndexes {
				tools = append(tools, state.tools[toolIndex])
			}
			state.message["tool_calls"], _ = json.Marshal(tools)
		}
		state.fields["message"], _ = json.Marshal(state.message)
		output = append(output, state.fields)
	}
	root["choices"], _ = json.Marshal(output)
	root["object"] = json.RawMessage(`"chat.completion"`)
	return json.Marshal(root)
}

func mergeOpenCodeZenDelta(previous, next json.RawMessage, field string) json.RawMessage {
	if len(next) == 0 || string(next) == "null" {
		return previous
	}
	if len(previous) == 0 || string(previous) == "null" || field == "id" || field == "role" || field == "type" || field == "finish_reason" {
		return next
	}
	var left, right string
	if json.Unmarshal(previous, &left) == nil && json.Unmarshal(next, &right) == nil {
		result, _ := json.Marshal(left + right)
		return result
	}
	var leftObject, rightObject map[string]json.RawMessage
	if json.Unmarshal(previous, &leftObject) == nil && json.Unmarshal(next, &rightObject) == nil && leftObject != nil && rightObject != nil {
		for key, value := range rightObject {
			leftObject[key] = mergeOpenCodeZenDelta(leftObject[key], value, key)
		}
		result, _ := json.Marshal(leftObject)
		return result
	}
	var leftArray, rightArray []json.RawMessage
	if json.Unmarshal(previous, &leftArray) == nil && json.Unmarshal(next, &rightArray) == nil {
		result, _ := json.Marshal(append(leftArray, rightArray...))
		return result
	}
	return next
}
