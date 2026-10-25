package service

import (
	"context"
	"errors"
)

var ErrGPTQoSRequestDeadline = errors.New("QoS request budget exhausted")
var ErrGPTQoSLeaseLost = errors.New("QoS capacity lease lost")

type gptQoSBudgetCancelKey struct{}

func GPTQoSLocalCancellation(ctx context.Context) (int, string) {
	if errors.Is(context.Cause(ctx), ErrGPTQoSRequestDeadline) {
		return 504, "QoS request budget exhausted"
	}
	if errors.Is(context.Cause(ctx), ErrGPTQoSLeaseLost) {
		return 503, "QoS capacity lease lost; retry later"
	}
	return 0, ""
}
func GPTQoSClientCanceled(ctx context.Context) bool {
	status, _ := GPTQoSLocalCancellation(ctx)
	return status == 0 && errors.Is(ctx.Err(), context.Canceled)
}
func GPTQoSRequestBudgetExceeded(ctx context.Context) bool {
	status, _ := GPTQoSLocalCancellation(ctx)
	return status != 0
}
