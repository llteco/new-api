package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetNextEnabledKeyPollingAdvancesForDetachedSnapshot protects locked-channel
// key rotation (task remix/continuation): the locked channel is a detached DB
// snapshot, and polling must still advance the shared cached index so each
// retry uses the next key instead of repeating the same one.
func TestGetNextEnabledKeyPollingAdvancesForDetachedSnapshot(t *testing.T) {
	setupChannelFailoverDB(t)
	useFailoverMemoryCache(t, true)

	channel := &Channel{
		Name:   "polling-locked",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "k0\nk1\nk2",
		Status: common.ChannelStatusEnabled,
		Models: "failover-model",
		Group:  "default",
		ChannelInfo: ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 3,
			MultiKeyMode: constant.MultiKeyModePolling,
		},
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	InitChannelCache()

	snapshot, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)

	seen := make(map[string]bool)
	for i := 0; i < 3; i++ {
		key, _, keyErr := snapshot.GetNextEnabledKey()
		// GetNextEnabledKey returns a concrete *NewAPIError; require.Nil handles
		// the typed-nil case that require.NoError would flag via the interface.
		require.Nil(t, keyErr)
		assert.False(t, seen[key], "polling repeated key %q on a detached snapshot", key)
		seen[key] = true
	}
	assert.Equal(t, map[string]bool{"k0": true, "k1": true, "k2": true}, seen)
}
