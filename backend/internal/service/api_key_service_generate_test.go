package service

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyServiceGenerateKeyUsesDefaultPrefix(t *testing.T) {
	svc := &APIKeyService{cfg: &config.Config{}}

	key, err := svc.GenerateKey()

	require.NoError(t, err)
	require.True(t, strings.HasPrefix(key, "yc_"))
	require.Len(t, key, len("yc_")+64)
}

func TestAPIKeyServiceGenerateKeyUsesConfiguredPrefix(t *testing.T) {
	svc := &APIKeyService{cfg: &config.Config{
		Default: config.DefaultConfig{APIKeyPrefix: "custom_"},
	}}

	key, err := svc.GenerateKey()

	require.NoError(t, err)
	require.True(t, strings.HasPrefix(key, "custom_"))
	require.Len(t, key, len("custom_")+64)
}
