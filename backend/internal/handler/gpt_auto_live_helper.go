package handler

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// Response headers that tell a gpt-auto client which model and effort served
// the request. They carry no internal routing detail.
const (
	gptAutoModelHeader  = "X-GPT-Auto-Model"
	gptAutoEffortHeader = "X-GPT-Auto-Reasoning-Effort"
)

// resolveGPTAutoLive resolves the gpt-auto virtual model before any
// scheduling. For other models it returns the inputs unchanged. When ok is
// false the error response has already been written.
//
// The call site runs after authentication and body parsing and before group
// reasoning-effort policy, composite routing, channel mapping, billing
// eligibility and account selection, so all of those see the executed model
// exactly as if the customer had named it.
func resolveGPTAutoLive(c *gin.Context, svc *service.GPTAutoShadowService, apiKey *service.APIKey, endpoint string, body []byte, reqModel string, gateway ...*service.OpenAIGatewayService) ([]byte, string, *service.GPTAutoRoute, bool) {
	if !service.IsGPTAutoVirtualModel(reqModel) {
		return body, reqModel, nil, true
	}
	if svc == nil || apiKey == nil || c == nil || c.Request == nil {
		writeGPTAutoRouteError(c, &service.GPTAutoRouteError{Status: 404, Type: "invalid_request_error", Code: "model_not_found",
			Message: "The model `" + service.GPTAutoVirtualModel + "` does not exist or you do not have access to it."})
		return nil, "", nil, false
	}
	ctx := c.Request.Context()
	input := service.GPTAutoRouteInput{
		Endpoint:       endpoint,
		Body:           body,
		APIKeyID:       apiKey.ID,
		GroupID:        apiKey.GroupID,
		Group:          apiKey.Group,
		SessionKey:     gptAutoSessionKey(c, body),
		ModelPermitted: gptAutoModelPermitted(apiKey),
	}
	if apiKey.User != nil {
		input.UserID = apiKey.User.ID
	} else {
		input.UserID = apiKey.UserID
	}
	if len(gateway) > 0 && gateway[0] != nil {
		input.CustomerRate = func(group *service.Group, at time.Time) float64 {
			key := *apiKey
			key.Group = group
			return gateway[0].GPTAutoCustomerRateSnapshot(ctx, &key, at)
		}
	}
	route, err := svc.Route(ctx, input)
	if err != nil {
		if ctx.Err() == nil {
			routeErr, ok := err.(*service.GPTAutoRouteError)
			if !ok {
				routeErr = &service.GPTAutoRouteError{Status: 503, Type: "api_error", Code: "gpt_auto_routing_unavailable", Message: "Automatic model routing is temporarily unavailable."}
			}
			writeGPTAutoRouteError(c, routeErr)
		}
		return nil, "", nil, false
	}
	c.Request = c.Request.WithContext(service.WithGPTAutoRoute(ctx, route))
	setGPTAutoRouteHeaders(c, route)
	return route.Body(), route.Model(), route, true
}

func setGPTAutoRouteHeaders(c *gin.Context, route *service.GPTAutoRoute) {
	if c == nil || route == nil || c.Writer.Written() {
		return
	}
	c.Header(gptAutoModelHeader, route.Model())
	if effort := route.Effort(); effort != "" {
		c.Header(gptAutoEffortHeader, effort)
	}
}

// rerouteGPTAutoLive moves a gpt-auto request to the next eligible model when
// the selected one has no schedulable upstream account and nothing has been
// sent upstream yet. body is the request as currently processed (prompt
// rules, effort policy); only its model and effort are replaced, and the
// group reasoning-effort policy is applied again.
func rerouteGPTAutoLive(c *gin.Context, route *service.GPTAutoRoute, apiKey *service.APIKey, endpoint string, body []byte) ([]byte, string, bool) {
	if route == nil || c == nil || c.Writer.Written() {
		return nil, "", false
	}
	if !route.Reroute(service.GPTAutoFailureNoUpstream) {
		return nil, "", false
	}
	next := service.GPTAutoRewriteBody(endpoint, body, route.Model(), route.Effort())
	if capped, changed, err := applyOpenAIReasoningEffortPolicyForRequest(c, apiKey, next); err == nil && changed {
		next = capped
	}
	route.ApplyExecutionEffort(next)
	setGPTAutoRouteHeaders(c, route)
	return next, route.Model(), true
}

// finishGPTAutoLive records the outcome of a routed request. Deferred by the
// handler; a nil route is a no-op.
func finishGPTAutoLive(c *gin.Context, route *service.GPTAutoRoute, streamStarted bool) {
	if route == nil || c == nil {
		return
	}
	outcome := service.GPTAutoRouteOutcome{StreamStarted: streamStarted}
	if c.Writer != nil {
		outcome.HTTPStatus = c.Writer.Status()
	}
	if value, ok := c.Get(opsAccountIDKey); ok {
		if id, isInt := value.(int64); isInt {
			outcome.AccountID = id
		}
	}
	if c.Request != nil {
		outcome.ClientGone = c.Request.Context().Err() != nil
	}
	if _, streamFailed := service.GetOpsStreamError(c); streamStarted && streamFailed {
		outcome.UpstreamFailed = true
		outcome.ErrorCategory = service.GPTAutoFailureUpstream
	}
	ctx := c.Request.Context()
	route.Finish(ctx, outcome)
}

func writeGPTAutoRouteError(c *gin.Context, err *service.GPTAutoRouteError) {
	if c == nil || err == nil || c.Writer.Written() {
		return
	}
	c.JSON(err.Status, gin.H{"error": gin.H{"type": err.Type, "code": err.Code, "message": err.Message}})
}

// gptAutoModelPermitted applies the group model allowlist and the user's
// per-group model denials to each underlying pool model, so the virtual model
// cannot reach a model the key could not request directly.
func gptAutoModelPermitted(apiKey *service.APIKey) func(string) bool {
	denied := apiKey.DeniedModelsInGroup()
	return func(model string) bool {
		if apiKey.Group != nil && apiKey.Group.ModelAllowlistEnabled() && !apiKey.Group.ModelAllowlist.Allows(model) {
			return false
		}
		return service.FirstUserGroupDeniedModel(denied, []string{model}) == ""
	}
}

// gptAutoSessionKey returns the client's explicit session reference: the
// session headers the gateway already recognises, else prompt_cache_key.
// It is scoped to the API key before use and never stored in clear.
func gptAutoSessionKey(c *gin.Context, body []byte) string {
	if session := strings.TrimSpace(service.ExtractClientSessionID(c)); session != "" {
		return session
	}
	return strings.TrimSpace(gjson.GetBytes(body, "prompt_cache_key").String())
}

// gptAutoListKey marks a /v1/models request whose API key may use gpt-auto.
const gptAutoListKey = "gpt_auto_virtual_model_listed"

// markGPTAutoModelListing flags the model listing to include gpt-auto when
// the key is currently eligible: live routing on, the group's own switch on,
// the rollout restrictions and model permissions pass, a tariff is
// configured and at least one underlying model is usable.
func markGPTAutoModelListing(c *gin.Context, svc *service.GPTAutoShadowService, apiKey *service.APIKey, platform string) {
	if c == nil || c.Request == nil || svc == nil || apiKey == nil || platform != service.PlatformOpenAI {
		return
	}
	if svc.LiveAdmits(c.Request.Context(), service.GPTAutoRouteInput{
		APIKeyID: apiKey.ID, GroupID: apiKey.GroupID, Group: apiKey.Group, ModelPermitted: gptAutoModelPermitted(apiKey),
	}) {
		c.Set(gptAutoListKey, true)
	}
}

// withGPTAutoVirtualModel appends the virtual model to an OpenAI model list.
func withGPTAutoVirtualModel(c *gin.Context, models any) any {
	list, ok := models.([]openai.Model)
	if !ok {
		return models
	}
	out := make([]openai.Model, 0, len(list)+1)
	for _, model := range list {
		if !service.IsGPTAutoVirtualModel(model.ID) {
			out = append(out, model)
		}
	}
	if c != nil && c.GetBool(gptAutoListKey) {
		out = append(out, openai.Model{ID: service.GPTAutoVirtualModel, Object: "model", Created: 1704067200, OwnedBy: "sub2api", Type: "model", DisplayName: "GPT Auto"})
	}
	return out
}

// Filter provider-supplied and pinned catalogues as well: an upstream cannot
// advertise our virtual model on behalf of an unauthorized key.
func gptAutoCatalogue(c *gin.Context, body []byte) ([]byte, error) {
	filtered, err := removeDeniedCatalogEntries(body, []string{service.GPTAutoVirtualModel})
	if err != nil || c == nil || !c.GetBool(gptAutoListKey) {
		return filtered, err
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(filtered, &envelope); err != nil {
		return nil, err
	}
	if raw, ok := envelope["data"]; ok {
		var entries []json.RawMessage
		if err := json.Unmarshal(raw, &entries); err != nil {
			return nil, err
		}
		entries = append(entries, json.RawMessage(`{"id":"gpt-auto","object":"model","created":1704067200,"owned_by":"sub2api"}`))
		envelope["data"], err = json.Marshal(entries)
		if err != nil {
			return nil, err
		}
	}
	return json.Marshal(envelope)
}
