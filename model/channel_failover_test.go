package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupChannelFailoverDB lazily initializes a SQLite database for the failover
// selection tests and clears channel/ability rows between tests.
func setupChannelFailoverDB(t *testing.T) {
	t.Helper()
	if DB == nil {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		// :memory: 下每个连接是独立库，单连接保证所有读写落在同一份数据上
		sqlDB.SetMaxOpenConns(1)
		DB = db
		common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
		require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))
	}
	require.NoError(t, DB.Exec("DELETE FROM abilities").Error)
	require.NoError(t, DB.Exec("DELETE FROM channels").Error)
}

func createFailoverChannel(t *testing.T, name string, priority int64) *Channel {
	t.Helper()
	weight := uint(10)
	channel := &Channel{
		Name:     name,
		Type:     constant.ChannelTypeOpenAI,
		Key:      "key-" + name,
		Status:   common.ChannelStatusEnabled,
		Models:   "failover-model",
		Group:    "default",
		Priority: &priority,
		Weight:   &weight,
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	return channel
}

func useFailoverMemoryCache(t *testing.T, enabled bool) {
	t.Helper()
	original := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = enabled
	t.Cleanup(func() {
		common.MemoryCacheEnabled = original
		if original {
			InitChannelCache()
		}
	})
	if enabled {
		InitChannelCache()
	}
}

// TestGetRandomSatisfiedChannelExcludesTriedChannels protects the core failover
// invariant: with channels already attempted excluded, selection never returns
// a tried channel again, covers every remaining channel, and returns nil once
// all channels are exhausted. Runs against both the cache and DB paths.
func TestGetRandomSatisfiedChannelExcludesTriedChannels(t *testing.T) {
	for _, memoryCache := range []bool{true, false} {
		name := "MemoryCache"
		if !memoryCache {
			name = "Database"
		}
		t.Run(name, func(t *testing.T) {
			setupChannelFailoverDB(t)
			a := createFailoverChannel(t, "a", 0)
			b := createFailoverChannel(t, "b", 0)
			c := createFailoverChannel(t, "c", 0)
			useFailoverMemoryCache(t, memoryCache)

			seen := make(map[int]bool)
			var excluded []int
			for i := 0; i < 3; i++ {
				channel, err := GetRandomSatisfiedChannel("default", "failover-model", excluded, "/v1/chat/completions")
				require.NoError(t, err)
				require.NotNil(t, channel)
				assert.False(t, seen[channel.Id], "channel %d selected twice", channel.Id)
				seen[channel.Id] = true
				excluded = append(excluded, channel.Id)
			}
			assert.Equal(t, map[int]bool{a.Id: true, b.Id: true, c.Id: true}, seen)

			channel, err := GetRandomSatisfiedChannel("default", "failover-model", excluded, "/v1/chat/completions")
			require.NoError(t, err)
			assert.Nil(t, channel, "selection must exhaust after all channels were tried")
		})
	}
}

// TestGetRandomSatisfiedChannelFallsBackToLowerPriority protects the priority
// contract of failover: every channel of the highest priority tier is tried
// before falling back to the next tier, and exhaustion still ends in nil.
func TestGetRandomSatisfiedChannelFallsBackToLowerPriority(t *testing.T) {
	for _, memoryCache := range []bool{true, false} {
		name := "MemoryCache"
		if !memoryCache {
			name = "Database"
		}
		t.Run(name, func(t *testing.T) {
			setupChannelFailoverDB(t)
			highA := createFailoverChannel(t, "high-a", 10)
			highB := createFailoverChannel(t, "high-b", 10)
			low := createFailoverChannel(t, "low", 0)
			useFailoverMemoryCache(t, memoryCache)

			highTier := map[int]bool{highA.Id: true, highB.Id: true}
			var excluded []int
			for i := 0; i < 2; i++ {
				channel, err := GetRandomSatisfiedChannel("default", "failover-model", excluded, "/v1/chat/completions")
				require.NoError(t, err)
				require.NotNil(t, channel)
				assert.True(t, highTier[channel.Id], "attempt %d must stay in the highest priority tier, got channel %d", i, channel.Id)
				delete(highTier, channel.Id)
				excluded = append(excluded, channel.Id)
			}

			channel, err := GetRandomSatisfiedChannel("default", "failover-model", excluded, "/v1/chat/completions")
			require.NoError(t, err)
			require.NotNil(t, channel)
			assert.Equal(t, low.Id, channel.Id, "lower tier is used only after the higher tier is exhausted")
			excluded = append(excluded, channel.Id)

			channel, err = GetRandomSatisfiedChannel("default", "failover-model", excluded, "/v1/chat/completions")
			require.NoError(t, err)
			assert.Nil(t, channel)
		})
	}
}
