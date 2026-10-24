package service

import "encoding/json"

// Group model pricing historically persisted Go field names, while its Vue
// editor sends snake_case. Read both forms; write the public snake_case API.
// No stored rows need rewriting and explicit zero rates remain pointers.
func (p *ChannelModelPricing) UnmarshalJSON(data []byte) error {
	type plain ChannelModelPricing
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for old, current := range map[string]string{
		"ID":                         "id",
		"ChannelID":                  "channel_id",
		"Platform":                   "platform",
		"Models":                     "models",
		"BillingMode":                "billing_mode",
		"InputPrice":                 "input_price",
		"OutputPrice":                "output_price",
		"CacheWritePrice":            "cache_write_price",
		"CacheWrite1hPrice":          "cache_write_1h_price",
		"CacheReadPrice":             "cache_read_price",
		"FastMultiplier":             "fast_multiplier",
		"FlexMultiplier":             "flex_multiplier",
		"ImageInputPrice":            "image_input_price",
		"ImageOutputPrice":           "image_output_price",
		"PerRequestPrice":            "per_request_price",
		"ReasoningEffortMultipliers": "reasoning_effort_multipliers",
		"PriorityMultiplier":         "priority_multiplier",
		"Intervals":                  "intervals",
		"TimePricing":                "time_pricing",
		"CreatedAt":                  "created_at",
		"UpdatedAt":                  "updated_at",
	} {
		if value, ok := fields[old]; ok {
			if _, exists := fields[current]; !exists {
				fields[current] = value
			}
			delete(fields, old)
		}
	}
	normalized, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	return json.Unmarshal(normalized, (*plain)(p))
}
