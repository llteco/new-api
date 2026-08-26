package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalcNextTokenResetTime(t *testing.T) {
	operation_setting.GetTokenSetting().ResetTimeZone = "UTC"
	loc := time.UTC
	cases := []struct {
		period   string
		base     time.Time
		expected time.Time
	}{
		{TokenResetDaily, time.Date(2026, 7, 3, 16, 58, 0, 0, loc), time.Date(2026, 7, 4, 0, 0, 0, 0, loc)},
		{TokenResetWeekly, time.Date(2026, 7, 3, 16, 58, 0, 0, loc), time.Date(2026, 7, 6, 0, 0, 0, 0, loc)},
		{TokenResetMonthly, time.Date(2026, 7, 3, 16, 58, 0, 0, loc), time.Date(2026, 8, 1, 0, 0, 0, 0, loc)},
	}

	for _, tc := range cases {
		got := time.Unix(CalcNextTokenResetTime(tc.base, tc.period), 0).In(loc)
		if !got.Equal(tc.expected) {
			t.Errorf("%s: base=%v got=%v want=%v", tc.period, tc.base, got, tc.expected)
		}
	}
	operation_setting.GetTokenSetting().ResetTimeZone = ""
}

func TestCalcNextTokenResetTimeCustomTimezone(t *testing.T) {
	operation_setting.GetTokenSetting().ResetTimeZone = "Asia/Shanghai"
	defer func() { operation_setting.GetTokenSetting().ResetTimeZone = "" }()

	// 16:00 UTC == 次日 00:00 上海时间，恰好落在边界上，下一个日边界为上海次日午夜。
	base := time.Date(2026, 7, 3, 16, 0, 0, 0, time.UTC)
	expected := time.Date(2026, 7, 4, 16, 0, 0, 0, time.UTC)
	got := time.Unix(CalcNextTokenResetTime(base, TokenResetDaily), 0)
	if !got.Equal(expected) {
		t.Errorf("daily: base=%v got=%v want=%v", base, got, expected)
	}

	// 月边界：上海 2026-08-01 00:00 = UTC 2026-07-31 16:00。
	base = time.Date(2026, 7, 3, 16, 58, 0, 0, time.UTC)
	expected = time.Date(2026, 7, 31, 16, 0, 0, 0, time.UTC)
	got = time.Unix(CalcNextTokenResetTime(base, TokenResetMonthly), 0)
	if !got.Equal(expected) {
		t.Errorf("monthly: base=%v got=%v want=%v", base, got, expected)
	}
}

// simulateResetLoop mirrors the corrected loop inside MaybeResetTokenQuota
func simulateResetLoop(period string, initialNextReset int64, now int64) (remainQuota, usedQuota, nextReset int64, resetCount int) {
	fresh := Token{
		ResetPeriod:   period,
		ResetQuota:    1000,
		RemainQuota:   100,
		UsedQuota:     900,
		NextResetTime: initialNextReset,
	}

	base := time.Unix(fresh.NextResetTime, 0)
	for fresh.NextResetTime > 0 && fresh.NextResetTime <= now {
		fresh.RemainQuota = fresh.ResetQuota
		fresh.UsedQuota = 0
		next := CalcNextTokenResetTime(base, period)
		if next <= 0 || next == fresh.NextResetTime {
			break
		}
		fresh.NextResetTime = next
		base = time.Unix(next, 0)
		resetCount++
	}

	return int64(fresh.RemainQuota), int64(fresh.UsedQuota), fresh.NextResetTime, resetCount
}

func TestResetLoopPeriods(t *testing.T) {
	operation_setting.GetTokenSetting().ResetTimeZone = "UTC"
	defer func() { operation_setting.GetTokenSetting().ResetTimeZone = "" }()
	loc := time.UTC

	cases := []struct {
		name          string
		period        string
		created       time.Time
		now           time.Time
		wantResets    int
		wantNextReset time.Time
	}{
		{
			name:          "daily resets twice when spanning two midnights",
			period:        TokenResetDaily,
			created:       time.Date(2026, 7, 3, 16, 58, 0, 0, loc),
			now:           time.Date(2026, 7, 5, 12, 0, 0, 0, loc),
			wantResets:    2,
			wantNextReset: time.Date(2026, 7, 6, 0, 0, 0, 0, loc),
		},
		{
			name:          "weekly resets once",
			period:        TokenResetWeekly,
			created:       time.Date(2026, 7, 3, 16, 58, 0, 0, loc),
			now:           time.Date(2026, 7, 8, 12, 0, 0, 0, loc),
			wantResets:    1,
			wantNextReset: time.Date(2026, 7, 13, 0, 0, 0, 0, loc),
		},
		{
			name:          "monthly resets once",
			period:        TokenResetMonthly,
			created:       time.Date(2026, 7, 3, 16, 58, 0, 0, loc),
			now:           time.Date(2026, 8, 5, 12, 0, 0, 0, loc),
			wantResets:    1,
			wantNextReset: time.Date(2026, 9, 1, 0, 0, 0, 0, loc),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			initialNext := CalcNextTokenResetTime(tc.created, tc.period)
			remain, used, next, count := simulateResetLoop(tc.period, initialNext, tc.now.Unix())

			if count != tc.wantResets {
				t.Errorf("resets=%d want=%d", count, tc.wantResets)
			}
			if remain != 1000 {
				t.Errorf("remain_quota=%d want=1000", remain)
			}
			if used != 0 {
				t.Errorf("used_quota=%d want=0", used)
			}
			got := time.Unix(next, 0).In(loc)
			if !got.Equal(tc.wantNextReset) {
				t.Errorf("next_reset=%v want=%v", got, tc.wantNextReset)
			}
		})
	}
}

func TestMaybeResetTokenQuota_ChannelMode(t *testing.T) {
	truncateTables(t)
	tok := &Token{
		UserId: 1, Key: "sk-reset-chan", Status: 1,
		ChannelQuotaMode: true,
		ResetPeriod:      TokenResetMonthly,
		NextResetTime:    time.Now().Add(-1 * time.Hour).Unix(),
	}
	require.NoError(t, tok.Insert())
	require.NoError(t, UpsertTokenChannelQuota(tok.Id, 10, 5000))
	require.NoError(t, UpsertTokenChannelQuota(tok.Id, 20, 8000))
	require.NoError(t, DecreaseTokenChannelQuota(tok.Id, 10, 2000))
	require.NoError(t, DecreaseTokenChannelQuota(tok.Id, 20, 3000))

	require.True(t, MaybeResetTokenQuota(tok))

	r10, _ := GetTokenChannelQuota(tok.Id, 10)
	r20, _ := GetTokenChannelQuota(tok.Id, 20)
	assert.Equal(t, 5000, r10.RemainQuota)
	assert.Equal(t, 0, r10.UsedQuota)
	assert.Equal(t, 8000, r20.RemainQuota)
	assert.Equal(t, 0, r20.UsedQuota)
	assert.Greater(t, tok.NextResetTime, time.Now().Unix())
}

func TestMaybeResetTokenQuota_ChannelMode_NotYetTime(t *testing.T) {
	truncateTables(t)
	tok := &Token{
		UserId: 1, Key: "sk-reset-chan-future", Status: 1,
		ChannelQuotaMode: true,
		ResetPeriod:      TokenResetMonthly,
		NextResetTime:    time.Now().Add(1 * time.Hour).Unix(),
	}
	require.NoError(t, tok.Insert())
	require.NoError(t, UpsertTokenChannelQuota(tok.Id, 10, 5000))
	require.NoError(t, DecreaseTokenChannelQuota(tok.Id, 10, 2000))

	require.False(t, MaybeResetTokenQuota(tok))
	r10, _ := GetTokenChannelQuota(tok.Id, 10)
	assert.Equal(t, 3000, r10.RemainQuota)
	assert.Equal(t, 2000, r10.UsedQuota)
}
