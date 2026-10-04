//go:build unit

package service

import (
	"context"
	"net/http"

	kiropkg "github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
)

func (s *GatewayService) buildKiroPayloadForAccount(ctx context.Context, account *Account, parsed *ParsedRequest, anthropicBody []byte, modelID, token, requestModel string, headers http.Header) (*kiropkg.KiroBuildResult, error) {
	// 镜像生产逻辑：Q / KRS 端点都解析 profileArn（API Key → 空；缺失时上游 403）。
	profileArn := kiroResolveRequestProfileArn(account)
	return s.buildKiroPayloadForAccountWithArn(ctx, account, parsed, anthropicBody, modelID, token, requestModel, headers, profileArn)
}
