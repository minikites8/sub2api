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

	"github.com/google/uuid"
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
	autoRefresh401  atomic.Bool
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
	}
	client.enabled.Store(cfg.Enabled)
	client.autoRefresh401.Store(cfg.AutoRefresh401)
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
	return c != nil && c.enabled.Load() && c.autoRefresh401.Load()
}

// SetAutoRefresh401 updates the runtime 401 credential reclaim switch.
func (c *Client) SetAutoRefresh401(enabled bool) {
	if c != nil {
		c.autoRefresh401.Store(enabled)
	}
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
	QueryOnly bool     `json:"query_only"`
}

type ReclaimTask struct {
	CardCode       string `json:"card_code"`
	OrderNo        string `json:"order_no"`
	ResourceUID    string `json:"resource_uid"`
	Status         string `json:"status"`
	Message        string `json:"message"`
	ErrorCode      string `json:"error_code"`
	FailureClass   string `json:"failure_class"`
	Permanent      bool   `json:"permanent"`
	ProviderStatus int    `json:"provider_status"`
	NoAction       bool   `json:"no_action"`
	DownloadToken  string `json:"download_token"`
	DownloadError  string `json:"download_error"`
}

// ReclaimTaskError preserves the terminal task metadata returned by ye.team.
// Permanent unreclaimable tasks are eligible for the refresh_bound fallback.
type ReclaimTaskError struct {
	Status         string
	Message        string
	ResourceUID    string
	ErrorCode      string
	FailureClass   string
	Permanent      bool
	ProviderStatus int
}

func (e *ReclaimTaskError) Error() string {
	if e == nil || strings.TrimSpace(e.Message) == "" {
		return "ye.team reclaim task failed"
	}
	return e.Message
}

func (t *ReclaimTask) UnmarshalJSON(data []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	t.CardCode = nestedString(raw, "card_code", "cardCode")
	t.OrderNo = nestedString(raw, "order_no", "orderNo", "order")
	t.ResourceUID = nestedString(raw, "resource_uid", "resourceUid")
	t.Status = nestedString(raw, "status", "state")
	t.Message = nestedString(raw, "message", "detail", "error")
	t.ErrorCode = nestedString(raw, "error_code", "errorCode")
	t.FailureClass = nestedString(raw, "failure_class", "failureClass")
	t.DownloadToken = nestedString(raw, "download_token", "downloadToken", "token")
	t.DownloadError = nestedString(raw, "download_error", "downloadError")
	if value, ok := raw["permanent"].(bool); ok {
		t.Permanent = value
	}
	if value, ok := raw["provider_status"].(float64); ok {
		t.ProviderStatus = int(value)
	} else if value, ok := raw["providerStatus"].(float64); ok {
		t.ProviderStatus = int(value)
	}
	if value, ok := raw["no_action"].(bool); ok {
		t.NoAction = value
	} else if value, ok := raw["noAction"].(bool); ok {
		t.NoAction = value
	}
	return nil
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

type BatchDownloadItem struct {
	OrderNo       string `json:"order_no"`
	DownloadToken string `json:"download_token,omitempty"`
}

type BatchDownloadRequest struct {
	ExportMode string              `json:"export_mode"`
	Items      []BatchDownloadItem `json:"items"`
	Summary    []any               `json:"summary"`
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
	if len(r.Cards) == 0 {
		r.Cards = reclaimCardsAlias(raw["card_codes"])
	}
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

func reclaimCardsAlias(value any) []ReclaimCard {
	if value == nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var cards []ReclaimCard
	if err := json.Unmarshal(data, &cards); err == nil {
		return cards
	}
	objects, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	cards = make([]ReclaimCard, 0, len(objects))
	for cardCode, object := range objects {
		cardData, ok := object.(map[string]any)
		if !ok {
			continue
		}
		if _, exists := cardData["card_code"]; !exists {
			cardData["card_code"] = cardCode
		}
		data, err := json.Marshal(cardData)
		if err != nil {
			continue
		}
		var card ReclaimCard
		if json.Unmarshal(data, &card) == nil {
			cards = append(cards, card)
		}
	}
	return cards
}

type AccountCredentials struct {
	Name        string
	Credentials map[string]any
	Extra       map[string]any
}

type ReclaimFlowStage struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	At      string `json:"at"`
}

type ReclaimFlowBatch struct {
	OK             bool `json:"ok"`
	Total          int  `json:"total"`
	Queued         int  `json:"queued"`
	AlreadyRunning int  `json:"already_running"`
	Done           int  `json:"done"`
	Failed         int  `json:"failed"`
	Unreclaimable  int  `json:"unreclaimable"`
	NotOwned       int  `json:"not_owned"`
	Skipped        int  `json:"skipped"`
	Cards          int  `json:"cards"`
	Tasks          int  `json:"tasks"`
}

type ReclaimFlowTask struct {
	Status         string `json:"status"`
	OrderNo        string `json:"order_no,omitempty"`
	ResourceUID    string `json:"resource_uid,omitempty"`
	Message        string `json:"message,omitempty"`
	ErrorCode      string `json:"error_code,omitempty"`
	FailureClass   string `json:"failure_class,omitempty"`
	Permanent      bool   `json:"permanent,omitempty"`
	ProviderStatus int    `json:"provider_status,omitempty"`
}

type ReclaimFlow struct {
	Status            string             `json:"status"`
	Trigger           string             `json:"trigger,omitempty"`
	StartedAt         string             `json:"started_at"`
	FinishedAt        string             `json:"finished_at,omitempty"`
	FallbackUsed      bool               `json:"fallback_used,omitempty"`
	OrderNo           string             `json:"order_no,omitempty"`
	PackageCount      int                `json:"package_count,omitempty"`
	CredentialChanged *bool              `json:"credential_changed,omitempty"`
	CacheInvalidated  bool               `json:"cache_invalidated,omitempty"`
	Batch             *ReclaimFlowBatch  `json:"batch,omitempty"`
	Task              *ReclaimFlowTask   `json:"task,omitempty"`
	Tasks             []ReclaimFlowTask  `json:"tasks,omitempty"`
	Stages            []ReclaimFlowStage `json:"stages"`
}

func NewReclaimFlow() ReclaimFlow {
	return ReclaimFlow{Status: "running", StartedAt: time.Now().UTC().Format(time.RFC3339Nano), Stages: make([]ReclaimFlowStage, 0, 8)}
}

func (f *ReclaimFlow) AddStage(name, status, message string) {
	if f == nil {
		return
	}
	f.Stages = append(f.Stages, ReclaimFlowStage{
		Name: name, Status: status, Message: strings.TrimSpace(message), At: time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (f *ReclaimFlow) Finish(status string) {
	if f == nil {
		return
	}
	f.Status = strings.TrimSpace(status)
	f.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
}

func flowBatch(result BatchReclaimResult) *ReclaimFlowBatch {
	return &ReclaimFlowBatch{
		OK: result.OK, Total: result.Total, Queued: result.Queued, AlreadyRunning: result.AlreadyRunning,
		Done: result.Done, Failed: result.Failed, Unreclaimable: result.Unreclaimable, NotOwned: result.NotOwned,
		Skipped: result.Skipped, Cards: len(result.Cards), Tasks: len(result.AllTasks),
	}
}

func flowTask(task ReclaimTask) *ReclaimFlowTask {
	return &ReclaimFlowTask{
		Status: task.Status, OrderNo: task.OrderNo, ResourceUID: task.ResourceUID, Message: task.Message,
		ErrorCode: task.ErrorCode, FailureClass: task.FailureClass, Permanent: task.Permanent,
		ProviderStatus: task.ProviderStatus,
	}
}

func flowTasks(tasks []ReclaimTask) []ReclaimFlowTask {
	if len(tasks) == 0 {
		return nil
	}
	out := make([]ReclaimFlowTask, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, *flowTask(task))
	}
	return out
}

// Reclaim401Packages submits and polls the batch-cards flow, then downloads
// every completed task that exposes a download token.
func (c *Client) Reclaim401Packages(ctx context.Context, cardCode string) ([][]byte, error) {
	packages, _, err := c.Reclaim401PackagesWithTrace(ctx, cardCode)
	return packages, err
}

func (c *Client) Reclaim401PackagesWithTrace(ctx context.Context, cardCode string) (packages [][]byte, flow ReclaimFlow, err error) {
	flow = NewReclaimFlow()
	defer func() {
		if err != nil {
			flow.Finish("failed")
		} else {
			flow.Finish("success")
		}
	}()
	cardCode = strings.TrimSpace(cardCode)
	if cardCode == "" {
		err = errors.New("ye.team reclaim card code is empty")
		return nil, flow, err
	}
	flow.AddStage("batch_reclaim", "running", "")
	request := ReclaimRequest{CardCodes: []string{cardCode}, Mode: "401"}
	initial, err := c.BatchReclaim(ctx, request)
	if err != nil {
		flow.AddStage("batch_reclaim", "failed", err.Error())
		return nil, flow, err
	}
	flow.Batch = flowBatch(initial)
	if len(initial.AllTasks) > 0 {
		flow.Task = flowTask(initial.AllTasks[0])
		flow.Tasks = flowTasks(initial.AllTasks)
	}
	if !initial.OK {
		if strings.TrimSpace(initial.Error) == "" {
			initial.Error = "ye.team reclaim submission failed"
		}
		err = errors.New(initial.Error)
		flow.AddStage("batch_reclaim", "failed", err.Error())
		return nil, flow, err
	}
	flow.AddStage("batch_reclaim", "success", fmt.Sprintf("done=%d queued=%d failed=%d unreclaimable=%d", initial.Done, initial.Queued, initial.Failed, initial.Unreclaimable))
	final := initial
	if initial.Queued > 0 || initial.AlreadyRunning > 0 || (!hasReclaimNoAction(initial) && len(collectBatchDownloadItems(initial)) == 0) {
		if err := reclaimTerminalError(initial); err != nil {
			flow.AddStage("batch_result", "failed", err.Error())
			if packages, fallbackErr, handled := c.tryRefreshBoundAfterPermanentFailure(ctx, cardCode, "", err, &flow); handled {
				return packages, flow, fallbackErr
			}
			return nil, flow, err
		}
		flow.AddStage("poll_reclaim", "running", "waiting for terminal task metadata")
		final, err = c.pollReclaimUntilDone(ctx, request)
		if err != nil {
			flow.AddStage("poll_reclaim", "failed", err.Error())
			if packages, fallbackErr, handled := c.tryRefreshBoundAfterPermanentFailure(ctx, cardCode, "", err, &flow); handled {
				return packages, flow, fallbackErr
			}
			return nil, flow, err
		}
		flow.Batch = flowBatch(final)
		if len(final.AllTasks) > 0 {
			flow.Task = flowTask(final.AllTasks[0])
			flow.Tasks = flowTasks(final.AllTasks)
		}
		flow.AddStage("poll_reclaim", "success", fmt.Sprintf("done=%d", final.Done))
	}
	items := collectBatchDownloadItems(initial, final)
	if len(items) == 0 {
		err = fmt.Errorf("ye.team reclaim completed without downloadable account packages (cards=%d tasks=%d done=%d)", len(final.Cards), len(final.AllTasks), final.Done)
		flow.AddStage("batch_download", "failed", err.Error())
		return nil, flow, err
	}
	flow.AddStage("batch_download", "running", fmt.Sprintf("items=%d", len(items)))
	data, err := c.BatchDownload(ctx, BatchDownloadRequest{
		ExportMode: "multi_account_json",
		Items:      items,
		Summary:    []any{},
	})
	if err != nil {
		flow.AddStage("batch_download", "failed", err.Error())
		return nil, flow, err
	}
	flow.AddStage("batch_download", "success", "account package downloaded")
	flow.PackageCount = 1
	return [][]byte{data}, flow, nil
}

// RefreshBoundPackages regenerates the delivery package for an already-bound
// card through the standard order flow. ye.team's batch-cards endpoint can
// report a stale permanent account-deactivated task while this path still has
// a valid refresh_bound order available.
func (c *Client) RefreshBoundPackages(ctx context.Context, cardCode, targetID string) ([][]byte, error) {
	packages, _, err := c.refreshBoundPackagesWithTrace(ctx, cardCode, targetID, nil)
	return packages, err
}

func (c *Client) refreshBoundPackagesWithTrace(ctx context.Context, cardCode, targetID string, flow *ReclaimFlow) ([][]byte, ReclaimFlow, error) {
	localFlow := ReclaimFlow{}
	if flow == nil {
		localFlow = NewReclaimFlow()
		flow = &localFlow
	}
	cardCode = strings.TrimSpace(cardCode)
	if cardCode == "" {
		err := errors.New("ye.team refresh card code is empty")
		flow.AddStage("refresh_bound_order", "failed", err.Error())
		return nil, *flow, err
	}
	flow.AddStage("refresh_bound_order", "running", "")
	order, err := c.Redeem(ctx, RedeemRequest{
		CardCode:        cardCode,
		Format:          "sub2api",
		Project:         "k12",
		TargetID:        strings.TrimSpace(targetID),
		Action:          "refresh_bound",
		ClientRequestID: uuid.NewString(),
	})
	if err != nil {
		flow.AddStage("refresh_bound_order", "failed", err.Error())
		return nil, *flow, err
	}
	if strings.TrimSpace(order.OrderNo) == "" {
		err = errors.New("ye.team refresh response did not include order_no")
		flow.AddStage("refresh_bound_order", "failed", err.Error())
		return nil, *flow, err
	}
	flow.OrderNo = order.OrderNo
	flow.AddStage("refresh_bound_order", "success", "order created")
	initialToken := strings.TrimSpace(order.DownloadToken)
	if initialToken == "" {
		initialToken = strings.TrimSpace(order.Token)
	}
	finalOrder, err := c.PollUntilDone(ctx, order.OrderNo, initialToken)
	if err != nil {
		flow.AddStage("refresh_bound_poll", "failed", err.Error())
		return nil, *flow, err
	}
	flow.AddStage("refresh_bound_poll", "success", "order completed")
	token := firstString(finalOrder.Raw, "download_token", "downloadToken", "token")
	if token == "" {
		token = strings.TrimSpace(finalOrder.DownloadToken)
	}
	if token == "" {
		token = strings.TrimSpace(finalOrder.Token)
	}
	if token == "" {
		token = firstString(order.Raw, "download_token", "downloadToken", "token")
	}
	if token == "" {
		token = strings.TrimSpace(order.DownloadToken)
	}
	if token == "" {
		token = strings.TrimSpace(order.Token)
	}
	if token == "" {
		err = errors.New("ye.team refresh order completed without a download token")
		flow.AddStage("download", "failed", err.Error())
		return nil, *flow, err
	}
	flow.AddStage("download", "running", "")
	data, err := c.Download(ctx, finalOrder.OrderNo, token)
	if err != nil {
		flow.AddStage("download", "failed", err.Error())
		return nil, *flow, err
	}
	flow.AddStage("download", "success", "account package downloaded")
	flow.PackageCount = 1
	return [][]byte{data}, *flow, nil
}

func (c *Client) tryRefreshBoundAfterPermanentFailure(ctx context.Context, cardCode, targetID string, reclaimErr error, flow *ReclaimFlow) ([][]byte, error, bool) {
	var taskErr *ReclaimTaskError
	if !errors.As(reclaimErr, &taskErr) || !taskErr.Permanent || taskErr.Status != "unreclaimable" {
		return nil, reclaimErr, false
	}
	if strings.TrimSpace(targetID) == "" {
		targetID = strings.TrimSpace(taskErr.ResourceUID)
	}
	flow.FallbackUsed = true
	flow.AddStage("refresh_bound", "running", "batch result requested standard bound refresh")
	packages, _, err := c.refreshBoundPackagesWithTrace(ctx, cardCode, targetID, flow)
	if err == nil {
		flow.AddStage("refresh_bound", "success", "fallback package downloaded")
		return packages, nil, true
	}
	flow.AddStage("refresh_bound", "failed", err.Error())
	return nil, errors.Join(reclaimErr, fmt.Errorf("ye.team refresh_bound fallback failed: %w", err)), true
}

func hasReclaimNoAction(result BatchReclaimResult) bool {
	for _, task := range result.AllTasks {
		if task.NoAction && isReclaimTaskDone(task.Status) {
			return true
		}
	}
	return false
}

func collectBatchDownloadItems(results ...BatchReclaimResult) []BatchDownloadItem {
	items := make([]BatchDownloadItem, 0)
	indexes := make(map[string]int)
	for _, result := range results {
		for _, task := range result.AllTasks {
			if !isReclaimTaskDone(task.Status) {
				continue
			}
			orderNo := strings.TrimSpace(task.OrderNo)
			token := strings.TrimSpace(task.DownloadToken)
			if orderNo == "" || (token == "" && !task.NoAction) {
				continue
			}
			if index, ok := indexes[orderNo]; ok {
				items[index].DownloadToken = token
				continue
			}
			indexes[orderNo] = len(items)
			items = append(items, BatchDownloadItem{OrderNo: orderNo, DownloadToken: token})
		}
	}
	return items
}

func isReclaimTaskDone(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "done", "completed", "complete", "success", "succeeded", "finished", "ok", "healthy":
		return true
	default:
		return false
	}
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
	request.Mode = ""
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
		// ye.team can briefly return an empty terminal-looking snapshot while it
		// restores task metadata. A download token or completed no_action task is
		// the authoritative signal that the batch result is ready for download.
		if err := reclaimTerminalError(current); err != nil {
			return current, err
		}
		if hasReclaimNoAction(current) || len(collectBatchDownloadItems(current)) > 0 {
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

func reclaimTerminalError(result BatchReclaimResult) error {
	for _, task := range result.AllTasks {
		if err := reclaimTaskTerminalError(task); err != nil {
			return err
		}
	}
	if result.Failed > 0 {
		return errors.New("ye.team reclaim task failed")
	}
	if result.Unreclaimable > 0 {
		return errors.New("ye.team reclaim task is unreclaimable")
	}
	if result.NotOwned > 0 {
		return errors.New("ye.team reclaim card is not owned")
	}
	return nil
}

func reclaimTaskTerminalError(task ReclaimTask) error {
	status := strings.ToLower(strings.TrimSpace(task.Status))
	switch status {
	case "failed", "error", "cancelled", "canceled", "expired", "unreclaimable", "not_owned", "not-owned":
		message := strings.TrimSpace(task.Message)
		if message == "" {
			switch status {
			case "unreclaimable":
				message = "ye.team reclaim task is unreclaimable"
			case "not_owned", "not-owned":
				message = "ye.team reclaim card is not owned"
			default:
				message = "ye.team reclaim task failed with status " + status
			}
		}
		errorCode := strings.TrimSpace(task.ErrorCode)
		if errorCode != "" && !strings.Contains(strings.ToLower(message), strings.ToLower(errorCode)) {
			message += " (" + errorCode + ")"
		}
		return &ReclaimTaskError{
			Status:         status,
			Message:        message,
			ResourceUID:    strings.TrimSpace(task.ResourceUID),
			ErrorCode:      errorCode,
			FailureClass:   strings.TrimSpace(task.FailureClass),
			Permanent:      task.Permanent,
			ProviderStatus: task.ProviderStatus,
		}
	default:
		return nil
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

func (c *Client) BatchDownload(ctx context.Context, req BatchDownloadRequest) ([]byte, error) {
	if strings.TrimSpace(req.ExportMode) == "" {
		req.ExportMode = "multi_account_json"
	}
	if len(req.Items) == 0 {
		return nil, errors.New("ye.team batch download items are empty")
	}
	if req.Summary == nil {
		req.Summary = []any{}
	}
	return c.doBytes(ctx, http.MethodPost, "/api/redeem/batch-download", req)
}

func (c *Client) BatchReclaim(ctx context.Context, req ReclaimRequest) (BatchReclaimResult, error) {
	var out BatchReclaimResult
	err := c.doJSON(ctx, http.MethodPost, "/api/redeem/reclaim/batch-cards", req, &out)
	return out, err
}

// PollUntilDone waits for an order and returns the terminal order metadata.
func (c *Client) PollUntilDone(ctx context.Context, orderNo string, downloadToken ...string) (Order, error) {
	orderNo = strings.TrimSpace(orderNo)
	if orderNo == "" {
		return Order{}, errors.New("ye.team order number is empty")
	}
	initialDownloadToken := ""
	if len(downloadToken) > 0 {
		initialDownloadToken = strings.TrimSpace(downloadToken[0])
	}
	deadline := time.Now().Add(c.maxPollDuration)
	for {
		order, err := c.OrderStatus(ctx, orderNo, downloadToken...)
		if err != nil {
			return Order{}, err
		}
		if order.OrderNo == "" {
			order.OrderNo = orderNo
		}
		if order.DownloadToken == "" {
			order.DownloadToken = initialDownloadToken
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
