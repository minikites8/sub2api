package service

const featureKeyMarketplaceProviderVisible = "marketplace_provider_visible"

// IsMarketplaceProviderVisible reports whether a channel exposes its provider
// identity for a platform in the model marketplace.
func (c *Channel) IsMarketplaceProviderVisible(platform string) bool {
	if c == nil {
		return false
	}
	override := platformBoolOverride(c.FeaturesConfig, featureKeyMarketplaceProviderVisible, platform)
	return override != nil && *override
}
