package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultAccountTokenGuardConfigRequiresExplicitEndpoints(t *testing.T) {
	cfg := defaultAccountTokenGuardConfig()

	require.False(t, cfg.Enabled)
	require.Equal(t, "https://session.ameng2027.xyz/api/v1/relogin/probe", cfg.ProbeEndpoint)
	require.Equal(t, "https://session.ameng2027.xyz/api/v1/relogin", cfg.ReloginEndpoint)
	require.True(t, cfg.AutoRelogin)
	require.Equal(t, "1", cfg.ProbeHeaders["X-Session-Studio-Probe"])
	require.Equal(t, "1", cfg.ReloginHeaders["X-Session-Studio-Relogin"])
	require.NoError(t, ValidateAccountTokenGuardConfig(cfg))
}
