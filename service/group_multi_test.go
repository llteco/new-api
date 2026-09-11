package service

import (
	"testing"

	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func configureMultiGroupTest(t *testing.T) {
	t.Helper()
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	originalRatios := ratio_setting.GroupRatio2JSONString()
	originalGroupGroupRatios := ratio_setting.GroupGroupRatio2JSONString()
	specialUsable := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup
	originalSpecialUsable := specialUsable.ReadAll()

	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"VIP分组"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"group0":1,"group1":1,"group2":1,"premium":1}`))

	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatios))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(originalGroupGroupRatios))
		specialUsable.Clear()
		specialUsable.AddAll(originalSpecialUsable)
	})
}

func TestGetUserUsableGroupsMultiGroupUnion(t *testing.T) {
	configureMultiGroupTest(t)

	groups := GetUserUsableGroups("group0,group1")

	// 全局可用分组 + 用户所属的每个分组本身
	for _, group := range []string{"default", "vip", "group0", "group1"} {
		assert.Contains(t, groups, group)
	}
	assert.NotContains(t, groups, "group2")
}

func TestGetUserUsableGroupsMultiGroupSpecialRulesAppliedInOrder(t *testing.T) {
	configureMultiGroupTest(t)
	specialUsable := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup
	specialUsable.Set("group0", map[string]string{"+:premium": "Premium"})
	specialUsable.Set("group1", map[string]string{"-:vip": ""})

	groups := GetUserUsableGroups("group0,group1")

	// group0 的规则先添加 premium，group1 的规则随后移除 vip
	assert.Contains(t, groups, "premium")
	assert.NotContains(t, groups, "vip")
	assert.Contains(t, groups, "group0")
	assert.Contains(t, groups, "group1")
}

func TestGetUserGroupRatioMultiGroupFirstMatchWins(t *testing.T) {
	configureMultiGroupTest(t)
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(
		`{"group1":{"vip":0.8},"group0":{"vip":0.5,"group1":1.2}}`))

	// 两个所属分组都配置了 vip 的专属倍率时，按列表顺序取第一个（主分组优先）
	ratio := GetUserGroupRatio("group0,group1", "vip")
	assert.Equal(t, 0.5, ratio)

	// 仅第二个分组配置时使用第二个分组
	ratio = GetUserGroupRatio("group2,group1", "vip")
	assert.Equal(t, 0.8, ratio)

	// 没有任何专属倍率时回退到全局分组倍率
	ratio = GetUserGroupRatio("group0,group1", "premium")
	assert.Equal(t, 1.0, ratio)
}

func TestGetUserGroupRatioSingleGroupUnchanged(t *testing.T) {
	configureMultiGroupTest(t)
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(
		`{"vip":{"default":0.9}}`))

	assert.Equal(t, 0.9, GetUserGroupRatio("vip", "default"))
	assert.Equal(t, 1.0, GetUserGroupRatio("default", "default"))
}

func TestIsUserSelectableGroupMultiGroup(t *testing.T) {
	configureMultiGroupTest(t)
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["vip","group1"]`))
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateAutoGroupsByJsonString("[]"))
	})

	// 所属任一分组的可选项均可用
	assert.True(t, IsUserSelectableGroup("group0,group1", "vip"))
	assert.True(t, IsUserSelectableGroup("group0,group1", "group1"))
	assert.False(t, IsUserSelectableGroup("group0,group1", "group2"))
}

func TestGetUserAutoGroupMultiGroup(t *testing.T) {
	configureMultiGroupTest(t)
	originalAutoGroups := setting.AutoGroups2JsonString()
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["vip","group0","group1"]`))
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
	})

	groups := GetUserAutoGroup("group1,group0")

	// Auto 列表顺序保持不变：vip 来自全局可用分组，group0/group1 来自用户所属分组
	assert.Equal(t, []string{"vip", "group0", "group1"}, groups)
}

func TestGetUserUsableGroupsEmptyIsGlobalOnly(t *testing.T) {
	configureMultiGroupTest(t)

	groups := GetUserUsableGroups("")
	delete(groups, "default")
	delete(groups, "vip")
	assert.Empty(t, groups)
}
