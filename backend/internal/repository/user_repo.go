package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/apikey"
	"github.com/Wei-Shaw/sub2api/ent/authidentity"
	"github.com/Wei-Shaw/sub2api/ent/authidentitychannel"
	dbgroup "github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/ent/identityadoptiondecision"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	dbuser "github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/ent/userallowedgroup"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"

	entsql "entgo.io/ent/dialect/sql"
)

type userRepository struct {
	client *dbent.Client
	sql    sqlExecutor
}

var _ service.RedeemUserAdjustmentRepository = (*userRepository)(nil)
var _ service.RiskControlBalanceRecorder = (*userRepository)(nil)
var _ service.UserGroupBanRepository = (*userRepository)(nil)
var _ service.DailyCheckinRewardExpirer = (*userRepository)(nil)

func NewUserRepository(client *dbent.Client, sqlDB *sql.DB) service.UserRepository {
	return newUserRepositoryWithSQL(client, sqlDB)
}

func (r *userRepository) antiAbuseExecutor(ctx context.Context) sqlExecutor {
	if r == nil {
		return nil
	}
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return tx.Client()
	}
	return r.sql
}

func (r *userRepository) StoreSignupRiskSignals(ctx context.Context, userID int64, fingerprints []string, assessment service.AntiAbuseAssessment) error {
	exec := r.antiAbuseExecutor(ctx)
	if r == nil || exec == nil || userID <= 0 {
		return nil
	}
	for _, fingerprint := range service.HashBrowserFingerprints(fingerprints) {
		bucket := fingerprint
		if strings.HasPrefix(bucket, "b:") {
			bucket = strings.TrimPrefix(bucket, "b:")
		}
		if len(bucket) > 16 {
			bucket = bucket[:16]
		}
		_, err := exec.ExecContext(ctx, `
			INSERT INTO anti_abuse_user_fingerprints (user_id, fingerprint_hash, fingerprint_bucket, risk_score, risk_factors)
			VALUES ($1, $2, $3, $4, $5::jsonb)
			ON CONFLICT (user_id, fingerprint_hash) DO UPDATE SET risk_score = EXCLUDED.risk_score, risk_factors = EXCLUDED.risk_factors`,
			userID, fingerprint, bucket, assessment.Score, marshalRiskFactors(assessment.Factors))
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *userRepository) GetUserFingerprints(ctx context.Context, userID int64) ([]string, error) {
	exec := r.antiAbuseExecutor(ctx)
	if r == nil || exec == nil {
		return nil, nil
	}
	rows, err := exec.QueryContext(ctx, `SELECT fingerprint_hash FROM anti_abuse_user_fingerprints WHERE user_id = $1 ORDER BY created_at ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (r *userRepository) CountUsersByFingerprintHashes(ctx context.Context, fingerprints []string, since time.Time) (int, error) {
	exec := r.antiAbuseExecutor(ctx)
	if r == nil || exec == nil || len(fingerprints) == 0 {
		return 0, nil
	}
	rows, err := exec.QueryContext(ctx, `SELECT COUNT(DISTINCT user_id) FROM anti_abuse_user_fingerprints WHERE fingerprint_hash = ANY($1) AND created_at >= $2`, pq.Array(fingerprints), since)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var count int
	if rows.Next() {
		err = rows.Scan(&count)
	}
	return count, err
}

func (r *userRepository) StoreTransportFingerprints(ctx context.Context, userID int64, ja3, ja4 string, assessment service.AntiAbuseAssessment) error {
	exec := r.antiAbuseExecutor(ctx)
	if r == nil || exec == nil || userID <= 0 {
		return nil
	}
	for _, pair := range [][2]string{{"ja3", ja3}, {"ja4", ja4}} {
		hash := service.HashTransportFingerprint(pair[0], pair[1])
		if hash == "" {
			continue
		}
		_, err := exec.ExecContext(ctx, `INSERT INTO anti_abuse_user_fingerprints (user_id, fingerprint_hash, fingerprint_bucket, risk_score, risk_factors) VALUES ($1, $2, $3, $4, $5::jsonb) ON CONFLICT (user_id, fingerprint_hash) DO UPDATE SET risk_score = EXCLUDED.risk_score, risk_factors = EXCLUDED.risk_factors`, userID, hash, pair[0], assessment.Score, marshalRiskFactors(assessment.Factors))
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *userRepository) CountUsersByTransportFingerprints(ctx context.Context, ja3, ja4 string, since time.Time) (int, error) {
	exec := r.antiAbuseExecutor(ctx)
	if r == nil || exec == nil {
		return 0, nil
	}
	hashes := make([]string, 0, 2)
	if hash := service.HashTransportFingerprint("ja3", ja3); hash != "" {
		hashes = append(hashes, hash)
	}
	if hash := service.HashTransportFingerprint("ja4", ja4); hash != "" {
		hashes = append(hashes, hash)
	}
	if len(hashes) == 0 {
		return 0, nil
	}
	rows, err := exec.QueryContext(ctx, `SELECT COUNT(DISTINCT user_id) FROM anti_abuse_user_fingerprints WHERE fingerprint_hash = ANY($1) AND created_at >= $2`, pq.Array(hashes), since)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var count int
	if rows.Next() {
		err = rows.Scan(&count)
	}
	return count, err
}

func marshalRiskFactors(factors map[string]int) string {
	if len(factors) == 0 {
		return "{}"
	}
	raw, err := json.Marshal(factors)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func (r *userRepository) RecordAntiAbuseEvent(ctx context.Context, event *service.AntiAbuseEvent) error {
	if event == nil {
		return nil
	}
	exec := r.antiAbuseExecutor(ctx)
	if exec == nil {
		return nil
	}
	factors, err := json.Marshal(event.Factors)
	if err != nil {
		return err
	}
	reasons, err := json.Marshal(event.Reasons)
	if err != nil {
		return err
	}
	var userID any
	if event.UserID != nil && *event.UserID > 0 {
		userID = *event.UserID
	}
	rows, err := exec.QueryContext(ctx, `
		INSERT INTO anti_abuse_events (
			user_id, event_type, action, score, factors, reasons, ip_address, email,
			user_agent, fingerprint_hash_count, ja3_hash, ja4_hash, gift_balance_deducted
		) VALUES ($1, $2, $3, $4, $5::jsonb, $6::jsonb, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, created_at`,
		userID, event.EventType, event.Action, event.Score, string(factors), string(reasons),
		event.IPAddress, event.Email, event.UserAgent, event.FingerprintHashCount,
		event.JA3Hash, event.JA4Hash, event.GiftBalanceDeducted)
	if err != nil {
		return err
	}
	defer rows.Close()
	if !rows.Next() {
		return rows.Err()
	}
	return rows.Scan(&event.ID, &event.CreatedAt)
}

func (r *userRepository) ListAntiAbuseEvents(ctx context.Context, filter service.AntiAbuseEventFilter) ([]service.AntiAbuseEvent, int64, error) {
	exec := r.antiAbuseExecutor(ctx)
	if exec == nil {
		return []service.AntiAbuseEvent{}, 0, nil
	}
	where := []string{"1=1"}
	args := make([]any, 0, 8)
	add := func(clause string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if value := strings.TrimSpace(filter.EventType); value != "" {
		add("e.event_type = $%d", value)
	}
	if value := strings.TrimSpace(filter.Action); value != "" {
		add("e.action = $%d", value)
	}
	if filter.DeductionsOnly {
		where = append(where, "e.gift_balance_deducted > 0")
	}
	if value := strings.TrimSpace(filter.Search); value != "" {
		args = append(args, "%"+value+"%")
		index := len(args)
		where = append(where, fmt.Sprintf("(e.email ILIKE $%d OR u.email ILIKE $%d OR CAST(e.user_id AS TEXT) = $%d OR e.ip_address ILIKE $%d)", index, index, index, index))
	}
	if filter.From != nil {
		add("e.created_at >= $%d", *filter.From)
	}
	if filter.To != nil {
		add("e.created_at <= $%d", *filter.To)
	}
	whereSQL := strings.Join(where, " AND ")
	var total int64
	countRows, err := exec.QueryContext(ctx, `SELECT COUNT(*) FROM anti_abuse_events e LEFT JOIN users u ON u.id = e.user_id WHERE `+whereSQL, args...)
	if err != nil {
		return nil, 0, err
	}
	if countRows.Next() {
		err = countRows.Scan(&total)
	}
	closeErr := countRows.Close()
	if err != nil {
		return nil, 0, err
	}
	if closeErr != nil {
		return nil, 0, closeErr
	}
	args = append(args, filter.Pagination.Limit(), filter.Pagination.Offset())
	rows, err := exec.QueryContext(ctx, fmt.Sprintf(`
		SELECT e.id, e.user_id, COALESCE(u.email, ''), e.event_type, e.action, e.score,
			e.factors, e.reasons, e.ip_address, e.email, e.user_agent,
			e.fingerprint_hash_count, e.ja3_hash, e.ja4_hash, e.gift_balance_deducted, e.created_at
		FROM anti_abuse_events e
		LEFT JOIN users u ON u.id = e.user_id
		WHERE %s
		ORDER BY e.created_at DESC, e.id DESC
		LIMIT $%d OFFSET $%d`, whereSQL, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]service.AntiAbuseEvent, 0)
	for rows.Next() {
		var item service.AntiAbuseEvent
		var factorsRaw, reasonsRaw []byte
		if err := rows.Scan(&item.ID, &item.UserID, &item.UserEmail, &item.EventType, &item.Action, &item.Score,
			&factorsRaw, &reasonsRaw, &item.IPAddress, &item.Email, &item.UserAgent,
			&item.FingerprintHashCount, &item.JA3Hash, &item.JA4Hash, &item.GiftBalanceDeducted, &item.CreatedAt); err != nil {
			return nil, 0, err
		}
		_ = json.Unmarshal(factorsRaw, &item.Factors)
		_ = json.Unmarshal(reasonsRaw, &item.Reasons)
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func newUserRepositoryWithSQL(client *dbent.Client, sqlq sqlExecutor) *userRepository {
	return &userRepository{client: client, sql: sqlq}
}

func (r *userRepository) Create(ctx context.Context, userIn *service.User) error {
	return r.create(ctx, userIn, false, "")
}

// CreateWithEmailAliasGuard 见 service.UserRepository：在邮箱唯一性锁内复查收件箱身份，
// 供注册路径使用。
func (r *userRepository) CreateWithEmailAliasGuard(ctx context.Context, userIn *service.User) error {
	return r.create(ctx, userIn, true, "")
}

// CountUsersByEmailDomain 统计指定可注册主域名及其子域名下的未删除用户。
func (r *userRepository) CountUsersByEmailDomain(ctx context.Context, domain string) (int, error) {
	return countUsersByEmailDomainWithClient(ctx, clientFromContext(ctx, r.client), domain)
}

// CreateWithEmailAliasGuardAndDomainLimit 串行化非白名单域名的注册请求，
// 并在用户写入的同一事务内复查域名额度。
func (r *userRepository) CreateWithEmailAliasGuardAndDomainLimit(ctx context.Context, userIn *service.User, domain string) error {
	return r.create(ctx, userIn, true, normalizeEmailDomain(domain))
}

func (r *userRepository) create(ctx context.Context, userIn *service.User, guardEmailAlias bool, domainLimit string) error {
	if userIn == nil {
		return nil
	}

	// 统一使用 ent 的事务：保证用户与允许分组的更新原子化，
	// 并避免基于 *sql.Tx 手动构造 ent client 导致的 ExecQuerier 断言错误。
	//
	// 注意：ent 的 Client.Tx 不感知上下文中的事务（只检查 driver 类型），
	// 因此必须显式检查 TxFromContext：当调用方已开启外部事务（如注册时的
	// “建用户 + 占用邀请码”原子事务），直接复用其 client，由调用方统一提交/回滚，
	// 否则用户写入会落入独立事务并自行提交，导致外层事务无法回滚（孤儿用户）。
	var txClient *dbent.Client
	txCtx := ctx
	var ownedTx *dbent.Tx
	if existingTx := dbent.TxFromContext(ctx); existingTx != nil {
		txClient = existingTx.Client()
	} else {
		tx, err := r.client.Tx(ctx)
		switch {
		case errors.Is(err, dbent.ErrTxStarted):
			// r.client 本身已是事务绑定 client（client 注入式事务，如集成测试
			// 夹具 tx.Client()）：直接复用，提交/回滚由 client 的持有方负责。
			txClient = r.client
		case err != nil:
			return err
		default:
			ownedTx = tx
			defer func() { _ = ownedTx.Rollback() }()
			txClient = tx.Client()
			txCtx = dbent.NewTxContext(ctx, tx)
		}
	}

	lockKeys := []string{normalizedEmailUniquenessLockKey(userIn.Email)}
	if guardEmailAlias {
		// 别名变体的字面量不同，唯一索引无法兜底；用收件箱身份锁把同一收件箱的并发注册串行化。
		lockKeys = append(lockKeys, emailAliasUniquenessLockKey(userIn.Email))
	}
	if domainLimit != "" {
		lockKeys = append(lockKeys, registrationEmailDomainLockKey(domainLimit))
	}
	releaseEmailLock, err := lockRepositoryScopedKeys(
		txCtx,
		txClient,
		txAwareSQLExecutor(txCtx, r.sql, r.client),
		lockKeys...,
	)
	if err != nil {
		return err
	}
	defer releaseEmailLock()

	if domainLimit != "" {
		count, err := countUsersByEmailDomainWithClient(txCtx, txClient, domainLimit)
		if err != nil {
			return err
		}
		if count > 0 {
			return service.ErrEmailDomainRegistrationLimit
		}
	}

	if err := ensureNormalizedEmailAvailableWithClient(txCtx, txClient, 0, userIn.Email); err != nil {
		return err
	}

	if guardEmailAlias {
		aliasExists, err := existsByEmailAliasWithClient(txCtx, txClient, userIn.Email)
		if err != nil {
			return err
		}
		if aliasExists {
			return service.ErrEmailExists
		}
	}

	created, err := txClient.User.Create().
		SetEmail(userIn.Email).
		SetNillableSignupIP(userIn.SignupIP).
		SetUsername(userIn.Username).
		SetNotes(userIn.Notes).
		SetPasswordHash(userIn.PasswordHash).
		SetRole(userIn.Role).
		SetBalance(userIn.Balance).
		SetConcurrency(userIn.Concurrency).
		SetStatus(userIn.Status).
		SetNillableDisabledUntil(userIn.DisabledUntil).
		SetSignupSource(userSignupSourceOrDefault(userIn.SignupSource)).
		SetNillableLastLoginAt(userIn.LastLoginAt).
		SetNillableLastActiveAt(userIn.LastActiveAt).
		SetRpmLimit(userIn.RPMLimit).
		Save(txCtx)
	if err != nil {
		return translatePersistenceError(err, nil, service.ErrEmailExists)
	}

	if err := r.syncUserAllowedGroupsWithClient(txCtx, txClient, created.ID, userIn.AllowedGroups); err != nil {
		return err
	}
	if err := ensureEmailAuthIdentityWithClient(txCtx, txClient, created.ID, created.Email, "user_repo_create"); err != nil {
		return err
	}

	if ownedTx != nil {
		if err := ownedTx.Commit(); err != nil {
			return err
		}
	}

	applyUserEntityToService(userIn, created)
	return nil
}

func (r *userRepository) GetByID(ctx context.Context, id int64) (*service.User, error) {
	m, err := r.client.User.Query().Where(dbuser.IDEQ(id)).Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrUserNotFound, nil)
	}

	out := userEntityToService(m)
	groups, err := r.loadAllowedGroups(ctx, []int64{id})
	if err != nil {
		return nil, err
	}
	if v, ok := groups[id]; ok {
		out.AllowedGroups = v
	}
	banned, expirations, err := r.loadBannedGroups(ctx, []int64{id})
	if err != nil {
		return nil, err
	}
	if v, ok := banned[id]; ok {
		out.BannedGroupIDs = v
	}
	if v, ok := expirations[id]; ok {
		out.BannedGroupExpirations = v
	}
	return out, nil
}

func (r *userRepository) GetByIDIncludeDeleted(ctx context.Context, id int64) (*service.User, error) {
	ctx = mixins.SkipSoftDelete(ctx)
	m, err := r.client.User.Query().Where(dbuser.IDEQ(id)).Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrUserNotFound, nil)
	}
	out := userEntityToService(m)
	groups, err := r.loadAllowedGroups(ctx, []int64{id})
	if err != nil {
		return nil, err
	}
	if v, ok := groups[id]; ok {
		out.AllowedGroups = v
	}
	banned, expirations, err := r.loadBannedGroups(ctx, []int64{id})
	if err != nil {
		return nil, err
	}
	if v, ok := banned[id]; ok {
		out.BannedGroupIDs = v
	}
	if v, ok := expirations[id]; ok {
		out.BannedGroupExpirations = v
	}
	return out, nil
}

func (r *userRepository) GetByEmail(ctx context.Context, email string) (*service.User, error) {
	matches, err := r.client.User.Query().
		Where(userEmailLookupPredicate(email)).
		Order(dbent.Asc(dbuser.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, service.ErrUserNotFound
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("normalized email lookup matched multiple users for %q", strings.TrimSpace(email))
	}
	m := matches[0]

	out := userEntityToService(m)
	groups, err := r.loadAllowedGroups(ctx, []int64{m.ID})
	if err != nil {
		return nil, err
	}
	if v, ok := groups[m.ID]; ok {
		out.AllowedGroups = v
	}
	banned, expirations, err := r.loadBannedGroups(ctx, []int64{m.ID})
	if err != nil {
		return nil, err
	}
	if v, ok := banned[m.ID]; ok {
		out.BannedGroupIDs = v
	}
	if v, ok := expirations[m.ID]; ok {
		out.BannedGroupExpirations = v
	}
	return out, nil
}

func (r *userRepository) Update(ctx context.Context, userIn *service.User, fields service.UserUpdateFields) error {
	if userIn == nil {
		return nil
	}
	// 空掩码代表调用方不改任何列，直接返回，避免产生一次无意义的整行写。
	if fields.IsEmpty() {
		return nil
	}

	// 使用 ent 事务包裹用户更新与 allowed_groups 同步，避免跨层事务不一致。
	tx, err := r.client.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return err
	}

	var txClient *dbent.Client
	txCtx := ctx
	if err == nil {
		defer func() { _ = tx.Rollback() }()
		txClient = tx.Client()
		txCtx = dbent.NewTxContext(ctx, tx)
	} else {
		// 已处于外部事务中（ErrTxStarted），复用当前事务 client 并由调用方负责提交/回滚。
		if existingTx := dbent.TxFromContext(ctx); existingTx != nil {
			txClient = existingTx.Client()
		} else {
			txClient = r.client
		}
	}

	// 邮箱唯一性锁与查重只在本次确实要改邮箱时才做：不改邮箱的更新既不需要
	// 串行化，也不该因为快照里的旧邮箱已被他人占用而报 ErrEmailExists。
	if fields.Email {
		releaseEmailLock, err := lockRepositoryScopedKeys(
			txCtx,
			txClient,
			txAwareSQLExecutor(txCtx, r.sql, r.client),
			normalizedEmailUniquenessLockKey(userIn.Email),
		)
		if err != nil {
			return err
		}
		defer releaseEmailLock()

		if err := ensureNormalizedEmailAvailableWithClient(txCtx, txClient, userIn.ID, userIn.Email); err != nil {
			return err
		}
	}

	existing, err := clientFromContext(txCtx, txClient).User.Get(txCtx, userIn.ID)
	if err != nil {
		return translatePersistenceError(err, service.ErrUserNotFound, nil)
	}
	oldEmail := existing.Email

	updateOp := txClient.User.UpdateOneID(userIn.ID)
	if fields.Email {
		updateOp = updateOp.SetEmail(userIn.Email)
	}
	if fields.Username {
		updateOp = updateOp.SetUsername(userIn.Username)
	}
	if fields.Notes {
		updateOp = updateOp.SetNotes(userIn.Notes)
	}
	if fields.PasswordHash {
		updateOp = updateOp.SetPasswordHash(userIn.PasswordHash)
	}
	if fields.Role {
		updateOp = updateOp.SetRole(userIn.Role)
	}
	if fields.Concurrency {
		updateOp = updateOp.SetConcurrency(userIn.Concurrency)
	}
	if fields.RPMLimit {
		updateOp = updateOp.SetRpmLimit(userIn.RPMLimit)
	}
	if fields.Status {
		updateOp = updateOp.SetStatus(userIn.Status)
	}
	if fields.DisabledUntil {
		if userIn.DisabledUntil == nil {
			updateOp = updateOp.ClearDisabledUntil()
		} else {
			updateOp = updateOp.SetDisabledUntil(*userIn.DisabledUntil)
		}
	}
	if fields.BalanceNotifySettings {
		updateOp = updateOp.
			SetBalanceNotifyEnabled(userIn.BalanceNotifyEnabled).
			SetBalanceNotifyThresholdType(userIn.BalanceNotifyThresholdType).
			SetNillableBalanceNotifyThreshold(userIn.BalanceNotifyThreshold)
		if userIn.BalanceNotifyThreshold == nil {
			updateOp = updateOp.ClearBalanceNotifyThreshold()
		}
	}
	if fields.BalanceNotifyExtraEmails {
		updateOp = updateOp.SetBalanceNotifyExtraEmails(marshalExtraEmails(userIn.BalanceNotifyExtraEmails))
	}
	if fields.SignupSource && userIn.SignupSource != "" {
		updateOp = updateOp.SetSignupSource(userIn.SignupSource)
	}
	if fields.LastLoginAt && userIn.LastLoginAt != nil {
		updateOp = updateOp.SetLastLoginAt(*userIn.LastLoginAt)
	}
	if fields.LastActiveAt && userIn.LastActiveAt != nil {
		updateOp = updateOp.SetLastActiveAt(*userIn.LastActiveAt)
	}
	updated, err := updateOp.Save(txCtx)
	if err != nil {
		return translatePersistenceError(err, service.ErrUserNotFound, service.ErrEmailExists)
	}

	if fields.AllowedGroups {
		if err := r.syncUserAllowedGroupsWithClient(txCtx, txClient, updated.ID, userIn.AllowedGroups); err != nil {
			return err
		}
	}
	// 始终以库中的邮箱为准补齐 email 身份：未改邮箱时 updated.Email == oldEmail，
	// 这里退化为幂等的身份补写，与改邮箱前的行为一致。
	if err := replaceEmailAuthIdentityWithClient(txCtx, txClient, updated.ID, oldEmail, updated.Email, "user_repo_update"); err != nil {
		return err
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return err
		}
	}

	userIn.UpdatedAt = updated.UpdatedAt
	return nil
}

func ensureEmailAuthIdentityWithClient(ctx context.Context, client *dbent.Client, userID int64, email string, source string) error {
	client = clientFromContext(ctx, client)
	if client == nil || userID <= 0 {
		return nil
	}

	subject := normalizeEmailAuthIdentitySubject(email)
	if subject == "" {
		return nil
	}

	if err := client.AuthIdentity.Create().
		SetUserID(userID).
		SetProviderType("email").
		SetProviderKey("email").
		SetProviderSubject(subject).
		SetVerifiedAt(time.Now().UTC()).
		SetMetadata(map[string]any{"source": source}).
		OnConflictColumns(
			authidentity.FieldProviderType,
			authidentity.FieldProviderKey,
			authidentity.FieldProviderSubject,
		).
		DoNothing().
		Exec(ctx); err != nil {
		if !isSQLNoRowsError(err) {
			return err
		}
	}

	identity, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("email"),
			authidentity.ProviderKeyEQ("email"),
			authidentity.ProviderSubjectEQ(subject),
		).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil
		}
		return err
	}
	if identity.UserID != userID {
		return ErrAuthIdentityOwnershipConflict
	}
	return nil
}

func replaceEmailAuthIdentityWithClient(ctx context.Context, client *dbent.Client, userID int64, oldEmail, newEmail string, source string) error {
	newSubject := normalizeEmailAuthIdentitySubject(newEmail)
	if err := ensureEmailAuthIdentityWithClient(ctx, client, userID, newEmail, source); err != nil {
		return err
	}

	oldSubject := normalizeEmailAuthIdentitySubject(oldEmail)
	if oldSubject == "" || oldSubject == newSubject {
		return nil
	}

	_, err := clientFromContext(ctx, client).AuthIdentity.Delete().
		Where(
			authidentity.UserIDEQ(userID),
			authidentity.ProviderTypeEQ("email"),
			authidentity.ProviderKeyEQ("email"),
			authidentity.ProviderSubjectEQ(oldSubject),
		).
		Exec(ctx)
	return err
}

func normalizeEmailAuthIdentitySubject(email string) string {
	normalized := strings.ToLower(strings.TrimSpace(email))
	if normalized == "" {
		return ""
	}
	if strings.HasSuffix(normalized, service.LinuxDoConnectSyntheticEmailDomain) ||
		strings.HasSuffix(normalized, service.OIDCConnectSyntheticEmailDomain) ||
		strings.HasSuffix(normalized, service.WeChatConnectSyntheticEmailDomain) ||
		strings.HasSuffix(normalized, service.DingTalkConnectSyntheticEmailDomain) {
		return ""
	}
	return normalized
}

func (r *userRepository) Delete(ctx context.Context, id int64) error {
	// 复用 context 中已存在的事务（如 AdminService.DeleteUser 把删 Key 与删 User 包在同一事务中），
	// 由调用方负责提交/回滚，保证两者的原子性。
	if existingTx := dbent.TxFromContext(ctx); existingTx != nil {
		return r.deleteUser(ctx, existingTx.Client(), id)
	}

	tx, err := r.client.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return translatePersistenceError(err, service.ErrUserNotFound, nil)
	}
	exec := r.client
	if err == nil {
		defer func() { _ = tx.Rollback() }()
		exec = tx.Client()
	}
	// err == dbent.ErrTxStarted 时复用当前事务（exec = r.client）。

	if err := r.deleteUser(ctx, exec, id); err != nil {
		return err
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return translatePersistenceError(err, service.ErrUserNotFound, nil)
		}
	}
	return nil
}

// deleteUser 在给定 client（可能是外部事务 client）上删除用户及其身份关联记录，自身不开启/提交事务。
func (r *userRepository) deleteUser(ctx context.Context, exec *dbent.Client, id int64) error {
	identityIDs, err := exec.AuthIdentity.Query().
		Where(authidentity.UserIDEQ(id)).
		IDs(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrUserNotFound, nil)
	}
	if len(identityIDs) > 0 {
		if _, err := exec.IdentityAdoptionDecision.Update().
			Where(identityadoptiondecision.IdentityIDIn(identityIDs...)).
			ClearIdentityID().
			Save(ctx); err != nil {
			return translatePersistenceError(err, service.ErrUserNotFound, nil)
		}
		if _, err := exec.AuthIdentityChannel.Delete().
			Where(authidentitychannel.IdentityIDIn(identityIDs...)).
			Exec(ctx); err != nil {
			return translatePersistenceError(err, service.ErrUserNotFound, nil)
		}
		if _, err := exec.AuthIdentity.Delete().
			Where(authidentity.UserIDEQ(id)).
			Exec(ctx); err != nil {
			return translatePersistenceError(err, service.ErrUserNotFound, nil)
		}
	}

	affected, err := exec.User.Delete().Where(dbuser.IDEQ(id)).Exec(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrUserNotFound, nil)
	}
	if affected == 0 {
		return service.ErrUserNotFound
	}
	return nil
}

func (r *userRepository) List(ctx context.Context, params pagination.PaginationParams) ([]service.User, *pagination.PaginationResult, error) {
	return r.ListWithFilters(ctx, params, service.UserListFilters{})
}

func (r *userRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters service.UserListFilters) ([]service.User, *pagination.PaginationResult, error) {
	// SkipSoftDelete 仅作用于 User 身份解析（下方 Count/All）；订阅、分组等关联实体沿用原始 ctx，避免穿透到这些同样带软删除的实体而带出已删除行。
	userCtx := ctx
	if filters.IncludeDeleted {
		userCtx = mixins.SkipSoftDelete(ctx)
	}

	q := r.client.User.Query()

	if filters.Status != "" {
		q = q.Where(dbuser.StatusEQ(filters.Status))
	}
	if filters.Role != "" {
		q = q.Where(dbuser.RoleEQ(filters.Role))
	}
	if filters.Search != "" {
		q = q.Where(
			dbuser.Or(
				dbuser.EmailContainsFold(filters.Search),
				dbuser.UsernameContainsFold(filters.Search),
				dbuser.NotesContainsFold(filters.Search),
				dbuser.HasAPIKeysWith(apikey.KeyContainsFold(filters.Search)),
			),
		)
	}

	if filters.GroupName != "" {
		q = q.Where(dbuser.HasAllowedGroupsWith(
			dbgroup.NameContainsFold(filters.GroupName),
		))
	}

	if filters.APIKeyGroupID > 0 {
		// 按"API Key 实际绑定的分组"过滤：用户只要有任意一个未软删除的 API Key
		// 绑定到该分组即命中（EXISTS 语义）。
		// 注意：SoftDeleteMixin 的拦截器不会自动下沉到 HasAPIKeysWith 子查询，
		// 必须显式加 apikey.DeletedAtIsNil()，否则已软删除的 key 会污染过滤结果。
		q = q.Where(dbuser.HasAPIKeysWith(
			apikey.GroupIDEQ(filters.APIKeyGroupID),
			apikey.DeletedAtIsNil(),
		))
	}

	// If attribute filters are specified, we need to filter by user IDs first
	var allowedUserIDs []int64
	if len(filters.Attributes) > 0 {
		var attrErr error
		allowedUserIDs, attrErr = r.filterUsersByAttributes(ctx, filters.Attributes)
		if attrErr != nil {
			return nil, nil, attrErr
		}
		if len(allowedUserIDs) == 0 {
			// No users match the attribute filters
			return []service.User{}, paginationResultFromTotal(0, params), nil
		}
		q = q.Where(dbuser.IDIn(allowedUserIDs...))
	}

	total, err := q.Clone().Count(userCtx)
	if err != nil {
		return nil, nil, err
	}

	usersQuery := q.
		Offset(params.Offset()).
		Limit(params.Limit())
	for _, order := range userListOrder(params) {
		usersQuery = usersQuery.Order(order)
	}

	users, err := usersQuery.All(userCtx)
	if err != nil {
		return nil, nil, err
	}

	outUsers := make([]service.User, 0, len(users))
	if len(users) == 0 {
		return outUsers, paginationResultFromTotal(int64(total), params), nil
	}

	userIDs := make([]int64, 0, len(users))
	userMap := make(map[int64]*service.User, len(users))
	for i := range users {
		userIDs = append(userIDs, users[i].ID)
		u := userEntityToService(users[i])
		outUsers = append(outUsers, *u)
		userMap[u.ID] = &outUsers[len(outUsers)-1]
	}

	shouldLoadSubscriptions := filters.IncludeSubscriptions == nil || *filters.IncludeSubscriptions
	if shouldLoadSubscriptions {
		// Batch load active subscriptions with groups to avoid N+1.
		subs, err := r.client.UserSubscription.Query().
			Where(
				usersubscription.UserIDIn(userIDs...),
				usersubscription.StatusEQ(service.SubscriptionStatusActive),
			).
			WithGroup().
			All(ctx)
		if err != nil {
			return nil, nil, err
		}

		for i := range subs {
			if u, ok := userMap[subs[i].UserID]; ok {
				u.Subscriptions = append(u.Subscriptions, *userSubscriptionEntityToService(subs[i]))
			}
		}
	}

	allowedGroupsByUser, err := r.loadAllowedGroups(ctx, userIDs)
	if err != nil {
		return nil, nil, err
	}
	for id, u := range userMap {
		if groups, ok := allowedGroupsByUser[id]; ok {
			u.AllowedGroups = groups
		}
	}
	bannedGroupsByUser, bannedExpirationsByUser, err := r.loadBannedGroups(ctx, userIDs)
	if err != nil {
		return nil, nil, err
	}
	for id, u := range userMap {
		if groups, ok := bannedGroupsByUser[id]; ok {
			u.BannedGroupIDs = groups
		}
		if expirations, ok := bannedExpirationsByUser[id]; ok {
			u.BannedGroupExpirations = expirations
		}
	}

	return outUsers, paginationResultFromTotal(int64(total), params), nil
}

func userListOrder(params pagination.PaginationParams) []func(*entsql.Selector) {
	sortBy := strings.ToLower(strings.TrimSpace(params.SortBy))
	sortOrder := params.NormalizedSortOrder(pagination.SortOrderDesc)

	if sortBy == "last_used_at" {
		return userLastUsedAtOrder(sortOrder)
	}

	var field string
	defaultField := true
	nullsLastField := false
	switch sortBy {
	case "email":
		field = dbuser.FieldEmail
		defaultField = false
	case "username":
		field = dbuser.FieldUsername
		defaultField = false
	case "role":
		field = dbuser.FieldRole
		defaultField = false
	case "balance":
		field = dbuser.FieldBalance
		defaultField = false
	case "concurrency":
		field = dbuser.FieldConcurrency
		defaultField = false
	case "status":
		field = dbuser.FieldStatus
		defaultField = false
	case "created_at":
		field = dbuser.FieldCreatedAt
		defaultField = false
	case "last_active_at":
		field = dbuser.FieldLastActiveAt
		defaultField = false
		nullsLastField = true
	default:
		field = dbuser.FieldID
	}

	if sortOrder == pagination.SortOrderAsc {
		if defaultField && field == dbuser.FieldID {
			return []func(*entsql.Selector){dbent.Asc(dbuser.FieldID)}
		}
		if nullsLastField {
			return []func(*entsql.Selector){
				entsql.OrderByField(field, entsql.OrderNullsLast()).ToFunc(),
				dbent.Asc(dbuser.FieldID),
			}
		}
		return []func(*entsql.Selector){dbent.Asc(field), dbent.Asc(dbuser.FieldID)}
	}
	if defaultField && field == dbuser.FieldID {
		return []func(*entsql.Selector){dbent.Desc(dbuser.FieldID)}
	}
	if nullsLastField {
		return []func(*entsql.Selector){
			entsql.OrderByField(field, entsql.OrderDesc(), entsql.OrderNullsLast()).ToFunc(),
			dbent.Desc(dbuser.FieldID),
		}
	}
	return []func(*entsql.Selector){dbent.Desc(field), dbent.Desc(dbuser.FieldID)}
}

func (r *userRepository) GetLatestUsedAtByUserIDs(ctx context.Context, userIDs []int64) (map[int64]*time.Time, error) {
	result := make(map[int64]*time.Time, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}
	if r.sql == nil {
		return nil, fmt.Errorf("sql executor is not configured")
	}

	const query = `
		SELECT user_id, MAX(created_at) AS last_used_at
		FROM usage_logs
		WHERE user_id = ANY($1)
		GROUP BY user_id
	`

	rows, err := r.sql.QueryContext(ctx, query, pq.Array(userIDs))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			userID     int64
			lastUsedAt time.Time
		)
		if scanErr := rows.Scan(&userID, &lastUsedAt); scanErr != nil {
			return nil, scanErr
		}
		ts := lastUsedAt.UTC()
		result[userID] = &ts
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *userRepository) GetLatestUsedAtByUserID(ctx context.Context, userID int64) (*time.Time, error) {
	latestByUserID, err := r.GetLatestUsedAtByUserIDs(ctx, []int64{userID})
	if err != nil {
		return nil, err
	}
	return latestByUserID[userID], nil
}

func userLastUsedAtOrder(sortOrder string) []func(*entsql.Selector) {
	orderExpr := func(direction, nulls string, tieOrder func(string) string) func(*entsql.Selector) {
		return func(s *entsql.Selector) {
			subquery := fmt.Sprintf("(SELECT MAX(created_at) FROM usage_logs WHERE user_id = %s)", s.C(dbuser.FieldID))
			s.OrderExpr(entsql.Expr(subquery + " " + direction + " NULLS " + nulls))
			s.OrderBy(tieOrder(s.C(dbuser.FieldID)))
		}
	}

	if sortOrder == pagination.SortOrderAsc {
		return []func(*entsql.Selector){
			orderExpr("ASC", "FIRST", entsql.Asc),
		}
	}
	return []func(*entsql.Selector){
		orderExpr("DESC", "LAST", entsql.Desc),
	}
}

// filterUsersByAttributes returns user IDs that match ALL the given attribute filters
func (r *userRepository) filterUsersByAttributes(ctx context.Context, attrs map[int64]string) ([]int64, error) {
	if len(attrs) == 0 {
		return nil, nil
	}

	if r.sql == nil {
		return nil, fmt.Errorf("sql executor is not configured")
	}

	clauses := make([]string, 0, len(attrs))
	args := make([]any, 0, len(attrs)*2+1)
	argIndex := 1
	for attrID, value := range attrs {
		clauses = append(clauses, fmt.Sprintf("(attribute_id = $%d AND value ILIKE $%d)", argIndex, argIndex+1))
		args = append(args, attrID, "%"+value+"%")
		argIndex += 2
	}

	query := fmt.Sprintf(
		`SELECT user_id
		 FROM user_attribute_values
		 WHERE %s
		 GROUP BY user_id
		 HAVING COUNT(DISTINCT attribute_id) = $%d`,
		strings.Join(clauses, " OR "),
		argIndex,
	)
	args = append(args, len(attrs))

	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make([]int64, 0)
	for rows.Next() {
		var userID int64
		if scanErr := rows.Scan(&userID); scanErr != nil {
			return nil, scanErr
		}
		result = append(result, userID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *userRepository) UpdateBalance(ctx context.Context, id int64, amount float64) error {
	client := clientFromContext(ctx, r.client)
	update := client.User.Update().Where(dbuser.IDEQ(id)).AddBalance(amount)
	// Track cumulative recharge amount for percentage-based notifications
	if amount > 0 {
		update = update.AddTotalRecharged(amount)
	}
	n, err := update.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrUserNotFound, nil)
	}
	if n == 0 {
		return service.ErrUserNotFound
	}
	return nil
}

// ExpireDailyCheckinRewards atomically removes expired check-in rewards from
// the user's balance. The ledger rows are cleared even when the balance was
// already spent, preserving the single-use semantics of the grant.
func (r *userRepository) ExpireDailyCheckinRewards(ctx context.Context, userID int64) (bool, error) {
	exec := r.antiAbuseExecutor(ctx)
	if r == nil || exec == nil || userID <= 0 {
		return false, nil
	}
	result, err := exec.ExecContext(ctx, `
WITH expired AS (
	SELECT id, remaining_amount
	FROM daily_checkin_rewards
	WHERE user_id = $1 AND remaining_amount > 0 AND expires_at <= NOW()
	FOR UPDATE
), marked AS (
	UPDATE daily_checkin_rewards r
	SET remaining_amount = 0, updated_at = NOW()
	FROM expired e
	WHERE r.id = e.id
	RETURNING e.remaining_amount
), total AS (
	SELECT COALESCE(SUM(remaining_amount), 0) AS amount FROM marked
)
UPDATE users
SET balance = balance - LEAST((SELECT amount FROM total), GREATEST(balance, 0)), updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL AND (SELECT amount FROM total) > 0`, userID)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}

func (r *userRepository) ApplyRedeemBalanceAdjustment(ctx context.Context, id int64, delta float64) error {
	const updateSQL = `
		UPDATE users
		SET balance = GREATEST(balance + $1, 0), updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
	`
	client := clientFromContext(ctx, r.client)
	result, err := client.ExecContext(ctx, updateSQL, delta, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrUserNotFound
	}
	return nil
}

// RecordRiskControlBalanceDeduction writes an already-applied risk-control
// deduction into redeem_codes so user and admin history views share it.
func (r *userRepository) RecordRiskControlBalanceDeduction(ctx context.Context, userID int64, amount float64, notes string) error {
	if userID <= 0 || amount <= 0 {
		return nil
	}
	code, err := service.GenerateRedeemCode()
	if err != nil {
		return err
	}
	usedAt := time.Now().UTC()
	created, err := clientFromContext(ctx, r.client).RedeemCode.Create().
		SetCode(code).
		SetType(service.AdjustmentTypeRiskControlBalance).
		SetValue(-amount).
		SetStatus(service.StatusUsed).
		SetNillableUsedBy(&userID).
		SetNillableUsedAt(&usedAt).
		SetNotes(strings.TrimSpace(notes)).
		SetValidityDays(0).
		Save(ctx)
	if err != nil {
		return err
	}
	_ = created
	return nil
}

// DeductBalance 扣除用户余额
// 透支策略：允许余额变为负数，确保当前请求能够完成
// 中间件会阻止余额 <= 0 的用户发起后续请求
func (r *userRepository) DeductBalance(ctx context.Context, id int64, amount float64) error {
	client := clientFromContext(ctx, r.client)
	n, err := client.User.Update().
		Where(dbuser.IDEQ(id), dbuser.BalanceGTE(amount)).
		AddBalance(-amount).
		Save(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}

	n, err = client.User.Update().
		Where(dbuser.IDEQ(id)).
		AddBalance(-amount).
		Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return service.ErrUserNotFound
	}
	return nil
}

// DeductAvailableBalance atomically deducts min(amount, max(balance, 0)).
// Unlike DeductBalance, this refund-specific operation never increases an
// existing deficit or permits a concurrent deduction to cause an overdraft.
func (r *userRepository) DeductAvailableBalance(ctx context.Context, id int64, amount float64) (deducted float64, err error) {
	if amount < 0 {
		return 0, fmt.Errorf("deduction amount must be nonnegative")
	}
	const updateSQL = `
		WITH target AS (
			SELECT id, balance
			FROM users
			WHERE id = $2 AND deleted_at IS NULL
			FOR UPDATE
		), updated AS (
			UPDATE users AS u
			SET balance = target.balance - LEAST($1, GREATEST(target.balance, 0)), updated_at = NOW()
			FROM target
			WHERE u.id = target.id AND u.deleted_at IS NULL
			RETURNING target.balance - u.balance AS deducted
		)
		SELECT deducted FROM updated
	`
	rows, err := clientFromContext(ctx, r.client).QueryContext(ctx, updateSQL, amount, id)
	if err != nil {
		return 0, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()
	if !rows.Next() {
		if rowsErr := rows.Err(); rowsErr != nil {
			return 0, rowsErr
		}
		return 0, service.ErrUserNotFound
	}
	if err := rows.Scan(&deducted); err != nil {
		return 0, err
	}
	return deducted, rows.Err()
}

// DeductAvailableGiftBalance atomically deducts balance while the user has no
// recharge history. A concurrent paid recharge makes the predicate fail.
func (r *userRepository) DeductAvailableGiftBalance(ctx context.Context, id int64, amount float64) (deducted float64, err error) {
	if amount < 0 {
		return 0, fmt.Errorf("deduction amount must be nonnegative")
	}
	const updateSQL = `
		WITH target AS (
			SELECT id, balance
			FROM users
			WHERE id = $2 AND deleted_at IS NULL AND total_recharged <= 0
			FOR UPDATE
		), updated AS (
			UPDATE users AS u
			SET balance = target.balance - LEAST($1, GREATEST(target.balance, 0)), updated_at = NOW()
			FROM target
			WHERE u.id = target.id AND u.deleted_at IS NULL
			RETURNING target.balance - u.balance AS deducted
		)
		SELECT deducted FROM updated
	`
	rows, err := clientFromContext(ctx, r.client).QueryContext(ctx, updateSQL, amount, id)
	if err != nil {
		return 0, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()
	if !rows.Next() {
		if rowsErr := rows.Err(); rowsErr != nil {
			return 0, rowsErr
		}
		return 0, nil
	}
	if err := rows.Scan(&deducted); err != nil {
		return 0, err
	}
	return deducted, rows.Err()
}

// AdjustBalance 原子地把 delta 累加到余额上，结果为负时整条语句不生效。
// 相比"读余额 → 算新值 → 整行写回"，这里把读与写压进同一条 UPDATE，
// 并发的计费扣款不会被旧快照覆盖。
func (r *userRepository) AdjustBalance(ctx context.Context, id int64, delta float64) (service.BalanceChange, error) {
	const updateSQL = `
		UPDATE users
		SET balance = balance + $1, updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL AND balance + $1 >= 0
		RETURNING balance - $1, balance
	`
	change, ok, err := scanBalanceChange(ctx, clientFromContext(ctx, r.client), updateSQL, delta, id)
	if err != nil {
		return service.BalanceChange{}, err
	}
	if ok {
		return change, nil
	}

	// 0 行既可能是用户不存在，也可能是余额不足以承受这次扣减，需要区分。
	current, err := r.currentBalance(ctx, id)
	if err != nil {
		return service.BalanceChange{}, err
	}
	return service.BalanceChange{Old: current, New: current + delta}, service.ErrBalanceNegative
}

// SetBalance 原子地把余额置为 value，并返回变更前后的值。
func (r *userRepository) SetBalance(ctx context.Context, id int64, value float64) (service.BalanceChange, error) {
	if value < 0 {
		// 连同当前余额一起返回，便于上层给出可读的错误信息。
		current, err := r.currentBalance(ctx, id)
		if err != nil {
			return service.BalanceChange{}, err
		}
		return service.BalanceChange{Old: current, New: value}, service.ErrBalanceNegative
	}
	const updateSQL = `
		UPDATE users AS u
		SET balance = $1, updated_at = NOW()
		FROM (SELECT id, balance FROM users WHERE id = $2 AND deleted_at IS NULL) AS prev
		WHERE u.id = prev.id AND u.deleted_at IS NULL
		RETURNING prev.balance, u.balance
	`
	change, ok, err := scanBalanceChange(ctx, clientFromContext(ctx, r.client), updateSQL, value, id)
	if err != nil {
		return service.BalanceChange{}, err
	}
	if !ok {
		return service.BalanceChange{}, service.ErrUserNotFound
	}
	return change, nil
}

// currentBalance 读取用户当前余额，用户不存在时返回 ErrUserNotFound。
func (r *userRepository) currentBalance(ctx context.Context, id int64) (balance float64, err error) {
	rows, err := clientFromContext(ctx, r.client).QueryContext(ctx,
		`SELECT balance FROM users WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return 0, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()
	if !rows.Next() {
		if rowsErr := rows.Err(); rowsErr != nil {
			return 0, rowsErr
		}
		return 0, service.ErrUserNotFound
	}
	if err := rows.Scan(&balance); err != nil {
		return 0, err
	}
	return balance, rows.Err()
}

// scanBalanceChange 执行一条 RETURNING 旧余额、新余额的语句。ok 为 false 表示语句未命中任何行。
func scanBalanceChange(ctx context.Context, client *dbent.Client, query string, args ...any) (change service.BalanceChange, ok bool, err error) {
	rows, err := client.QueryContext(ctx, query, args...)
	if err != nil {
		return service.BalanceChange{}, false, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()
	if !rows.Next() {
		if rowsErr := rows.Err(); rowsErr != nil {
			return service.BalanceChange{}, false, rowsErr
		}
		return service.BalanceChange{}, false, nil
	}
	if err := rows.Scan(&change.Old, &change.New); err != nil {
		return service.BalanceChange{}, false, err
	}
	return change, true, rows.Err()
}

func (r *userRepository) UpdateConcurrency(ctx context.Context, id int64, amount int) error {
	client := clientFromContext(ctx, r.client)
	n, err := client.User.Update().Where(dbuser.IDEQ(id)).AddConcurrency(amount).Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrUserNotFound, nil)
	}
	if n == 0 {
		return service.ErrUserNotFound
	}
	return nil
}

func (r *userRepository) ApplyRedeemConcurrencyAdjustment(ctx context.Context, id int64, delta int) error {
	const updateSQL = `
		UPDATE users
		SET concurrency = GREATEST(concurrency + $1, 0), updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
	`
	client := clientFromContext(ctx, r.client)
	result, err := client.ExecContext(ctx, updateSQL, delta, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrUserNotFound
	}
	return nil
}

func (r *userRepository) BatchSetConcurrency(ctx context.Context, userIDs []int64, value int) (int, error) {
	if len(userIDs) == 0 {
		return 0, nil
	}
	if value < 0 {
		value = 0
	}
	res, err := r.sql.ExecContext(ctx,
		"UPDATE users SET concurrency = $1, updated_at = NOW() WHERE id = ANY($2) AND deleted_at IS NULL",
		value, pq.Array(userIDs))
	if err != nil {
		return 0, fmt.Errorf("batch set concurrency: %w", err)
	}
	affected, _ := res.RowsAffected()
	return int(affected), nil
}

func (r *userRepository) BatchAddConcurrency(ctx context.Context, userIDs []int64, delta int) (int, error) {
	if len(userIDs) == 0 {
		return 0, nil
	}
	res, err := r.sql.ExecContext(ctx,
		"UPDATE users SET concurrency = GREATEST(concurrency + $1, 0), updated_at = NOW() WHERE id = ANY($2) AND deleted_at IS NULL",
		delta, pq.Array(userIDs))
	if err != nil {
		return 0, fmt.Errorf("batch add concurrency: %w", err)
	}
	affected, _ := res.RowsAffected()
	return int(affected), nil
}

func (r *userRepository) BatchUpdateLimits(ctx context.Context, userIDs []int64, concurrency, rpmLimit *int) (int, error) {
	if len(userIDs) == 0 || (concurrency == nil && rpmLimit == nil) {
		return 0, nil
	}

	setClauses := make([]string, 0, 3)
	args := make([]any, 0, 3)
	if concurrency != nil {
		value := max(*concurrency, 0)
		args = append(args, value)
		setClauses = append(setClauses, fmt.Sprintf("concurrency = $%d", len(args)))
	}
	if rpmLimit != nil {
		value := max(*rpmLimit, 0)
		args = append(args, value)
		setClauses = append(setClauses, fmt.Sprintf("rpm_limit = $%d", len(args)))
	}
	setClauses = append(setClauses, "updated_at = NOW()")
	args = append(args, pq.Array(userIDs))

	query := fmt.Sprintf(
		"UPDATE users SET %s WHERE id = ANY($%d) AND deleted_at IS NULL",
		strings.Join(setClauses, ", "),
		len(args),
	)
	res, err := r.sql.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("batch update user limits: %w", err)
	}
	affected, _ := res.RowsAffected()
	return int(affected), nil
}

func (r *userRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	return r.client.User.Query().Where(userEmailLookupPredicate(email)).Exist(ctx)
}

// emailAliasCandidateLimit 限制一次别名查重最多取回的候选行数。探针都以去点后的
// 本地部分为前缀锚定（见 dotStrippedEmailExpr），正常收件箱的变体只有个位数；
// 上限只是兜底，避免公开未鉴权的注册/发码端点把大表整张读进内存。
const emailAliasCandidateLimit = 50

// ExistsByEmailAlias 见 service.UserRepository。软删除过滤沿用 ExistsByEmail 的默认行为。
func (r *userRepository) ExistsByEmailAlias(ctx context.Context, email string) (bool, error) {
	return existsByEmailAliasWithClient(ctx, clientFromContext(ctx, r.client), email)
}

func existsByEmailAliasWithClient(ctx context.Context, client *dbent.Client, email string) (bool, error) {
	if client == nil {
		return false, nil
	}
	probes := service.EmailAliasDedupProbes(email)
	if len(probes) == 0 {
		return false, nil
	}

	preds := make([]predicate.User, 0, 2*len(probes))
	for _, probe := range probes {
		preds = append(preds,
			dotStrippedEmailEQ(probe.Local+"@"+probe.Domain),
			// "+后缀"的内容未知，只能按前缀匹配。
			dotStrippedEmailLike(escapeLikeWildcards(probe.Local)+"+%@"+escapeLikeWildcards(probe.Domain)),
		)
	}
	candidates, err := client.User.Query().
		Where(dbuser.Or(preds...)).
		Limit(emailAliasCandidateLimit).
		Select(dbuser.FieldEmail).
		Strings(ctx)
	if err != nil {
		return false, err
	}

	// 探针会有过度匹配（点号只在 Gmail 家族无意义），最终判定必须回到完整归一化规则。
	identity := service.NormalizeEmailForAliasDedup(email)
	for _, candidate := range candidates {
		if service.NormalizeEmailForAliasDedup(candidate) == identity {
			return true, nil
		}
	}
	return false, nil
}

// dotStrippedEmailExpr 渲染下面的表达式：去掉存量邮箱的大小写、首尾空白（与
// userEmailLookupPredicate 的精确匹配口径一致，历史数据存在带空白的行）以及全部点号。
//
//	REPLACE(LOWER(TRIM(email)), '.', '')
//
// 两侧都去点，因此一个域名探针即可同时覆盖 Gmail 点号变体与 FQDN 根点（user@gmail.com.）。
// migrations/190 为同一表达式建了索引。
func dotStrippedEmailExpr(b *entsql.Builder, s *entsql.Selector) *entsql.Builder {
	return b.WriteString("REPLACE(LOWER(TRIM(").
		Ident(s.C(dbuser.FieldEmail)).
		WriteString(")), '.', '')")
}

func dotStrippedEmailEQ(value string) predicate.User {
	return predicate.User(func(s *entsql.Selector) {
		s.Where(entsql.P(func(b *entsql.Builder) {
			dotStrippedEmailExpr(b, s).WriteString(" = ").Arg(value)
		}))
	})
}

func dotStrippedEmailLike(pattern string) predicate.User {
	return predicate.User(func(s *entsql.Selector) {
		s.Where(entsql.P(func(b *entsql.Builder) {
			dotStrippedEmailExpr(b, s).WriteString(" LIKE ").Arg(pattern).WriteString(` ESCAPE '\'`)
		}))
	})
}

// escapeLikeWildcards 转义 LIKE 元字符：本地部分合法可含 % 与 _，不转义会扩大匹配面。
var likeWildcardEscaper = strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`)

func escapeLikeWildcards(value string) string {
	return likeWildcardEscaper.Replace(value)
}

func ensureNormalizedEmailAvailableWithClient(ctx context.Context, client *dbent.Client, userID int64, email string) error {
	client = clientFromContext(ctx, client)
	if client == nil {
		return nil
	}

	matches, err := client.User.Query().
		Where(userEmailLookupPredicate(email)).
		All(ctx)
	if err != nil {
		return err
	}
	for _, match := range matches {
		if match.ID != userID {
			return service.ErrEmailExists
		}
	}
	return nil
}

func userEmailLookupPredicate(email string) predicate.User {
	normalized := normalizeEmailLookupValue(email)
	if normalized == "" {
		return dbuser.EmailEQ(email)
	}
	return predicate.User(func(s *entsql.Selector) {
		s.Where(entsql.P(func(b *entsql.Builder) {
			b.WriteString("LOWER(TRIM(").
				Ident(s.C(dbuser.FieldEmail)).
				WriteString(")) = ").
				Arg(normalized)
		}))
	})
}

func normalizeEmailLookupValue(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func normalizedEmailUniquenessLockKey(email string) string {
	normalized := normalizeEmailLookupValue(email)
	if normalized == "" {
		return ""
	}
	return "users:normalized-email:" + normalized
}

func registrationEmailDomainLockKey(domain string) string {
	domain = normalizeEmailDomain(domain)
	if domain == "" {
		return ""
	}
	return "users:registration-email-domain:" + domain
}

func normalizeEmailDomain(domain string) string {
	return service.NormalizeRegistrationEmailDomain(domain)
}

func countUsersByEmailDomainWithClient(ctx context.Context, client *dbent.Client, domain string) (int, error) {
	client = clientFromContext(ctx, client)
	domain = normalizeEmailDomain(domain)
	if client == nil || domain == "" {
		return 0, nil
	}
	return client.User.Query().Where(userEmailDomainPredicate(domain)).Count(ctx)
}

func userEmailDomainPredicate(domain string) predicate.User {
	domain = normalizeEmailDomain(domain)
	escapedDomain := escapeLikeWildcards(domain)
	exactPattern := "%@" + escapedDomain
	subdomainPattern := "%@%." + escapedDomain
	return predicate.User(func(s *entsql.Selector) {
		s.Where(entsql.P(func(b *entsql.Builder) {
			b.WriteString("(RTRIM(LOWER(TRIM(").
				Ident(s.C(dbuser.FieldEmail)).
				WriteString(")), '.') LIKE ").
				Arg(exactPattern).
				WriteString(` ESCAPE '\' OR RTRIM(LOWER(TRIM(`).
				Ident(s.C(dbuser.FieldEmail)).
				WriteString(")), '.') LIKE ").
				Arg(subdomainPattern).
				WriteString(` ESCAPE '\'`).
				WriteString(")")
		}))
	})
}

// emailAliasUniquenessLockKey 按收件箱身份（而非邮箱字面量）加锁，使同一收件箱的不同
// 别名变体在注册时互斥。
func emailAliasUniquenessLockKey(email string) string {
	identity := service.NormalizeEmailForAliasDedup(email)
	if identity == "" {
		return ""
	}
	return "users:email-alias-identity:" + identity
}

func (r *userRepository) AddGroupToAllowedGroups(ctx context.Context, userID int64, groupID int64) error {
	client := clientFromContext(ctx, r.client)
	err := client.UserAllowedGroup.Create().
		SetUserID(userID).
		SetGroupID(groupID).
		OnConflictColumns(userallowedgroup.FieldUserID, userallowedgroup.FieldGroupID).
		DoNothing().
		Exec(ctx)
	if isSQLNoRowsError(err) {
		return nil
	}
	return err
}

func (r *userRepository) RemoveGroupFromAllowedGroups(ctx context.Context, groupID int64) (int64, error) {
	// 仅操作 user_allowed_groups 联接表，legacy users.allowed_groups 列已弃用。
	affected, err := r.client.UserAllowedGroup.Delete().
		Where(userallowedgroup.GroupIDEQ(groupID)).
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	return int64(affected), nil
}

// RemoveGroupFromUserAllowedGroups 移除单个用户的指定分组权限
func (r *userRepository) RemoveGroupFromUserAllowedGroups(ctx context.Context, userID int64, groupID int64) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.UserAllowedGroup.Delete().
		Where(userallowedgroup.UserIDEQ(userID), userallowedgroup.GroupIDEQ(groupID)).
		Exec(ctx)
	return err
}

// BanGroupForUser records a user-scoped ban for a group. The operation is
// idempotent so repeated moderation hits do not create duplicate rows.
func (r *userRepository) BanGroupForUser(ctx context.Context, userID, groupID int64, expiresAt *time.Time) error {
	if userID <= 0 || groupID <= 0 || r.sql == nil {
		return nil
	}
	var expiry any
	if expiresAt != nil {
		expiry = *expiresAt
	}
	_, err := r.sql.ExecContext(ctx, `
INSERT INTO user_banned_groups (user_id, group_id, expires_at)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, group_id) DO UPDATE SET expires_at = EXCLUDED.expires_at`, userID, groupID, expiry)
	return err
}

// UnbanGroupForUser removes only the requested user-group ban.
func (r *userRepository) UnbanGroupForUser(ctx context.Context, userID, groupID int64) error {
	if userID <= 0 || groupID <= 0 || r.sql == nil {
		return nil
	}
	_, err := r.sql.ExecContext(ctx, `DELETE FROM user_banned_groups WHERE user_id = $1 AND group_id = $2`, userID, groupID)
	return err
}

func (r *userRepository) ListBannedGroupIDs(ctx context.Context, userID int64) ([]int64, map[int64]time.Time, error) {
	if userID <= 0 || r.sql == nil {
		return []int64{}, nil, nil
	}
	rows, err := r.sql.QueryContext(ctx, `SELECT group_id, expires_at FROM user_banned_groups WHERE user_id = $1 ORDER BY group_id`, userID)
	if err != nil {
		if isMissingUserBannedGroupsTable(err) {
			return []int64{}, nil, nil
		}
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make([]int64, 0)
	expirations := make(map[int64]time.Time)
	for rows.Next() {
		var groupID int64
		var expiresAt *time.Time
		if err := rows.Scan(&groupID, &expiresAt); err != nil {
			return nil, nil, err
		}
		result = append(result, groupID)
		if expiresAt != nil {
			expirations[groupID] = *expiresAt
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return result, expirations, nil
}

func (r *userRepository) GetFirstAdmin(ctx context.Context) (*service.User, error) {
	m, err := r.client.User.Query().
		Where(
			dbuser.RoleEQ(service.RoleAdmin),
			dbuser.StatusEQ(service.StatusActive),
		).
		Order(dbent.Asc(dbuser.FieldID)).
		First(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrUserNotFound, nil)
	}

	out := userEntityToService(m)
	groups, err := r.loadAllowedGroups(ctx, []int64{m.ID})
	if err != nil {
		return nil, err
	}
	if v, ok := groups[m.ID]; ok {
		out.AllowedGroups = v
	}
	banned, expirations, err := r.loadBannedGroups(ctx, []int64{m.ID})
	if err != nil {
		return nil, err
	}
	if v, ok := banned[m.ID]; ok {
		out.BannedGroupIDs = v
	}
	if v, ok := expirations[m.ID]; ok {
		out.BannedGroupExpirations = v
	}
	return out, nil
}

func (r *userRepository) loadAllowedGroups(ctx context.Context, userIDs []int64) (map[int64][]int64, error) {
	out := make(map[int64][]int64, len(userIDs))
	if len(userIDs) == 0 {
		return out, nil
	}

	rows, err := r.client.UserAllowedGroup.Query().
		Where(userallowedgroup.UserIDIn(userIDs...)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	for i := range rows {
		out[rows[i].UserID] = append(out[rows[i].UserID], rows[i].GroupID)
	}

	for userID := range out {
		sort.Slice(out[userID], func(i, j int) bool { return out[userID][i] < out[userID][j] })
	}

	return out, nil
}

func (r *userRepository) loadBannedGroups(ctx context.Context, userIDs []int64) (map[int64][]int64, map[int64]map[int64]time.Time, error) {
	out := make(map[int64][]int64, len(userIDs))
	expirations := make(map[int64]map[int64]time.Time, len(userIDs))
	if len(userIDs) == 0 || r.sql == nil {
		return out, expirations, nil
	}
	rows, err := r.sql.QueryContext(ctx, `
SELECT user_id, group_id, expires_at
FROM user_banned_groups
WHERE user_id = ANY($1)
ORDER BY user_id, group_id`, pq.Array(userIDs))
	if err != nil {
		if isMissingUserBannedGroupsTable(err) {
			return out, expirations, nil
		}
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var userID, groupID int64
		var expiresAt *time.Time
		if err := rows.Scan(&userID, &groupID, &expiresAt); err != nil {
			return nil, nil, err
		}
		out[userID] = append(out[userID], groupID)
		if expiresAt != nil {
			if expirations[userID] == nil {
				expirations[userID] = make(map[int64]time.Time)
			}
			expirations[userID][groupID] = *expiresAt
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return out, expirations, nil
}

func isMissingUserBannedGroupsTable(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "user_banned_groups") &&
		(strings.Contains(message, "no such table") || strings.Contains(message, "does not exist"))
}

// syncUserAllowedGroupsWithClient 在 ent client/事务内同步用户允许分组：
// 仅操作 user_allowed_groups 联接表，legacy users.allowed_groups 列已弃用。
func (r *userRepository) syncUserAllowedGroupsWithClient(ctx context.Context, client *dbent.Client, userID int64, groupIDs []int64) error {
	if client == nil {
		return nil
	}

	existingRows, err := client.UserAllowedGroup.Query().
		Where(userallowedgroup.UserIDEQ(userID)).
		All(ctx)
	if err != nil {
		return err
	}

	desired := make(map[int64]struct{}, len(groupIDs))
	for _, id := range groupIDs {
		if id <= 0 {
			continue
		}
		desired[id] = struct{}{}
	}

	existing := make(map[int64]struct{}, len(existingRows))
	removed := make([]int64, 0)
	for _, row := range existingRows {
		existing[row.GroupID] = struct{}{}
		if _, keep := desired[row.GroupID]; !keep {
			removed = append(removed, row.GroupID)
		}
	}
	if len(removed) > 0 {
		if _, err := client.UserAllowedGroup.Delete().
			Where(userallowedgroup.UserIDEQ(userID), userallowedgroup.GroupIDIn(removed...)).
			Exec(ctx); err != nil {
			return err
		}
	}

	creates := make([]*dbent.UserAllowedGroupCreate, 0, len(desired))
	for groupID := range desired {
		if _, present := existing[groupID]; !present {
			creates = append(creates, client.UserAllowedGroup.Create().SetUserID(userID).SetGroupID(groupID))
		}
	}
	if len(creates) > 0 {
		if err := client.UserAllowedGroup.
			CreateBulk(creates...).
			OnConflictColumns(userallowedgroup.FieldUserID, userallowedgroup.FieldGroupID).
			DoNothing().
			Exec(ctx); err != nil {
			if isSQLNoRowsError(err) {
				return nil
			}
			return err
		}
	}

	return nil
}

func applyUserEntityToService(dst *service.User, src *dbent.User) {
	if dst == nil || src == nil {
		return
	}
	dst.ID = src.ID
	dst.SignupIP = src.SignupIP
	dst.DisabledUntil = src.DisabledUntil
	dst.SignupSource = src.SignupSource
	dst.LastLoginAt = src.LastLoginAt
	dst.LastActiveAt = src.LastActiveAt
	dst.CreatedAt = src.CreatedAt
	dst.UpdatedAt = src.UpdatedAt
}

func userSignupSourceOrDefault(signupSource string) string {
	switch strings.TrimSpace(strings.ToLower(signupSource)) {
	case "", "email":
		return "email"
	case "linuxdo", "wechat", "oidc", "dingtalk":
		return strings.TrimSpace(strings.ToLower(signupSource))
	default:
		return "email"
	}
}

// marshalExtraEmails serializes notify email entries to JSON for storage.
func marshalExtraEmails(entries []service.NotifyEmailEntry) string {
	return service.MarshalNotifyEmails(entries)
}

// UpdateTotpSecret 更新用户的 TOTP 加密密钥
func (r *userRepository) UpdateTotpSecret(ctx context.Context, userID int64, encryptedSecret *string) error {
	client := clientFromContext(ctx, r.client)
	update := client.User.UpdateOneID(userID)
	if encryptedSecret == nil {
		update = update.ClearTotpSecretEncrypted()
	} else {
		update = update.SetTotpSecretEncrypted(*encryptedSecret)
	}
	_, err := update.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrUserNotFound, nil)
	}
	return nil
}

// EnableTotp 启用用户的 TOTP 双因素认证
func (r *userRepository) EnableTotp(ctx context.Context, userID int64) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.User.UpdateOneID(userID).
		SetTotpEnabled(true).
		SetTotpEnabledAt(time.Now()).
		Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrUserNotFound, nil)
	}
	return nil
}

// DisableTotp 禁用用户的 TOTP 双因素认证
func (r *userRepository) DisableTotp(ctx context.Context, userID int64) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.User.UpdateOneID(userID).
		SetTotpEnabled(false).
		ClearTotpEnabledAt().
		ClearTotpSecretEncrypted().
		Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrUserNotFound, nil)
	}
	return nil
}
