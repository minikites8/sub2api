package service

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Zen also checks the system environment block. Report the missing client
// workspace explicitly: never expose the gateway's filesystem or invent one.
const openCodeZenGatewayEnvironment = `<env>
  Working directory: unavailable (API request)
  Workspace root folder: unavailable (API request)
  Is directory a git repo: unknown
  Platform: API gateway
</env>`

// Preserve existing instructions, content parts and raw JSON numbers. An
// environment already supplied by the caller takes precedence and is not
// duplicated across protocol conversions or retries.
func ensureOpenCodeZenEnvironment(body []byte, responses bool) []byte {
	if !gjson.ValidBytes(body) {
		return body
	}
	field := "messages"
	if responses {
		field = "input"
	}
	instructions := gjson.GetBytes(body, "instructions")
	if responses && openCodeZenHasEnvironment(instructions) {
		return body
	}
	messages := gjson.GetBytes(body, field)
	for _, message := range messages.Array() {
		role := message.Get("role").String()
		if (role == "system" || role == "developer") && openCodeZenHasEnvironment(message.Get("content")) {
			return body
		}
	}
	var updated []byte
	var err error
	if responses {
		if instructions.Exists() && instructions.Type != gjson.Null && instructions.Type != gjson.String {
			return body
		}
		value := openCodeZenGatewayEnvironment
		if instructions.String() != "" {
			value = instructions.String() + "\n\n" + value
		}
		updated, err = sjson.SetBytes(body, "instructions", value)
	} else {
		if !messages.IsArray() {
			return body
		}
		for index, message := range messages.Array() {
			if role := message.Get("role").String(); role != "system" && role != "developer" {
				continue
			}
			content := message.Get("content")
			path := "messages." + strconv.Itoa(index) + ".content"
			switch {
			case content.Type == gjson.String:
				updated, err = sjson.SetBytes(body, path, content.String()+"\n\n"+openCodeZenGatewayEnvironment)
			case content.IsArray():
				updated, err = sjson.SetBytes(body, path+".-1", map[string]string{"type": "text", "text": openCodeZenGatewayEnvironment})
			}
			if updated != nil || err != nil {
				break
			}
		}
		if updated == nil && err == nil {
			message, _ := json.Marshal(map[string]string{"role": "system", "content": openCodeZenGatewayEnvironment})
			original := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(messages.Raw, "["), "]"))
			combined := "[" + string(message)
			if original != "" {
				combined += "," + original
			}
			updated, err = sjson.SetRawBytes(body, "messages", []byte(combined+"]"))
		}
	}
	if err != nil {
		return body
	}
	return updated
}

func openCodeZenHasEnvironment(content gjson.Result) bool {
	if content.Type == gjson.String {
		return strings.Contains(content.Str, "<env>") && strings.Contains(content.Str, "</env>")
	}
	for _, part := range content.Array() {
		if text := part.Get("text"); text.Type == gjson.String && strings.Contains(text.Str, "<env>") && strings.Contains(text.Str, "</env>") {
			return true
		}
	}
	return false
}
