package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreConsumeTokenQuota_ChannelMode_HitRow(t *testing.T) {
	truncate(t)
	tok := &model.Token{
		UserId: 1, Key: "sk-pc-chan", Status: 1,
		ChannelQuotaMode: true,
	}
	require.NoError(t, tok.Insert())
	require.NoError(t, model.UpsertTokenChannelQuota(tok.Id, 42, 1000))

	info := &relaycommon.RelayInfo{TokenId: tok.Id, TokenKey: tok.Key, ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 42}}
	hit, err := PreConsumeTokenQuotaChannel(info, 300)
	require.NoError(t, err)
	assert.True(t, hit)

	row, _ := model.GetTokenChannelQuota(tok.Id, 42)
	assert.Equal(t, 700, row.RemainQuota)
	assert.Equal(t, 300, row.UsedQuota)
}

func TestPreConsumeTokenQuota_ChannelMode_Insufficient(t *testing.T) {
	truncate(t)
	tok := &model.Token{
		UserId: 1, Key: "sk-pc-chan-ins", Status: 1,
		ChannelQuotaMode: true,
	}
	require.NoError(t, tok.Insert())
	require.NoError(t, model.UpsertTokenChannelQuota(tok.Id, 42, 100))

	info := &relaycommon.RelayInfo{TokenId: tok.Id, TokenKey: tok.Key, ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 42}}
	_, err := PreConsumeTokenQuotaChannel(info, 300)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "分渠道额度不足")
}

func TestPreConsumeTokenQuota_ChannelMode_NoRow_NoLimit(t *testing.T) {
	truncate(t)
	tok := &model.Token{
		UserId: 1, Key: "sk-pc-chan-norow", Status: 1,
		ChannelQuotaMode: true,
	}
	require.NoError(t, tok.Insert())
	info := &relaycommon.RelayInfo{TokenId: tok.Id, TokenKey: tok.Key, ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 99}}
	hit, err := PreConsumeTokenQuotaChannel(info, 300)
	require.NoError(t, err)
	assert.False(t, hit)
}

func TestPreConsumeTokenQuota_TotalMode_Delegates(t *testing.T) {
	truncate(t)
	tok := &model.Token{
		UserId: 1, Key: "sk-pc-total", Status: 1,
		ChannelQuotaMode: false,
		RemainQuota: 1000, UnlimitedQuota: false,
	}
	require.NoError(t, tok.Insert())
	info := &relaycommon.RelayInfo{TokenId: tok.Id, TokenKey: tok.Key, ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 42}}
	hit, err := PreConsumeTokenQuotaChannel(info, 300)
	require.NoError(t, err)
	assert.False(t, hit)
}
