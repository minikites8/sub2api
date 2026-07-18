package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

const (
	quotaLeaseDemoMirrorNodeIDExtraKey   = "quota_lease_demo_mirror_node_id"
	quotaLeaseDemoMirrorSyncedAtExtraKey = "quota_lease_demo_mirror_synced_at"
	quotaLeaseDemoMirrorFlagExtraKey     = "quota_lease_demo_mirror"
)

type quotaLeaseDemoMirrorStore struct {
	client      *dbent.Client
	sql         sqlExecutor
	accountRepo service.AccountRepository
}

func NewQuotaLeaseDemoMirrorStore(client *dbent.Client, sqlDB *sql.DB, accountRepo service.AccountRepository) service.QuotaLeaseDemoMirrorStore {
	if client == nil {
		return nil
	}
	return &quotaLeaseDemoMirrorStore{
		client:      client,
		sql:         sqlDB,
		accountRepo: accountRepo,
	}
}

func (s *quotaLeaseDemoMirrorStore) ApplySnapshot(ctx context.Context, snapshot service.QuotaLeaseDemoMirrorSnapshot) error {
	if s == nil || s.client == nil {
		return nil
	}
	snapshot.NodeID = strings.TrimSpace(snapshot.NodeID)
	if snapshot.SyncedAt.IsZero() {
		snapshot.SyncedAt = time.Now().UTC()
	} else {
		snapshot.SyncedAt = snapshot.SyncedAt.UTC()
	}

	groups := cloneQuotaLeaseDemoMirrorGroups(snapshot.Groups)
	proxies := cloneQuotaLeaseDemoMirrorProxies(snapshot.Proxies)
	accounts := cloneQuotaLeaseDemoMirrorAccounts(snapshot.Accounts, snapshot)
	accountGroups := cloneQuotaLeaseDemoMirrorAccountGroups(snapshot.AccountGroups, accounts)

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	exec := tx.Client()
	if err := s.upsertGroups(ctx, exec, groups); err != nil {
		return err
	}
	if err := s.upsertProxies(ctx, exec, proxies); err != nil {
		return err
	}
	if err := s.upsertAccounts(ctx, exec, accounts, snapshot); err != nil {
		return err
	}
	if err := s.reconcileMirrorAccountGroups(ctx, exec, accounts, accountGroups, snapshot.NodeID); err != nil {
		return err
	}
	if err := s.reconcileMirrorAccountParents(ctx, exec, accounts); err != nil {
		return err
	}
	if err := s.reconcileMirrorAccounts(ctx, exec, accounts, snapshot.NodeID); err != nil {
		return err
	}
	if err := s.bumpSequences(ctx, exec); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *quotaLeaseDemoMirrorStore) UpsertAccount(ctx context.Context, account service.QuotaLeaseDemoAccountSnapshot) error {
	if s == nil || s.client == nil || account.ID <= 0 {
		return nil
	}
	account.GroupIDs = nil
	account.AccountGroups = nil
	snapshot := service.QuotaLeaseDemoMirrorSnapshot{
		NodeID:   quotaLeaseDemoMirrorExtraString(account.Extra, "node_oauth_assigned_node_id"),
		SyncedAt: time.Now().UTC(),
		Accounts: []service.QuotaLeaseDemoAccountSnapshot{account},
	}
	if snapshot.NodeID == "" {
		snapshot.NodeID = quotaLeaseDemoMirrorExtraString(account.Extra, quotaLeaseDemoMirrorNodeIDExtraKey)
	}
	if account.Proxy != nil {
		snapshot.Proxies = []service.QuotaLeaseDemoProxySnapshot{*account.Proxy}
	}
	return s.ApplySnapshot(ctx, snapshot)
}

func (s *quotaLeaseDemoMirrorStore) ListSchedulableAccounts(ctx context.Context, groupID *int64, platform string) ([]service.Account, error) {
	if s == nil || s.accountRepo == nil {
		return nil, nil
	}
	platform = strings.TrimSpace(platform)

	var (
		accounts []service.Account
		err      error
	)
	switch {
	case groupID != nil && platform != "":
		accounts, err = s.accountRepo.ListSchedulableByGroupIDAndPlatform(ctx, *groupID, platform)
	case groupID != nil:
		accounts, err = s.accountRepo.ListSchedulableByGroupID(ctx, *groupID)
	case platform != "":
		accounts, err = s.accountRepo.ListSchedulableUngroupedByPlatform(ctx, platform)
	default:
		accounts, err = s.accountRepo.ListSchedulable(ctx)
	}
	if err != nil {
		return nil, err
	}
	return filterMirrorSchedulableAccounts(accounts, groupID, platform), nil
}

func (s *quotaLeaseDemoMirrorStore) GetAccountByID(ctx context.Context, accountID int64) (*service.Account, error) {
	if s == nil || s.accountRepo == nil || accountID <= 0 {
		return nil, nil
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil {
		return nil, err
	}
	if !quotaLeaseDemoMirrorAccountVisible(*account) {
		return nil, nil
	}
	return account, nil
}

func (s *quotaLeaseDemoMirrorStore) upsertGroups(ctx context.Context, exec sqlExecutor, groups []service.Group) error {
	if len(groups) == 0 {
		return nil
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].ID < groups[j].ID
	})
	for _, group := range groups {
		if err := s.upsertGroup(ctx, exec, group); err != nil {
			return err
		}
	}
	return nil
}

func (s *quotaLeaseDemoMirrorStore) upsertGroup(ctx context.Context, exec sqlExecutor, group service.Group) error {
	modelRouting := group.ModelRouting
	if modelRouting == nil {
		modelRouting = make(map[string][]int64)
	}
	payload, err := json.Marshal(modelRouting)
	if err != nil {
		return err
	}
	supportedModelScopes := group.SupportedModelScopes
	if supportedModelScopes == nil {
		supportedModelScopes = []string{}
	}
	scopes, err := json.Marshal(supportedModelScopes)
	if err != nil {
		return err
	}
	messagesDispatchConfig, err := json.Marshal(group.MessagesDispatchModelConfig)
	if err != nil {
		return err
	}
	modelsListConfig, err := json.Marshal(group.ModelsListConfig)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, `
		INSERT INTO groups (
			id, name, description, platform, rate_multiplier, peak_rate_enabled, peak_start, peak_end,
			peak_rate_multiplier, is_exclusive, status, subscription_type, daily_limit_usd, weekly_limit_usd,
			monthly_limit_usd, default_validity_days, allow_image_generation, allow_batch_image_generation,
			image_rate_independent, image_rate_multiplier, image_price_1k, image_price_2k, image_price_4k,
			batch_image_discount_multiplier, batch_image_hold_multiplier, video_rate_independent,
			video_rate_multiplier, video_price_480p, video_price_720p, video_price_1080p,
			web_search_price_per_call, claude_code_only, fallback_group_id,
			fallback_group_id_on_invalid_request, model_routing, model_routing_enabled, mcp_xml_inject,
			supported_model_scopes, sort_order, allow_messages_dispatch, require_oauth_only,
			require_privacy_set, default_mapped_model, messages_dispatch_model_config, models_list_config,
			rpm_limit, kiro_cache_emulation_enabled, kiro_auto_sticky_enabled,
			kiro_sticky_session_ttl_seconds, kiro_cache_emulation_ratio, kiro_endpoint_mode,
			created_at, updated_at, deleted_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8,
			$9, $10, $11, $12, $13, $14,
			$15, $16, $17, $18,
			$19, $20, $21, $22, $23,
			$24, $25, $26,
			$27, $28, $29, $30,
			$31, $32, $33,
			$34, $35::jsonb, $36, $37,
			$38::jsonb, $39, $40, $41,
			$42, $43, $44::jsonb, $45::jsonb,
			$46, $47, $48, $49, $50, $51,
			$52, $53, NULL
		)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			platform = EXCLUDED.platform,
			rate_multiplier = EXCLUDED.rate_multiplier,
			peak_rate_enabled = EXCLUDED.peak_rate_enabled,
			peak_start = EXCLUDED.peak_start,
			peak_end = EXCLUDED.peak_end,
			peak_rate_multiplier = EXCLUDED.peak_rate_multiplier,
			is_exclusive = EXCLUDED.is_exclusive,
			status = EXCLUDED.status,
			subscription_type = EXCLUDED.subscription_type,
			daily_limit_usd = EXCLUDED.daily_limit_usd,
			weekly_limit_usd = EXCLUDED.weekly_limit_usd,
			monthly_limit_usd = EXCLUDED.monthly_limit_usd,
			default_validity_days = EXCLUDED.default_validity_days,
			allow_image_generation = EXCLUDED.allow_image_generation,
			allow_batch_image_generation = EXCLUDED.allow_batch_image_generation,
			image_rate_independent = EXCLUDED.image_rate_independent,
			image_rate_multiplier = EXCLUDED.image_rate_multiplier,
			image_price_1k = EXCLUDED.image_price_1k,
			image_price_2k = EXCLUDED.image_price_2k,
			image_price_4k = EXCLUDED.image_price_4k,
			batch_image_discount_multiplier = EXCLUDED.batch_image_discount_multiplier,
			batch_image_hold_multiplier = EXCLUDED.batch_image_hold_multiplier,
			video_rate_independent = EXCLUDED.video_rate_independent,
			video_rate_multiplier = EXCLUDED.video_rate_multiplier,
			video_price_480p = EXCLUDED.video_price_480p,
			video_price_720p = EXCLUDED.video_price_720p,
			video_price_1080p = EXCLUDED.video_price_1080p,
			web_search_price_per_call = EXCLUDED.web_search_price_per_call,
			claude_code_only = EXCLUDED.claude_code_only,
			fallback_group_id = EXCLUDED.fallback_group_id,
			fallback_group_id_on_invalid_request = EXCLUDED.fallback_group_id_on_invalid_request,
			model_routing = EXCLUDED.model_routing,
			model_routing_enabled = EXCLUDED.model_routing_enabled,
			mcp_xml_inject = EXCLUDED.mcp_xml_inject,
			supported_model_scopes = EXCLUDED.supported_model_scopes,
			sort_order = EXCLUDED.sort_order,
			allow_messages_dispatch = EXCLUDED.allow_messages_dispatch,
			require_oauth_only = EXCLUDED.require_oauth_only,
			require_privacy_set = EXCLUDED.require_privacy_set,
			default_mapped_model = EXCLUDED.default_mapped_model,
			messages_dispatch_model_config = EXCLUDED.messages_dispatch_model_config,
			models_list_config = EXCLUDED.models_list_config,
			rpm_limit = EXCLUDED.rpm_limit,
			kiro_cache_emulation_enabled = EXCLUDED.kiro_cache_emulation_enabled,
			kiro_auto_sticky_enabled = EXCLUDED.kiro_auto_sticky_enabled,
			kiro_sticky_session_ttl_seconds = EXCLUDED.kiro_sticky_session_ttl_seconds,
			kiro_cache_emulation_ratio = EXCLUDED.kiro_cache_emulation_ratio,
			kiro_endpoint_mode = EXCLUDED.kiro_endpoint_mode,
			updated_at = EXCLUDED.updated_at,
			deleted_at = NULL
	`, group.ID, group.Name, nullableString(group.Description), group.Platform, group.RateMultiplier, group.PeakRateEnabled, group.PeakStart, group.PeakEnd,
		group.PeakRateMultiplier, group.IsExclusive, group.Status, group.SubscriptionType, group.DailyLimitUSD, group.WeeklyLimitUSD,
		group.MonthlyLimitUSD, group.DefaultValidityDays, group.AllowImageGeneration, group.AllowBatchImageGeneration,
		group.ImageRateIndependent, group.ImageRateMultiplier, group.ImagePrice1K, group.ImagePrice2K, group.ImagePrice4K,
		group.BatchImageDiscountMultiplier, group.BatchImageHoldMultiplier, group.VideoRateIndependent,
		group.VideoRateMultiplier, group.VideoPrice480P, group.VideoPrice720P, group.VideoPrice1080P,
		group.WebSearchPricePerCall, group.ClaudeCodeOnly, group.FallbackGroupID,
		group.FallbackGroupIDOnInvalidRequest, string(payload), group.ModelRoutingEnabled, group.MCPXMLInject,
		string(scopes), group.SortOrder, group.AllowMessagesDispatch, group.RequireOAuthOnly,
		group.RequirePrivacySet, group.DefaultMappedModel, string(messagesDispatchConfig), string(modelsListConfig),
		group.RPMLimit, group.KiroCacheEmulationEnabled, group.KiroAutoStickyEnabled,
		group.KiroStickySessionTTLSeconds, group.KiroCacheEmulationRatio, group.KiroEndpointMode,
		group.CreatedAt, group.UpdatedAt)
	return err
}

func (s *quotaLeaseDemoMirrorStore) upsertProxies(ctx context.Context, exec sqlExecutor, proxies []service.Proxy) error {
	if len(proxies) == 0 {
		return nil
	}
	sort.Slice(proxies, func(i, j int) bool {
		return proxies[i].ID < proxies[j].ID
	})
	for _, proxy := range proxies {
		if err := s.upsertProxy(ctx, exec, proxy); err != nil {
			return err
		}
	}
	for _, proxy := range proxies {
		if proxy.BackupProxyID == nil || *proxy.BackupProxyID <= 0 {
			continue
		}
		if _, err := exec.ExecContext(ctx, `
			UPDATE proxies
			SET backup_proxy_id = $2, updated_at = $3
			WHERE id = $1 AND deleted_at IS NULL
		`, proxy.ID, *proxy.BackupProxyID, proxy.UpdatedAt); err != nil {
			return err
		}
	}
	return nil
}

func (s *quotaLeaseDemoMirrorStore) upsertProxy(ctx context.Context, exec sqlExecutor, proxy service.Proxy) error {
	_, err := exec.ExecContext(ctx, `
		INSERT INTO proxies (
			id, name, protocol, host, port, username, password, status, expires_at,
			fallback_mode, backup_proxy_id, expiry_warn_days, created_at, updated_at, deleted_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9,
			$10, $11, $12, $13, $14, NULL
		)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			protocol = EXCLUDED.protocol,
			host = EXCLUDED.host,
			port = EXCLUDED.port,
			username = EXCLUDED.username,
			password = EXCLUDED.password,
			status = EXCLUDED.status,
			expires_at = EXCLUDED.expires_at,
			fallback_mode = EXCLUDED.fallback_mode,
			backup_proxy_id = EXCLUDED.backup_proxy_id,
			expiry_warn_days = EXCLUDED.expiry_warn_days,
			updated_at = EXCLUDED.updated_at,
			deleted_at = NULL
	`, proxy.ID, proxy.Name, proxy.Protocol, proxy.Host, proxy.Port, nullableString(proxy.Username), nullableString(proxy.Password),
		proxy.Status, proxy.ExpiresAt, proxy.FallbackMode, nil, proxy.ExpiryWarnDays, proxy.CreatedAt, proxy.UpdatedAt)
	return err
}

func (s *quotaLeaseDemoMirrorStore) upsertAccounts(ctx context.Context, exec sqlExecutor, accounts []service.Account, snapshot service.QuotaLeaseDemoMirrorSnapshot) error {
	if len(accounts) == 0 {
		return nil
	}
	for i := range accounts {
		accounts[i].Extra = cloneStringAnyMap(accounts[i].Extra)
		if accounts[i].Extra == nil {
			accounts[i].Extra = make(map[string]any)
		}
		accounts[i].Extra[quotaLeaseDemoMirrorFlagExtraKey] = true
		accounts[i].Extra[quotaLeaseDemoMirrorNodeIDExtraKey] = snapshot.NodeID
		accounts[i].Extra[quotaLeaseDemoMirrorSyncedAtExtraKey] = snapshot.SyncedAt.Format(time.RFC3339Nano)
		if accounts[i].CreatedAt.IsZero() {
			accounts[i].CreatedAt = snapshot.SyncedAt
		}
		if accounts[i].UpdatedAt.IsZero() {
			accounts[i].UpdatedAt = snapshot.SyncedAt
		}
		if accounts[i].Status == "" {
			accounts[i].Status = service.StatusActive
		}
		if accounts[i].Type == "" {
			accounts[i].Type = service.AccountTypeOAuth
		}
		if accounts[i].Platform == "" {
			accounts[i].Platform = ""
		}
		if accounts[i].Concurrency <= 0 {
			accounts[i].Concurrency = 1
		}
		if accounts[i].RateMultiplier == nil {
			one := 1.0
			accounts[i].RateMultiplier = &one
		}
		if len(accounts[i].GroupIDs) == 0 && len(accounts[i].AccountGroups) > 0 {
			accounts[i].GroupIDs = quotaLeaseDemoMirrorGroupIDsFromAccountGroups(accounts[i].AccountGroups)
		}
	}
	sort.Slice(accounts, func(i, j int) bool {
		return accounts[i].ID < accounts[j].ID
	})
	for _, account := range accounts {
		if err := s.upsertAccount(ctx, exec, account); err != nil {
			return err
		}
	}
	return nil
}

func (s *quotaLeaseDemoMirrorStore) upsertAccount(ctx context.Context, exec sqlExecutor, account service.Account) error {
	credentialsMap := cloneStringAnyMap(account.Credentials)
	if credentialsMap == nil {
		credentialsMap = make(map[string]any)
	}
	credentials, err := json.Marshal(credentialsMap)
	if err != nil {
		return err
	}
	extraMap := cloneStringAnyMap(account.Extra)
	if extraMap == nil {
		extraMap = make(map[string]any)
	}
	extra, err := json.Marshal(extraMap)
	if err != nil {
		return err
	}
	notes := nullableStringPtr(account.Notes)
	var proxyID any
	if account.ProxyID != nil && *account.ProxyID > 0 {
		proxyID = *account.ProxyID
	}
	var proxyFallbackOriginID any
	if account.ProxyFallbackOriginID != nil && *account.ProxyFallbackOriginID > 0 {
		proxyFallbackOriginID = *account.ProxyFallbackOriginID
	}
	var loadFactor any
	if account.LoadFactor != nil && *account.LoadFactor > 0 {
		loadFactor = *account.LoadFactor
	}
	var lastUsedAt any = account.LastUsedAt
	var expiresAt any = account.ExpiresAt
	var rateLimitedAt any = account.RateLimitedAt
	var rateLimitResetAt any = account.RateLimitResetAt
	var overloadUntil any = account.OverloadUntil
	var tempUnschedulableUntil any = account.TempUnschedulableUntil
	var sessionWindowStart any = account.SessionWindowStart
	var sessionWindowEnd any = account.SessionWindowEnd
	_, err = exec.ExecContext(ctx, `
		INSERT INTO accounts (
			id, name, notes, platform, type, credentials, extra, proxy_id, proxy_fallback_origin_id,
			concurrency, load_factor, priority, rate_multiplier, status, error_message, last_used_at,
			expires_at, auto_pause_on_expired, schedulable, rate_limited_at, rate_limit_reset_at,
			overload_until, temp_unschedulable_until, temp_unschedulable_reason,
			session_window_start, session_window_end, session_window_status, parent_account_id, quota_dimension,
			created_at, updated_at, deleted_at
		) VALUES (
			$1, $2, $3, $4, $5, $6::jsonb, $7::jsonb, $8, $9,
			$10, $11, $12, $13, $14, $15, $16,
			$17, $18, $19, $20, $21,
			$22, $23, $24,
			$25, $26, $27, $28, $29,
			$30, $31, NULL
		)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			notes = EXCLUDED.notes,
			platform = EXCLUDED.platform,
			type = EXCLUDED.type,
			credentials = EXCLUDED.credentials,
			extra = EXCLUDED.extra,
			proxy_id = EXCLUDED.proxy_id,
			proxy_fallback_origin_id = EXCLUDED.proxy_fallback_origin_id,
			concurrency = EXCLUDED.concurrency,
			load_factor = EXCLUDED.load_factor,
			priority = EXCLUDED.priority,
			rate_multiplier = EXCLUDED.rate_multiplier,
			status = EXCLUDED.status,
			error_message = EXCLUDED.error_message,
			last_used_at = EXCLUDED.last_used_at,
			expires_at = EXCLUDED.expires_at,
			auto_pause_on_expired = EXCLUDED.auto_pause_on_expired,
			schedulable = EXCLUDED.schedulable,
			rate_limited_at = EXCLUDED.rate_limited_at,
			rate_limit_reset_at = EXCLUDED.rate_limit_reset_at,
			overload_until = EXCLUDED.overload_until,
			temp_unschedulable_until = EXCLUDED.temp_unschedulable_until,
			temp_unschedulable_reason = EXCLUDED.temp_unschedulable_reason,
			session_window_start = EXCLUDED.session_window_start,
			session_window_end = EXCLUDED.session_window_end,
			session_window_status = EXCLUDED.session_window_status,
			parent_account_id = EXCLUDED.parent_account_id,
			quota_dimension = EXCLUDED.quota_dimension,
			updated_at = EXCLUDED.updated_at,
			deleted_at = NULL
	`, account.ID, account.Name, notes, account.Platform, account.Type, string(credentials), string(extra), proxyID, proxyFallbackOriginID,
		account.Concurrency, loadFactor, account.Priority, rateMultiplierOrDefault(account.RateMultiplier), account.Status, nullableString(account.ErrorMessage), lastUsedAt,
		expiresAt, account.AutoPauseOnExpired, account.Schedulable, rateLimitedAt, rateLimitResetAt,
		overloadUntil, tempUnschedulableUntil, nullableString(account.TempUnschedulableReason),
		sessionWindowStart, sessionWindowEnd, nullableString(account.SessionWindowStatus), nil, service.QuotaDimensionGlobal,
		account.CreatedAt, account.UpdatedAt)
	return err
}

func (s *quotaLeaseDemoMirrorStore) reconcileMirrorAccountParents(ctx context.Context, exec sqlExecutor, accounts []service.Account) error {
	if len(accounts) == 0 {
		return nil
	}
	accountIDs := make(map[int64]struct{}, len(accounts))
	for _, account := range accounts {
		if account.ID > 0 {
			accountIDs[account.ID] = struct{}{}
		}
	}
	for _, account := range accounts {
		if account.ParentAccountID == nil || *account.ParentAccountID <= 0 {
			continue
		}
		if _, ok := accountIDs[*account.ParentAccountID]; !ok {
			continue
		}
		quotaDimension := strings.TrimSpace(account.QuotaDimension)
		if quotaDimension == "" || quotaDimension == service.QuotaDimensionGlobal {
			quotaDimension = service.QuotaDimensionSpark
		}
		if _, err := exec.ExecContext(ctx, `
			UPDATE accounts
			SET parent_account_id = $2,
				quota_dimension = $3,
				updated_at = $4,
				deleted_at = NULL
			WHERE id = $1
		`, account.ID, *account.ParentAccountID, quotaDimension, account.UpdatedAt); err != nil {
			return err
		}
	}
	return nil
}

func (s *quotaLeaseDemoMirrorStore) reconcileMirrorAccounts(ctx context.Context, exec sqlExecutor, accounts []service.Account, nodeID string) error {
	if nodeID == "" {
		return nil
	}
	accountIDs := make([]int64, 0, len(accounts))
	for _, account := range accounts {
		if account.ID > 0 {
			accountIDs = append(accountIDs, account.ID)
		}
	}
	if len(accountIDs) == 0 {
		if _, err := exec.ExecContext(ctx, `
			DELETE FROM account_groups
			WHERE account_id IN (
				SELECT id
				FROM accounts
				WHERE extra ->> $1 = $2
				  AND deleted_at IS NULL
			)
		`, quotaLeaseDemoMirrorNodeIDExtraKey, nodeID); err != nil {
			return err
		}
		if _, err := exec.ExecContext(ctx, `
			UPDATE accounts
			SET deleted_at = NOW(), updated_at = NOW()
			WHERE extra ->> $1 = $2
			  AND deleted_at IS NULL
		`, quotaLeaseDemoMirrorNodeIDExtraKey, nodeID); err != nil {
			return err
		}
		return nil
	}
	if _, err := exec.ExecContext(ctx, `
		DELETE FROM account_groups
		WHERE account_id IN (
			SELECT id
			FROM accounts
			WHERE extra ->> $1 = $2
			  AND deleted_at IS NULL
		)
		AND account_id <> ALL($3::bigint[])
	`, quotaLeaseDemoMirrorNodeIDExtraKey, nodeID, pq.Array(accountIDs)); err != nil {
		return err
	}
	if _, err := exec.ExecContext(ctx, `
		UPDATE accounts
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE extra ->> $1 = $2
		  AND deleted_at IS NULL
		  AND NOT (id = ANY($3::bigint[]))
	`, quotaLeaseDemoMirrorNodeIDExtraKey, nodeID, pq.Array(accountIDs)); err != nil {
		return err
	}
	return nil
}

func (s *quotaLeaseDemoMirrorStore) reconcileMirrorAccountGroups(ctx context.Context, exec sqlExecutor, accounts []service.Account, groups []service.AccountGroup, nodeID string) error {
	if nodeID == "" {
		return nil
	}
	accountIDs := make([]int64, 0, len(accounts))
	for _, account := range accounts {
		if account.ID > 0 {
			accountIDs = append(accountIDs, account.ID)
		}
	}
	if len(accountIDs) > 0 {
		if _, err := exec.ExecContext(ctx, `
			DELETE FROM account_groups
			WHERE account_id = ANY($1::bigint[])
		`, pq.Array(accountIDs)); err != nil {
			return err
		}
	}
	if len(groups) == 0 {
		return nil
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].AccountID == groups[j].AccountID {
			return groups[i].GroupID < groups[j].GroupID
		}
		return groups[i].AccountID < groups[j].AccountID
	})
	values := make([]any, 0, len(groups)*4)
	placeholders := make([]string, 0, len(groups))
	for _, item := range groups {
		if item.AccountID <= 0 || item.GroupID <= 0 {
			continue
		}
		base := len(placeholders) * 4
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d)", base+1, base+2, base+3, base+4))
		values = append(values, item.AccountID, item.GroupID, item.Priority, item.CreatedAt)
	}
	if len(placeholders) == 0 {
		return nil
	}
	query := `
		INSERT INTO account_groups (account_id, group_id, priority, created_at)
		VALUES ` + strings.Join(placeholders, ", ") + `
		ON CONFLICT (account_id, group_id) DO UPDATE SET
			priority = EXCLUDED.priority
	`
	_, err := exec.ExecContext(ctx, query, values...)
	return err
}

func (s *quotaLeaseDemoMirrorStore) bumpSequences(ctx context.Context, exec sqlExecutor) error {
	queries := []struct {
		table string
	}{
		{table: "groups"},
		{table: "proxies"},
		{table: "accounts"},
	}
	for _, item := range queries {
		if _, err := exec.ExecContext(ctx, fmt.Sprintf(`
			SELECT setval(
				pg_get_serial_sequence('%s', 'id'),
				COALESCE((SELECT MAX(id) FROM %s), 1),
				TRUE
			)
		`, item.table, item.table)); err != nil {
			return err
		}
	}
	return nil
}

func cloneQuotaLeaseDemoMirrorGroups(src []service.QuotaLeaseDemoGroupSnapshot) []service.Group {
	if len(src) == 0 {
		return nil
	}
	out := make([]service.Group, 0, len(src))
	for _, item := range src {
		out = append(out, service.QuotaLeaseDemoGroupSnapshotToGroup(item))
	}
	return out
}

func cloneQuotaLeaseDemoMirrorProxies(src []service.QuotaLeaseDemoProxySnapshot) []service.Proxy {
	if len(src) == 0 {
		return nil
	}
	out := make([]service.Proxy, 0, len(src))
	for _, item := range src {
		if proxy := service.QuotaLeaseDemoProxySnapshotToProxy(&item); proxy != nil {
			out = append(out, *proxy)
		}
	}
	return out
}

func cloneQuotaLeaseDemoMirrorAccounts(src []service.QuotaLeaseDemoAccountSnapshot, snapshot service.QuotaLeaseDemoMirrorSnapshot) []service.Account {
	if len(src) == 0 {
		return nil
	}
	out := make([]service.Account, 0, len(src))
	for _, item := range src {
		account := service.QuotaLeaseDemoAccountSnapshotToAccount(item)
		if account.ID <= 0 {
			continue
		}
		if account.Extra == nil {
			account.Extra = make(map[string]any)
		}
		account.Extra[quotaLeaseDemoMirrorFlagExtraKey] = true
		account.Extra[quotaLeaseDemoMirrorNodeIDExtraKey] = snapshot.NodeID
		account.Extra[quotaLeaseDemoMirrorSyncedAtExtraKey] = snapshot.SyncedAt.Format(time.RFC3339Nano)
		if account.CreatedAt.IsZero() {
			account.CreatedAt = snapshot.SyncedAt
		}
		if account.UpdatedAt.IsZero() {
			account.UpdatedAt = snapshot.SyncedAt
		}
		if account.Status == "" {
			account.Status = service.StatusActive
		}
		if account.Concurrency <= 0 {
			account.Concurrency = 1
		}
		if account.RateMultiplier == nil {
			one := 1.0
			account.RateMultiplier = &one
		}
		if account.QuotaDimension == "" {
			if account.ParentAccountID != nil && *account.ParentAccountID > 0 {
				account.QuotaDimension = service.QuotaDimensionSpark
			} else {
				account.QuotaDimension = service.QuotaDimensionGlobal
			}
		}
		out = append(out, account)
	}
	return out
}

func cloneQuotaLeaseDemoMirrorAccountGroups(src []service.QuotaLeaseDemoAccountGroupSnapshot, accountSets ...[]service.Account) []service.AccountGroup {
	seen := make(map[[2]int64]service.AccountGroup)
	for _, item := range src {
		if item.AccountID <= 0 || item.GroupID <= 0 {
			continue
		}
		key := [2]int64{item.AccountID, item.GroupID}
		seen[key] = service.AccountGroup{
			AccountID: item.AccountID,
			GroupID:   item.GroupID,
			Priority:  item.Priority,
			CreatedAt: quotaLeaseDemoMirrorTimeOrNow(item.CreatedAt),
		}
	}
	for _, accounts := range accountSets {
		for _, account := range accounts {
			for _, item := range account.AccountGroups {
				if item.AccountID <= 0 {
					item.AccountID = account.ID
				}
				if item.AccountID <= 0 || item.GroupID <= 0 {
					continue
				}
				item.CreatedAt = quotaLeaseDemoMirrorTimeOrNow(item.CreatedAt)
				key := [2]int64{item.AccountID, item.GroupID}
				seen[key] = item
			}
			for _, groupID := range account.GroupIDs {
				if account.ID <= 0 || groupID <= 0 {
					continue
				}
				key := [2]int64{account.ID, groupID}
				if _, exists := seen[key]; exists {
					continue
				}
				seen[key] = service.AccountGroup{
					AccountID: account.ID,
					GroupID:   groupID,
					Priority:  account.Priority,
					CreatedAt: quotaLeaseDemoMirrorTimeOrNow(account.CreatedAt),
				}
			}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]service.AccountGroup, 0, len(seen))
	for _, item := range seen {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].AccountID == out[j].AccountID {
			return out[i].GroupID < out[j].GroupID
		}
		return out[i].AccountID < out[j].AccountID
	})
	return out
}

func filterMirrorSchedulableAccounts(accounts []service.Account, groupID *int64, platform string) []service.Account {
	if len(accounts) == 0 {
		return nil
	}
	out := make([]service.Account, 0, len(accounts))
	for _, account := range accounts {
		if quotaLeaseDemoMirrorAccountVisible(account) && quotaLeaseDemoMirrorAccountMatches(account, groupID, platform) {
			out = append(out, account)
		}
	}
	return out
}

func quotaLeaseDemoMirrorAccountMatches(account service.Account, groupID *int64, platform string) bool {
	platform = strings.TrimSpace(platform)
	if platform != "" && account.Platform != platform {
		return false
	}
	if groupID == nil {
		return len(account.GroupIDs) == 0
	}
	for _, id := range account.GroupIDs {
		if id == *groupID {
			return true
		}
	}
	return false
}

func quotaLeaseDemoMirrorGroupIDsFromAccountGroups(src []service.AccountGroup) []int64 {
	if len(src) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(src))
	out := make([]int64, 0, len(src))
	for _, item := range src {
		if item.GroupID <= 0 {
			continue
		}
		if _, exists := seen[item.GroupID]; exists {
			continue
		}
		seen[item.GroupID] = struct{}{}
		out = append(out, item.GroupID)
	}
	if len(out) == 0 {
		return nil
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i] < out[j]
	})
	return out
}

func quotaLeaseDemoMirrorTimeOrNow(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC()
	}
	return value
}

func quotaLeaseDemoMirrorAccountVisible(account service.Account) bool {
	if account.ID <= 0 {
		return false
	}
	if account.Extra == nil {
		return false
	}
	_, ok := account.Extra[quotaLeaseDemoMirrorFlagExtraKey]
	return ok
}

func cloneStringAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	for k, v := range src {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		out[key] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func quotaLeaseDemoMirrorExtraString(src map[string]any, key string) string {
	if len(src) == 0 {
		return ""
	}
	value, ok := src[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func nullableString(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func nullableStringPtr(value *string) any {
	if value == nil {
		return nil
	}
	return nullableString(*value)
}

func rateMultiplierOrDefault(value *float64) float64 {
	if value == nil || *value < 0 {
		return 1
	}
	return *value
}
