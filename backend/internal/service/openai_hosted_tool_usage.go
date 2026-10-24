package service

import (
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
)

// Observe completed upstream items only. A declaration in tools, an added item
// or a function named web_search is not a provider-hosted search charge.
func observeOpenAIHostedTools(body []byte, usage *OpenAIUsage) {
	if usage == nil || !gjson.ValidBytes(body) {
		return
	}
	root := gjson.ParseBytes(body)
	event := root.Get("type").String()
	if event != "" && event != "response" && event != "response.output_item.done" &&
		event != "response.completed" && event != "response.done" && event != "response.failed" && event != "response.incomplete" {
		return
	}
	observe := func(item gjson.Result, index int64) {
		kind := item.Get("type").String()
		status := item.Get("status").String()
		if status != "" && status != "completed" {
			return
		}
		if kind != "web_search_call" {
			if strings.HasSuffix(kind, "_call") && kind != "function_call" && kind != "custom_tool_call" && kind != "image_generation_call" {
				for _, seen := range usage.UnpricedHostedTools {
					if seen == kind {
						return
					}
				}
				usage.UnpricedHostedTools = append(usage.UnpricedHostedTools, kind)
			}
			return
		}
		key := item.Get("id").String()
		if key == "" {
			key = "index:" + strconv.FormatInt(index, 10)
		}
		if usage.hostedSearchSeen == nil {
			usage.hostedSearchSeen = make(map[string]struct{})
		}
		if _, seen := usage.hostedSearchSeen[key]; !seen {
			usage.hostedSearchSeen[key] = struct{}{}
			usage.HostedWebSearchCalls++
		}
	}
	if event == "response.output_item.done" {
		observe(root.Get("item"), root.Get("output_index").Int())
		return
	}
	for _, path := range []string{"response.output", "output", "data.response.output", "data.output"} {
		if items := root.Get(path); items.IsArray() {
			for i, item := range items.Array() {
				observe(item, int64(i))
			}
			break
		}
	}
}

func preserveOpenAIHostedTools(dst *OpenAIUsage, src OpenAIUsage) {
	if len(src.hostedSearchSeen) > 0 {
		if dst.hostedSearchSeen == nil {
			dst.hostedSearchSeen = make(map[string]struct{})
		}
		for key := range src.hostedSearchSeen {
			dst.hostedSearchSeen[key] = struct{}{}
		}
	}
	dst.HostedWebSearchCalls = max(max(dst.HostedWebSearchCalls, src.HostedWebSearchCalls), len(dst.hostedSearchSeen))
	for _, kind := range src.UnpricedHostedTools {
		found := false
		for _, existing := range dst.UnpricedHostedTools {
			if existing == kind {
				found = true
				break
			}
		}
		if !found {
			dst.UnpricedHostedTools = append(dst.UnpricedHostedTools, kind)
		}
	}
}
