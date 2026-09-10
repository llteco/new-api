package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectKeyLimitParsesChineseQuota(t *testing.T) {
	info := model.ChannelInfo{
		MultiKeyLimitPatterns: []model.LimitPattern{
			{
				Name:           "7-day quota",
				Regex:          `已达到 \d+ 天使用上限，(?P<reset>\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}) 后可继续使用`,
				DateLayout:     "2006-01-02 15:04:05",
				DefaultMinutes: 10,
			},
		},
	}
	// ponytail: compute the reset date from time.Now so the test never rots
	// into the past-time fallback branch of DetectKeyLimit.
	resetTime := time.Now().In(time.Local).Add(2 * time.Hour).Truncate(time.Second)
	msg := "已达到 7 天使用上限，" + resetTime.Format("2006-01-02 15:04:05") + " 后可继续使用"
	matched, cooldownUntil, reason := DetectKeyLimit(info, msg)
	require.True(t, matched)
	assert.Contains(t, reason, "7-day quota")
	assert.InDelta(t, resetTime.Unix(), cooldownUntil, 2)
}

func TestDetectKeyLimitFallbackWhenNoResetGroup(t *testing.T) {
	info := model.ChannelInfo{
		MultiKeyLimitPatterns: []model.LimitPattern{
			{
				Name:           "rate limited",
				Regex:          `rate limit exceeded`,
				DateLayout:     "2006-01-02 15:04:05",
				DefaultMinutes: 10,
			},
		},
	}
	msg := "rate limit exceeded"
	matched, cooldownUntil, reason := DetectKeyLimit(info, msg)
	require.True(t, matched)
	assert.Contains(t, reason, "rate limited")
	assert.InDelta(t, common.GetTimestamp()+10*60, cooldownUntil, 2)
}

func TestDetectKeyLimitNoMatch(t *testing.T) {
	info := model.ChannelInfo{
		MultiKeyLimitPatterns: []model.LimitPattern{
			{
				Name:  "quota",
				Regex: `quota exceeded`,
			},
		},
	}
	matched, _, _ := DetectKeyLimit(info, "something else")
	assert.False(t, matched)
}

func TestDetectKeyLimitPastResetTimeFallsBack(t *testing.T) {
	info := model.ChannelInfo{
		MultiKeyLimitPatterns: []model.LimitPattern{
			{
				Name:           "past quota",
				Regex:          `limit (?P<reset>\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})`,
				DateLayout:     "2006-01-02 15:04:05",
				DefaultMinutes: 5,
			},
		},
	}
	// A reset time clearly in the past -> parser must fall back to DefaultMinutes.
	msg := "limit 2000-01-01 00:00:00"
	matched, cooldownUntil, reason := DetectKeyLimit(info, msg)
	require.True(t, matched)
	assert.Contains(t, reason, "fallback")
	assert.InDelta(t, common.GetTimestamp()+5*60, cooldownUntil, 3)
}

func TestDetectKeyLimitDefaultMinutesFallbackToTen(t *testing.T) {
	info := model.ChannelInfo{
		MultiKeyLimitPatterns: []model.LimitPattern{
			{
				Name:   "no minutes",
				Regex:  `rate limited`,
			},
		},
	}
	matched, cooldownUntil, _ := DetectKeyLimit(info, "rate limited")
	require.True(t, matched)
	assert.InDelta(t, common.GetTimestamp()+10*60, cooldownUntil, 3)
}

func TestDetectKeyLimitSkipsInvalidRegex(t *testing.T) {
	info := model.ChannelInfo{
		MultiKeyLimitPatterns: []model.LimitPattern{
			{Name: "bad", Regex: `[`, DefaultMinutes: 10},
			{Name: "good", Regex: `quota`, DefaultMinutes: 7},
		},
	}
	matched, _, reason := DetectKeyLimit(info, "quota exceeded")
	require.True(t, matched)
	assert.Contains(t, reason, "good")
}

func TestNextCycleResetTime(t *testing.T) {
	// 2026-09-10 是周四
	now := time.Date(2026, 9, 10, 12, 30, 0, 0, time.Local)
	local := func(year int, month time.Month, day int) time.Time {
		return time.Date(year, month, day, 0, 0, 0, 0, time.Local)
	}

	cases := []struct {
		name  string
		cycle string
		want  time.Time
	}{
		{"daily to next midnight", "daily", local(2026, 9, 11)},
		{"weekly to next friday", "weekly:5", local(2026, 9, 11)},
		{"weekly same weekday rolls to next week", "weekly:4", local(2026, 9, 17)},
		{"weekly to next sunday", "weekly:7", local(2026, 9, 13)},
		{"monthly later this month", "monthly:15", local(2026, 9, 15)},
		{"monthly same day rolls to next month", "monthly:10", local(2026, 10, 10)},
		{"monthly last day of month", "monthly:30", local(2026, 9, 30)},
		{"monthly day beyond month end rolls to next month 1st", "monthly:31", local(2026, 10, 1)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := nextCycleResetTime(c.cycle, now)
			require.True(t, ok)
			assert.True(t, got.After(now), "reset time must be in the future")
			assert.Equal(t, c.want, got)
		})
	}

	for _, cycle := range []string{"", "garbage", "weekly:0", "weekly:8", "monthly:0", "monthly:32", "weekly:x", "monthly:"} {
		_, ok := nextCycleResetTime(cycle, now)
		assert.False(t, ok, "cycle %q should be rejected", cycle)
	}
}

func TestDetectKeyLimitUsesResetCycle(t *testing.T) {
	info := model.ChannelInfo{
		MultiKeyLimitPatterns: []model.LimitPattern{
			{
				Name:       "aliyun token-plan",
				Regex:      `Your token-plan quota has been exhausted\.`,
				ResetCycle: "monthly:1",
			},
		},
	}
	matched, cooldownUntil, reason := DetectKeyLimit(info, "Your token-plan quota has been exhausted.")
	require.True(t, matched)
	assert.Contains(t, reason, "reset cycle monthly:1")
	// 无论何时触发，重置时间都必须落在未来某月 1 号 0 点（本地时区）
	reset := time.Unix(cooldownUntil, 0).In(time.Local)
	assert.True(t, reset.After(time.Now()))
	assert.Equal(t, 1, reset.Day())
	assert.Equal(t, 0, reset.Hour())
	assert.Equal(t, 0, reset.Minute())
}

func TestDetectKeyLimitInvalidCycleFallsBackToMinutes(t *testing.T) {
	info := model.ChannelInfo{
		MultiKeyLimitPatterns: []model.LimitPattern{
			{
				Name:           "bad cycle",
				Regex:          `quota exhausted`,
				ResetCycle:     "garbage",
				DefaultMinutes: 15,
			},
		},
	}
	matched, cooldownUntil, reason := DetectKeyLimit(info, "quota exhausted again")
	require.True(t, matched)
	assert.Contains(t, reason, "fallback")
	assert.InDelta(t, common.GetTimestamp()+15*60, cooldownUntil, 3)
}

func TestDetectKeyLimitCapturedDateWinsOverCycle(t *testing.T) {
	info := model.ChannelInfo{
		MultiKeyLimitPatterns: []model.LimitPattern{
			{
				Name:           "captured",
				Regex:          `limit (?P<reset>\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})`,
				DateLayout:     "2006-01-02 15:04:05",
				ResetCycle:     "monthly:1",
				DefaultMinutes: 10,
			},
		},
	}
	resetTime := time.Now().In(time.Local).Add(2 * time.Hour).Truncate(time.Second)
	msg := "limit " + resetTime.Format("2006-01-02 15:04:05")
	matched, cooldownUntil, reason := DetectKeyLimit(info, msg)
	require.True(t, matched)
	assert.Contains(t, reason, "reset at")
	assert.InDelta(t, resetTime.Unix(), cooldownUntil, 2)
}
