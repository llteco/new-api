package controller

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTokenChannelQuotas_OwnerAllowed(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.TokenChannelQuota{}))
	tok := seedToken(t, db, 5, "t", "sk-owner")
	require.NoError(t, model.UpsertTokenChannelQuota(tok.Id, 3, 2000))

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/token/"+strconv.Itoa(tok.Id)+"/channel_quotas", nil, 5)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(tok.Id)}}
	ctx.Set("role", common.RoleCommonUser)
	GetTokenChannelQuotas(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	resp := struct {
		Success bool                      `json:"success"`
		Data    []model.TokenChannelQuota `json:"data"`
	}{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Len(t, resp.Data, 1)
	assert.Equal(t, 3, resp.Data[0].ChannelId)
	assert.Equal(t, 2000, resp.Data[0].RemainQuota)
}

func TestGetTokenChannelQuotas_ForbiddenForOtherUser(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.TokenChannelQuota{}))
	tok := seedToken(t, db, 5, "t", "sk-other")

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/token/"+strconv.Itoa(tok.Id)+"/channel_quotas", nil, 999)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(tok.Id)}}
	ctx.Set("role", common.RoleCommonUser)
	GetTokenChannelQuotas(ctx)

	assert.Equal(t, http.StatusForbidden, recorder.Code)
}

func TestUpdateTokenChannelQuotas_Replace(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.TokenChannelQuota{}))
	tok := seedToken(t, db, 5, "t", "sk-replace")
	require.NoError(t, model.UpsertTokenChannelQuota(tok.Id, 1, 1000))

	body := map[string]any{"items": []map[string]any{
		{"channel_id": 2, "reset_quota": 3000},
		{"channel_id": 3, "reset_quota": 4000},
	}}
	ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/token/"+strconv.Itoa(tok.Id)+"/channel_quotas", body, 5)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(tok.Id)}}
	ctx.Set("role", common.RoleCommonUser)
	UpdateTokenChannelQuotas(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)

	rows, err := model.GetAllTokenChannelQuotas(tok.Id)
	require.NoError(t, err)
	assert.Len(t, rows, 2)
}
