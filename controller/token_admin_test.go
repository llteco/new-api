package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const adminTokenAdminUserID = 1

// setupAdminTokenControllerTest 准备 tokens + users 表：
// 管理员 adminTokenAdminUserID（分组 default），属主 101（分组 vip）。
func setupAdminTokenControllerTest(t *testing.T) {
	t.Helper()
	db := setupTokenControllerTestDB(t)
	model.InitColumnNames()
	require.NoError(t, db.AutoMigrate(&model.User{}))
	users := []model.User{
		{Id: adminTokenAdminUserID, Username: "token-admin", Password: "password", Group: "default", Status: common.UserStatusEnabled, AffCode: "token-admin-aff"},
		{Id: 101, Username: "token-owner", Password: "password", Group: "vip", Status: common.UserStatusEnabled, AffCode: "token-owner-aff"},
		{Id: 102, Username: "token-other", Password: "password", Group: "default", Status: common.UserStatusEnabled, AffCode: "token-other-aff"},
	}
	for i := range users {
		require.NoError(t, db.Create(&users[i]).Error)
	}
}

// configureOwnerRestrictedGroups 让 "vip" 分组用户不可选 "default" 分组（-: 前缀移除），
// 用于验证 auto_groups 按令牌属主而非管理员分组校验。
func configureOwnerRestrictedGroups(t *testing.T) {
	t.Helper()
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	originalAutoGroups := setting.AutoGroups2JsonString()
	originalRatios := ratio_setting.GroupRatio2JSONString()
	special := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup
	originalSpecial, hadSpecial := special.Get("vip")

	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP"}`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default","vip"]`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1}`))
	special.Set("vip", map[string]string{"-:default": "removed"})
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatios))
		if hadSpecial {
			special.Set("vip", originalSpecial)
		} else {
			// RWMap 无按 key 删除；nil map 在遍历时等价于无特殊分组
			special.Set("vip", nil)
		}
	})
}

func newAdminTokenContext(t *testing.T, method string, target string, body any) (*gin.Context, *httptest.ResponseRecorder) {
	return newAuthenticatedContext(t, method, target, body, adminTokenAdminUserID)
}

func TestAdminGetAllTokensListsAllUsersAndFiltersByUser(t *testing.T) {
	setupAdminTokenControllerTest(t)
	ownerToken := seedToken(t, model.DB, 101, "owner-token", "adml1111keyaaaa1111")
	seedToken(t, model.DB, 102, "other-token", "adml2222keybbbb2222")

	// 无 user_id：跨用户列出全部
	ctx, recorder := newAdminTokenContext(t, http.MethodGet, "/api/token/admin/?p=1&size=10", nil)
	AdminGetAllTokens(ctx)

	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	var page tokenPageResponse
	require.NoError(t, common.Unmarshal(response.Data, &page))
	assert.Len(t, page.Items, 2)
	for _, item := range page.Items {
		assert.NotEqual(t, "adml1111keyaaaa1111", item.Key, "admin list must mask keys")
		assert.Contains(t, item.Key, "*", "admin list must mask keys")
	}
	assert.NotContains(t, recorder.Body.String(), ownerToken.Key)

	// 带 user_id：只列出该用户
	filteredCtx, filteredRecorder := newAdminTokenContext(t, http.MethodGet, "/api/token/admin/?user_id=101&p=1&size=10", nil)
	AdminGetAllTokens(filteredCtx)

	filteredResponse := decodeAPIResponse(t, filteredRecorder)
	require.True(t, filteredResponse.Success, filteredResponse.Message)
	var filteredPage tokenPageResponse
	require.NoError(t, common.Unmarshal(filteredResponse.Data, &filteredPage))
	require.Len(t, filteredPage.Items, 1)
	assert.Equal(t, "owner-token", filteredPage.Items[0].Name)
}

func TestAdminSearchTokensAcrossUsers(t *testing.T) {
	setupAdminTokenControllerTest(t)
	seedToken(t, model.DB, 101, "cross-user-needle", "adml3333keycccc3333")
	seedToken(t, model.DB, 102, "unrelated", "adml4444keydddd4444")

	ctx, recorder := newAdminTokenContext(t, http.MethodGet, "/api/token/admin/search?keyword=cross-user-needle&p=1&size=10", nil)
	AdminSearchTokens(ctx)

	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	var page tokenPageResponse
	require.NoError(t, common.Unmarshal(response.Data, &page))
	require.Len(t, page.Items, 1)
	assert.Equal(t, "cross-user-needle", page.Items[0].Name)
}

func TestAdminUpdateTokenCrossUserStatusOnly(t *testing.T) {
	setupAdminTokenControllerTest(t)
	token := seedToken(t, model.DB, 101, "status-token", "adml5555keyeeee5555")

	body := map[string]any{"id": token.Id, "status": common.TokenStatusDisabled}
	ctx, recorder := newAdminTokenContext(t, http.MethodPut, "/api/token/admin/?status_only=true", body)
	AdminUpdateToken(ctx)

	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)

	var updated model.Token
	require.NoError(t, model.DB.First(&updated, token.Id).Error)
	assert.Equal(t, common.TokenStatusDisabled, updated.Status)
	assert.Equal(t, "status-token", updated.Name, "status_only must not touch other fields")
}

func TestAdminUpdateTokenCrossUserFullEdit(t *testing.T) {
	setupAdminTokenControllerTest(t)
	token := seedToken(t, model.DB, 101, "editable-token", "adml6666keyffff6666")

	body := map[string]any{
		"id":                   token.Id,
		"name":                 "admin-renamed",
		"status":               common.TokenStatusEnabled,
		"expired_time":         -1,
		"remain_quota":         500,
		"unlimited_quota":      false,
		"model_limits_enabled": false,
		"model_limits":         "",
		"group":                "vip",
		"cross_group_retry":    false,
	}
	ctx, recorder := newAdminTokenContext(t, http.MethodPut, "/api/token/admin/", body)
	AdminUpdateToken(ctx)

	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	assert.NotContains(t, recorder.Body.String(), token.Key, "admin update must mask key")

	var updated model.Token
	require.NoError(t, model.DB.First(&updated, token.Id).Error)
	assert.Equal(t, "admin-renamed", updated.Name)
	assert.Equal(t, 500, updated.RemainQuota)
	assert.Equal(t, 101, updated.UserId, "admin edit must not change token owner")
}

func TestAdminUpdateTokenAutoGroupsValidatedAgainstOwnerGroup(t *testing.T) {
	configureOwnerRestrictedGroups(t)
	setupAdminTokenControllerTest(t)
	token := seedToken(t, model.DB, 101, "auto-token", "adml7777keygggg7777")
	token.Group = "auto"
	require.NoError(t, model.DB.Save(token).Error)

	request := map[string]any{
		"id":                token.Id,
		"name":              "auto-token",
		"status":            common.TokenStatusEnabled,
		"expired_time":      -1,
		"remain_quota":      0,
		"unlimited_quota":   true,
		"group":             "auto",
		"cross_group_retry": true,
		"auto_groups":       []string{"default"},
	}

	// "default" 对管理员（default 分组）可选，但对属主（vip 分组，已移除 default）不可选，必须拒绝
	rejectedCtx, rejectedRecorder := newAdminTokenContext(t, http.MethodPut, "/api/token/admin/", request)
	AdminUpdateToken(rejectedCtx)
	rejected := decodeAPIResponse(t, rejectedRecorder)
	assert.False(t, rejected.Success, "auto_groups must be validated against the token owner's group")

	// "vip" 对属主可选，应成功
	accepted := map[string]any{}
	for k, v := range request {
		accepted[k] = v
	}
	accepted["auto_groups"] = []string{"vip"}
	acceptedCtx, acceptedRecorder := newAdminTokenContext(t, http.MethodPut, "/api/token/admin/", accepted)
	AdminUpdateToken(acceptedCtx)
	acceptedResponse := decodeAPIResponse(t, acceptedRecorder)
	require.True(t, acceptedResponse.Success, acceptedResponse.Message)

	var updated model.Token
	require.NoError(t, model.DB.First(&updated, token.Id).Error)
	assert.JSONEq(t, `["vip"]`, updated.AutoGroups)
}

func TestAdminGetTokenAutoGroupsUsesTargetUser(t *testing.T) {
	configureOwnerRestrictedGroups(t)
	setupAdminTokenControllerTest(t)

	ctx, recorder := newAdminTokenContext(t, http.MethodGet, "/api/token/admin/auto-groups?user_id=101", nil)
	AdminGetTokenAutoGroups(ctx)

	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	var data struct {
		Groups []string `json:"groups"`
	}
	require.NoError(t, common.Unmarshal(response.Data, &data))
	assert.Equal(t, []string{"vip"}, data.Groups, "auto-groups options must follow the target user's group")
}

func TestAdminGetTokenAutoGroupsRequiresUserId(t *testing.T) {
	setupAdminTokenControllerTest(t)

	ctx, recorder := newAdminTokenContext(t, http.MethodGet, "/api/token/admin/auto-groups", nil)
	AdminGetTokenAutoGroups(ctx)

	assert.False(t, decodeAPIResponse(t, recorder).Success)
}

func TestAdminDeleteTokenCrossUser(t *testing.T) {
	setupAdminTokenControllerTest(t)
	token := seedToken(t, model.DB, 101, "doomed-token", "adml8888keyhhhh8888")

	ctx, recorder := newAdminTokenContext(t, http.MethodDelete, "/api/token/admin/"+strconv.Itoa(token.Id)+"/", nil)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(token.Id)}}
	AdminDeleteToken(ctx)

	require.True(t, decodeAPIResponse(t, recorder).Success)
	var count int64
	require.NoError(t, model.DB.Model(&model.Token{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestAdminDeleteTokenBatchCrossUser(t *testing.T) {
	setupAdminTokenControllerTest(t)
	first := seedToken(t, model.DB, 101, "batch-a", "adml9999keyiiii9999")
	second := seedToken(t, model.DB, 102, "batch-b", "adml1010keyjjjj1010")

	body := map[string]any{"ids": []int{first.Id, second.Id}}
	ctx, recorder := newAdminTokenContext(t, http.MethodPost, "/api/token/admin/batch", body)
	AdminDeleteTokenBatch(ctx)

	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	var count int64
	require.NoError(t, model.DB.Model(&model.Token{}).Count(&count).Error)
	assert.Zero(t, count)
}
