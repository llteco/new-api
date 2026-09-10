package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupDistributorFallbackDB lazily initializes a SQLite database backing the
// channel selection cache for the fallback tests. Middleware package tests do
// not bootstrap model.DB by default, and package tests run sequentially.
func setupDistributorFallbackDB(t *testing.T) {
	t.Helper()
	if model.DB == nil {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		if err != nil {
			panic("failed to open test db: " + err.Error())
		}
		// :memory: 下每个连接是独立库，单连接保证所有读写落在同一份数据上
		sqlDB, err := db.DB()
		if err != nil {
			panic("failed to get sql.DB: " + err.Error())
		}
		sqlDB.SetMaxOpenConns(1)
		model.DB = db
		common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
		if err := db.AutoMigrate(&model.Channel{}, &model.Ability{}); err != nil {
			panic("failed to migrate test db: " + err.Error())
		}
	}
	require.NoError(t, model.DB.Exec("DELETE FROM abilities").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM channels").Error)

	memoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() {
		common.MemoryCacheEnabled = memoryCacheEnabled
		model.InitChannelCache()
	})
}

// TestSelectChannelWithAvailableKeyFallsBackToLowerPriority protects the
// scenario where the highest-priority channel stays enabled but has no usable
// key (all keys disabled or cooling down): the request must fall through to the
// lower-priority channel instead of being sent upstream with an empty key.
func TestSelectChannelWithAvailableKeyFallsBackToLowerPriority(t *testing.T) {
	setupDistributorFallbackDB(t)

	highPriority := int64(10)
	high := &model.Channel{
		Name:     "high-multi-key",
		Type:     constant.ChannelTypeOpenAI,
		Key:      "hk1\nhk2",
		Status:   common.ChannelStatusEnabled,
		Models:   "fallback-model",
		Group:    "default",
		Priority: &highPriority,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:            true,
			MultiKeySize:          2,
			MultiKeyMode:          constant.MultiKeyModeRandom,
			MultiKeyStatusList:    map[int]int{0: common.ChannelStatusTempDisabled, 1: common.ChannelStatusTempDisabled},
			MultiKeyCooldownUntil: map[int]int64{0: common.GetTimestamp() + 3600, 1: common.GetTimestamp() + 3600},
		},
	}
	require.NoError(t, model.DB.Create(high).Error)
	require.NoError(t, high.AddAbilities(nil))

	lowPriority := int64(0)
	low := &model.Channel{
		Name:     "low-single-key",
		Type:     constant.ChannelTypeOpenAI,
		Key:      "low-key",
		Status:   common.ChannelStatusEnabled,
		Models:   "fallback-model",
		Group:    "default",
		Priority: &lowPriority,
	}
	require.NoError(t, model.DB.Create(low).Error)
	require.NoError(t, low.AddAbilities(nil))

	model.InitChannelCache()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")

	selected, _, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{
		Ctx:         c,
		ModelName:   "fallback-model",
		TokenGroup:  "default",
		RequestPath: "/v1/chat/completions",
		Retry:       common.GetPointer(0),
	})
	require.NoError(t, err)
	require.NotNil(t, selected)
	require.Equal(t, high.Id, selected.Id)

	setupErr := SetupContextForSelectedChannel(c, selected, "fallback-model")
	require.NotNil(t, setupErr)
	assert.Equal(t, types.ErrorCodeChannelNoAvailableKey, setupErr.GetErrorCode())

	next := selectChannelWithAvailableKey(c, selected.Id, "fallback-model")
	require.NotNil(t, next)
	assert.Equal(t, low.Id, next.Id)
	assert.Equal(t, "low-key", common.GetContextKeyString(c, constant.ContextKeyChannelKey))
}

// TestSelectChannelWithAvailableKeyStopsWhenAllChannelsRepeat ensures the
// fallback terminates (returns nil) when every candidate channel has already
// been tried, instead of looping forever on a saturated lowest priority tier.
func TestSelectChannelWithAvailableKeyStopsWhenAllChannelsRepeat(t *testing.T) {
	setupDistributorFallbackDB(t)

	only := &model.Channel{
		Name:   "only-cooling",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "k1",
		Status: common.ChannelStatusEnabled,
		Models: "fallback-model",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:            true,
			MultiKeySize:          1,
			MultiKeyMode:          constant.MultiKeyModeRandom,
			MultiKeyStatusList:    map[int]int{0: common.ChannelStatusTempDisabled},
			MultiKeyCooldownUntil: map[int]int64{0: common.GetTimestamp() + 3600},
		},
	}
	require.NoError(t, model.DB.Create(only).Error)
	require.NoError(t, only.AddAbilities(nil))

	model.InitChannelCache()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")

	assert.Nil(t, selectChannelWithAvailableKey(c, only.Id, "fallback-model"))
}

// TestSelectChannelWithAvailableKeyTerminatesAcrossTiers drives the fallback
// through multiple priority tiers whose channels all lack a usable key; it
// must exhaust the random same-tier retries and return nil deterministically.
func TestSelectChannelWithAvailableKeyTerminatesAcrossTiers(t *testing.T) {
	setupDistributorFallbackDB(t)

	highPriority, lowPriority := int64(10), int64(0)
	newCoolingChannel := func(name string, priority *int64) *model.Channel {
		return &model.Channel{
			Name: name, Type: constant.ChannelTypeOpenAI,
			Key: "k1\nk2", Status: common.ChannelStatusEnabled,
			Models: "fallback-model", Group: "default", Priority: priority,
			ChannelInfo: model.ChannelInfo{
				IsMultiKey:            true,
				MultiKeySize:          2,
				MultiKeyMode:          constant.MultiKeyModeRandom,
				MultiKeyStatusList:    map[int]int{0: common.ChannelStatusTempDisabled, 1: common.ChannelStatusTempDisabled},
				MultiKeyCooldownUntil: map[int]int64{0: common.GetTimestamp() + 3600, 1: common.GetTimestamp() + 3600},
			},
		}
	}
	for _, def := range []struct {
		name     string
		priority *int64
	}{{"cool-high", &highPriority}, {"cool-low-a", &lowPriority}, {"cool-low-b", &lowPriority}} {
		ch := newCoolingChannel(def.name, def.priority)
		require.NoError(t, model.DB.Create(ch).Error)
		require.NoError(t, ch.AddAbilities(nil))
	}

	model.InitChannelCache()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")

	assert.Nil(t, selectChannelWithAvailableKey(c, 1, "fallback-model"))
}
