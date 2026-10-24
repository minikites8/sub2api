package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// gptAutoPricingResponse is the pricing page payload: the stored tariff,
// readiness and every OpenAI group's effective gpt-auto price and access.
type gptAutoPricingResponse struct {
	*service.GPTAutoPricingState
	Groups []service.GPTAutoGroupPricingView `json:"groups"`
}

func (h *GPTAutoHandler) pricingResponse(c *gin.Context, state *service.GPTAutoPricingState) (*gptAutoPricingResponse, bool) {
	out := &gptAutoPricingResponse{GPTAutoPricingState: state, Groups: []service.GPTAutoGroupPricingView{}}
	if h.groupRepo == nil {
		return out, true
	}
	groups, err := h.groupRepo.ListActiveByPlatform(c.Request.Context(), service.PlatformOpenAI)
	if err != nil {
		response.ErrorFrom(c, err)
		return nil, false
	}
	views, err := h.shadowService.GroupPricingViews(c.Request.Context(), groups)
	if err != nil {
		response.ErrorFrom(c, err)
		return nil, false
	}
	out.Groups = views
	return out, true
}

// GetPricing returns the gpt-auto tariff.
// GET /api/v1/admin/gpt-auto/pricing
func (h *GPTAutoHandler) GetPricing(c *gin.Context) {
	state, err := h.shadowService.PricingState(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if out, ok := h.pricingResponse(c, state); ok {
		response.Success(c, out)
	}
}

type updateGPTAutoPricingRequest struct {
	Pricing         service.GPTAutoPricingConfig `json:"pricing"`
	ExpectedVersion int                          `json:"expected_version"`
	Confirm         bool                         `json:"confirm"`
}

// UpdatePricing saves a confirmed tariff change and bumps its version.
// PUT /api/v1/admin/gpt-auto/pricing
func (h *GPTAutoHandler) UpdatePricing(c *gin.Context) {
	var req updateGPTAutoPricingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	var actor int64
	if id := gptAutoAdminID(c); id != nil {
		actor = *id
	}
	state, err := h.shadowService.UpdatePricing(c.Request.Context(), service.GPTAutoPricingUpdate{
		Pricing: req.Pricing, ExpectedVersion: req.ExpectedVersion, Confirmed: req.Confirm, ActorID: actor,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if out, ok := h.pricingResponse(c, state); ok {
		response.Success(c, out)
	}
}

type simulateGPTAutoPricingRequest struct {
	// Pricing previews an unsaved tariff; omitted uses the stored tariff.
	Pricing               *service.GPTAutoPricingConfig `json:"pricing"`
	GroupID               *int64                        `json:"group_id"`
	InputTokens           int                           `json:"input_tokens"`
	CachedInputTokens     int                           `json:"cached_input_tokens"`
	OutputTokens          int                           `json:"output_tokens"`
	ExecutedModel         string                        `json:"executed_model"`
	ClassifierInputTokens int                           `json:"classifier_input_tokens"`
}

// SimulatePricing projects the customer charge and margins of one usage.
// POST /api/v1/admin/gpt-auto/pricing/simulate
func (h *GPTAutoHandler) SimulatePricing(c *gin.Context) {
	var req simulateGPTAutoPricingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	input := service.GPTAutoPricingSimulationInput{
		Pricing: req.Pricing, TotalInputTokens: req.InputTokens, CachedInputTokens: req.CachedInputTokens,
		OutputTokens: req.OutputTokens, ExecutedModel: req.ExecutedModel, ClassifierInputTokens: req.ClassifierInputTokens,
	}
	if req.GroupID != nil && h.groupRepo != nil {
		group, err := h.groupRepo.GetByID(c.Request.Context(), *req.GroupID)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		input.Group = group
	}
	result, err := h.shadowService.SimulatePricing(c.Request.Context(), input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
