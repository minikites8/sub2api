//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"

	kiropkg "github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func (s *GatewayService) buildKiroPayloadForAccount(ctx context.Context, account *Account, parsed *ParsedRequest, anthropicBody []byte, modelID, token, requestModel string, headers http.Header) (*kiropkg.KiroBuildResult, error) {
	var profileArn string
	if kiroEndpointModeForRequest(account, parsed) == KiroEndpointModeKRS {
		profileArn = kiroResolveProfileArnForKRS(account)
	}
	return s.buildKiroPayloadForAccountWithArn(ctx, account, parsed, anthropicBody, modelID, token, requestModel, headers, profileArn)
}

func TestBuildKiroPayloadForAccountUsesStableConversationIDs(t *testing.T) {
	svc := &GatewayService{}
	account := &Account{ID: 40, Credentials: map[string]any{"profile_arn": "profile-a"}}
	body := []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hello","additional_kwargs":{"conversationId":"client-conv","continuationId":"client-cont"}}]}`)

	first, err := svc.buildKiroPayloadForAccount(context.Background(), account, nil, body, "claude-sonnet-4.5", "token", "claude-sonnet-4-5", nil)
	require.NoError(t, err)
	second, err := svc.buildKiroPayloadForAccount(context.Background(), account, nil, body, "claude-sonnet-4.5", "token", "claude-sonnet-4-5", nil)
	require.NoError(t, err)

	firstConversationID := gjson.GetBytes(first.Payload, "conversationState.conversationId").String()
	secondConversationID := gjson.GetBytes(second.Payload, "conversationState.conversationId").String()
	require.NotEmpty(t, firstConversationID)
	require.NotEmpty(t, secondConversationID)
	require.Equal(t, firstConversationID, secondConversationID)
	require.NotEqual(t, "client-conv", firstConversationID)
	require.False(t, gjson.GetBytes(first.Payload, "conversationState.agentContinuationId").Exists())
	require.False(t, gjson.GetBytes(second.Payload, "conversationState.agentContinuationId").Exists())
}

func TestBuildKiroPayloadForAccountReplaysFullMessagesIntoHistory(t *testing.T) {
	svc := &GatewayService{}
	account := &Account{ID: 40, Credentials: map[string]any{"profile_arn": "profile-a"}}
	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"system":"system prompt",
		"messages":[
			{"role":"user","content":"first"},
			{"role":"assistant","content":"answer"},
			{"role":"user","content":"second"}
		]
	}`)

	result, err := svc.buildKiroPayloadForAccount(context.Background(), account, nil, body, "claude-sonnet-4.5", "token", "claude-sonnet-4-5", nil)
	require.NoError(t, err)

	history := gjson.GetBytes(result.Payload, "conversationState.history").Array()
	require.Len(t, history, 4)
	require.Contains(t, history[0].Get("userInputMessage.content").String(), "system prompt")
	require.Equal(t, "first", history[2].Get("userInputMessage.content").String())
	require.Equal(t, "answer", history[3].Get("assistantResponseMessage.content").String())
	require.Equal(t, "second", gjson.GetBytes(result.Payload, "conversationState.currentMessage.userInputMessage.content").String())
}
