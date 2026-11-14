package service

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type RetryParam struct {
	Ctx         *gin.Context
	TokenGroup  string
	ModelName   string
	RequestPath string
	// Retry is the attempt index (0 = first attempt), used for logging and
	// RelayInfo.RetryIndex; it no longer bounds failover.
	Retry *int
	// TriedChannels holds channels already attempted in this request. It
	// persists across auto-group switches so a failed channel is never retried.
	TriedChannels map[int]bool
	// Failover marks selections made by the relay retry loop (as opposed to
	// initial selection in the distributor). Only failover honors the token's
	// cross-group retry switch when a group's channels are exhausted.
	Failover bool
}

func (p *RetryParam) GetRetry() int {
	if p.Retry == nil {
		return 0
	}
	return *p.Retry
}

func (p *RetryParam) IncreaseRetry() {
	if p.Retry == nil {
		p.Retry = new(int)
	}
	*p.Retry++
}

func (p *RetryParam) MarkTried(channelId int) {
	if p.TriedChannels == nil {
		p.TriedChannels = make(map[int]bool)
	}
	p.TriedChannels[channelId] = true
}

func (p *RetryParam) triedChannelIds() []int {
	ids := make([]int, 0, len(p.TriedChannels))
	for channelId := range p.TriedChannels {
		ids = append(ids, channelId)
	}
	return ids
}

// CacheGetRandomSatisfiedChannel tries to get a random channel that satisfies
// the requirements, excluding channels already attempted in this request.
// 尝试获取一个满足要求的随机渠道，排除本次请求已尝试过的渠道。
//
// For "auto" tokenGroup:
// 对于 "auto" tokenGroup：
//
//   - Selection stays in the current group until all its channels have been
//     attempted, then moves to the next group.
//     选择会留在当前分组，直到其所有渠道都被尝试过，然后切换到下一个分组。
//
//   - Initial selection (Failover=false) always advances through groups until
//     a channel is found; failover (Failover=true) only crosses groups when the
//     token enables cross-group retry.
//     初始选择（Failover=false）总是逐组查找直到找到渠道；故障转移
//     （Failover=true）仅在令牌开启跨分组重试时才切换分组。
//
//   - Uses ContextKeyAutoGroupIndex to track the current group index.
//     使用 ContextKeyAutoGroupIndex 跟踪当前分组索引。
func CacheGetRandomSatisfiedChannel(param *RetryParam) (*model.Channel, string, error) {
	selectGroup := param.TokenGroup
	excludeChannelIds := param.triedChannelIds()

	if param.TokenGroup == "auto" {
		userGroup := common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup)
		autoGroups := GetRequestAutoGroups(param.Ctx, userGroup)
		if len(autoGroups) == 0 {
			return nil, selectGroup, errors.New("auto groups is not enabled")
		}

		startGroupIndex := 0
		if lastGroupIndex, exists := common.GetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex); exists {
			if idx, ok := lastGroupIndex.(int); ok {
				startGroupIndex = idx
			}
		}
		crossGroupRetry := common.GetContextKeyBool(param.Ctx, constant.ContextKeyTokenCrossGroupRetry)

		for i := startGroupIndex; i < len(autoGroups); i++ {
			autoGroup := autoGroups[i]
			logger.LogDebug(param.Ctx, "Auto selecting group: %s, tried channels: %d", autoGroup, len(excludeChannelIds))

			channel, _ := model.GetRandomSatisfiedChannel(autoGroup, param.ModelName, excludeChannelIds, param.RequestPath)
			if channel == nil {
				// Current group has no untried channel for this model.
				// 当前分组没有该模型的未尝试渠道。
				if param.Failover && !crossGroupRetry {
					return nil, autoGroup, nil
				}
				logger.LogDebug(param.Ctx, "No available channel in group %s for model %s, trying next group", autoGroup, param.ModelName)
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
				continue
			}
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, autoGroup)
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i)
			logger.LogDebug(param.Ctx, "Auto selected group: %s", autoGroup)
			return channel, autoGroup, nil
		}
		return nil, selectGroup, nil
	}

	channel, err := model.GetRandomSatisfiedChannel(param.TokenGroup, param.ModelName, excludeChannelIds, param.RequestPath)
	if err != nil {
		return nil, param.TokenGroup, err
	}
	return channel, selectGroup, nil
}
