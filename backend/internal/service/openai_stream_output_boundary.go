package service

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const openAIStreamFirstOutputBoundaryKey = "openai_stream_first_output_boundary"

type openAIStreamOutputBoundary struct {
	EventType string
	ItemType  string
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
		EventType: truncateString(strings.TrimSpace(eventType), 96),
		ItemType:  truncateString(strings.TrimSpace(gjson.GetBytes(payload, "item.type").String()), 64),
	})
}
