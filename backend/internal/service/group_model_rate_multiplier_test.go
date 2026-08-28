package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupRateMultiplierForModel(t *testing.T) {
	group := &Group{
		RateMultiplier: 1.5,
		ModelRateMultipliers: map[string]float64{
			"gpt-4.1":           0.8,
			" claude-sonnet-4 ": 1.2,
		},
	}

	require.Equal(t, 0.8, group.RateMultiplierForModel("GPT-4.1"))
	require.Equal(t, 1.2, group.RateMultiplierForModel("claude-sonnet-4"))
	require.Equal(t, 1.5, group.RateMultiplierForModel("gemini-2.5-pro"))
	require.Equal(t, 0.8, group.RateMultiplierForModels("openai/gpt-4.1", "gpt-4.1"))
}

func TestNormalizeModelRateMultipliers(t *testing.T) {
	got, err := NormalizeModelRateMultipliers(map[string]float64{" GPT-4.1 ": 0.8})
	require.NoError(t, err)
	require.Equal(t, map[string]float64{"gpt-4.1": 0.8}, got)

	_, err = NormalizeModelRateMultipliers(map[string]float64{"gpt-4.1": -0.1})
	require.Error(t, err)
	_, err = NormalizeModelRateMultipliers(map[string]float64{" ": 1})
	require.Error(t, err)
	_, err = NormalizeModelRateMultipliers(map[string]float64{"GPT-4.1": 0.8, " gpt-4.1 ": 0.9})
	require.Error(t, err)
}
