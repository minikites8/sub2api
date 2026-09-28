package service

import (
	"math"
	"sort"
	"strings"
	"time"
)

// These types preserve the original ai-transit.v1 monitoring contract.
// Each V2 platform/model row becomes one V1 monitor with one primary model.
type PublicTransitExtraModelStatus struct {
	Model     string `json:"model"`
	Status    string `json:"status"`
	LatencyMs *int64 `json:"latency_ms,omitempty"`
}

type PublicTransitMonitorModel struct {
	Model           string  `json:"model"`
	LatestStatus    string  `json:"latest_status"`
	LatestLatencyMs *int64  `json:"latest_latency_ms,omitempty"`
	Availability7d  float64 `json:"availability_7d"`
	Availability15d float64 `json:"availability_15d"`
	Availability30d float64 `json:"availability_30d"`
	AvgLatency7dMs  *int64  `json:"avg_latency_7d_ms,omitempty"`
}

type PublicTransitV1MonitorTimeline struct {
	Status    string `json:"status"`
	LatencyMs *int64 `json:"latency_ms,omitempty"`
	CheckedAt string `json:"checked_at"`
}

func populatePublicTransitV1Monitor(item *PublicTransitMonitor, row ChannelMonitorV2MatrixRow) {
	item.Name = row.Platform + " / " + row.Model
	item.Provider = row.Platform
	item.GroupName = row.GroupName
	item.PrimaryModel = row.Model
	item.PrimaryStatus = publicTransitV1MonitorStatus(row.Metrics, row.Health)
	item.ExtraModels = []PublicTransitExtraModelStatus{}
	item.Timeline = publicTransitV1Timeline(row.Buckets)
	if avg := row.Metrics.Duration.AvgMs; avg != nil {
		latency := int64(math.Round(*avg))
		item.AvgLatency7dMs = &latency
	}

	if len(item.Timeline) > 0 {
		latest := item.Timeline[0]
		item.PrimaryStatus = latest.Status
		item.LatestLatencyMs = latest.LatencyMs
		item.LastCheckedAt = latest.CheckedAt
	}
	item.Models = []PublicTransitMonitorModel{{
		Model:           item.PrimaryModel,
		LatestStatus:    item.PrimaryStatus,
		LatestLatencyMs: item.LatestLatencyMs,
		Availability7d:  item.Availability7d,
		Availability15d: item.Availability15d,
		Availability30d: item.Availability30d,
		AvgLatency7dMs:  item.AvgLatency7dMs,
	}}
}

// Prefer five-minute observations for V1's latest check. Keep the seven-day
// history when the recent window has no requests, preserving its real timestamp.
func attachPublicTransitV1RecentTimeline(monitors []PublicTransitMonitor, recent *ChannelMonitorV2Matrix) {
	if recent == nil {
		return
	}
	rows := make(map[string]ChannelMonitorV2MatrixRow, len(recent.Items))
	for _, row := range recent.Items {
		rows[publicMonitorRowKey(row)] = row
	}
	for i := range monitors {
		item := &monitors[i]
		row, ok := rows[publicMonitorKey(item.Platform, item.GroupID, item.Model)]
		if !ok {
			continue
		}
		timeline := publicTransitV1Timeline(row.Buckets)
		if len(timeline) == 0 {
			continue
		}
		item.Timeline = timeline
		latest := timeline[0]
		item.PrimaryStatus = latest.Status
		item.LatestLatencyMs = latest.LatencyMs
		item.LastCheckedAt = latest.CheckedAt
		for j := range item.Models {
			item.Models[j].LatestStatus = latest.Status
			item.Models[j].LatestLatencyMs = latest.LatencyMs
		}
	}
}

func publicTransitV1Timeline(src []ChannelMonitorV2TrendPoint) []PublicTransitV1MonitorTimeline {
	timeline := make([]PublicTransitV1MonitorTimeline, 0, len(src))

	// V1 timelines are newest first. Empty V2 buckets carry no observation.
	buckets := append([]ChannelMonitorV2TrendPoint(nil), src...)
	sort.SliceStable(buckets, func(i, j int) bool {
		return buckets[i].BucketStart.After(buckets[j].BucketStart)
	})
	for _, bucket := range buckets {
		if bucket.Metrics.RequestCount == 0 {
			continue
		}
		point := PublicTransitV1MonitorTimeline{
			Status:    publicTransitV1MonitorStatus(bucket.Metrics, bucket.Health),
			LatencyMs: bucket.Metrics.Duration.P50Ms,
			CheckedAt: bucket.BucketStart.UTC().Format(time.RFC3339),
		}
		timeline = append(timeline, point)
	}
	return timeline
}

func publicTransitV1MonitorStatus(metrics ChannelMonitorV2Metric, health ChannelMonitorV2Health) string {
	if metrics.RequestCount == 0 {
		return "unknown"
	}
	switch strings.ToLower(strings.TrimSpace(health.Overall)) {
	case "healthy":
		return "operational"
	case "warning":
		return "degraded"
	case "critical":
		return "failed"
	default:
		return "unknown"
	}
}
