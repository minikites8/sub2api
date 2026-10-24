package service

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestGroupPricingJSONLegacyAndPublicAPI(t *testing.T) {
	for _, raw := range []string{
		`{"Models":["gpt-auto"],"BillingMode":"token","InputPrice":0.000002,"CacheReadPrice":0,"OutputPrice":0.000008,"CacheWrite1hPrice":0.000004}`,
		`{"models":["gpt-auto"],"billing_mode":"token","input_price":0.000002,"cache_read_price":0,"output_price":0.000008,"cache_write_1h_price":0.000004}`,
	} {
		var entry ChannelModelPricing
		require.NoError(t, json.Unmarshal([]byte(raw), &entry))
		require.Equal(t, []string{"gpt-auto"}, entry.Models)
		require.Equal(t, BillingModeToken, entry.BillingMode)
		require.Equal(t, 0.000002, *entry.InputPrice)
		require.Equal(t, 0.0, *entry.CacheReadPrice)
		require.Equal(t, 0.000008, *entry.OutputPrice)
		require.Equal(t, 0.000004, *entry.CacheWrite1hPrice)
		encoded, err := json.Marshal(entry)
		require.NoError(t, err)
		require.Contains(t, string(encoded), `"input_price":0.000002`)
	}
}
