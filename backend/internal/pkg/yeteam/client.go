package yeteam

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

const defaultBaseURL = "https://ye.team"

// Config controls the ye.team client. The service authenticates requests with
// the card code, so no server-side API key is required.
type Config struct {
	Enabled         bool
	BaseURL         string
	Timeout         time.Duration
	PollInterval    time.Duration
	MaxPollDuration time.Duration
	AutoRefresh401  bool
}

type Client struct {
	baseURL         string
	httpClient      *http.Client
	pollInterval    time.Duration
	maxPollDuration time.Duration
	enabled         atomic.Bool
	autoRefresh401  bool
}

func NewClient(cfg Config) *Client {
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		base = defaultBaseURL
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	poll := cfg.PollInterval
	if poll <= 0 {
		poll = 12 * time.Second
	}
	maxPoll := cfg.MaxPollDuration
	if maxPoll <= 0 {
		maxPoll = 10 * time.Minute
	}
	client := &Client{
		baseURL:         base,
		httpClient:      &http.Client{Timeout: timeout},
		pollInterval:    poll,
		maxPollDuration: maxPoll,
		autoRefresh401:  cfg.AutoRefresh401,
	}
	client.enabled.Store(cfg.Enabled)
	return client
}

func (c *Client) Enabled() bool {
	return c != nil && c.enabled.Load()
}

// SetEnabled updates the runtime integration switch shared by all ye.team callers.
func (c *Client) SetEnabled(enabled bool) {
	if c != nil {
		c.enabled.Store(enabled)
	}
}

func (c *Client) AutoRefresh401Enabled() bool {
	return c != nil && c.enabled.Load() && c.autoRefresh401
}

type PreviewRequest struct {
	CardCode string `json:"card_code"`
	Format   string `json:"format,omitempty"`
	Project  string `json:"project,omitempty"`
	TargetID string `json:"target_id"`
}

type PreviewResult struct {
	Action             string         `json:"action,omitempty"`
	RedeemAction       string         `json:"redeem_action,omitempty"`
	Available          bool           `json:"available,omitempty"`
	CanFulfill         bool           `json:"can_fulfill,omitempty"`
	CanRedeemRemaining bool           `json:"can_redeem_remaining,omitempty"`
	CanRefreshBound    bool           `json:"can_refresh_bound,omitempty"`
	CardQuotaRemaining int            `json:"card_quota_remaining,omitempty"`
	BoundCount         int            `json:"bound_count,omitempty"`
	Message            string         `json:"message,omitempty"`
	Raw                map[string]any `json:"-"`
}

func (p *PreviewResult) UnmarshalJSON(data []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	p.Raw = raw
	p.Action = firstString(raw, "action", "redeem_action", "recommended_action")
	p.RedeemAction = firstString(raw, "redeem_action", "action")
	if value, ok := raw["can_fulfill"].(bool); ok {
		p.CanFulfill = value
	} else {
		p.CanFulfill = true
	}
	if value, ok := raw["can_redeem_remaining"].(bool); ok {
		p.CanRedeemRemaining = value
	}
	if value, ok := raw["can_refresh_bound"].(bool); ok {
		p.CanRefreshBound = value
	}
	if value, ok := raw["card_quota_remaining"].(float64); ok {
		p.CardQuotaRemaining = int(value)
	}
	if value, ok := raw["bound_count"].(float64); ok {
		p.BoundCount = int(value)
	}
	if value, ok := raw["available"].(bool); ok {
		p.Available = value
	} else {
		p.Available = true
	}
	p.Message = firstString(raw, "message", "detail", "error")
	return nil
}

type RedeemRequest struct {
	CardCode        string `json:"card_code"`
	Format          string `json:"format,omitempty"`
	Project         string `json:"project,omitempty"`
	TargetID        string `json:"target_id"`
	Action          string `json:"action"`
	ClientRequestID string `json:"client_request_id"`
}

type Order struct {
	OrderNo       string         `json:"order_no,omitempty"`
	Status        string         `json:"status,omitempty"`
	DownloadToken string         `json:"download_token,omitempty"`
	Token         string         `json:"token,omitempty"`
	Message       string         `json:"message,omitempty"`
	Raw           map[string]any `json:"-"`
}

func (o *Order) UnmarshalJSON(data []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	o.Raw = raw
	o.OrderNo = nestedString(raw, "order_no", "orderNo", "id", "task_id", "taskId")
	o.Status = nestedString(raw, "status", "state")
	o.DownloadToken = nestedString(raw, "download_token", "downloadToken")
	o.Token = nestedString(raw, "token")
	o.Message = nestedString(raw, "message", "detail", "error")
	return nil
}

func firstString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := raw[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// NormalizeAccountPayload accepts the account package returned by ye.team,
// including the common {data:{accounts:[]}} and bare array wrappers, and
// returns the sub2api data shape consumed by the admin import endpoint.
func NormalizeAccountPayload(data []byte) ([]byte, error) {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("decode account package: %w", err)
	}
	var object map[string]any
	switch typed := value.(type) {
	case []any:
		object = map[string]any{"accounts": typed}
	case map[string]any:
		object = typed
	default:
		return nil, errors.New("ye.team account package must be an object or array")
	}
	for _, key := range []string{"data", "result", "payload"} {
		if nested, ok := object[key].(map[string]any); ok {
			object = nested
			break
		}
	}
	accounts, ok := object["accounts"].([]any)
	if !ok || len(accounts) == 0 {
		return nil, errors.New("ye.team account package contains no accounts")
	}
	if _, ok := object["type"]; !ok {
		object["type"] = "sub2api-data"
	}
	if _, ok := object["version"]; !ok {
		object["version"] = 1
	}
	return json.Marshal(object)
}

type ReclaimRequest struct {
	CardCodes []string `json:"card_codes"`
	Mode      string   `json:"mode,omitempty"`
	QueryOnly bool     `json:"query_only,omitempty"`
}

type HealthCheckResult struct {
	OK            bool             `json:"ok"`
	NeedReclaim   int              `json:"need_reclaim"`
	Healthy       int              `json:"healthy"`
	CannotReclaim int              `json:"cannot_reclaim"`
	Unknown       int              `json:"unknown"`
	Total         int              `json:"total"`
	NotLoadable   int              `json:"not_loadable"`
	Credentials   []map[string]any `json:"credentials,omitempty"`
	Error         string           `json:"error,omitempty"`
	Raw           map[string]any   `json:"-"`
}

func (r *HealthCheckResult) UnmarshalJSON(data []byte) error {
	type alias HealthCheckResult
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*r = HealthCheckResult(decoded)
	r.Raw = raw
	return nil
}

type ReclaimTask struct {
	CardCode      string `json:"card_code"`
	OrderNo       string `json:"order_no"`
	ResourceUID   string `json:"resource_uid"`
	Status        string `json:"status"`
	Message       string `json:"message"`
	NoAction      bool   `json:"no_action"`
	DownloadToken string `json:"download_token"`
	DownloadError string `json:"download_error"`
}

type ReclaimCard struct {
	CardCode string        `json:"card_code"`
	Tasks    []ReclaimTask `json:"tasks"`
}

type BatchReclaimResult struct {
	OK             bool           `json:"ok"`
	Total          int            `json:"total"`
	Queued         int            `json:"queued"`
	AlreadyRunning int            `json:"already_running"`
	Done           int            `json:"done"`
	Unreclaimable  int            `json:"unreclaimable"`
	NotOwned       int            `json:"not_owned"`
	Skipped        int            `json:"skipped"`
	Failed         int            `json:"failed"`
	Cards          []ReclaimCard  `json:"cards"`
	AllTasks       []ReclaimTask  `json:"-"`
	Error          string         `json:"error"`
	Raw            map[string]any `json:"-"`
}

func (r *BatchReclaimResult) UnmarshalJSON(data []byte) error {
	type alias BatchReclaimResult
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*r = BatchReclaimResult(decoded)
	r.Raw = raw
	for cardIndex := range r.Cards {
		for taskIndex := range r.Cards[cardIndex].Tasks {
			task := &r.Cards[cardIndex].Tasks[taskIndex]
			if task.CardCode == "" {
				task.CardCode = r.Cards[cardIndex].CardCode
			}
			r.AllTasks = append(r.AllTasks, *task)
		}
	}
	return nil
}

type AccountCredentials struct {
	Name        string
	Credentials map[string]any
	Extra       map[string]any
}

// Reclaim401Packages runs the documented health-check and batch-cards flow,
// then downloads every completed task that exposes a download token.
func (c *Client) Reclaim401Packages(ctx context.Context, cardCode string) ([][]byte, error) {
	cardCode = strings.TrimSpace(cardCode)
	if cardCode == "" {
		return nil, errors.New("ye.team reclaim card code is empty")
	}
	health, err := c.HealthCheck(ctx, ReclaimRequest{CardCodes: []string{cardCode}})
	if err != nil {
		return nil, err
	}
	if !health.OK {
		if strings.TrimSpace(health.Error) == "" {
			health.Error = "ye.team health check failed"
		}
		return nil, errors.New(health.Error)
	}
	request := ReclaimRequest{CardCodes: []string{cardCode}, Mode: "401"}
	initial, err := c.BatchReclaim(ctx, request)
	if err != nil {
		return nil, err
	}
	if !initial.OK {
		if strings.TrimSpace(initial.Error) == "" {
			initial.Error = "ye.team reclaim submission failed"
		}
		return nil, errors.New(initial.Error)
	}
	final, err := c.pollReclaimUntilDone(ctx, request)
	if err != nil {
		return nil, err
	}
	var packages [][]byte
	for _, task := range final.AllTasks {
		if strings.ToLower(strings.TrimSpace(task.Status)) != "done" || task.OrderNo == "" || task.DownloadToken == "" {
			continue
		}
		data, downloadErr := c.Download(ctx, task.OrderNo, task.DownloadToken)
		if downloadErr != nil {
			continue
		}
		packages = append(packages, data)
	}
	if len(packages) == 0 {
		return nil, errors.New("ye.team reclaim completed without downloadable account packages")
	}
	return packages, nil
}

// Reclaim401 preserves the single-package helper for callers that only need
// one result. Account-aware callers should use Reclaim401Packages.
func (c *Client) Reclaim401(ctx context.Context, cardCode string) ([]byte, error) {
	packages, err := c.Reclaim401Packages(ctx, cardCode)
	if err != nil {
		return nil, err
	}
	return packages[0], nil
}

func (c *Client) pollReclaimUntilDone(ctx context.Context, request ReclaimRequest) (BatchReclaimResult, error) {
	deadline := time.Now().Add(c.maxPollDuration)
	request.QueryOnly = true
	for {
		current, err := c.BatchReclaim(ctx, request)
		if err != nil {
			return BatchReclaimResult{}, err
		}
		if !current.OK {
			if strings.TrimSpace(current.Error) == "" {
				current.Error = "ye.team reclaim progress query failed"
			}
			return current, errors.New(current.Error)
		}
		if current.Queued == 0 && current.AlreadyRunning == 0 {
			return current, nil
		}
		if time.Now().After(deadline) {
			return current, fmt.Errorf("ye.team reclaim polling timed out: %s", strings.Join(request.CardCodes, ","))
		}
		timer := time.NewTimer(c.pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return BatchReclaimResult{}, ctx.Err()
		case <-timer.C:
		}
	}
}

// FindAccountCredentials selects an account from a downloaded package. A
// single-account package is accepted directly; multi-account packages require
// a name/email/id hint to prevent updating the wrong local account.
func FindAccountCredentials(data []byte, hints ...string) (AccountCredentials, error) {
	normalized, err := NormalizeAccountPayload(data)
	if err != nil {
		return AccountCredentials{}, err
	}
	var payload map[string]any
	if err := json.Unmarshal(normalized, &payload); err != nil {
		return AccountCredentials{}, err
	}
	items, ok := payload["accounts"].([]any)
	if !ok || len(items) == 0 {
		return AccountCredentials{}, errors.New("account package contains no accounts")
	}
	needle := make([]string, 0, len(hints))
	for _, hint := range hints {
		if hint = strings.ToLower(strings.TrimSpace(hint)); hint != "" {
			needle = append(needle, hint)
		}
	}
	var selected map[string]any
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if selected == nil && len(items) == 1 {
			selected = obj
		}
		name := accountMatchText(obj)
		for _, hint := range needle {
			if strings.Contains(name, hint) {
				selected = obj
				break
			}
		}
	}
	if selected == nil {
		return AccountCredentials{}, errors.New("account package did not match the current account")
	}
	credentials, ok := selected["credentials"].(map[string]any)
	if !ok || len(credentials) == 0 {
		return AccountCredentials{}, errors.New("matched account has no credentials")
	}
	extra, _ := selected["extra"].(map[string]any)
	return AccountCredentials{Name: firstString(selected, "name", "email"), Credentials: credentials, Extra: extra}, nil
}

func accountMatchText(account map[string]any) string {
	var parts []string
	for _, key := range []string{"name", "email", "account_id", "account_uuid", "id", "resource_uid"} {
		if value := firstString(account, key); value != "" {
			parts = append(parts, value)
		}
	}
	if credentials, ok := account["credentials"].(map[string]any); ok {
		for _, key := range []string{"email", "account_id", "account_uuid", "chatgpt_account_id", "user_id", "id"} {
			if value := firstString(credentials, key); value != "" {
				parts = append(parts, value)
			}
		}
	}
	return strings.ToLower(strings.Join(parts, " "))
}

func nestedString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := raw[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	for _, value := range raw {
		if nested, ok := value.(map[string]any); ok {
			if found := nestedString(nested, keys...); found != "" {
				return found
			}
		}
	}
	return ""
}

func (c *Client) Preview(ctx context.Context, req PreviewRequest) (PreviewResult, error) {
	var out PreviewResult
	err := c.doJSON(ctx, http.MethodPost, "/api/redeem/preview", req, &out)
	return out, err
}

func (c *Client) Redeem(ctx context.Context, req RedeemRequest) (Order, error) {
	var out Order
	err := c.doJSON(ctx, http.MethodPost, "/api/redeem/orders", req, &out)
	return out, err
}

func (c *Client) OrderStatus(ctx context.Context, orderNo string, downloadToken ...string) (Order, error) {
	var out Order
	path := "/api/redeem/orders/" + url.PathEscape(strings.TrimSpace(orderNo))
	var headers http.Header
	if len(downloadToken) > 0 && strings.TrimSpace(downloadToken[0]) != "" {
		headers = make(http.Header)
		headers.Set("Authorization", "Bearer "+strings.TrimSpace(downloadToken[0]))
	}
	err := c.doJSONWithHeaders(ctx, http.MethodGet, path, nil, headers, &out)
	return out, err
}

func (c *Client) Download(ctx context.Context, orderNo, token string) ([]byte, error) {
	path := "/api/redeem/orders/" + url.PathEscape(strings.TrimSpace(orderNo)) + "/download?token=" + url.QueryEscape(strings.TrimSpace(token))
	return c.doBytes(ctx, http.MethodGet, path, nil)
}

func (c *Client) HealthCheck(ctx context.Context, req ReclaimRequest) (HealthCheckResult, error) {
	var out HealthCheckResult
	err := c.doJSON(ctx, http.MethodPost, "/api/redeem/reclaim/health-check", req, &out)
	return out, err
}

func (c *Client) BatchReclaim(ctx context.Context, req ReclaimRequest) (BatchReclaimResult, error) {
	var out BatchReclaimResult
	err := c.doJSON(ctx, http.MethodPost, "/api/redeem/reclaim/batch-cards", req, &out)
	return out, err
}

// PollUntilDone waits for an order and returns the terminal order metadata.
func (c *Client) PollUntilDone(ctx context.Context, orderNo string, downloadToken ...string) (Order, error) {
	if strings.TrimSpace(orderNo) == "" {
		return Order{}, errors.New("ye.team order number is empty")
	}
	deadline := time.Now().Add(c.maxPollDuration)
	for {
		order, err := c.OrderStatus(ctx, orderNo, downloadToken...)
		if err != nil {
			return Order{}, err
		}
		status := strings.ToLower(strings.TrimSpace(order.Status))
		switch status {
		case "completed", "complete", "success", "succeeded", "done", "finished":
			return order, nil
		case "failed", "error", "cancelled", "canceled", "expired":
			if order.Message == "" {
				order.Message = "ye.team order failed with status " + status
			}
			return order, errors.New(order.Message)
		}
		if time.Now().After(deadline) {
			return order, fmt.Errorf("ye.team order polling timed out: %s", orderNo)
		}
		timer := time.NewTimer(c.pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return Order{}, ctx.Err()
		case <-timer.C:
		}
	}
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, out any) error {
	return c.doJSONWithHeaders(ctx, method, path, body, nil, out)
}

func (c *Client) doJSONWithHeaders(ctx context.Context, method, path string, body any, headers http.Header, out any) error {
	data, err := c.doBytesWithHeaders(ctx, method, path, body, headers)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(data)) == 0 || out == nil {
		return nil
	}
	var envelope struct {
		Data    json.RawMessage `json:"data"`
		Success *bool           `json:"success"`
		Message string          `json:"message"`
		Error   string          `json:"error"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("decode ye.team response: %w", err)
	}
	payload := data
	if len(envelope.Data) > 0 && string(envelope.Data) != "null" {
		payload = envelope.Data
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("decode ye.team payload: %w", err)
	}
	if msg := strings.TrimSpace(envelope.Error); msg != "" {
		return errors.New(msg)
	}
	return nil
}

func (c *Client) doBytes(ctx context.Context, method, path string, body any) ([]byte, error) {
	return c.doBytesWithHeaders(ctx, method, path, body, nil)
}

func (c *Client) doBytesWithHeaders(ctx context.Context, method, path string, body any, headers http.Header) ([]byte, error) {
	if c == nil {
		return nil, errors.New("ye.team client is nil")
	}
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ye.team request: %w", err)
	}
	defer resp.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if readErr != nil {
		return nil, readErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ye.team HTTP %d: %s", resp.StatusCode, compactMessage(data))
	}
	return data, nil
}

func compactMessage(data []byte) string {
	var payload map[string]any
	if json.Unmarshal(data, &payload) == nil {
		for _, key := range []string{"message", "error", "detail"} {
			if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	}
	return strings.TrimSpace(string(data))
}
