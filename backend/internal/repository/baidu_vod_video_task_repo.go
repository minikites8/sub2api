package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type baiduVODVideoTaskRepository struct{ client *dbent.Client }

func NewBaiduVODVideoTaskRepository(client *dbent.Client) service.BaiduVODVideoTaskRepository {
	return &baiduVODVideoTaskRepository{client: client}
}

const baiduVODTaskColumns = `id, platform, provider, task_id, upstream_task_id, upstream_request_id,
 user_id, api_key_id, account_id, group_id, model, upstream_model, capability,
 status, upstream_status, resolution, ratio, requested_duration, output_duration,
 input_video_duration, video_count, billing_mode, estimated_cost, hold_amount, actual_cost,
 group_rate_multiplier, video_rate_multiplier, account_rate_multiplier, request_hash,
 result_url, result_expires_at, last_error_code, last_error_message, retry_count,
 version, next_poll_at, poll_claimed_until, last_polled_at, created_at, updated_at,
 submitted_at, started_at, finished_at, settled_at`

func (r *baiduVODVideoTaskRepository) Create(ctx context.Context, task *service.BaiduVODVideoTask) error {
	if r == nil || r.client == nil || task == nil {
		return errors.New("baidu vod task repository is not configured")
	}
	if task.NextPollAt.IsZero() {
		task.NextPollAt = time.Now()
	}
	rows, err := r.client.QueryContext(ctx, `INSERT INTO baidu_vod_video_tasks
 (platform, provider, task_id, upstream_task_id, upstream_request_id, user_id, api_key_id, account_id, group_id,
  model, upstream_model, capability, status, upstream_status, resolution, ratio, requested_duration,
  output_duration, input_video_duration, video_count, billing_mode, estimated_cost, hold_amount,
  group_rate_multiplier, video_rate_multiplier, account_rate_multiplier, request_hash, next_poll_at,
  submitted_at)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29)
 RETURNING id`,
		task.Platform, task.Provider, task.TaskID, task.UpstreamTaskID, baiduVODNullableString(task.UpstreamRequestID), task.UserID, task.APIKeyID,
		task.AccountID, baiduVODNullableInt64(task.GroupID), task.Model, task.UpstreamModel, string(task.Capability), task.Status,
		task.UpstreamStatus, task.Resolution, task.Ratio, task.RequestedDuration, task.OutputDuration,
		task.InputVideoDuration, task.VideoCount, task.BillingMode, task.EstimatedCost, task.HoldAmount, task.GroupRateMultiplier,
		task.VideoRateMultiplier, task.AccountRateMultiplier, task.RequestHash, task.NextPollAt, task.SubmittedAt)
	if err != nil {
		return err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return errors.New("baidu vod task insert returned no id")
	}
	return rows.Scan(&task.ID)
}

func (r *baiduVODVideoTaskRepository) GetByTaskID(ctx context.Context, taskID string) (*service.BaiduVODVideoTask, error) {
	return r.get(ctx, `WHERE platform = $1 AND task_id = $2`, service.PlatformBaiduVOD, strings.TrimSpace(taskID))
}

func (r *baiduVODVideoTaskRepository) GetForOwner(ctx context.Context, userID, apiKeyID int64, taskID string) (*service.BaiduVODVideoTask, error) {
	return r.get(ctx, `WHERE platform = $1 AND task_id = $2 AND user_id = $3 AND api_key_id = $4`, service.PlatformBaiduVOD, strings.TrimSpace(taskID), userID, apiKeyID)
}

func (r *baiduVODVideoTaskRepository) get(ctx context.Context, where string, args ...any) (*service.BaiduVODVideoTask, error) {
	if r == nil || r.client == nil {
		return nil, errors.New("baidu vod task repository is not configured")
	}
	rows, err := r.client.QueryContext(ctx, `SELECT `+baiduVODTaskColumns+` FROM baidu_vod_video_tasks `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		if errors.Is(rows.Err(), sql.ErrNoRows) || rows.Err() == nil {
			return nil, sql.ErrNoRows
		}
		return nil, rows.Err()
	}
	task, err := scanBaiduVODVideoTask(rows)
	if err != nil {
		return nil, err
	}
	return task, nil
}

func (r *baiduVODVideoTaskRepository) ListDue(ctx context.Context, now time.Time, limit int) ([]*service.BaiduVODVideoTask, error) {
	if r == nil || r.client == nil {
		return nil, errors.New("baidu vod task repository is not configured")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.client.QueryContext(ctx, `SELECT `+baiduVODTaskColumns+` FROM baidu_vod_video_tasks
 WHERE platform = $1 AND status IN ($2,$3,$4) AND next_poll_at <= $5
   AND (poll_claimed_until IS NULL OR poll_claimed_until < $5)
 ORDER BY next_poll_at ASC LIMIT $6`, service.PlatformBaiduVOD, service.BaiduVODTaskStatusSubmitting,
		service.BaiduVODTaskStatusQueued, service.BaiduVODTaskStatusInProgress, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]*service.BaiduVODVideoTask, 0, limit)
	for rows.Next() {
		task, scanErr := scanBaiduVODVideoTask(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, task)
	}
	return result, rows.Err()
}

func (r *baiduVODVideoTaskRepository) Claim(ctx context.Context, taskID string, version int, until time.Time) (bool, error) {
	if r == nil || r.client == nil {
		return false, errors.New("baidu vod task repository is not configured")
	}
	result, err := r.client.ExecContext(ctx, `UPDATE baidu_vod_video_tasks
 SET poll_claimed_until = $1, version = version + 1, updated_at = NOW()
 WHERE platform = $2 AND task_id = $3 AND version = $4
   AND status IN ($5,$6,$7) AND (poll_claimed_until IS NULL OR poll_claimed_until < NOW())`, until,
		service.PlatformBaiduVOD, strings.TrimSpace(taskID), version, service.BaiduVODTaskStatusSubmitting,
		service.BaiduVODTaskStatusQueued, service.BaiduVODTaskStatusInProgress)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

func (r *baiduVODVideoTaskRepository) MarkSubmitted(ctx context.Context, taskID string, submitted service.BaiduVODSubmitResult, nextPollAt time.Time) (bool, error) {
	if r == nil || r.client == nil {
		return false, errors.New("baidu vod task repository is not configured")
	}
	result, err := r.client.ExecContext(ctx, `UPDATE baidu_vod_video_tasks SET
 upstream_task_id=$1, upstream_request_id=$2, status=$3, upstream_status=$4,
 next_poll_at=$5, submitted_at=NOW(), poll_claimed_until=NULL, version=version+1, updated_at=NOW()
 WHERE platform=$6 AND task_id=$7 AND status=$8`, strings.TrimSpace(submitted.TaskID),
		baiduVODNullableString(optionalBaiduVODString(submitted.RequestID)), service.BaiduVODTaskStatusQueued,
		firstBaiduVODNonEmpty(submitted.TaskStatus, "PENDING"), nextPollAt, service.PlatformBaiduVOD,
		strings.TrimSpace(taskID), service.BaiduVODTaskStatusSubmitting)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

func (r *baiduVODVideoTaskRepository) MarkSubmissionFailed(ctx context.Context, taskID, code, message string, finishedAt time.Time) (bool, error) {
	if r == nil || r.client == nil {
		return false, errors.New("baidu vod task repository is not configured")
	}
	result, err := r.client.ExecContext(ctx, `UPDATE baidu_vod_video_tasks SET
 status=$1, upstream_status=$2, last_error_code=$3, last_error_message=$4,
 next_poll_at=$5, finished_at=$5, settled_at=$5, poll_claimed_until=NULL,
 version=version+1, updated_at=NOW()
 WHERE platform=$6 AND task_id=$7 AND status=$8`, service.BaiduVODTaskStatusFailed, "FAILED",
		strings.TrimSpace(code), strings.TrimSpace(message), finishedAt, service.PlatformBaiduVOD,
		strings.TrimSpace(taskID), service.BaiduVODTaskStatusSubmitting)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

func (r *baiduVODVideoTaskRepository) UpdatePoll(ctx context.Context, taskID string, update service.BaiduVODVideoTaskPollUpdate) (bool, error) {
	if r == nil || r.client == nil {
		return false, errors.New("baidu vod task repository is not configured")
	}
	result, err := r.client.ExecContext(ctx, `UPDATE baidu_vod_video_tasks SET
 status=$1, upstream_status=$2, result_url=COALESCE($3,result_url), result_expires_at=COALESCE($4,result_expires_at),
 output_duration=$5, input_video_duration=$6, video_count=$7, actual_cost=COALESCE($8,actual_cost),
 last_error_code=$9, last_error_message=$10, next_poll_at=$11, retry_count=$12,
 poll_claimed_until=NULL, last_polled_at=NOW(), version=version+1,
 finished_at=COALESCE($13,finished_at), settled_at=COALESCE($14,settled_at), updated_at=NOW()
 WHERE platform=$15 AND task_id=$16 AND version=$17`,
		update.Status, update.UpstreamStatus, baiduVODNullableString(update.ResultURL), baiduVODNullableTime(update.ResultExpiresAt), update.OutputDuration,
		update.InputVideoDuration, update.VideoCount, baiduVODNullableFloat64(update.ActualCost), baiduVODNullableString(update.LastErrorCode),
		baiduVODNullableString(update.LastErrorMessage), update.NextPollAt, update.RetryCount, baiduVODNullableTime(update.FinishedAt), baiduVODNullableTime(update.SettledAt),
		service.PlatformBaiduVOD, strings.TrimSpace(taskID), update.Version)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

type baiduVODTaskScanner interface{ Scan(dest ...any) error }

func scanBaiduVODVideoTask(scanner baiduVODTaskScanner) (*service.BaiduVODVideoTask, error) {
	var task service.BaiduVODVideoTask
	var upstreamReq, resultURL, errorCode, errorMessage sql.NullString
	var groupIDInt sql.NullInt64
	var actualCostFloat sql.NullFloat64
	var resultExpiry, claimedAt, polledAt, created, updated, submitted, startedAt, finishedAt, settledAt sql.NullTime
	var capability string
	err := scanner.Scan(&task.ID, &task.Platform, &task.Provider, &task.TaskID, &task.UpstreamTaskID, &upstreamReq, &task.UserID, &task.APIKeyID, &task.AccountID,
		&groupIDInt, &task.Model, &task.UpstreamModel, &capability, &task.Status, &task.UpstreamStatus, &task.Resolution, &task.Ratio,
		&task.RequestedDuration, &task.OutputDuration, &task.InputVideoDuration, &task.VideoCount, &task.BillingMode, &task.EstimatedCost, &task.HoldAmount,
		&actualCostFloat, &task.GroupRateMultiplier, &task.VideoRateMultiplier, &task.AccountRateMultiplier, &task.RequestHash,
		&resultURL, &resultExpiry, &errorCode, &errorMessage, &task.RetryCount, &task.Version, &task.NextPollAt, &claimedAt,
		&polledAt, &created, &updated, &submitted, &startedAt, &finishedAt, &settledAt)
	if err != nil {
		return nil, err
	}
	task.Capability = service.BaiduVODVideoCapability(capability)
	if upstreamReq.Valid {
		task.UpstreamRequestID = &upstreamReq.String
	}
	if groupIDInt.Valid {
		task.GroupID = &groupIDInt.Int64
	}
	if actualCostFloat.Valid {
		task.ActualCost = &actualCostFloat.Float64
	}
	if resultURL.Valid {
		task.ResultURL = &resultURL.String
	}
	if resultExpiry.Valid {
		task.ResultExpiresAt = &resultExpiry.Time
	}
	if errorCode.Valid {
		task.LastErrorCode = &errorCode.String
	}
	if errorMessage.Valid {
		task.LastErrorMessage = &errorMessage.String
	}
	if claimedAt.Valid {
		task.PollClaimedUntil = &claimedAt.Time
	}
	if polledAt.Valid {
		task.LastPolledAt = &polledAt.Time
	}
	if created.Valid {
		task.CreatedAt = created.Time
	}
	if updated.Valid {
		task.UpdatedAt = updated.Time
	}
	if submitted.Valid {
		task.SubmittedAt = submitted.Time
	}
	if startedAt.Valid {
		task.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		task.FinishedAt = &finishedAt.Time
	}
	if settledAt.Valid {
		task.SettledAt = &settledAt.Time
	}
	return &task, nil
}

func baiduVODNullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}
func baiduVODNullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return *value
}
func baiduVODNullableInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}
func baiduVODNullableFloat64(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func optionalBaiduVODString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func firstBaiduVODNonEmpty(value, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}
