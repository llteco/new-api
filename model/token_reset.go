package model

import (
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"gorm.io/gorm"
)

const (
	TokenResetNever  = "never"
	TokenResetDaily  = "daily"
	TokenResetWeekly = "weekly"
	TokenResetMonthly = "monthly"
)

func NormalizeTokenResetPeriod(period string) string {
	switch period {
	case TokenResetDaily, TokenResetWeekly, TokenResetMonthly:
		return period
	default:
		return TokenResetNever
	}
}

// tokenResetLocation 返回周期重置边界所用的时区：
// 配置为空时跟随服务器系统时区，配置无效时回退系统时区并记录错误。
func tokenResetLocation() *time.Location {
	name := operation_setting.GetTokenSetting().ResetTimeZone
	if name == "" {
		return time.Local
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		common.SysError("invalid token reset timezone " + name + ", falling back to system timezone: " + err.Error())
		return time.Local
	}
	return loc
}

func CalcNextTokenResetTime(base time.Time, period string) int64 {
	switch period {
	case TokenResetDaily:
		local := base.In(tokenResetLocation())
		return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location()).
			AddDate(0, 0, 1).Unix()
	case TokenResetWeekly:
		local := base.In(tokenResetLocation())
		weekday := int(local.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		daysUntil := 8 - weekday
		return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location()).
			AddDate(0, 0, daysUntil).Unix()
	case TokenResetMonthly:
		local := base.In(tokenResetLocation())
		return time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location()).
			AddDate(0, 1, 0).Unix()
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

	if token.ChannelQuotaMode {
		return maybeResetTokenChannelQuotas(token, period, now)
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

	_ = invalidateTokenCacheForMutation(token.Key)

	token.RemainQuota = fresh.RemainQuota
	token.UsedQuota = fresh.UsedQuota
	token.NextResetTime = fresh.NextResetTime
	token.ResetQuota = fresh.ResetQuota
	token.ResetPeriod = fresh.ResetPeriod

	return true
}

func maybeResetTokenChannelQuotas(token *Token, period string, now int64) bool {
	var fresh Token
	if err := DB.Where("id = ?", token.Id).First(&fresh).Error; err != nil {
		return false
	}
	if fresh.NextResetTime <= 0 || fresh.NextResetTime > now {
		token.NextResetTime = fresh.NextResetTime
		return false
	}

	resetCount := 0
	base := time.Unix(fresh.NextResetTime, 0)
	for fresh.NextResetTime > 0 && fresh.NextResetTime <= now {
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

	if err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&TokenChannelQuota{}).
			Where("token_id = ?", fresh.Id).
			Updates(map[string]interface{}{
				"remain_quota": gorm.Expr("reset_quota"),
				"used_quota":   0,
			}).Error; err != nil {
			return err
		}
		return tx.Model(&Token{}).Where("id = ?", fresh.Id).
			Update("next_reset_time", fresh.NextResetTime).Error
	}); err != nil {
		return false
	}

	_ = invalidateTokenCacheForMutation(token.Key)
	token.NextResetTime = fresh.NextResetTime
	return true
}
