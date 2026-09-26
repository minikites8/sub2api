package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultAccountTokenGuardConfigRequiresExplicitEndpoints(t *testing.T) {
	cfg := defaultAccountTokenGuardConfig()

	require.False(t, cfg.Enabled)
	require.Empty(t, cfg.ProbeEndpoint)
	require.Empty(t, cfg.ReloginEndpoint)
	require.False(t, cfg.AutoRelogin)
	require.NoError(t, ValidateAccountTokenGuardConfig(cfg))
}
