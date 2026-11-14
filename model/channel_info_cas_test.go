package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelInfoCASTest(t *testing.T) {
	t.Helper()
	truncateTables(t)
	require.NoError(t, DB.Exec("DELETE FROM abilities").Error)
	require.NoError(t, DB.Exec("DELETE FROM channels").Error)

	memoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		common.MemoryCacheEnabled = memoryCacheEnabled
	})
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

	// 请求结束，尝试回写轮询索引；CAS 拒绝且不报错
	inFlight.ChannelInfo.MultiKeyPollingIndex = 1
	saved, err := inFlight.SaveChannelInfo()
	require.NoError(t, err)
	assert.False(t, saved)

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

	saved, err := stale.saveStatusState()
	require.NoError(t, err)
	assert.False(t, saved)

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

	saved, err := fresh.saveStatusState()
	require.NoError(t, err)
	assert.True(t, saved)

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
	saved, err := loaded.SaveChannelInfo()
	require.NoError(t, err)
	assert.True(t, saved)

	stored, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, 1, stored.ChannelInfo.MultiKeyPollingIndex)

	// 同一快照的第二次回写仍应成功（基线已刷新为上次写入的内容）
	loaded.ChannelInfo.MultiKeyPollingIndex = 0
	saved, err = loaded.SaveChannelInfo()
	require.NoError(t, err)
	assert.True(t, saved)

	stored, err = GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Zero(t, stored.ChannelInfo.MultiKeyPollingIndex)
}

// injectOneConcurrentChannelWrite 在第一次携带 channel_info 的 channels 更新执行前，
// 先把该行改成并发写入方的内容（模拟其他实例在快照加载后、回写前推进轮询游标或
// 完成密钥模式转换），使该次回写的乐观并发校验失配。只注入一次。
// 注意 GORM 默认为每条更新包事务：注入写必须复用外层语句的连接
// （Session(NewDB) 共享 ConnPool），另开连接会在 MaxOpenConns(1) 的测试库上死锁。
func injectOneConcurrentChannelWrite(t *testing.T, channelId int, concurrentKey string, concurrentInfo string) {
	t.Helper()
	var fired bool
	callback := func(tx *gorm.DB) {
		if fired || tx.Statement == nil || tx.Statement.Table != "channels" {
			return
		}
		dest, ok := tx.Statement.Dest.(map[string]interface{})
		if !ok {
			return
		}
		if _, writesInfo := dest["channel_info"]; !writesInfo {
			return
		}
		fired = true
		// channel_info 按 []byte 绑定（与 ChannelInfo.Value() 的存储形态一致），
		// string 参数会被存成 TEXT 列值，Scan 的 []byte 断言读不回来
		if err := tx.Session(&gorm.Session{NewDB: true}).Exec(
			"UPDATE channels SET key = ?, channel_info = ? WHERE id = ?",
			concurrentKey, []byte(concurrentInfo), channelId,
		).Error; err != nil {
			panic(err)
		}
	}
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register("test:concurrent_channel_write", callback))
	t.Cleanup(func() {
		DB.Callback().Update().Remove("test:concurrent_channel_write")
	})
}

// 状态回写与其他实例的 channel_info 并发写竞争时，重载快照重放变更，
// per-key 禁用状态必须最终落库而不是被 CAS 静默挤掉。
func TestUpdateChannelStatusRetriesAfterConcurrentChannelInfoWrite(t *testing.T) {
	setupChannelInfoCASTest(t)
	channel := insertChannelInfoCASTestChannel(t)

	concurrent, err := common.Marshal(ChannelInfo{
		IsMultiKey:           true,
		MultiKeySize:         2,
		MultiKeyMode:         constant.MultiKeyModePolling,
		MultiKeyPollingIndex: 1,
	})
	require.NoError(t, err)
	injectOneConcurrentChannelWrite(t, channel.Id, "key-a\nkey-b", string(concurrent))

	cooldown := common.GetTimestamp() + 60
	changed := UpdateChannelKeyLimitStatus(channel.Id, "key-a", cooldown, "rate limited")
	require.True(t, changed)

	stored, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
	assert.Equal(t, common.ChannelStatusTempDisabled, stored.ChannelInfo.MultiKeyStatusList[0])
	assert.Equal(t, cooldown, stored.ChannelInfo.MultiKeyCooldownUntil[0])
	// 并发推进的轮询游标不得被重放覆盖回旧值
	assert.Equal(t, 1, stored.ChannelInfo.MultiKeyPollingIndex)
}

// 重放时发现密钥模式已被转换为单 key：放弃回写让转换结果生效，
// 不把 per-key 状态写回单 key 渠道。
func TestUpdateChannelStatusGivesUpWhenConvertedToSingleKey(t *testing.T) {
	setupChannelInfoCASTest(t)
	channel := insertChannelInfoCASTestChannel(t)

	converted, err := common.Marshal(ChannelInfo{})
	require.NoError(t, err)
	injectOneConcurrentChannelWrite(t, channel.Id, "key-b", string(converted))

	changed := UpdateChannelKeyLimitStatus(channel.Id, "key-a", common.GetTimestamp()+60, "rate limited")
	require.False(t, changed)

	stored, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, "key-b", stored.Key)
	assert.False(t, stored.ChannelInfo.IsMultiKey)
	assert.Empty(t, stored.ChannelInfo.MultiKeyStatusList)
}