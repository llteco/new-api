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
	msg := "已达到 7 天使用上限，2026-07-05 00:06:06 后可继续使用"
	matched, cooldownUntil, reason := DetectKeyLimit(info, msg)
	require.True(t, matched)
	assert.Contains(t, reason, "7-day quota")
	// ponytail: brief hardcoded 1751671566 (a 2025 date) — compute deterministically
	// so the test passes regardless of the runner's local timezone.
	expected, _ := time.ParseInLocation("2006-01-02 15:04:05", "2026-07-05 00:06:06", time.Local)
	assert.Equal(t, expected.Unix(), cooldownUntil)
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
