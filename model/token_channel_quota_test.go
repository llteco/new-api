package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenChannelQuota_CRUD(t *testing.T) {
	truncateTables(t)

	tok := &Token{UserId: 1, Key: "sk-test-crud", RemainQuota: 0, Status: 1}
	require.NoError(t, tok.Insert())

	require.NoError(t, UpsertTokenChannelQuota(tok.Id, 100, 5000))

	row, err := GetTokenChannelQuota(tok.Id, 100)
	require.NoError(t, err)
	assert.Equal(t, 5000, row.RemainQuota)
	assert.Equal(t, 5000, row.ResetQuota)
	assert.Equal(t, 0, row.UsedQuota)

	require.NoError(t, DecreaseTokenChannelQuota(tok.Id, 100, 300))
	row, _ = GetTokenChannelQuota(tok.Id, 100)
	assert.Equal(t, 4700, row.RemainQuota)
	assert.Equal(t, 300, row.UsedQuota)

	require.NoError(t, IncreaseTokenChannelQuota(tok.Id, 100, 100))
	row, _ = GetTokenChannelQuota(tok.Id, 100)
	assert.Equal(t, 4800, row.RemainQuota)
	assert.Equal(t, 200, row.UsedQuota)

	_, err = GetTokenChannelQuota(tok.Id, 999)
	assert.Error(t, err)

	require.NoError(t, UpsertTokenChannelQuota(tok.Id, 200, 1000))
	require.NoError(t, DecreaseTokenChannelQuota(tok.Id, 200, 400))
	require.NoError(t, ResetAllTokenChannelQuotas(tok.Id))
	row100, _ := GetTokenChannelQuota(tok.Id, 100)
	row200, _ := GetTokenChannelQuota(tok.Id, 200)
	assert.Equal(t, 5000, row100.RemainQuota)
	assert.Equal(t, 0, row100.UsedQuota)
	assert.Equal(t, 1000, row200.RemainQuota)
	assert.Equal(t, 0, row200.UsedQuota)
}

func TestTokenChannelQuota_NegativeQuotaRejected(t *testing.T) {
	truncateTables(t)
	tok := &Token{UserId: 1, Key: "sk-neg", Status: 1}
	require.NoError(t, tok.Insert())
	require.NoError(t, UpsertTokenChannelQuota(tok.Id, 5, 100))
	assert.Error(t, DecreaseTokenChannelQuota(tok.Id, 5, -1))
	assert.Error(t, IncreaseTokenChannelQuota(tok.Id, 5, -1))
}
