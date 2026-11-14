package service

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const openAIStreamFirstOutputBoundaryKey = "openai_stream_first_output_boundary"

type openAIStreamOutputBoundary struct {
	EventType           string
	ItemType            string
	HasEncryptedContent bool
	HasVisibleContent   bool
}

func resetOpenAIStreamFirstOutputBoundary(c *gin.Context) {
	if c != nil {
		c.Set(openAIStreamFirstOutputBoundaryKey, openAIStreamOutputBoundary{})
	}
}

func openAIStreamFirstOutputBoundary(c *gin.Context) openAIStreamOutputBoundary {
	if c != nil {
		if value, exists := c.Get(openAIStreamFirstOutputBoundaryKey); exists {
			boundary, _ := value.(openAIStreamOutputBoundary)
			return boundary
		}
	}
	return openAIStreamOutputBoundary{}
}

func observeOpenAIStreamFirstOutputBoundary(c *gin.Context, payload []byte, eventType string) {
	if c == nil || openAIStreamFirstOutputBoundary(c).EventType != "" {
		return
	}
	// Keep diagnostics bounded even if an upstream emits an unknown event.
	c.Set(openAIStreamFirstOutputBoundaryKey, openAIStreamOutputBoundary{
		EventType:           truncateString(strings.TrimSpace(eventType), 96),
		HasEncryptedContent: gjson.GetBytes(payload, "item.encrypted_content").String() != "",
		HasVisibleContent:   openAIStreamItemHasVisibleOutput(gjson.GetBytes(payload, "item")) || openAIStreamBoundaryHasText(payload),
		ItemType:            truncateString(strings.TrimSpace(gjson.GetBytes(payload, "item.type").String()), 64),
	})
}

func openAIStreamBoundaryHasText(payload []byte) bool {
	for _, field := range []string{"delta", "text", "refusal"} {
		value := gjson.GetBytes(payload, field)
		if value.Type == gjson.String && value.String() != "" {
			return true
		}
	}
	return false
}

// Recognize an explicit upstream restart signal, never arbitrary echoed prose.
func openAIStreamWebSocketRestart(payload []byte) bool {
	for _, prefix := range []string{"response.error", "error"} {
		kind := strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, prefix+".type").String()))
		if strings.HasPrefix(kind, "websocket_") && gjson.GetBytes(payload, prefix+".code").String() == "1012" {
			return true
		}
	}
	return false
}
