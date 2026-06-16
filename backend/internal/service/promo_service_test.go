//go:build unit

package service

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidatePromoFirstRechargeValue_BoundaryAndInvalid(t *testing.T) {
	t.Parallel()

	require.NoError(t, validatePromoFirstRechargeValue(nil, 0, math.MaxFloat64, "INVALID_FIRST_RECHARGE_BONUS", "invalid bonus"))
	for _, v := range []float64{0, 0.01, 10, 999999} {
		v := v
		require.NoError(t, validatePromoFirstRechargeValue(&v, 0, math.MaxFloat64, "INVALID_FIRST_RECHARGE_BONUS", "invalid bonus"))
	}
	for _, v := range []float64{-0.01, math.NaN(), math.Inf(1), math.Inf(-1)} {
		v := v
		require.Error(t, validatePromoFirstRechargeValue(&v, 0, math.MaxFloat64, "INVALID_FIRST_RECHARGE_BONUS", "invalid bonus"))
	}

	for _, v := range []float64{0.01, 50, 100} {
		v := v
		require.NoError(t, validatePromoFirstRechargeValue(&v, 0.01, 100, "INVALID_FIRST_RECHARGE_DISCOUNT", "invalid discount"))
	}
	for _, v := range []float64{0, -0.01, 100.01, math.NaN(), math.Inf(1), math.Inf(-1)} {
		v := v
		require.Error(t, validatePromoFirstRechargeValue(&v, 0.01, 100, "INVALID_FIRST_RECHARGE_DISCOUNT", "invalid discount"))
	}
}
