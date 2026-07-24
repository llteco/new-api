package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBillingSession_SettleChannelMode_Supplement(t *testing.T) {
	truncate(t)
	tok := &model.Token{UserId: 1, Key: "sk-bs-chan", Status: 1, ChannelQuotaMode: true}
	require.NoError(t, tok.Insert())
	require.NoError(t, model.UpsertTokenChannelQuota(tok.Id, 7, 1000))
	require.NoError(t, model.DecreaseTokenChannelQuota(tok.Id, 7, 300))

	info := &relaycommon.RelayInfo{
		UserId: 1, TokenId: tok.Id, TokenKey: tok.Key,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 7},
	}
	s := &BillingSession{
		relayInfo:        info,
		funding:          &WalletFunding{},
		fundingSettled:   true,
		channelQuotaMode: true,
		channelId:        7,
		channelHit:       true,
		channelConsumed:  300,
		preConsumedQuota: 300,
	}
	require.NoError(t, s.Settle(400))
	row, _ := model.GetTokenChannelQuota(tok.Id, 7)
	assert.Equal(t, 600, row.RemainQuota)
	assert.Equal(t, 400, row.UsedQuota)
}

func TestBillingSession_SettleChannelMode_RefundDelta(t *testing.T) {
	truncate(t)
	tok := &model.Token{UserId: 1, Key: "sk-bs-chan-ref", Status: 1, ChannelQuotaMode: true}
	require.NoError(t, tok.Insert())
	require.NoError(t, model.UpsertTokenChannelQuota(tok.Id, 7, 1000))
	require.NoError(t, model.DecreaseTokenChannelQuota(tok.Id, 7, 500))

	info := &relaycommon.RelayInfo{
		UserId: 1, TokenId: tok.Id, TokenKey: tok.Key,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 7},
	}
	s := &BillingSession{
		relayInfo:        info,
		funding:          &WalletFunding{},
		fundingSettled:   true,
		channelQuotaMode: true,
		channelId:        7,
		channelHit:       true,
		channelConsumed:  500,
		preConsumedQuota: 500,
	}
	require.NoError(t, s.Settle(200))
	row, _ := model.GetTokenChannelQuota(tok.Id, 7)
	assert.Equal(t, 800, row.RemainQuota)
	assert.Equal(t, 200, row.UsedQuota)
}

func TestBillingSession_SettleChannelMode_UnconfiguredChannel_NoOp(t *testing.T) {
	truncate(t)
	tok := &model.Token{UserId: 1, Key: "sk-bs-norow", Status: 1, ChannelQuotaMode: true}
	require.NoError(t, tok.Insert())
	info := &relaycommon.RelayInfo{
		UserId: 1, TokenId: tok.Id, TokenKey: tok.Key,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 99},
	}
	s := &BillingSession{
		relayInfo:        info,
		funding:          &WalletFunding{},
		fundingSettled:   true,
		channelQuotaMode: true,
		channelId:        99,
		channelHit:       false,
		preConsumedQuota: 300,
	}
	require.NoError(t, s.Settle(400))
	_, err := model.GetTokenChannelQuota(tok.Id, 99)
	assert.Error(t, err, "no row should exist for unconfigured channel")
}

func TestBillingSession_RefundChannelMode(t *testing.T) {
	truncate(t)
	tok := &model.Token{UserId: 1, Key: "sk-bs-refund", Status: 1, ChannelQuotaMode: true}
	require.NoError(t, tok.Insert())
	require.NoError(t, model.UpsertTokenChannelQuota(tok.Id, 7, 1000))
	require.NoError(t, model.DecreaseTokenChannelQuota(tok.Id, 7, 300))
	info := &relaycommon.RelayInfo{
		UserId: 1, TokenId: tok.Id, TokenKey: tok.Key,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 7},
	}
	s := &BillingSession{
		relayInfo:        info,
		funding:          &WalletFunding{},
		channelQuotaMode: true,
		channelId:        7,
		channelHit:       true,
		channelConsumed:  300,
		preConsumedQuota: 300,
	}
	s.Refund(&gin.Context{})
	require.Eventually(t, func() bool {
		row, _ := model.GetTokenChannelQuota(tok.Id, 7)
		return row != nil && row.RemainQuota == 1000
	}, 2*time.Second, 20*time.Millisecond)
}
