package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"

	"go.uber.org/zap"
)

const (
	codexHarvestFlowCap          = 200
	codexHarvestFlowSkipDebounce = 8 * time.Second
)

// CodexHarvestFlowEvent is a redacted breadcrumb for the admin harvest pipeline.
type CodexHarvestFlowEvent struct {
	ID             string    `json:"id"`
	At             time.Time `json:"at"`
	Stage          string    `json:"stage"`
	Kind           string    `json:"kind"`
	AccountID      int64     `json:"account_id,omitempty"`
	AccountName    string    `json:"account_name,omitempty"`
	Model          string    `json:"model,omitempty"`
	Node           string    `json:"node,omitempty"`
	HTTPStatus     int       `json:"http_status,omitempty"`
	Length         int       `json:"length,omitempty"`
	Blocks         int       `json:"blocks,omitempty"`
	ExpectedLength int       `json:"expected_length,omitempty"`
	ExpectedBlocks int       `json:"expected_blocks,omitempty"`
	Accepted       bool      `json:"accepted,omitempty"`
	Standby        bool      `json:"standby,omitempty"`
	Result         string    `json:"result,omitempty"`
	Reason         string    `json:"reason,omitempty"`
	Detail         string    `json:"detail,omitempty"`
}

type CodexHarvestFlowStage struct {
	ID             string     `json:"id"`
	Status         string     `json:"status"`
	Detail         string     `json:"detail,omitempty"`
	At             *time.Time `json:"at,omitempty"`
	Node           string     `json:"node,omitempty"`
	Model          string     `json:"model,omitempty"`
	HTTPStatus     int        `json:"http_status,omitempty"`
	Length         int        `json:"length,omitempty"`
	Blocks         int        `json:"blocks,omitempty"`
	ExpectedLength int        `json:"expected_length,omitempty"`
	ExpectedBlocks int        `json:"expected_blocks,omitempty"`
}

type CodexHarvestFlowAccount struct {
	ID           int64                     `json:"id"`
	Name         string                    `json:"name"`
	Status       string                    `json:"status"`
	Schedulable  bool                      `json:"schedulable"`
	SkipHarvest  bool                      `json:"skip_harvest"`
	InScope      bool                      `json:"in_scope"`
	Availability string                    `json:"availability"`
	RecoverAt    *time.Time                `json:"recover_at,omitempty"`
	Tickets      []OpenAICodexTicketStatus `json:"tickets"`
	ReadyCount   int                       `json:"ready_count"`
	BlockedCount int                       `json:"blocked_count"`
}

type CodexHarvestFlowHarvest struct {
	Enabled           bool     `json:"enabled"`
	FailClosed        bool     `json:"fail_closed"`
	Strategy          string   `json:"strategy"`
	ScopeMode         string   `json:"scope_mode,omitempty"`
	ScopeError        bool     `json:"scope_error,omitempty"`
	AccountPolicy     string   `json:"account_policy,omitempty"`
	GroupIDs          []int64  `json:"group_ids,omitempty"`
	Models            []string `json:"models"`
	TargetLength      int      `json:"target_length"`
	ProbeIntervalSec  int      `json:"probe_interval_seconds"`
	CooldownSec       int      `json:"cooldown_seconds"`
	MaxProbesPerRound int      `json:"max_probes_per_round"`
	RefreshBeforeSec  int      `json:"refresh_before_seconds"`
	HarvestProxy      string   `json:"harvest_proxy,omitempty"`
}

type CodexHarvestFlowSidecar struct {
	Mode       string     `json:"mode,omitempty"`
	Reachable  bool       `json:"reachable"`
	Source     string     `json:"source,omitempty"`
	Controller string     `json:"controller,omitempty"`
	Group      string     `json:"group,omitempty"`
	Type       string     `json:"type,omitempty"`
	Now        string     `json:"now,omitempty"`
	AllCount   int        `json:"all_count,omitempty"`
	Error      string     `json:"error,omitempty"`
	ObservedAt *time.Time `json:"observed_at,omitempty"`
}

type CodexHarvestFlowCounts struct {
	ProbeHit       int `json:"probe_hit"`
	ProbeMiss      int `json:"probe_miss"`
	TicketAccept   int `json:"ticket_accept"`
	TicketReject   int `json:"ticket_reject"`
	SelectOK       int `json:"select_ok"`
	SelectSkip     int `json:"select_skip"`
	SelectFail     int `json:"select_fail"`
	TicketsReady   int `json:"tickets_ready"`
	TicketsBlocked int `json:"tickets_blocked"`
}

type CodexHarvestFlowSnapshot struct {
	GeneratedAt time.Time                 `json:"generated_at"`
	Harvest     CodexHarvestFlowHarvest   `json:"harvest"`
	Sidecar     CodexHarvestFlowSidecar   `json:"sidecar"`
	Stages      []CodexHarvestFlowStage   `json:"stages"`
	Accounts    []CodexHarvestFlowAccount `json:"accounts"`
	Counts      CodexHarvestFlowCounts    `json:"counts"`
	Events      []CodexHarvestFlowEvent   `json:"events"`
	Runtime     *CodexHarvestRuntime      `json:"runtime,omitempty"`
}

type CodexHarvestFlowRepository interface {
	List(context.Context, int) ([]CodexHarvestFlowEvent, error)
	Append(context.Context, CodexHarvestFlowEvent) error
}

type harvestFlowStoreBox struct {
	repo CodexHarvestFlowRepository
}

type codexHarvestFlowRing struct {
	mu       sync.Mutex
	seq      atomic.Uint64
	events   []CodexHarvestFlowEvent
	skips    map[string]time.Time
	lastNow  string
	lastType string
	lastAll  int
	persist  atomic.Value
}

var defaultCodexHarvestFlow = &codexHarvestFlowRing{skips: make(map[string]time.Time)}

func resetCodexHarvestFlow() {
	defaultCodexHarvestFlow.mu.Lock()
	defer defaultCodexHarvestFlow.mu.Unlock()
	defaultCodexHarvestFlow.events = nil
	defaultCodexHarvestFlow.skips = make(map[string]time.Time)
	defaultCodexHarvestFlow.lastNow = ""
	defaultCodexHarvestFlow.lastType = ""
	defaultCodexHarvestFlow.lastAll = 0
	defaultCodexHarvestFlow.seq.Store(0)
}

func recordCodexHarvestFlow(event CodexHarvestFlowEvent) {
	if event.At.IsZero() {
		event.At = time.Now()
	}
	event.AccountName = clipFlowText(event.AccountName, 80)
	event.Model = clipFlowText(event.Model, 64)
	event.Node = clipFlowText(event.Node, 160)
	event.Result = clipFlowText(event.Result, 64)
	event.Reason = clipFlowText(event.Reason, 64)
	event.Detail = clipFlowText(event.Detail, 240)
	defaultCodexHarvestFlow.mu.Lock()
	if event.ID == "" {
		event.ID = fmt.Sprintf("%d-%d", event.At.UnixNano(), defaultCodexHarvestFlow.seq.Add(1))
	}
	defaultCodexHarvestFlow.events = append(defaultCodexHarvestFlow.events, event)
	if overflow := len(defaultCodexHarvestFlow.events) - codexHarvestFlowCap; overflow > 0 {
		defaultCodexHarvestFlow.events = append([]CodexHarvestFlowEvent(nil), defaultCodexHarvestFlow.events[overflow:]...)
	}
	defaultCodexHarvestFlow.mu.Unlock()
	persistCodexHarvestFlow(event)
}

func bindCodexHarvestFlowStore(repo CodexHarvestFlowRepository) {
	defaultCodexHarvestFlow.persist.Store(&harvestFlowStoreBox{repo: repo})
	if repo == nil {
		return
	}
	hydrateCodexHarvestFlow(repo)
}

func harvestFlowStore() CodexHarvestFlowRepository {
	box, _ := defaultCodexHarvestFlow.persist.Load().(*harvestFlowStoreBox)
	if box == nil {
		return nil
	}
	return box.repo
}

func hydrateCodexHarvestFlow(repo CodexHarvestFlowRepository) {
	if repo == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	events, err := repo.List(ctx, codexHarvestFlowCap)
	cancel()
	if err != nil {
		logger.L().Warn("codex harvest flow hydrate failed", zap.Error(err))
		return
	}
	defaultCodexHarvestFlow.mu.Lock()
	defer defaultCodexHarvestFlow.mu.Unlock()
	if len(defaultCodexHarvestFlow.events) > 0 {
		return
	}
	defaultCodexHarvestFlow.events = append([]CodexHarvestFlowEvent(nil), events...)
}

func persistCodexHarvestFlow(event CodexHarvestFlowEvent) {
	repo := harvestFlowStore()
	if repo == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	if err := repo.Append(ctx, event); err != nil {
		logger.L().Warn("codex harvest flow persist failed", zap.Error(err))
	}
}

func listCodexHarvestFlowEvents() []CodexHarvestFlowEvent {
	defaultCodexHarvestFlow.mu.Lock()
	defer defaultCodexHarvestFlow.mu.Unlock()
	out := make([]CodexHarvestFlowEvent, len(defaultCodexHarvestFlow.events))
	copy(out, defaultCodexHarvestFlow.events)
	return out
}

func recordCodexHarvestNode(now, groupType string, allCount int) {
	now = strings.TrimSpace(now)
	if now == "" {
		return
	}
	now = strings.TrimSpace(now)
	defaultCodexHarvestFlow.mu.Lock()
	if allCount > 0 {
		defaultCodexHarvestFlow.lastAll = allCount
	} else {
		allCount = defaultCodexHarvestFlow.lastAll
	}
	if groupType != "" {
		defaultCodexHarvestFlow.lastType = groupType
	} else {
		groupType = defaultCodexHarvestFlow.lastType
	}
	changed := defaultCodexHarvestFlow.lastNow != now
	defaultCodexHarvestFlow.lastNow = now
	defaultCodexHarvestFlow.mu.Unlock()
	if !changed {
		return
	}
	recordCodexHarvestFlow(CodexHarvestFlowEvent{
		Stage:  "node",
		Kind:   "rotate",
		Node:   now,
		Result: groupType,
		Detail: fmt.Sprintf("pool=%d", allCount),
	})
}

func recordCodexHarvestProbe(account *Account, model, result, node, detail string, httpStatus, length, blocks, expectedLength, expectedBlocks int) {
	kind := "probe_miss"
	if result == "success" {
		kind = "probe_hit"
	}
	event := CodexHarvestFlowEvent{
		Stage:          "probe",
		Kind:           kind,
		Model:          model,
		Node:           node,
		HTTPStatus:     httpStatus,
		Length:         length,
		Blocks:         blocks,
		ExpectedLength: expectedLength,
		ExpectedBlocks: expectedBlocks,
		Accepted:       result == "success",
		Result:         result,
		Detail:         detail,
	}
	if result != "success" {
		message, _, _ := describeCodexHarvestOutcome(result, detail, httpStatus, length, blocks, expectedLength, expectedBlocks, model, node)
		event.Detail = message
		// Native mint errors contain only controlled diagnostic categories.
		if (result == "invalid_route" || result == "model_mismatch" || strings.HasPrefix(detail, "mint transport:")) && detail != "" {
			event.Detail += " · " + detail
		}
		event.Reason = result
	} else if strings.TrimSpace(detail) == "" && length > 0 {
		event.Detail = fmt.Sprintf("合格门票 %d 字节 / %d 块", length, blocks)
	}
	if account != nil {
		event.AccountID = account.ID
		event.AccountName = account.Name
	}
	recordCodexHarvestFlow(event)
}

func recordCodexHarvestTicketStore(account *Account, ticket *openAICodexTicket, standby bool) {
	if ticket == nil {
		return
	}
	event := CodexHarvestFlowEvent{
		Stage:    "ticket",
		Kind:     "accept",
		Model:    ticket.Model,
		Node:     ticket.HarvestNodeName,
		Length:   ticket.Length,
		Blocks:   ticket.Blocks,
		Accepted: true,
		Standby:  standby,
	}
	if account != nil {
		event.AccountID = account.ID
		event.AccountName = account.Name
		event.ExpectedLength = openAICodexTicketExpectedLength(account)
		event.ExpectedBlocks = openAICodexTicketExpectedBlocks(account)
	}
	if ticket.Length == 780 {
		event.ExpectedLength = 780
		event.ExpectedBlocks = 33
	}
	recordCodexHarvestFlow(event)
}

func recordCodexHarvestTicketReject(account *Account, model, detail string, length, blocks int) {
	event := CodexHarvestFlowEvent{
		Stage:  "ticket",
		Kind:   "reject",
		Model:  model,
		Length: length,
		Blocks: blocks,
		Result: "response_mismatch",
		Detail: detail,
	}
	if account != nil {
		event.AccountID = account.ID
		event.AccountName = account.Name
		event.ExpectedLength = openAICodexTicketExpectedLength(account)
		event.ExpectedBlocks = openAICodexTicketExpectedBlocks(account)
	}
	recordCodexHarvestFlow(event)
}

func recordCodexHarvestSelect(account *Account, model, kind, reason, detail string) {
	if kind == "skip" || kind == "failed" {
		key := fmt.Sprintf("%d\x00%s", 0, model)
		if account != nil {
			key = openAICodexTicketKey(account.ID, model)
		}
		now := time.Now()
		defaultCodexHarvestFlow.mu.Lock()
		if until, ok := defaultCodexHarvestFlow.skips[key]; ok && now.Before(until) {
			defaultCodexHarvestFlow.mu.Unlock()
			return
		}
		defaultCodexHarvestFlow.skips[key] = now.Add(codexHarvestFlowSkipDebounce)
		defaultCodexHarvestFlow.mu.Unlock()
	}
	event := CodexHarvestFlowEvent{
		Stage:  "select",
		Kind:   kind,
		Model:  model,
		Reason: reason,
		Detail: detail,
	}
	if account != nil {
		event.AccountID = account.ID
		event.AccountName = account.Name
	}
	recordCodexHarvestFlow(event)
}

func clipFlowText(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}

func watchCodexHarvestExit(string) func() string { return func() string { return "" } }

func observeCodexHarvestProxy(_ context.Context, proxyURL string) CodexHarvestFlowSidecar {
	if strings.TrimSpace(proxyURL) == "" {
		return CodexHarvestFlowSidecar{Mode: "unconfigured"}
	}
	return CodexHarvestFlowSidecar{Mode: "external"}
}
func BuildCodexHarvestFlow(ctx context.Context, cfg *config.Config, settings *SettingService, accounts []Account, controls ...*CodexHarvestService) CodexHarvestFlowSnapshot {
	now := time.Now()
	ticketCfg := config.OpenAICodexTicketConfig{}
	if cfg != nil {
		ticketCfg = cfg.Gateway.OpenAICodexTicket
	}
	if ticketCfg.TargetLength <= 0 {
		ticketCfg.TargetLength = 292
	}
	if ticketCfg.HarvestProbeIntervalSeconds < 30 {
		ticketCfg.HarvestProbeIntervalSeconds = 180
	}
	if ticketCfg.HarvestCooldownSeconds <= 0 {
		ticketCfg.HarvestCooldownSeconds = 180
	}
	if ticketCfg.MaxProbesPerRound <= 0 {
		ticketCfg.MaxProbesPerRound = 6
	}
	if ticketCfg.Models == nil {
		ticketCfg.Models = []string{openAICodexTicketDefaultModel, openAICodexTicketDefaultSolModel}
	}
	enabled := ticketCfg.Enabled
	failClosed := ticketCfg.FailClosed
	harvestProxy := strings.TrimSpace(ticketCfg.HarvestProxyURL)
	strategy := "standby"
	scopeError := false
	scope := CodexTicketHarvestScope{Mode: "all", AccountPolicy: CodexHarvestSchedulableOnly}
	if settings != nil {
		enabled = settings.GetOpenAICodexTicketEnabled(ctx, enabled)
		failClosed = settings.GetOpenAICodexTicketFailClosed(ctx)
		ticketCfg.Models = settings.GetOpenAICodexTicketModels(ctx, ticketCfg.Models)
		if proxy := settings.GetOpenAICodexTicketHarvestProxyURL(ctx); proxy != "" {
			harvestProxy = proxy
		}
		strategy = settings.GetCodexTicketStrategy(ctx)
		if current, err := settings.GetCodexTicketHarvestScope(ctx); err == nil {
			scope = current
		} else {
			scopeError = true
			scope.Mode = "selected"
			scope.GroupIDs = []int64{}
		}
	}
	policy := CodexHarvestControls{Transport: "sse", TargetGateway: "unified-95"}
	var runtime *CodexHarvestRuntime
	if len(controls) > 0 && controls[0] != nil {
		v, _, _ := controls[0].Controls(ctx)
		policy = v
		applyHarvestSpeed(&ticketCfg, v.Speed)
		state := controls[0].Runtime()
		runtime = &state
	}
	ticketCfg.Enabled = enabled
	ticketCfg.FailClosed = failClosed
	rawEvents := listCodexHarvestFlowEvents()
	events := make([]CodexHarvestFlowEvent, 0, len(rawEvents))
	for _, event := range rawEvents {
		if event.Stage == "select" {
			continue
		}
		events = append(events, event)
	}
	for i := range events {
		if strings.TrimSpace(events[i].Node) != "" {
			events[i].Node = strings.TrimSpace(events[i].Node)
		}
	}
	snapshot := CodexHarvestFlowSnapshot{
		Runtime:     runtime,
		GeneratedAt: now,
		Harvest: CodexHarvestFlowHarvest{
			Enabled:           enabled,
			FailClosed:        failClosed,
			Strategy:          strategy,
			ScopeMode:         scope.Mode,
			ScopeError:        scopeError,
			AccountPolicy:     scope.AccountPolicy,
			GroupIDs:          scope.GroupIDs,
			Models:            ticketCfg.Models,
			TargetLength:      ticketCfg.TargetLength,
			ProbeIntervalSec:  ticketCfg.HarvestProbeIntervalSeconds,
			CooldownSec:       ticketCfg.HarvestCooldownSeconds,
			MaxProbesPerRound: ticketCfg.MaxProbesPerRound,
			RefreshBeforeSec:  ticketCfg.RefreshBeforeSeconds,
			HarvestProxy:      MaskProxyURL(harvestProxy),
		},
		Sidecar:  observeCodexHarvestProxy(ctx, harvestProxy),
		Events:   events,
		Accounts: []CodexHarvestFlowAccount{},
	}
	for _, account := range accounts {
		if !isOpenAICodexTicketAccount(&account) {
			continue
		}
		availability, recoverAt := ResolveCodexAccountAvailability(&account, now)
		item := CodexHarvestFlowAccount{
			ID:           account.ID,
			Name:         account.Name,
			Status:       account.Status,
			Schedulable:  account.Schedulable,
			SkipHarvest:  openAICodexSkipHarvest(&account),
			InScope:      !openAICodexSkipHarvest(&account) && scope.includes(&account),
			Availability: availability,
			RecoverAt:    recoverAt,
			Tickets:      OpenAICodexTicketStatuses(&account, ticketCfg, now),
		}
		for i := range item.Tickets {
			ticket := &item.Tickets[i]
			if ticket.Length == 780 && (ticket.Transport != policy.Transport || !codex780GatewayAllowed(ticket.Gateway, policy.TargetGateway)) {
				ticket.Ready = false
				ticket.RemainingSeconds = 0
				ticket.ExpiresAt = nil
				ticket.StandbyExpiresAt = nil
				ticket.Blocked = ticketCfg.FailClosed && !item.SkipHarvest
			}
			if ticketCfg.FailClosed && !item.SkipHarvest && !item.InScope {
				ticket.Blocked = true
			}
			if ticket.Ready && !ticket.Blocked {
				item.ReadyCount++
				snapshot.Counts.TicketsReady++
			}
			if ticket.Blocked {
				item.BlockedCount++
				snapshot.Counts.TicketsBlocked++
			}
		}
		snapshot.Accounts = append(snapshot.Accounts, item)
	}
	for _, event := range snapshot.Events {
		switch event.Kind {
		case "probe_hit":
			snapshot.Counts.ProbeHit++
		case "probe_miss":
			snapshot.Counts.ProbeMiss++
		case "accept":
			snapshot.Counts.TicketAccept++
		case "reject":
			snapshot.Counts.TicketReject++
		case "selected":
			snapshot.Counts.SelectOK++
		case "skip":
			snapshot.Counts.SelectSkip++
		case "failed", "unavailable":
			snapshot.Counts.SelectFail++
		}
	}
	snapshot.Stages = buildCodexHarvestFlowStages(snapshot)
	snapshot.Events = reverseCodexHarvestFlowEvents(snapshot.Events)
	return snapshot
}

func reverseCodexHarvestFlowEvents(events []CodexHarvestFlowEvent) []CodexHarvestFlowEvent {
	if len(events) == 0 {
		return events
	}
	out := make([]CodexHarvestFlowEvent, len(events))
	for i := range events {
		out[len(events)-1-i] = events[i]
	}
	return out
}

func lastCodexHarvestNode() string {
	defaultCodexHarvestFlow.mu.Lock()
	defer defaultCodexHarvestFlow.mu.Unlock()
	return defaultCodexHarvestFlow.lastNow
}

func buildCodexHarvestFlowStages(snapshot CodexHarvestFlowSnapshot) []CodexHarvestFlowStage {
	last := map[string]CodexHarvestFlowEvent{}
	for _, event := range snapshot.Events {
		last[event.Stage+":"+event.Kind] = event
		last[event.Stage] = event
	}
	node := CodexHarvestFlowStage{ID: "node", Status: "idle"}
	switch {
	case snapshot.Sidecar.Mode == "external":
		node.Detail = "external_proxy"
	case snapshot.Sidecar.Mode == "unconfigured":
		node.Detail = "proxy_unconfigured"
	}
	// Show the probe attribution when it is available.
	if event, ok := last["probe"]; ok && event.Node != "" {
		at := event.At
		node.At, node.Node, node.Detail = &at, event.Node, event.Node
	}

	probe := stageFromEvent("probe", last["probe"], "waiting for harvest probe")
	if event, ok := last["probe"]; ok {
		copyFlowMetrics(&probe, event)
		if event.Kind == "probe_hit" {
			probe.Status = "ok"
		} else {
			probe.Status = "warn"
			if event.HTTPStatus >= 500 || event.Result == "token_error" {
				probe.Status = "fail"
			}
		}
	}

	shape := shapeStageFromEvents(last, snapshot.Counts.TicketsReady)

	ticket := CodexHarvestFlowStage{ID: "ticket", Status: "idle", Detail: "no stored ticket"}
	switch {
	case snapshot.Counts.TicketsReady > 0:
		ticket.Status = "ok"
		ticket.Detail = fmt.Sprintf("%d ready", snapshot.Counts.TicketsReady)
	case snapshot.Counts.TicketsBlocked > 0:
		ticket.Status = "fail"
		ticket.Detail = fmt.Sprintf("%d paused", snapshot.Counts.TicketsBlocked)
	}
	if event, ok := last["ticket"]; ok {
		at := event.At
		ticket.At = &at
		ticket.Model = event.Model
		copyFlowMetrics(&ticket, event)
		if event.Kind == "reject" && snapshot.Counts.TicketsReady == 0 {
			ticket.Status = "fail"
			ticket.Detail = "response rejected"
		} else if event.Kind == "accept" && ticket.Status != "fail" {
			ticket.Status = "ok"
			if snapshot.Counts.TicketsReady == 0 && event.Standby {
				ticket.Detail = "standby stored"
			}
		}
	}

	selectStage := stageFromEvent("select", last["select"], "waiting for assistant request")
	if event, ok := last["select"]; ok {
		copyFlowMetrics(&selectStage, event)
		switch event.Kind {
		case "selected":
			selectStage.Status = "ok"
		case "skip":
			selectStage.Status = "warn"
		default:
			selectStage.Status = "fail"
		}
	}
	return []CodexHarvestFlowStage{node, probe, shape, ticket, selectStage}
}

func shapeStageFromEvents(last map[string]CodexHarvestFlowEvent, ready int) CodexHarvestFlowStage {
	shape := CodexHarvestFlowStage{ID: "shape", Status: "idle", Detail: "waiting for ticket validation"}
	latest := last["probe"]
	hit := last["probe:probe_hit"]
	if flowEventEmpty(latest) && flowEventEmpty(hit) {
		return shape
	}
	if latest.Result != "invalid_state" && latest.Kind != "probe_hit" && latest.Length == latest.ExpectedLength && latest.Blocks == latest.ExpectedBlocks && latest.Length > 0 {
		at := latest.At
		shape.At, shape.Model = &at, latest.Model
		copyFlowMetrics(&shape, latest)
		shape.Status = "warn"
		shape.Detail = "ticket shape matches; validation incomplete"
		return shape
	}
	if (latest.HTTPStatus == 200 || latest.HTTPStatus == 101) && latest.Length > 0 && (latest.Kind != "probe_hit" || (latest.ExpectedLength > 0 && latest.Length != latest.ExpectedLength)) {
		at := latest.At
		shape.At = &at
		shape.Model = latest.Model
		copyFlowMetrics(&shape, latest)
		shape.Status = "fail"
		shape.Detail = fmt.Sprintf("got %d/%d want %d/%d", latest.Length, latest.Blocks, latest.ExpectedLength, latest.ExpectedBlocks)
		return shape
	}
	source := hit
	if flowEventEmpty(source) {
		source = latest
	}
	if source.Kind == "probe_hit" && (source.ExpectedLength == 0 || source.Length == source.ExpectedLength) {
		at := source.At
		shape.At = &at
		shape.Model = source.Model
		copyFlowMetrics(&shape, source)
		shape.Status = "ok"
		shape.Detail = fmt.Sprintf("%d / %d blk", source.Length, source.Blocks)
		return shape
	}
	if ready > 0 {
		shape.Status = "ok"
		shape.Detail = fmt.Sprintf("%d ready", ready)
		return shape
	}
	if !flowEventEmpty(latest) {
		at := latest.At
		shape.At = &at
		shape.Model = latest.Model
		copyFlowMetrics(&shape, latest)
		shape.Status = "warn"
		shape.Detail = "no ticket body"
	}
	return shape
}

func flowEventEmpty(event CodexHarvestFlowEvent) bool {
	return event.ID == "" && event.At.IsZero()
}

func stageFromEvent(id string, event CodexHarvestFlowEvent, idle string) CodexHarvestFlowStage {
	stage := CodexHarvestFlowStage{ID: id, Status: "idle", Detail: idle}
	if event.ID == "" && event.At.IsZero() {
		return stage
	}
	at := event.At
	stage.At = &at
	stage.Model = event.Model
	copyFlowMetrics(&stage, event)
	if event.Detail != "" {
		stage.Detail = event.Detail
	} else if event.Result != "" {
		stage.Detail = event.Result
	} else if event.Reason != "" {
		stage.Detail = event.Reason
	} else if event.AccountName != "" {
		stage.Detail = event.AccountName
	}
	return stage
}

func copyFlowMetrics(stage *CodexHarvestFlowStage, event CodexHarvestFlowEvent) {
	if stage == nil {
		return
	}
	if event.Node != "" {
		stage.Node = event.Node
	}
	stage.HTTPStatus = event.HTTPStatus
	stage.Length = event.Length
	stage.Blocks = event.Blocks
	stage.ExpectedLength = event.ExpectedLength
	stage.ExpectedBlocks = event.ExpectedBlocks
}

func describeCodexHarvestOutcome(kind, raw string, status, length, blocks, expectedLen, expectedBlk int, model, node string) (message, level, detail string) {
	if kind == "model_mismatch" {
		_, _, detail = describeCodexProbeFailure(raw, status, model, node)
		return "上游模型声明与请求不一致，门票未入库", "WARN", detail
	}
	if kind == "invalid_route" {
		_, _, detail = describeCodexProbeFailure(raw, status, model, node)
		return fmt.Sprintf("已收到 %d 字节票体，但路由 Cookie 验收失败，未入库", length), "WARN", detail
	}
	if kind == "success" {
		detailParts := []string{fmt.Sprintf("len=%d blk=%d", length, blocks)}
		if node != "" {
			detailParts = append(detailParts, "node="+node)
		}
		return fmt.Sprintf("成功捕获合规门票（%d 字节 / %d 块）", length, blocks), "OK", strings.Join(detailParts, " · ")
	}
	if kind == "invalid_state" && (blocks == 11 || blocks == 13 || length == 312 || length == 356) {
		_, _, detail = describeCodexProbeFailure(raw, status, model, node)
		return fmt.Sprintf("拿到的是降智票据（%d 字节 / %d 块），已拒收 → 换节点重试", length, blocks), "WARN",
			fmt.Sprintf("%s · 合规应为 %d/%d", detail, expectedLen, expectedBlk)
	}
	if kind == "rate_limited" || status == http.StatusTooManyRequests {
		_, _, detail = describeCodexProbeFailure(raw, status, model, node)
		return "上游限流了，进入冷静期后自动继续", "WARN", detail
	}
	return describeCodexProbeFailure(raw, status, model, node)
}
