package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetNextEnabledKeySkipsTempDisabledKey(t *testing.T) {
	channel := &Channel{
		Id:  1,
		Key: "k1\nk2",
		ChannelInfo: ChannelInfo{
			IsMultiKey:            true,
			MultiKeyMode:          "random",
			MultiKeySize:          2,
			MultiKeyStatusList:    map[int]int{0: common.ChannelStatusTempDisabled},
			MultiKeyCooldownUntil: map[int]int64{0: common.GetTimestamp() + 3600},
		},
	}
	key, idx, apiErr := channel.GetNextEnabledKey()
	require.Nil(t, apiErr)
	assert.Equal(t, "k2", key)
	assert.Equal(t, 1, idx)
}

func TestGetNextEnabledKeyReenablesExpiredCooldown(t *testing.T) {
	channel := &Channel{
		Id:  2,
		Key: "k1",
		ChannelInfo: ChannelInfo{
			IsMultiKey:            true,
			MultiKeyMode:          "random",
			MultiKeySize:          1,
			MultiKeyStatusList:    map[int]int{0: common.ChannelStatusTempDisabled},
			MultiKeyCooldownUntil: map[int]int64{0: common.GetTimestamp() - 1},
		},
	}
	key, idx, apiErr := channel.GetNextEnabledKey()
	require.Nil(t, apiErr)
	assert.Equal(t, "k1", key)
	assert.Equal(t, 0, idx)
	assert.NotContains(t, channel.ChannelInfo.MultiKeyStatusList, 0)
	assert.NotContains(t, channel.ChannelInfo.MultiKeyCooldownUntil, 0)
}

func TestHandlerMultiKeyUpdateAllCoolingDoesNotAutoDisable(t *testing.T) {
	channel := &Channel{
		Id:  1,
		Key: "k1\nk2",
		ChannelInfo: ChannelInfo{
			IsMultiKey:            true,
			MultiKeySize:          2,
			MultiKeyStatusList:    map[int]int{0: common.ChannelStatusTempDisabled},
			MultiKeyCooldownUntil: map[int]int64{0: common.GetTimestamp() + 3600},
		},
	}
	cooldown := common.GetTimestamp() + 3600
	handlerMultiKeyUpdate(channel, "k2", common.ChannelStatusTempDisabled, "limit hit", &cooldown)
	assert.NotEqual(t, common.ChannelStatusAutoDisabled, channel.Status)
}

func TestHandlerMultiKeyUpdateAllManuallyDisabledStillAutoDisables(t *testing.T) {
	channel := &Channel{
		Id:  2,
		Key: "k1\nk2",
		ChannelInfo: ChannelInfo{
			IsMultiKey:         true,
			MultiKeySize:       2,
			MultiKeyStatusList: map[int]int{0: common.ChannelStatusManuallyDisabled},
		},
	}
	handlerMultiKeyUpdate(channel, "k2", common.ChannelStatusManuallyDisabled, "manual", nil)
	assert.Equal(t, common.ChannelStatusAutoDisabled, channel.Status)
}
