package service

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type codexHarvestRoundKey struct{}
type codexHarvestRound struct {
	limit   int
	used    atomic.Int64
	stopped sync.Map
}

func (r *codexHarvestRound) AccountStopped(id int64) bool {
	_, stopped := r.stopped.Load(id)
	return stopped
}

func (r *codexHarvestRound) Used() int { return int(r.used.Load()) }

func (r *codexHarvestRound) Take(limit int) bool {
	for {
		used := r.used.Load()
		if used >= int64(min(r.limit, limit)) {
			return false
		}
		if r.used.CompareAndSwap(used, used+1) {
			return true
		}
	}
}

type codexHarvestAttempt struct {
	node    HarvestNode
	proxy   string
	release func()
}

func (s *OpenAIGatewayService) harvestControls(ctx context.Context) (CodexHarvestControls, bool) {
	if s.codexHarvest != nil {
		v, saved, err := s.codexHarvest.Controls(ctx)
		if err != nil {
			s.codexHarvest.degrade("settings unavailable; retaining last valid speed")
		}
		return v, saved
	}
	cfg := s.openAICodexTicketConfig()
	return CodexHarvestControls{Version: 1, Speed: CodexHarvestSpeed{
		RoundIntervalSeconds:  cfg.HarvestProbeIntervalSeconds,
		ProbeIntervalSeconds:  0,
		AttemptTimeoutSeconds: cfg.HarvestAttemptTimeoutSeconds,
		CooldownSeconds:       cfg.HarvestCooldownSeconds,
		MaxRequestsPerRound:   cfg.MaxProbesPerRound,
		MaxNodeAttempts:       1,
		RefreshBeforeSeconds:  cfg.RefreshBeforeSeconds,
	}}, false
}

func (s *OpenAIGatewayService) harvestTicketConfig(ctx context.Context) config.OpenAICodexTicketConfig {
	cfg := s.openAICodexTicketConfig()
	controls, _ := s.harvestControls(ctx)
	applyHarvestSpeed(&cfg, controls.Speed)
	if cfg.TargetLength == 780 && cfg.RefreshBeforeSeconds > 60 {
		cfg.RefreshBeforeSeconds = 60
	}
	return cfg
}

func (s *OpenAIGatewayService) freshHarvestAccount(ctx context.Context, account *Account, model string) (*Account, bool) {
	if ctx.Err() != nil || account == nil || openAICodexSkipHarvest(account) || !s.openAICodexTicketEnabledContext(ctx) {
		return nil, false
	}
	if s.codexHarvest != nil {
		fresh, err := s.accountRepo.GetByID(ctx, account.ID)
		if err != nil || fresh == nil || ticketIdentity(fresh) != ticketIdentity(account) {
			return nil, false
		}
		account = fresh
		scope, err := s.settingService.GetCodexTicketHarvestScope(ctx)
		if err != nil || !scope.includes(account) || !scope.allowsAccount(account) {
			return nil, false
		}
	}
	if !isOpenAICodexTicketAccount(account) || account.IsRateLimited() || openAICodexSkipHarvest(account) || s.codexTicketChatHeld(account.ID) || s.ticketProbeCoolingDown(account.ID, model, time.Now()) {
		return nil, false
	}
	cfg := s.openAICodexTicketConfig()
	found := false
	for _, m := range cfg.Models {
		if normalizeOpenAICodexTicketModel(m) == model {
			found = true
		}
	}
	if !found {
		return nil, false
	}
	if !s.codexHarvestNeedsTicket(account, model, s.harvestTicketConfig(ctx)) {
		return nil, false
	}
	return account, true
}

func (s *OpenAIGatewayService) codexHarvestNeedsTicket(account *Account, model string, cfg config.OpenAICodexTicketConfig) bool {
	refresh := time.Duration(cfg.RefreshBeforeSeconds) * time.Second
	if s.settingService.GetCodexTicketStrategy(context.Background()) == "fixed" {
		refresh = 0
	}
	now := time.Now()
	ticket := s.lookupOpenAICodexTicket(account, model)
	target := openAICodexTicketTargetLength(account, cfg)
	if ticket.valid(now, target) && !ticket.needsRefresh(now, refresh) {
		return false
	}
	return ticket == nil || !ticket.Standby.valid(now, target) || ticket.Standby.needsRefresh(now, refresh)
}

func (s *OpenAIGatewayService) prepareHarvestAttempt(_ context.Context, account *Account, model, proxy string, _ map[string]bool, _ CodexHarvestControls) (codexHarvestAttempt, bool) {
	attempt := codexHarvestAttempt{proxy: proxy, release: func() {}}
	pinned := s.lookupOpenAICodexTicket(account, model)
	if pinned != nil && pinned.HarvestNodeID == "" && pinned.HarvestNodeName == "" && strings.TrimSpace(pinned.HarvestProxyURL) != "" {
		attempt.proxy = pinned.HarvestProxyURL
	}
	return attempt, true
}

func (s *OpenAIGatewayService) waitHarvestPace(ctx context.Context, _ CodexHarvestControls, configured bool) bool {
	if s.codexHarvest == nil || !configured {
		return ctx.Err() == nil
	}
	for {
		if ctx.Err() != nil {
			return false
		}
		current, stillConfigured := s.harvestControls(ctx)
		if !stillConfigured {
			return true
		}
		s.codexHarvest.runtimeMu.Lock()
		delay := time.Until(s.codexHarvest.lastRequest.Add(time.Duration(current.Speed.ProbeIntervalSeconds) * time.Second))
		s.codexHarvest.runtimeMu.Unlock()
		if delay <= 0 {
			return true
		}
		if delay > 250*time.Millisecond {
			delay = 250 * time.Millisecond
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return false
		case <-timer.C:
		}
	}
}

func (s *OpenAIGatewayService) reserveHarvestRequest(ctx context.Context, account *Account, model string, round *codexHarvestRound, learning bool) bool {
	controls, _ := s.harvestControls(ctx)
	if learning && !controls.NodeMemoryEnabled {
		return false
	}
	if _, ok := s.freshHarvestAccount(ctx, account, model); !ok {
		return false
	}
	if round.AccountStopped(account.ID) || !round.Take(controls.Speed.MaxRequestsPerRound) {
		return false
	}
	if s.codexHarvest != nil {
		s.codexHarvest.runtimeMu.Lock()
		s.codexHarvest.lastRequest = time.Now()
		s.codexHarvest.runtime.RequestsUsed = round.Used()
		s.codexHarvest.runtime.RequestBudget = min(round.limit, controls.Speed.MaxRequestsPerRound)
		s.codexHarvest.runtimeMu.Unlock()
	}
	return true
}

func (s *OpenAIGatewayService) finishHarvestAttempt(_ context.Context, attempt codexHarvestAttempt, _ codexHarvestProbeResult, _ time.Duration, _ CodexHarvestControls) {
	attempt.release()
}
