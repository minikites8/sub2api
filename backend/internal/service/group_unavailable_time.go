package service

import (
	"errors"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

var ErrGroupUnavailableTime = infraerrors.Forbidden("GROUP_UNAVAILABLE_TIME", "当前订阅分组处于每日不可用时段")

// IsUnavailableAt evaluates the daily [start, end) window in the system timezone.
// A start later than the end spans midnight. Invalid enabled configurations
// fail closed; writes validate them before persistence.
func (g *Group) IsUnavailableAt(now time.Time) bool {
	if g == nil || !g.IsSubscriptionType() || !g.UnavailableTimeEnabled {
		return false
	}
	start, startOK := parseMinutes(g.UnavailableStart)
	end, endOK := parseMinutes(g.UnavailableEnd)
	if !startOK || !endOK || len(g.UnavailableStart) != 5 || len(g.UnavailableEnd) != 5 || start == end {
		return true
	}
	local := now.In(timezone.Location())
	minute := local.Hour()*60 + local.Minute()
	if start < end {
		return minute >= start && minute < end
	}
	return minute >= start || minute < end
}

func ValidateUnavailableTimeConfig(subscriptionType string, enabled bool, start, end string) error {
	if !enabled {
		return nil
	}
	if subscriptionType != SubscriptionTypeSubscription {
		return errors.New("每日不可用时段仅支持订阅类型分组")
	}
	if _, ok := parseMinutes(start); !ok || len(start) != 5 {
		return errors.New("unavailable_start 必须为 HH:MM 格式")
	}
	if _, ok := parseMinutes(end); !ok || len(end) != 5 {
		return errors.New("unavailable_end 必须为 HH:MM 格式")
	}
	if start == end {
		return errors.New("每日不可用时段的开始与结束时间不能相同")
	}
	return nil
}

func NormalizeUnavailableTimeConfig(subscriptionType string, enabled bool, start, end string) (bool, string, string) {
	if subscriptionType != SubscriptionTypeSubscription {
		return false, "", ""
	}
	if !enabled {
		if _, ok := parseMinutes(start); !ok || len(start) != 5 {
			start = ""
		}
		if _, ok := parseMinutes(end); !ok || len(end) != 5 {
			end = ""
		}
	}
	return enabled, start, end
}
