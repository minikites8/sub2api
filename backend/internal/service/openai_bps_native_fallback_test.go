package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBPSNativeFallbackIsRequestScoped(t *testing.T) {
	base := context.Background()
	marked := WithOpenAIBPSNativeFallback(base)
	scheduler := &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{}}
	account := excelAccount()
	req := OpenAIAccountScheduleRequest{RequestedModel: "gpt-6-astra"}
	require.False(t, OpenAIBPSNativeFallback(base))
	require.True(t, OpenAIBPSNativeFallback(marked))
	ok, reason := scheduler.isAccountRequestCompatibleReason(marked, account, req)
	require.False(t, ok)
	require.Equal(t, ExcelBPSLocalCapabilityReason, reason)
	require.True(t, account.IsExcelBPSEnabled())
	require.True(t, account.Schedulable)
}
