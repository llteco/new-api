package service

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelSelectAutoGroupsTest(t *testing.T) *gorm.DB {
	t.Helper()

	originalDB := model.DB
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalAutoGroups := setting.AutoGroups2JsonString()
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	originalGroupRatios := ratio_setting.GroupRatio2JSONString()
	originalMaxTokenAutoGroups := setting.GetMaxTokenAutoGroups()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	model.DB = db
	common.MemoryCacheEnabled = true

	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`[]`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":2}`))
	require.NoError(t, setting.UpdateMaxTokenAutoGroups("2"))

	t.Cleanup(func() {
		model.DB = originalDB
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatios))
		require.NoError(t, setting.UpdateMaxTokenAutoGroups(fmt.Sprintf("%d", originalMaxTokenAutoGroups)))

		if originalMemoryCacheEnabled && originalDB != nil &&
			originalDB.Migrator().HasTable(&model.Channel{}) && originalDB.Migrator().HasTable(&model.Ability{}) {
			model.InitChannelCache()
		}
		sqlDB, err := db.DB()
		if err == nil {
			require.NoError(t, sqlDB.Close())
		}
	})

	return db
}

func createChannelSelectAutoGroupsChannel(t *testing.T, db *gorm.DB, id int, groups, modelName string) {
	t.Helper()
	priority := int64(0)
	weight := uint(100)
	require.NoError(t, db.Create(&model.Channel{
		Id:       id,
		Type:     constant.ChannelTypeOpenAI,
		Key:      fmt.Sprintf("key-%d", id),
		Status:   common.ChannelStatusEnabled,
		Name:     fmt.Sprintf("channel-%d", id),
		Weight:   &weight,
		Models:   modelName,
		Group:    groups,
		Priority: &priority,
	}).Error)
	for _, group := range strings.Split(groups, ",") {
		require.NoError(t, db.Create(&model.Ability{
			Group:     group,
			Model:     modelName,
			ChannelId: id,
			Enabled:   true,
			Priority:  &priority,
			Weight:    weight,
		}).Error)
	}
}

func newAutoGroupsRetryParam(ctx *gin.Context, modelName string, failover bool) *RetryParam {
	return &RetryParam{
		Ctx:         ctx,
		TokenGroup:  "auto",
		ModelName:   modelName,
		RequestPath: "/v1/chat/completions",
		Retry:       common.GetPointer(0),
		Failover:    failover,
	}
}

func newAutoGroupsTestContext(t *testing.T, autoGroups []string, crossGroupRetry bool) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, autoGroups)
	common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, crossGroupRetry)
	return ctx
}

func TestCacheGetRandomSatisfiedChannelUsesTokenAutoGroupsWhenGlobalAutoIsEmpty(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "auto-groups-runtime-model"
	createChannelSelectAutoGroupsChannel(t, db, 2101, "vip", modelName)
	createChannelSelectAutoGroupsChannel(t, db, 2102, "default", modelName)
	model.InitChannelCache()

	ctx := newAutoGroupsTestContext(t, []string{"vip", "default"}, true)
	param := newAutoGroupsRetryParam(ctx, modelName, true)

	first, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, 2101, first.Id)
	assert.Equal(t, "vip", selectedGroup)
	assert.Equal(t, "vip", common.GetContextKeyString(ctx, constant.ContextKeyAutoGroup))
	assert.Empty(t, setting.GetAutoGroups(), "the selection must not depend on the global Auto list")

	// Failover marks the failed channel as tried; the exhausted group is then
	// left for the next auto group.
	param.MarkTried(first.Id)
	param.IncreaseRetry()
	second, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, 2102, second.Id)
	assert.Equal(t, "default", selectedGroup)
	assert.Equal(t, "default", common.GetContextKeyString(ctx, constant.ContextKeyAutoGroup))
}

// TestCacheGetRandomSatisfiedChannelFailoverStaysInGroupWithoutCrossGroupRetry
// protects the token-level cross-group switch: with cross-group retry disabled,
// failover exhausts the current group and stops instead of crossing groups.
func TestCacheGetRandomSatisfiedChannelFailoverStaysInGroupWithoutCrossGroupRetry(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "auto-groups-no-cross-model"
	createChannelSelectAutoGroupsChannel(t, db, 2201, "vip", modelName)
	createChannelSelectAutoGroupsChannel(t, db, 2202, "default", modelName)
	model.InitChannelCache()

	ctx := newAutoGroupsTestContext(t, []string{"vip", "default"}, false)
	param := newAutoGroupsRetryParam(ctx, modelName, true)

	first, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, 2201, first.Id)
	assert.Equal(t, "vip", selectedGroup)

	param.MarkTried(first.Id)
	param.IncreaseRetry()
	next, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	assert.Nil(t, next, "failover must not cross groups when the token disables cross-group retry")
}

// TestCacheGetRandomSatisfiedChannelInitialSelectionCrossesGroups protects
// initial selection (distributor): it must find a group serving the model even
// when the token disables cross-group retry, which only gates failover.
func TestCacheGetRandomSatisfiedChannelInitialSelectionCrossesGroups(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "auto-groups-initial-model"
	createChannelSelectAutoGroupsChannel(t, db, 2301, "default", modelName)
	model.InitChannelCache()

	ctx := newAutoGroupsTestContext(t, []string{"vip", "default"}, false)
	param := newAutoGroupsRetryParam(ctx, modelName, false)

	channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 2301, channel.Id)
	assert.Equal(t, "default", selectedGroup)
}

// TestCacheGetRandomSatisfiedChannelTriedChannelsPersistAcrossGroups protects
// the per-request exclusion set: a channel serving multiple auto groups must
// not be retried after a group switch.
func TestCacheGetRandomSatisfiedChannelTriedChannelsPersistAcrossGroups(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "auto-groups-shared-channel-model"
	createChannelSelectAutoGroupsChannel(t, db, 2401, "vip,default", modelName)
	createChannelSelectAutoGroupsChannel(t, db, 2402, "default", modelName)
	model.InitChannelCache()

	ctx := newAutoGroupsTestContext(t, []string{"vip", "default"}, true)
	param := newAutoGroupsRetryParam(ctx, modelName, true)

	first, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, 2401, first.Id)
	assert.Equal(t, "vip", selectedGroup)

	param.MarkTried(first.Id)
	param.IncreaseRetry()
	second, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, 2402, second.Id, "the tried channel must stay excluded after switching groups")
	assert.Equal(t, "default", selectedGroup)
}

// TestCacheGetRandomSatisfiedChannelExhaustsGroupBeforeSwitching protects the
// within-group failover order: all channels of the current group are tried
// before moving to the next auto group.
func TestCacheGetRandomSatisfiedChannelExhaustsGroupBeforeSwitching(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "auto-groups-exhaust-model"
	createChannelSelectAutoGroupsChannel(t, db, 2501, "vip", modelName)
	createChannelSelectAutoGroupsChannel(t, db, 2502, "vip", modelName)
	createChannelSelectAutoGroupsChannel(t, db, 2503, "default", modelName)
	model.InitChannelCache()

	ctx := newAutoGroupsTestContext(t, []string{"vip", "default"}, true)
	param := newAutoGroupsRetryParam(ctx, modelName, true)

	first, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, "vip", selectedGroup)
	assert.Contains(t, []int{2501, 2502}, first.Id)

	param.MarkTried(first.Id)
	param.IncreaseRetry()
	second, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, "vip", selectedGroup, "the group must not be left while it still has untried channels")
	assert.NotEqual(t, first.Id, second.Id)
	assert.Contains(t, []int{2501, 2502}, second.Id)

	param.MarkTried(second.Id)
	param.IncreaseRetry()
	third, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, third)
	assert.Equal(t, 2503, third.Id)
	assert.Equal(t, "default", selectedGroup)

	param.MarkTried(third.Id)
	param.IncreaseRetry()
	fourth, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	assert.Nil(t, fourth, "selection must return nil once every auto group is exhausted")
}
