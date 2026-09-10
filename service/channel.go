package service

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

func formatNotifyType(channelId int, status int) string {
	return fmt.Sprintf("%s_%d_%d", dto.NotifyTypeChannelUpdate, channelId, status)
}

// disable & notify
func DisableChannel(channelError types.ChannelError, reason string) {
	common.SysLog(fmt.Sprintf("通道「%s」（#%d）发生错误，准备禁用，原因：%s", channelError.ChannelName, channelError.ChannelId, common.LocalLogPreview(reason)))

	// 检查是否启用自动禁用功能
	if !channelError.AutoBan {
		common.SysLog(fmt.Sprintf("通道「%s」（#%d）未启用自动禁用功能，跳过禁用操作", channelError.ChannelName, channelError.ChannelId))
		return
	}

	success := model.UpdateChannelStatus(channelError.ChannelId, channelError.UsingKey, common.ChannelStatusAutoDisabled, reason)
	if success {
		subject := fmt.Sprintf("通道「%s」（#%d）已被禁用", channelError.ChannelName, channelError.ChannelId)
		content := fmt.Sprintf("通道「%s」（#%d）已被禁用，原因：%s", channelError.ChannelName, channelError.ChannelId, reason)
		NotifyRootUser(formatNotifyType(channelError.ChannelId, common.ChannelStatusAutoDisabled), subject, content)
	}
}

func EnableChannel(channelId int, usingKey string, channelName string) {
	success := model.UpdateChannelStatus(channelId, usingKey, common.ChannelStatusEnabled, "")
	if success {
		subject := fmt.Sprintf("通道「%s」（#%d）已被启用", channelName, channelId)
		content := fmt.Sprintf("通道「%s」（#%d）已被启用", channelName, channelId)
		NotifyRootUser(formatNotifyType(channelId, common.ChannelStatusEnabled), subject, content)
	}
}

func ShouldDisableChannel(err *types.NewAPIError) bool {
	if !common.AutomaticDisableChannelEnabled {
		return false
	}
	if err == nil {
		return false
	}
	if types.IsChannelError(err) {
		return true
	}
	if types.IsSkipRetryError(err) {
		return false
	}
	if operation_setting.ShouldDisableByStatusCode(err.StatusCode) {
		return true
	}

	lowerMessage := strings.ToLower(err.Error())
	search, _ := AcSearch(lowerMessage, operation_setting.AutomaticDisableKeywords, true)
	return search
}

func ShouldEnableChannel(newAPIError *types.NewAPIError, status int) bool {
	if !common.AutomaticEnableChannelEnabled {
		return false
	}
	if newAPIError != nil {
		return false
	}
	if status != common.ChannelStatusAutoDisabled {
		return false
	}
	return true
}

func DetectKeyLimit(channelInfo model.ChannelInfo, errMessage string) (matched bool, cooldownUntil int64, reason string) {
	for _, pattern := range channelInfo.MultiKeyLimitPatterns {
		if pattern.Regex == "" {
			continue
		}
		re, err := regexp.Compile(pattern.Regex)
		if err != nil {
			common.SysLog(fmt.Sprintf("invalid multi-key limit pattern %q: %v", pattern.Name, err))
			continue
		}
		groups := re.FindStringSubmatch(errMessage)
		if groups == nil {
			continue
		}
		resetCapture := ""
		for i, name := range re.SubexpNames() {
			if name == "reset" && i < len(groups) {
				resetCapture = groups[i]
				break
			}
		}
		fallbackMinutes := pattern.DefaultMinutes
		if fallbackMinutes <= 0 {
			fallbackMinutes = 10
		}
		now := common.GetTimestamp()
		if resetCapture != "" && pattern.DateLayout != "" {
			var parsed time.Time
			if strings.Contains(pattern.DateLayout, "Z07") || strings.Contains(pattern.DateLayout, "MST") {
				parsed, err = time.Parse(pattern.DateLayout, resetCapture)
			} else {
				parsed, err = time.ParseInLocation(pattern.DateLayout, resetCapture, time.Local)
			}
			if err == nil && parsed.Unix() > now {
				return true, parsed.Unix(), fmt.Sprintf("%s (reset at %s)", pattern.Name, resetCapture)
			}
		}
		if cycleReset, ok := nextCycleResetTime(pattern.ResetCycle, time.Now()); ok {
			return true, cycleReset.Unix(), fmt.Sprintf("%s (reset cycle %s)", pattern.Name, pattern.ResetCycle)
		}
		return true, now + int64(fallbackMinutes)*60, fmt.Sprintf("%s (fallback %d min)", pattern.Name, fallbackMinutes)
	}
	return false, 0, ""
}

// nextCycleResetTime 计算周期重置规则在 now 之后的下一个边界（本地时区 0 点）：
// "daily" 为次日 0 点；"weekly:N" 为下一个周 N（1=周一 .. 7=周日）的 0 点；
// "monthly:N" 为下一个月中的 N 号 0 点，当月不存在 N 号（如 2 月的 31 号）时顺延到
// 下月 1 号。规则为空或无法解析时返回 false，由调用方回退到 DefaultMinutes。
func nextCycleResetTime(resetCycle string, now time.Time) (time.Time, bool) {
	if resetCycle == "" {
		return time.Time{}, false
	}
	now = now.In(time.Local)
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	firstOfNextMonth := func(t time.Time) time.Time {
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.Local).AddDate(0, 1, 0)
	}
	dayOfMonthIn := func(firstOfMonth time.Time, day int) time.Time {
		candidate := time.Date(firstOfMonth.Year(), firstOfMonth.Month(), day, 0, 0, 0, 0, time.Local)
		if candidate.Day() != day {
			return firstOfNextMonth(firstOfMonth)
		}
		return candidate
	}
	switch {
	case resetCycle == "daily":
		return midnight.AddDate(0, 0, 1), true
	case strings.HasPrefix(resetCycle, "weekly:"):
		day, err := strconv.Atoi(strings.TrimPrefix(resetCycle, "weekly:"))
		if err != nil || day < 1 || day > 7 {
			return time.Time{}, false
		}
		// Go 的 Weekday 以周日为 0，将 1=周一..7=周日 转换为 Go 编号
		targetGoDay := day % 7
		next := midnight.AddDate(0, 0, 1)
		for next.Weekday() != time.Weekday(targetGoDay) {
			next = next.AddDate(0, 0, 1)
		}
		return next, true
	case strings.HasPrefix(resetCycle, "monthly:"):
		day, err := strconv.Atoi(strings.TrimPrefix(resetCycle, "monthly:"))
		if err != nil || day < 1 || day > 31 {
			return time.Time{}, false
		}
		thisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)
		candidate := dayOfMonthIn(thisMonth, day)
		if !candidate.After(now) {
			candidate = dayOfMonthIn(firstOfNextMonth(now), day)
		}
		return candidate, true
	}
	return time.Time{}, false
}
