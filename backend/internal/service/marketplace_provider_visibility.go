package service

const featureKeyMarketplaceProviderVisible = "marketplace_provider_visible"

// IsMarketplaceProviderVisible reports whether the channel allows a platform's
// provider identity to be shown in the public model marketplace.
func (c *Channel) IsMarketplaceProviderVisible(platform string) bool {
	if c == nil {
		return false
	}
	override := platformBoolOverride(c.FeaturesConfig, featureKeyMarketplaceProviderVisible, platform)
	return override != nil && *override
}
