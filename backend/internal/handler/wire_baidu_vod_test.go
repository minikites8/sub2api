package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestProvideOpenAIGatewayHandlerInjectsBaiduVODVideoService(t *testing.T) {
	cfg := &config.Config{}
	runtime := service.ProvideBaiduVODVideoWorkerRuntime(nil, nil, nil, nil, nil, nil, nil, nil, nil, cfg)
	t.Cleanup(runtime.Stop)

	h := ProvideOpenAIGatewayHandler(nil, nil, nil, nil, nil, nil, nil, nil, runtime, cfg)
	require.NotNil(t, h.baiduVODVideoService)
}
