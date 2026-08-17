package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const (
	autoSupplyOrderMarkerKey = "auto_supply_order_item"
	autoSupplyDefaultProduct = "oauth_30d"
	autoSupplyDefaultType    = AccountTypeOAuth
	autoSupplyMaxJSONBytes   = 8 << 20
	autoSupplyMaxBundleBytes = 64 << 20
)

// AutoSupplyService replenishes configured groups from the customer-token
// account supplier. It keeps order state in memory while using an hourly
// idempotency key so a process restart cannot create a duplicate order.
type AutoSupplyService struct {
	accountRepo AccountRepository
	admin       AutoSupplyAdmin
	cfg         *config.Config
	client      *http.Client
	settingRepo SettingRepository
	encryptor   SecretEncryptor

	settingsMu      sync.RWMutex
	runtimeSettings config.AutoSupplyConfig
	settingsVersion string
	wakeCh          chan struct{}
	startOnce       sync.Once

	mu       sync.Mutex
	runMu    sync.Mutex
	orders   map[int64]*autoSupplyOrder
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
	retryAt  time.Time
}

// AutoSupplyAdmin is the small slice of admin operations needed by the worker.
// Keeping this contract narrow makes the replenishment flow independently
// testable and avoids coupling it to unrelated admin features.
type AutoSupplyAdmin interface {
	GetGroup(ctx context.Context, id int64) (*Group, error)
	CreateAccount(ctx context.Context, input *CreateAccountInput) (*Account, error)
}

type autoSupplyOrder struct {
	ID       string
	GroupID  int64
	Product  string
	Quantity int
}

type autoSupplyInventoryResponse struct {
	Available       int  `json:"available"`
	Missing         int  `json:"missing"`
	NeedsProduction bool `json:"needs_production"`
}

type autoSupplyOrderResponse struct {
	OrderID string `json:"order_id"`
	ID      string `json:"id"`
	Status  string `json:"status"`
	State   string `json:"state"`
}

type autoSupplyOrderStatus struct {
	OrderID string `json:"order_id"`
	ID      string `json:"id"`
	Status  string `json:"status"`
	State   string `json:"state"`
}

type autoSupplyBundle struct {
	Accounts []json.RawMessage `json:"accounts"`
}

type autoSupplyAccount struct {
	Name               string          `json:"name"`
	Notes              *string         `json:"notes"`
	Platform           string          `json:"platform"`
	Type               string          `json:"type"`
	Credentials        map[string]any  `json:"credentials"`
	Extra              map[string]any  `json:"extra"`
	Concurrency        int             `json:"concurrency"`
	Priority           int             `json:"priority"`
	IsFallback         bool            `json:"is_fallback"`
	RateMultiplier     *float64        `json:"rate_multiplier"`
	ExpiresAt          json.RawMessage `json:"expires_at"`
	AutoPauseOnExpired *bool           `json:"auto_pause_on_expired"`
}

// NewAutoSupplyService creates the replenishment worker. The worker remains
// inert until auto_supply.enabled and a customer token are configured.
func NewAutoSupplyService(accountRepo AccountRepository, admin AutoSupplyAdmin, cfg *config.Config) *AutoSupplyService {
	runtimeSettings := config.AutoSupplyConfig{}
	if cfg != nil {
		runtimeSettings = cfg.AutoSupply
	}
	runtimeSettings = normalizeAutoSupplyConfig(runtimeSettings)
	return &AutoSupplyService{
		accountRepo:     accountRepo,
		admin:           admin,
		cfg:             cfg,
		client:          &http.Client{},
		runtimeSettings: runtimeSettings,
		settingsVersion: autoSupplyFallbackVersion,
		wakeCh:          make(chan struct{}, 1),
		orders:          make(map[int64]*autoSupplyOrder),
		stopCh:          make(chan struct{}),
	}
}

// Start starts the periodic replenishment scan.
func (s *AutoSupplyService) Start() {
	if s == nil {
		return
	}
	s.startOnce.Do(func() {
		if _, err := s.reloadRuntimeSettings(context.Background()); err != nil {
			slog.Warn("auto_supply_settings_load_failed", "error", err)
		}
		s.wg.Add(1)
		go s.run()
	})
}

func (s *AutoSupplyService) run() {
	defer s.wg.Done()
	reloadTicker := time.NewTicker(autoSupplyReloadInterval)
	defer reloadTicker.Stop()
	timer := time.NewTimer(0)
	defer timer.Stop()

	resetTimer := func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		settings := s.currentAutoSupplyConfig()
		timer.Reset(time.Duration(settings.IntervalSeconds) * time.Second)
	}

	for {
		select {
		case <-timer.C:
			s.RunOnce(context.Background())
			resetTimer()
		case <-s.wakeCh:
			s.RunOnce(context.Background())
			resetTimer()
		case <-reloadTicker.C:
			changed, err := s.reloadRuntimeSettings(context.Background())
			if err != nil {
				slog.Warn("auto_supply_settings_reload_failed", "error", err)
				continue
			}
			if changed {
				s.RunOnce(context.Background())
				resetTimer()
			}
		case <-s.stopCh:
			return
		}
	}
}

// Stop stops the worker and waits for an in-flight scan to finish.
func (s *AutoSupplyService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() { close(s.stopCh) })
	s.wg.Wait()
}

// RunOnce performs one bounded scan. It is exported for operational checks
// and unit tests; scheduled execution calls the same method.
func (s *AutoSupplyService) RunOnce(ctx context.Context) {
	if s == nil || s.accountRepo == nil || s.admin == nil {
		return
	}
	settings := s.currentAutoSupplyConfig()
	if !autoSupplyEnabled(settings) {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if s.isRetryDeferred() {
		return
	}
	s.runMu.Lock()
	defer s.runMu.Unlock()

	for _, groupCfg := range settings.Groups {
		if groupCfg.GroupID <= 0 {
			continue
		}
		groupCtx := ctx
		cancel := func() {}
		if timeoutSeconds := settings.RequestTimeoutSeconds; timeoutSeconds > 0 {
			groupCtx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
		}
		if err := s.replenishGroup(groupCtx, settings, groupCfg); err != nil {
			slog.Warn("auto_supply_group_failed", "group_id", groupCfg.GroupID, "error", err)
		}
		cancel()
	}
}

func (s *AutoSupplyService) replenishGroup(ctx context.Context, settings config.AutoSupplyConfig, groupCfg config.AutoSupplyGroupConfig) error {
	group, err := s.admin.GetGroup(ctx, groupCfg.GroupID)
	if err != nil {
		return fmt.Errorf("load group: %w", err)
	}
	if group == nil || group.ID <= 0 {
		return errors.New("group is unavailable")
	}

	platform := strings.TrimSpace(groupCfg.Platform)
	if platform == "" {
		platform = strings.TrimSpace(group.Platform)
	}
	if platform == "" {
		return errors.New("group platform is empty")
	}
	accounts, err := s.accountRepo.ListSchedulableByGroupIDAndPlatform(ctx, group.ID, platform)
	if err != nil {
		return fmt.Errorf("count schedulable accounts: %w", err)
	}
	minimum := groupCfg.MinAvailable
	if minimum < 0 {
		minimum = 0
	}
	if len(accounts) >= minimum {
		s.clearOrder(group.ID)
		return nil
	}

	if order := s.getActiveOrder(group.ID); order != nil {
		return s.pollAndImport(ctx, settings, group, groupCfg, order, platform)
	}

	quantity := groupCfg.Quantity
	if quantity <= 0 {
		quantity = minimum - len(accounts)
	}
	if quantity <= 0 {
		quantity = 1
	}
	if maximum := settings.MaxQuantityPerRun; maximum > 0 && quantity > maximum {
		quantity = maximum
	}
	product := strings.TrimSpace(groupCfg.Product)
	if product == "" {
		product = autoSupplyDefaultProduct
	}
	if _, err := s.getInventory(ctx, settings, product, quantity); err != nil {
		return err
	}

	orderID, err := s.createOrder(ctx, settings, group.ID, product, quantity)
	if err != nil {
		return err
	}
	order := &autoSupplyOrder{ID: orderID, GroupID: group.ID, Product: product, Quantity: quantity}
	s.setOrder(group.ID, order)
	slog.Info("auto_supply_order_created", "group_id", group.ID, "quantity", quantity, "order_id", orderID)
	return nil
}

func (s *AutoSupplyService) pollAndImport(ctx context.Context, settings config.AutoSupplyConfig, group *Group, groupCfg config.AutoSupplyGroupConfig, order *autoSupplyOrder, platform string) error {
	state, err := s.getOrder(ctx, settings, order.ID)
	if err != nil {
		return err
	}
	switch normalizeAutoSupplyState(state.Status, state.State) {
	case "completed":
		bundle, err := s.downloadBundle(ctx, settings, order.ID)
		if err != nil {
			return err
		}
		if err := s.importBundle(ctx, group, groupCfg, order.ID, bundle, platform); err != nil {
			return err
		}
		s.clearOrder(group.ID)
		slog.Info("auto_supply_order_imported", "group_id", group.ID, "order_id", order.ID)
	case "failed", "cancelled", "expired", "rejected":
		s.clearOrder(group.ID)
		return fmt.Errorf("upstream order %s ended with state %s", order.ID, normalizeAutoSupplyState(state.Status, state.State))
	}
	return nil
}

func normalizeAutoSupplyState(status, state string) string {
	value := strings.ToLower(strings.TrimSpace(status))
	if value == "" {
		value = strings.ToLower(strings.TrimSpace(state))
	}
	switch value {
	case "completed", "complete", "delivered", "fulfilled", "done", "success":
		return "completed"
	case "failed", "error":
		return "failed"
	case "cancelled", "canceled":
		return "cancelled"
	case "expired", "timeout", "timed_out":
		return "expired"
	case "rejected":
		return "rejected"
	default:
		return value
	}
}

func (s *AutoSupplyService) getInventory(ctx context.Context, settings config.AutoSupplyConfig, product string, quantity int) (*autoSupplyInventoryResponse, error) {
	endpoint, err := s.buildEndpoint(settings, "/api/customer/inventory")
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	query.Set("product", product)
	query.Set("quantity", strconv.Itoa(quantity))
	endpoint.RawQuery = query.Encode()
	var response autoSupplyInventoryResponse
	if err := s.doJSON(ctx, settings, http.MethodGet, endpoint.String(), nil, "", &response); err != nil {
		return nil, fmt.Errorf("query inventory: %w", err)
	}
	return &response, nil
}

func (s *AutoSupplyService) createOrder(ctx context.Context, settings config.AutoSupplyConfig, groupID int64, product string, quantity int) (string, error) {
	endpoint, err := s.buildEndpoint(settings, "/api/customer/pickup/orders")
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(map[string]any{"product": product, "quantity": quantity})
	if err != nil {
		return "", err
	}
	// The hour bucket is stable across restarts and still permits a new order
	// after the previous bucket has been fulfilled.
	idempotencyKey := fmt.Sprintf("sub2api-auto-supply-g%d-%d", groupID, time.Now().UTC().Unix()/3600)
	var response autoSupplyOrderResponse
	if err := s.doJSON(ctx, settings, http.MethodPost, endpoint.String(), body, idempotencyKey, &response); err != nil {
		return "", fmt.Errorf("create pickup order: %w", err)
	}
	orderID := strings.TrimSpace(response.OrderID)
	if orderID == "" {
		orderID = strings.TrimSpace(response.ID)
	}
	if orderID == "" {
		return "", errors.New("pickup order response has no order_id")
	}
	return orderID, nil
}

func (s *AutoSupplyService) getOrder(ctx context.Context, settings config.AutoSupplyConfig, orderID string) (*autoSupplyOrderStatus, error) {
	endpoint, err := s.buildEndpoint(settings, "/api/customer/pickup/orders/"+url.PathEscape(orderID))
	if err != nil {
		return nil, err
	}
	var response autoSupplyOrderStatus
	if err := s.doJSON(ctx, settings, http.MethodGet, endpoint.String(), nil, "", &response); err != nil {
		return nil, fmt.Errorf("query pickup order: %w", err)
	}
	return &response, nil
}

func (s *AutoSupplyService) downloadBundle(ctx context.Context, settings config.AutoSupplyConfig, orderID string) (*autoSupplyBundle, error) {
	endpoint, err := s.buildEndpoint(settings, "/api/customer/pickup/orders/"+url.PathEscape(orderID)+"/download")
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	query.Set("format", "sub2")
	endpoint.RawQuery = query.Encode()
	data, err := s.doBytes(ctx, settings, http.MethodGet, endpoint.String(), nil, "")
	if err != nil {
		return nil, fmt.Errorf("download pickup bundle: %w", err)
	}
	var bundle autoSupplyBundle
	if err := json.Unmarshal(data, &bundle); err != nil {
		return nil, fmt.Errorf("decode pickup bundle: %w", err)
	}
	return &bundle, nil
}

func (s *AutoSupplyService) importBundle(ctx context.Context, group *Group, groupCfg config.AutoSupplyGroupConfig, orderID string, bundle *autoSupplyBundle, platform string) error {
	if bundle == nil || len(bundle.Accounts) == 0 {
		return errors.New("pickup bundle has no accounts")
	}
	accountType := strings.TrimSpace(groupCfg.AccountType)
	if accountType == "" {
		accountType = autoSupplyDefaultType
	}
	deploymentGroupIDs := autoSupplyDeploymentGroupIDs(group.ID, groupCfg.DeployGroupIDs)
	imported := 0
	failed := 0
	for index, raw := range bundle.Accounts {
		marker := fmt.Sprintf("%s:%d", orderID, index)
		existing, err := s.accountRepo.FindByExtraField(ctx, autoSupplyOrderMarkerKey, marker)
		if err != nil {
			return fmt.Errorf("check imported account %d: %w", index, err)
		}
		if len(existing) > 0 {
			continue
		}

		item, err := decodeAutoSupplyAccount(raw)
		if err != nil {
			failed++
			continue
		}
		itemPlatform := strings.TrimSpace(item.Platform)
		if itemPlatform == "" {
			itemPlatform = platform
		}
		if itemPlatform != platform {
			failed++
			continue
		}
		if len(item.Credentials) == 0 {
			failed++
			continue
		}
		itemType := strings.TrimSpace(item.Type)
		if itemType == "" {
			itemType = accountType
		}
		extra := cloneAutoSupplyMap(item.Extra)
		if extra == nil {
			extra = make(map[string]any, 1)
		}
		extra[autoSupplyOrderMarkerKey] = marker
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = fmt.Sprintf("auto-supply-%s-%d", orderID, index+1)
		}
		expiresAt, err := parseAutoSupplyUnix(item.ExpiresAt)
		if err != nil {
			failed++
			continue
		}
		priority := item.Priority
		if priority == 0 {
			priority = groupCfg.Priority
		}
		concurrency := item.Concurrency
		if concurrency == 0 {
			concurrency = groupCfg.Concurrency
		}
		_, err = s.admin.CreateAccount(ctx, &CreateAccountInput{
			Name:                  name,
			Notes:                 item.Notes,
			Platform:              itemPlatform,
			Type:                  itemType,
			Credentials:           item.Credentials,
			Extra:                 extra,
			Concurrency:           concurrency,
			Priority:              priority,
			IsFallback:            item.IsFallback,
			RateMultiplier:        item.RateMultiplier,
			GroupIDs:              append([]int64(nil), deploymentGroupIDs...),
			ExpiresAt:             expiresAt,
			AutoPauseOnExpired:    item.AutoPauseOnExpired,
			SkipDefaultGroupBind:  true,
			SkipMixedChannelCheck: true,
		})
		if err != nil {
			failed++
			continue
		}
		imported++
	}
	if failed > 0 {
		return fmt.Errorf("imported %d accounts, %d accounts failed", imported, failed)
	}
	return nil
}

func autoSupplyDeploymentGroupIDs(triggerGroupID int64, deployGroupIDs []int64) []int64 {
	result := make([]int64, 0, len(deployGroupIDs)+1)
	seen := make(map[int64]struct{}, len(deployGroupIDs)+1)
	for _, groupID := range append([]int64{triggerGroupID}, deployGroupIDs...) {
		if groupID <= 0 {
			continue
		}
		if _, exists := seen[groupID]; exists {
			continue
		}
		seen[groupID] = struct{}{}
		result = append(result, groupID)
	}
	return result
}

func decodeAutoSupplyAccount(raw json.RawMessage) (*autoSupplyAccount, error) {
	var item autoSupplyAccount
	if err := json.Unmarshal(raw, &item); err != nil {
		return nil, err
	}
	if len(item.Credentials) > 0 {
		return &item, nil
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, err
	}
	credentialKeys := map[string]string{
		"access_token":       "access_token",
		"accessToken":        "access_token",
		"refresh_token":      "refresh_token",
		"refreshToken":       "refresh_token",
		"id_token":           "id_token",
		"idToken":            "id_token",
		"session_token":      "session_token",
		"sessionToken":       "session_token",
		"api_key":            "api_key",
		"apiKey":             "api_key",
		"token":              "token",
		"account_id":         "account_id",
		"accountId":          "account_id",
		"chatgpt_account_id": "chatgpt_account_id",
		"chatgpt_user_id":    "chatgpt_user_id",
		"organization_id":    "organization_id",
		"organizationId":     "organization_id",
		"org_id":             "org_id",
		"user_id":            "user_id",
		"userId":             "user_id",
		"email":              "email",
	}
	for sourceKey, targetKey := range credentialKeys {
		if value, ok := object[sourceKey]; ok {
			if item.Credentials == nil {
				item.Credentials = make(map[string]any)
			}
			item.Credentials[targetKey] = value
		}
	}
	return &item, nil
}

func cloneAutoSupplyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	output := make(map[string]any, len(input)+1)
	for key, value := range input {
		output[key] = value
	}
	return output
}

func parseAutoSupplyUnix(raw json.RawMessage) (*int64, error) {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return nil, nil
	}
	var number float64
	if err := json.Unmarshal(raw, &number); err == nil {
		value := int64(number)
		if value > 0 {
			return &value, nil
		}
		return nil, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return nil, fmt.Errorf("invalid expires_at")
	}
	text = strings.TrimSpace(text)
	if numeric, err := strconv.ParseInt(text, 10, 64); err == nil {
		if numeric > 0 {
			return &numeric, nil
		}
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return nil, fmt.Errorf("invalid expires_at")
	}
	value := parsed.Unix()
	return &value, nil
}

func (s *AutoSupplyService) buildEndpoint(settings config.AutoSupplyConfig, path string) (*url.URL, error) {
	base := strings.TrimRight(strings.TrimSpace(settings.BaseURL), "/")
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("auto supply base_url must be an absolute URL")
	}
	if parsed.User != nil {
		return nil, errors.New("auto supply base_url cannot contain user info")
	}
	if parsed.Scheme != "https" {
		host := strings.ToLower(parsed.Hostname())
		if host != "localhost" && host != "127.0.0.1" && host != "::1" {
			return nil, errors.New("auto supply base_url must use HTTPS")
		}
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/" + strings.TrimLeft(path, "/")
	parsed.RawQuery = ""
	return parsed, nil
}

func (s *AutoSupplyService) doJSON(ctx context.Context, settings config.AutoSupplyConfig, method, endpoint string, body []byte, idempotencyKey string, target any) error {
	data, err := s.doBytes(ctx, settings, method, endpoint, body, idempotencyKey)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode upstream response: %w", err)
	}
	return nil
}

func (s *AutoSupplyService) doBytes(ctx context.Context, settings config.AutoSupplyConfig, method, endpoint string, body []byte, idempotencyKey string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-Customer-Token", strings.TrimSpace(settings.CustomerToken))
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(idempotencyKey) != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	client := s.client
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	limit := int64(autoSupplyMaxJSONBytes)
	if strings.Contains(strings.ToLower(response.Header.Get("Content-Disposition")), ".zip") || strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "zip") {
		limit = autoSupplyMaxBundleBytes
	}
	data, readErr := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if readErr != nil {
		return nil, readErr
	}
	if int64(len(data)) > limit {
		return nil, errors.New("upstream response exceeds size limit")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if response.StatusCode == http.StatusTooManyRequests || response.StatusCode == http.StatusServiceUnavailable {
			s.deferRetry(response.Header.Get("Retry-After"))
		}
		return nil, fmt.Errorf("upstream returned HTTP %d", response.StatusCode)
	}
	return data, nil
}

func (s *AutoSupplyService) isRetryDeferred() bool {
	s.mu.Lock()
	deferred := !s.retryAt.IsZero() && time.Now().Before(s.retryAt)
	s.mu.Unlock()
	return deferred
}

func (s *AutoSupplyService) deferRetry(value string) {
	delay := parseAutoSupplyRetryAfter(value, time.Now())
	if delay <= 0 {
		return
	}
	if delay > 5*time.Minute {
		delay = 5 * time.Minute
	}
	s.mu.Lock()
	deferredUntil := time.Now().Add(delay)
	if deferredUntil.After(s.retryAt) {
		s.retryAt = deferredUntil
	}
	s.mu.Unlock()
}

func parseAutoSupplyRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds <= 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	when, err := http.ParseTime(value)
	if err != nil || !when.After(now) {
		return 0
	}
	return when.Sub(now)
}

func (s *AutoSupplyService) getActiveOrder(groupID int64) *autoSupplyOrder {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.orders[groupID]
}

func (s *AutoSupplyService) setOrder(groupID int64, order *autoSupplyOrder) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.orders[groupID] = order
}

func (s *AutoSupplyService) clearOrder(groupID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.orders, groupID)
}
