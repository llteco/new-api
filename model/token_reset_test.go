package model

import (
	"testing"
	"time"
)

func TestCalcNextTokenResetTime(t *testing.T) {
	loc := time.UTC
	cases := []struct {
		period   string
		base     time.Time
		expected time.Time
	}{
		{TokenResetDaily, time.Date(2026, 7, 3, 16, 58, 0, 0, loc), time.Date(2026, 7, 4, 0, 0, 0, 0, loc)},
		{TokenResetWeekly, time.Date(2026, 7, 3, 16, 58, 0, 0, loc), time.Date(2026, 7, 6, 0, 0, 0, 0, loc)},
		{TokenResetMonthly, time.Date(2026, 7, 3, 16, 58, 0, 0, loc), time.Date(2026, 8, 1, 0, 0, 0, 0, loc)},
		{TokenResetTest10s, time.Date(2026, 7, 3, 16, 58, 0, 0, loc), time.Date(2026, 7, 3, 16, 58, 10, 0, loc)},
	}

	for _, tc := range cases {
		got := time.Unix(CalcNextTokenResetTime(tc.base, tc.period), 0).In(loc)
		if !got.Equal(tc.expected) {
			t.Errorf("%s: base=%v got=%v want=%v", tc.period, tc.base, got, tc.expected)
		}
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
		{
			name:          "test_10s resets once",
			period:        TokenResetTest10s,
			created:       time.Date(2026, 7, 3, 16, 58, 0, 0, loc),
			now:           time.Date(2026, 7, 3, 16, 58, 15, 0, loc),
			wantResets:    1,
			wantNextReset: time.Date(2026, 7, 3, 16, 58, 20, 0, loc),
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
