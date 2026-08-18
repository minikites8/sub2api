package repository

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *usageLogRepository) GetAutoSupplyUsageWindow(ctx context.Context, groupID int64, startTime, midpoint, endTime time.Time) (service.AutoSupplyUsageWindow, error) {
	var window service.AutoSupplyUsageWindow
	err := scanSingleRow(ctx, r.sql, `
		SELECT
			COUNT(duration_ms) FILTER (WHERE created_at < $3 AND duration_ms > 0),
			COALESCE(SUM(duration_ms) FILTER (WHERE created_at < $3 AND duration_ms > 0), 0),
			COUNT(duration_ms) FILTER (WHERE created_at >= $3 AND duration_ms > 0),
			COALESCE(SUM(duration_ms) FILTER (WHERE created_at >= $3 AND duration_ms > 0), 0)
		FROM usage_logs
		WHERE group_id = $1 AND created_at >= $2 AND created_at < $4
	`, []any{groupID, startTime, midpoint, endTime},
		&window.Previous.Samples,
		&window.Previous.TotalDurationMs,
		&window.Recent.Samples,
		&window.Recent.TotalDurationMs,
	)
	return window, err
}

var _ service.AutoSupplyUsageRepository = (*usageLogRepository)(nil)
