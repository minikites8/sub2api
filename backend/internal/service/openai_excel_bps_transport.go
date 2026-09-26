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
	"net/http/httptrace"
	"sync/atomic"
)

var errExcelBPSProxyUnavailable = errors.New("BPS proxy unavailable")

type excelBPSLease interface {
	Release()
	ReportFailure()
	ReportStreamFailure()
	ReportSuccess()
	ReportUpstreamFailure()
}

type excelBPSAcquire func(context.Context, string, ...string) (string, excelBPSLease, error)

// Missing trace is not evidence of safety. Standard net/http emits GetConn
// before dialing and GotConn before handing a connection to request writing.
// Keep evidence for the whole attempt, including any internal reconnects.
type excelBPSWriteEvidence struct {
	started      atomic.Bool
	handedToHTTP atomic.Bool
}

func (e *excelBPSWriteEvidence) request(req *http.Request) *http.Request {
	mark := func() { e.handedToHTTP.Store(true) }
	trace := &httptrace.ClientTrace{
		GetConn:              func(string) { e.started.Store(true) },
		GotConn:              func(httptrace.GotConnInfo) { mark() },
		WroteHeaderField:     func(string, []string) { mark() },
		WroteHeaders:         mark,
		WroteRequest:         func(httptrace.WroteRequestInfo) { mark() },
		GotFirstResponseByte: mark,
	}
	req = req.Clone(httptrace.WithClientTrace(req.Context(), trace))
	if req.Body != nil {
		req.Body = &excelBPSTrackedBody{ReadCloser: req.Body, mark: mark}
	}
	if getBody := req.GetBody; getBody != nil {
		req.GetBody = func() (io.ReadCloser, error) {
			body, err := getBody()
			if err != nil {
				return nil, err
			}
			return &excelBPSTrackedBody{ReadCloser: body, mark: mark}, nil
		}
	}
	return req
}

func (e *excelBPSWriteEvidence) unsent() bool { return e.started.Load() && !e.handedToHTTP.Load() }

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
func (s *OpenAIGatewayService) doExcelBPSRequest(ctx context.Context, c *gin.Context, account *Account, scope string, body []byte, token, accountID string) (*http.Response, excelBPSLease, string, error) {
	proxy := ""
	if account != nil && account.Proxy != nil {
		proxy = account.Proxy.URL()
	}
	req, err := newExcelBPSRequest(ctx, body, token, accountID)
	if err != nil {
		return nil, nil, proxy, err
	}
	c.Set("excel_bps_upstream_attempt", 1)
	evidence := &excelBPSWriteEvidence{}
	resp, err := s.httpUpstream.Do(evidence.request(req), proxy, account.ID, account.Concurrency)
	if err != nil {
		recordExcelBPSTransportFailure(ctx, c, account, scope, proxy, err, "transport", 1, false)
	}
	return resp, nil, proxy, err
}

func recordExcelBPSTransportFailure(ctx context.Context, c *gin.Context, account *Account, scope, proxy string, err error, stage string, attempt int, retry bool) {
	kind := transportdiag.Classify(err)
	if errors.Is(err, errExcelBPSProxyUnavailable) {
		kind = "proxy_unavailable"
	}
	digest := sha256.Sum256([]byte(scope))
	sessionHash := hex.EncodeToString(digest[:8])
	port := 0
	detail, _ := json.Marshal(map[string]any{
		"error_kind": kind, "error_type": fmt.Sprintf("%T", err),
		"proxy_port": port, "session_hash": sessionHash, "attempt": attempt, "retry_before_send": retry,
	})
	message := "Excel BPS " + stage + " failed: " + kind
	// Keep UI client errors generic; persist only explicitly safe diagnostics.
	if !retry {
		setOpsUpstreamError(c, 0, message, string(detail))
	}
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform: account.Platform, AccountID: account.ID,
		UpstreamURL: basispoints.ResponsesURL, Kind: "request_error", Stage: stage,
		Scope: "excel_bps", Reason: kind, Message: message, Detail: string(detail),
	})
	logger.FromContext(ctx).Warn("excel_bps.transport_failed",
		zap.Int64("account_id", account.ID), zap.String("stage", stage),
		zap.String("error_kind", kind), zap.String("error_type", fmt.Sprintf("%T", err)),
		zap.Int("proxy_port", port), zap.String("session_hash", sessionHash),
		zap.Int("attempt", attempt), zap.Bool("retry_before_send", retry))
}
