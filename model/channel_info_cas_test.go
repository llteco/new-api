package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupChannelInfoCASTest(t *testing.T) {
	t.Helper()
	truncateTables(t)
	require.NoError(t, DB.Exec("DELETE FROM abilities").Error)
	require.NoError(t, DB.Exec("DELETE FROM channels").Error)
}

func insertChannelInfoCASTestChannel(t *testing.T) *Channel {
	t.Helper()

	channel := Channel{
		Name:   "channel-info-cas",
		Key:    "key-a\nkey-b",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-4o",
		Group:  "default",
		ChannelInfo: ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
			MultiKeyMode: constant.MultiKeyModePolling,
		},
	}
	require.NoError(t, DB.Create(&channel).Error)
	return &channel
}

// 密钥模式转换提交后，进行中请求持有的过期快照不得把旧的多 key 元数据
// 回写覆盖转换结果（保留 key 与旧状态索引并存会让该 key 不可用）。
func TestSaveChannelInfoRejectsStaleSnapshotAfterConversion(t *testing.T) {
	setupChannelInfoCASTest(t)
	channel := insertChannelInfoCASTestChannel(t)

	// 进行中的 polling 请求持有的快照
	inFlight, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)

	// 另一个写入方把渠道转换为单 key
	converted, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)
	converted.Key = "key-b"
	converted.ChannelInfo = ChannelInfo{}
	require.NoError(t, converted.UpdateWithChannelInfo())

	// 请求结束，尝试回写轮询索引
	inFlight.ChannelInfo.MultiKeyPollingIndex = 1
	require.NoError(t, inFlight.SaveChannelInfo())

	stored, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, "key-b", stored.Key)
	assert.False(t, stored.ChannelInfo.IsMultiKey)
	assert.Zero(t, stored.ChannelInfo.MultiKeySize)
	assert.Zero(t, stored.ChannelInfo.MultiKeyPollingIndex)
}

// 密钥模式转换提交后，失败请求的状态回写不得把旧的多 key 元数据写回：
// 状态本身必须落库，channel_info 走 CAS 放弃过期快照。
func TestSaveStatusStateKeepsConvertedChannelInfoWhilePersistingStatus(t *testing.T) {
	setupChannelInfoCASTest(t)
	channel := insertChannelInfoCASTestChannel(t)

	// 进行中的失败请求持有的快照（加载后、回写前）
	stale, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)
	stale.Status = common.ChannelStatusManuallyDisabled
	stale.ChannelInfo.MultiKeyStatusList = map[int]int{1: common.ChannelStatusTempDisabled}

	// 另一个写入方把渠道转换为单 key
	converted, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)
	converted.Key = "key-b"
	converted.ChannelInfo = ChannelInfo{}
	require.NoError(t, converted.UpdateWithChannelInfo())

	require.NoError(t, stale.saveStatusState())

	stored, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, stored.Status)
	assert.Equal(t, "key-b", stored.Key)
	assert.False(t, stored.ChannelInfo.IsMultiKey)
	assert.Empty(t, stored.ChannelInfo.MultiKeyStatusList)
}

func TestSaveStatusStatePersistsMultiKeyStatusOnFreshSnapshot(t *testing.T) {
	setupChannelInfoCASTest(t)
	channel := insertChannelInfoCASTestChannel(t)

	fresh, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)
	fresh.Status = common.ChannelStatusAutoDisabled
	fresh.ChannelInfo.MultiKeyStatusList = map[int]int{0: common.ChannelStatusTempDisabled}

	require.NoError(t, fresh.saveStatusState())

	stored, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusAutoDisabled, stored.Status)
	assert.Equal(t, map[int]int{0: common.ChannelStatusTempDisabled}, stored.ChannelInfo.MultiKeyStatusList)
}

func TestSaveChannelInfoPersistsAndRefreshesBaseline(t *testing.T) {
	setupChannelInfoCASTest(t)
	channel := insertChannelInfoCASTestChannel(t)

	loaded, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)
	loaded.ChannelInfo.MultiKeyPollingIndex = 1
	require.NoError(t, loaded.SaveChannelInfo())

	stored, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, 1, stored.ChannelInfo.MultiKeyPollingIndex)

	// 同一快照的第二次回写仍应成功（基线已刷新为上次写入的内容）
	loaded.ChannelInfo.MultiKeyPollingIndex = 0
	require.NoError(t, loaded.SaveChannelInfo())

	stored, err = GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Zero(t, stored.ChannelInfo.MultiKeyPollingIndex)
}