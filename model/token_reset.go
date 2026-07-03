package model

import (
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	TokenResetNever    = "never"
	TokenResetDaily    = "daily"
	TokenResetWeekly   = "weekly"
	TokenResetMonthly  = "monthly"
	TokenResetTest10s  = "test_10s"
)

func NormalizeTokenResetPeriod(period string) string {
	switch period {
	case TokenResetDaily, TokenResetWeekly, TokenResetMonthly, TokenResetTest10s:
		return period
	default:
		return TokenResetNever
	}
}

func CalcNextTokenResetTime(base time.Time, period string) int64 {
	switch period {
	case TokenResetDaily:
		next := time.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, time.UTC).
			AddDate(0, 0, 1)
		return next.Unix()
	case TokenResetWeekly:
		weekday := int(base.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		daysUntil := 8 - weekday
		next := time.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, time.UTC).
			AddDate(0, 0, daysUntil)
		return next.Unix()
	case TokenResetMonthly:
		next := time.Date(base.Year(), base.Month(), 1, 0, 0, 0, 0, time.UTC).
			AddDate(0, 1, 0)
		return next.Unix()
	case TokenResetTest10s:
		return base.Add(10 * time.Second).Unix()
	default:
		return 0
	}
}

func MaybeResetTokenQuota(token *Token) bool {
	if token == nil {
		return false
	}
	now := common.GetTimestamp()
	if token.NextResetTime <= 0 || token.NextResetTime > now {
		return false
	}
	period := NormalizeTokenResetPeriod(token.ResetPeriod)
	if period == TokenResetNever {
		return false
	}

	var fresh Token
	if err := DB.Where("id = ?", token.Id).First(&fresh).Error; err != nil {
		return false
	}
	if fresh.NextResetTime <= 0 || fresh.NextResetTime > now {
		token.RemainQuota = fresh.RemainQuota
		token.NextResetTime = fresh.NextResetTime
		token.ResetQuota = fresh.ResetQuota
		token.ResetPeriod = fresh.ResetPeriod
		token.UsedQuota = fresh.UsedQuota
		return false
	}

	resetCount := 0
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

	if resetCount == 0 {
		return false
	}

	if err := DB.Model(&Token{}).Where("id = ?", fresh.Id).
		Select("remain_quota", "used_quota", "next_reset_time", "reset_quota", "reset_period").
		Updates(&fresh).Error; err != nil {
		return false
	}

	_ = cacheDeleteToken(token.Key)

	token.RemainQuota = fresh.RemainQuota
	token.UsedQuota = fresh.UsedQuota
	token.NextResetTime = fresh.NextResetTime
	token.ResetQuota = fresh.ResetQuota
	token.ResetPeriod = fresh.ResetPeriod

	return true
}
