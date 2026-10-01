package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service/basispoints"
	"github.com/Wei-Shaw/sub2api/internal/util/transportdiag"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"io"
	"net/http"
)

var errExcelBPSProxyUnavailable = errors.New("BPS proxy unavailable")

// Preserve local proxy classification after writing the client error without
// changing its log message or retaining an acquisition cause with credentials.
type excelBPSForwardError struct{ code string }

func (e *excelBPSForwardError) Error() string { return "excel BPS: " + e.code }
func (e *excelBPSForwardError) Unwrap() error {
	if e.code == "basispoints_proxy_unavailable" {
		return errExcelBPSProxyUnavailable
	}
	return nil
}

// Keep the cause available to diagnostics without exposing a supplier URL in
// the error string if a caller logs the returned error.
type excelBPSAcquisitionFailure struct{ cause error }

func (e *excelBPSAcquisitionFailure) Error() string { return errExcelBPSProxyUnavailable.Error() }
func (e *excelBPSAcquisitionFailure) Unwrap() []error {
	return []error{errExcelBPSProxyUnavailable, e.cause}
}

type excelBPSLease interface {
	Release()
	ReportFailure()
	ReportStreamFailure()
	ReportSuccess()
	ReportUpstreamFailure()
}

type excelBPSAcquire func(context.Context, string, ...string) (string, excelBPSLease, error)

func (s *OpenAIGatewayService) excelBPSAcquireFor(*Account) excelBPSAcquire { return nil }

// Missing trace is not evidence of safety. Standard net/http emits GetConn
// before dialing and GotConn before handing a connection to request writing.
// Keep evidence for the whole attempt, including any internal reconnects.
type excelBPSWriteEvidence struct{ transportdiag.Trace }

func (e *excelBPSWriteEvidence) request(req *http.Request) *http.Request {
	req = e.Request(req)
	if req.Body != nil {
		req.Body = &excelBPSTrackedBody{ReadCloser: req.Body, mark: e.MarkBodyRead}
	}
	if getBody := req.GetBody; getBody != nil {
		req.GetBody = func() (io.ReadCloser, error) {
			body, err := getBody()
			if err != nil {
				return nil, err
			}
			return &excelBPSTrackedBody{ReadCloser: body, mark: e.MarkBodyRead}, nil
		}
	}
	return req
}
func (e *excelBPSWriteEvidence) unsent() bool { return e.DefinitelyUnsent() }

type excelBPSTrackedBody struct {
	io.ReadCloser
	mark func()
}

func (b *excelBPSTrackedBody) Read(p []byte) (int, error) {
	if len(p) > 0 {
		b.mark()
	}
	return b.ReadCloser.Read(p)
}

// At most one extra model attempt, on another healthy managed exit, and only
// before HTTP could have written anything. The caller owns the returned lease
// through response closure. Static account proxies retain their old behavior.
func (s *OpenAIGatewayService) doExcelBPSRequest(ctx context.Context, c *gin.Context, account *Account, scope string, body []byte, token, accountID string, acquire ...excelBPSAcquire) (*http.Response, excelBPSLease, string, error) {
	build := func(ctx context.Context) (*http.Request, error) {
		return newExcelBPSRequest(ctx, body, token, accountID)
	}
	return s.doExcelBPSRequestTo(ctx, c, account, scope, basispoints.ResponsesURL, build, acquire...)
}

// doExcelBPSRequestTo applies the same exit and no-replay rules to another BPS
// endpoint; build must return a fresh request for each attempt.
func (s *OpenAIGatewayService) doExcelBPSRequestTo(ctx context.Context, c *gin.Context, account *Account, scope, upstreamURL string, build func(context.Context) (*http.Request, error), acquire ...excelBPSAcquire) (*http.Response, excelBPSLease, string, error) {
	_ = acquire
	proxy := ""
	if account != nil && account.Proxy != nil {
		proxy = account.Proxy.URL()
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, proxy, err
	}
	if s.httpUpstream == nil {
		err := errors.New("Excel BPS upstream is unavailable")
		recordExcelBPSTransportFailureAt(ctx, c, account, upstreamURL, scope, proxy, err, "transport", 1, false)
		return nil, nil, proxy, err
	}
	req, err := build(ctx)
	if err != nil {
		return nil, nil, proxy, err
	}
	c.Set("excel_bps_upstream_attempt", 1)
	evidence := &excelBPSWriteEvidence{}
	resp, err := s.httpUpstream.Do(evidence.request(req), proxy, account.ID, account.Concurrency)
	if err == nil && resp == nil {
		err = errors.New("Excel BPS upstream returned no response")
	}
	if err != nil {
		recordExcelBPSTransportFailureAt(ctx, c, account, upstreamURL, scope, proxy, err, "transport", 1, false, evidence)
	}
	return resp, nil, proxy, err
}
func recordExcelBPSTransportFailure(ctx context.Context, c *gin.Context, account *Account, scope, proxy string, err error, stage string, attempt int, retry bool, evidence ...*excelBPSWriteEvidence) {
	recordExcelBPSTransportFailureAt(ctx, c, account, basispoints.ResponsesURL, scope, proxy, err, stage, attempt, retry, evidence...)
}

func recordExcelBPSTransportFailureAt(ctx context.Context, c *gin.Context, account *Account, upstreamURL, scope, proxy string, err error, stage string, attempt int, retry bool, evidence ...*excelBPSWriteEvidence) {
	if isExcelBPSClientCancellation(c, err) {
		logger.FromContext(ctx).Info("excel_bps.client_canceled",
			zap.Int64("account_id", account.ID), zap.String("stage", stage))
		return
	}
	kind := transportdiag.Classify(err)
	if errors.Is(err, errExcelBPSProxyUnavailable) {
		kind = "proxy_unavailable"
	}
	digest := sha256.Sum256([]byte(scope))
	sessionHash := hex.EncodeToString(digest[:8])
	port := 0
	diagnostics := map[string]any{
		"error_kind": kind, "error_type": fmt.Sprintf("%T", err),
		"proxy_port": port, "session_hash": sessionHash, "attempt": attempt, "retry_before_send": retry,
	}
	if len(evidence) > 0 && evidence[0] != nil {
		diagnostics["transport"] = evidence[0].Snapshot()
	}
	detail, _ := json.Marshal(diagnostics)
	message := "Excel BPS " + stage + " failed: " + kind
	// Keep UI client errors generic; persist only explicitly safe diagnostics.
	if !retry {
		setOpsUpstreamError(c, 0, message, string(detail))
	}
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform: account.Platform, AccountID: account.ID,
		UpstreamURL: upstreamURL, Kind: "request_error", Stage: stage,
		Scope: "excel_bps", Reason: kind, Message: message, Detail: string(detail),
	})
	logger.FromContext(ctx).Warn("excel_bps.transport_failed",
		zap.Int64("account_id", account.ID), zap.String("stage", stage),
		zap.String("error_kind", kind), zap.String("error_type", fmt.Sprintf("%T", err)),
		zap.Int("proxy_port", port), zap.String("session_hash", sessionHash),
		zap.Int("attempt", attempt), zap.Bool("retry_before_send", retry),
		zap.Any("transport", diagnostics["transport"]), zap.Any("acquisition_reason", diagnostics["acquisition_reason"]), zap.Any("candidates_checked", diagnostics["candidates_checked"]))
}
