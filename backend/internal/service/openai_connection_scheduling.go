package service

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
)

// Kept outside openai_auth_connection*.go: the connection account-type
// regression workflow restores those files to an older baseline and must
// still compile this scheduling bridge against them.

type openAIConnectionCapabilityContextKey struct{}
type openAIChatCompletionsConnectionContextKey struct{}
type openAIBPSChatBridgeEnabledContextKey struct{}

func isOpenAIChatCompletionsConnectionRequest(c *gin.Context) bool {
	return c != nil && c.Request != nil && c.Request.URL != nil && strings.HasSuffix(strings.TrimRight(c.Request.URL.Path, "/"), "/chat/completions")
}

func openAIConnectionRequestedCapability(capability string, candidate *OpenAIAuthConnection, allowBPSChatBridge bool) string {
	if allowBPSChatBridge && capability == "chat_completions" && candidate.Kind == "bps" {
		return "responses"
	}
	return capability
}

func (s *OpenAIGatewayService) withOpenAIBPSChatBridgeScheduling(ctx context.Context) context.Context {
	if _, ready := ctx.Value(openAIBPSChatBridgeEnabledContextKey{}).(bool); ready {
		return ctx
	}
	isChat, _ := ctx.Value(openAIChatCompletionsConnectionContextKey{}).(bool)
	if !isChat {
		return ctx
	}
	return context.WithValue(ctx, openAIBPSChatBridgeEnabledContextKey{}, s.excelBPSGloballyEnabled(ctx))
}

// WithOpenAIConnectionCapabilityForRequest records, for account selection, the
// connection capability that prepareAuthConnection will require for this
// inbound route. The scheduler expresses Responses turns with abstract
// capabilities (chat_completions for ordinary turns so Chat-only API key
// upstreams stay eligible, responses_compact for native compaction), while a
// managed parent is projected onto one connection by inbound route. Without
// this, a managed BPS grant, which advertises only native responses, is
// filtered before the send path that would accept it.
func WithOpenAIConnectionCapabilityForRequest(ctx context.Context, c *gin.Context) context.Context {
	if ctx == nil || c == nil || c.Request == nil || c.Request.URL == nil {
		return ctx
	}
	ctx = context.WithValue(ctx, openAIChatCompletionsConnectionContextKey{}, isOpenAIChatCompletionsConnectionRequest(c))
	return context.WithValue(ctx, openAIConnectionCapabilityContextKey{}, connectionCapabilityForRequest(c))
}

func openAIConnectionCapabilityFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	capability, ok := ctx.Value(openAIConnectionCapabilityContextKey{}).(string)
	return capability, ok
}

// supportsOpenAIEndpointCapabilityForRequest is the scheduling form of
// SupportsOpenAIEndpointCapability. For an unprojected managed parent and a
// Responses-family requirement it applies the same connection predicate as
// prepareAuthConnection; every other account and capability is unchanged.
func (a *Account) supportsOpenAIEndpointCapabilityForRequest(ctx context.Context, capability OpenAIEndpointCapability) bool {
	if a.HasManagedOpenAIConnections() && a.AuthConnectionID == "" {
		switch capability {
		case OpenAIEndpointCapabilityChatCompletions, OpenAIEndpointCapabilityResponses, OpenAIEndpointCapabilityResponsesCompact:
			if required, ok := openAIConnectionCapabilityFromContext(ctx); ok {
				allowBPSChatBridge, _ := ctx.Value(openAIBPSChatBridgeEnabledContextKey{}).(bool)
				return a.managedConnectionSupports(required, allowBPSChatBridge)
			}
		}
	}
	return a.SupportsOpenAIEndpointCapability(capability)
}
