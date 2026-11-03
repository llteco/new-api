package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type updateChannelTestResult struct {
	success bool
	message string
}

func performUpdateChannelRequest(t *testing.T, role int, body string) updateChannelTestResult {
	t.Helper()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/channel/", bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("role", role)

	UpdateChannel(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return updateChannelTestResult{success: response.Success, message: response.Message}
}

func insertChannelForUpdateTest(t *testing.T, channel *model.Channel) {
	t.Helper()

	require.NoError(t, model.DB.Create(channel).Error)
}

func reloadChannelForUpdateTest(t *testing.T, id int) *model.Channel {
	t.Helper()

	channel, err := model.GetChannelById(id, true)
	require.NoError(t, err)
	return channel
}

func TestUpdateChannelConvertsSingleKeyToMultiKey(t *testing.T) {
	setupModelListControllerTestDB(t)

	t.Run("appends new keys and keeps existing key", func(t *testing.T) {
		channel := &model.Channel{
			Type:   constant.ChannelTypeOpenAI,
			Key:    "sk-old",
			Name:   "single-to-multi-append",
			Models: "gpt-4o",
			Group:  "default",
			Status: common.ChannelStatusEnabled,
		}
		insertChannelForUpdateTest(t, channel)

		result := performUpdateChannelRequest(t, common.RoleRootUser, fmt.Sprintf(
			`{"id":%d,"is_multi_key":true,"multi_key_mode":"polling","key":"sk-new-1\nsk-new-2","key_mode":"append"}`,
			channel.Id,
		))

		require.True(t, result.success, result.message)
		stored := reloadChannelForUpdateTest(t, channel.Id)
		assert.Equal(t, "sk-old\nsk-new-1\nsk-new-2", stored.Key)
		assert.True(t, stored.ChannelInfo.IsMultiKey)
		assert.Equal(t, 3, stored.ChannelInfo.MultiKeySize)
		assert.Equal(t, constant.MultiKeyModePolling, stored.ChannelInfo.MultiKeyMode)
	})

	t.Run("keeps the existing key when no key is provided", func(t *testing.T) {
		channel := &model.Channel{
			Type:   constant.ChannelTypeOpenAI,
			Key:    "sk-only",
			Name:   "single-to-multi-default",
			Models: "gpt-4o",
			Group:  "default",
			Status: common.ChannelStatusEnabled,
		}
		insertChannelForUpdateTest(t, channel)

		result := performUpdateChannelRequest(t, common.RoleRootUser, fmt.Sprintf(
			`{"id":%d,"is_multi_key":true}`,
			channel.Id,
		))

		require.True(t, result.success, result.message)
		stored := reloadChannelForUpdateTest(t, channel.Id)
		assert.Equal(t, "sk-only", stored.Key)
		assert.True(t, stored.ChannelInfo.IsMultiKey)
		assert.Equal(t, 1, stored.ChannelInfo.MultiKeySize)
		assert.Equal(t, constant.MultiKeyModeRandom, stored.ChannelInfo.MultiKeyMode)
	})
}

func TestUpdateChannelConvertsMultiKeyToSingleKey(t *testing.T) {
	setupModelListControllerTestDB(t)

	channel := &model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-disabled\nsk-enabled",
		Name:   "multi-to-single",
		Models: "gpt-4o",
		Group:  "default",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:         true,
			MultiKeySize:       2,
			MultiKeyStatusList: map[int]int{0: common.ChannelStatusManuallyDisabled},
			MultiKeyMode:       constant.MultiKeyModeRandom,
		},
	}
	insertChannelForUpdateTest(t, channel)

	result := performUpdateChannelRequest(t, common.RoleRootUser, fmt.Sprintf(
		`{"id":%d,"is_multi_key":false}`,
		channel.Id,
	))

	require.True(t, result.success, result.message)
	stored := reloadChannelForUpdateTest(t, channel.Id)
	assert.Equal(t, "sk-enabled", stored.Key)
	assert.False(t, stored.ChannelInfo.IsMultiKey)
	assert.Zero(t, stored.ChannelInfo.MultiKeySize)
	assert.Nil(t, stored.ChannelInfo.MultiKeyStatusList)
}

func TestUpdateChannelMultiKeyToSingleRejectsMultipleKeys(t *testing.T) {
	setupModelListControllerTestDB(t)

	channel := &model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-a\nsk-b",
		Name:   "multi-to-single-reject",
		Models: "gpt-4o",
		Group:  "default",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
			MultiKeyMode: constant.MultiKeyModeRandom,
		},
	}
	insertChannelForUpdateTest(t, channel)

	result := performUpdateChannelRequest(t, common.RoleRootUser, fmt.Sprintf(
		`{"id":%d,"is_multi_key":false,"key":"sk-new-1\nsk-new-2"}`,
		channel.Id,
	))

	require.False(t, result.success)
	assert.Contains(t, result.message, "只能保留一个密钥")
	stored := reloadChannelForUpdateTest(t, channel.Id)
	assert.True(t, stored.ChannelInfo.IsMultiKey)
}

func TestUpdateChannelMultiKeyToSingleRequiresAvailableKey(t *testing.T) {
	setupModelListControllerTestDB(t)

	channel := &model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-a\nsk-b",
		Name:   "multi-to-single-no-enabled-key",
		Models: "gpt-4o",
		Group:  "default",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
			MultiKeyStatusList: map[int]int{
				0: common.ChannelStatusManuallyDisabled,
				1: common.ChannelStatusTempDisabled,
			},
			MultiKeyMode: constant.MultiKeyModeRandom,
		},
	}
	insertChannelForUpdateTest(t, channel)

	result := performUpdateChannelRequest(t, common.RoleRootUser, fmt.Sprintf(
		`{"id":%d,"is_multi_key":false}`,
		channel.Id,
	))

	require.False(t, result.success)
	assert.Contains(t, result.message, "没有可用的密钥")
	stored := reloadChannelForUpdateTest(t, channel.Id)
	assert.True(t, stored.ChannelInfo.IsMultiKey)
}

func TestUpdateChannelRejectsUnknownMultiKeyMode(t *testing.T) {
	setupModelListControllerTestDB(t)

	channel := &model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-old",
		Name:   "invalid-multi-key-mode",
		Models: "gpt-4o",
		Group:  "default",
		Status: common.ChannelStatusEnabled,
	}
	insertChannelForUpdateTest(t, channel)

	result := performUpdateChannelRequest(t, common.RoleRootUser, fmt.Sprintf(
		`{"id":%d,"is_multi_key":true,"multi_key_mode":"weighted"}`,
		channel.Id,
	))

	require.False(t, result.success)
	assert.Contains(t, result.message, "不支持的多密钥策略")
	stored := reloadChannelForUpdateTest(t, channel.Id)
	assert.False(t, stored.ChannelInfo.IsMultiKey)
}

func TestUpdateChannelMultiKeyConversionRequiresSensitiveWrite(t *testing.T) {
	setupModelListControllerTestDB(t)

	channel := &model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-old",
		Name:   "conversion-permission",
		Models: "gpt-4o",
		Group:  "default",
		Status: common.ChannelStatusEnabled,
	}
	insertChannelForUpdateTest(t, channel)

	result := performUpdateChannelRequest(t, 0, fmt.Sprintf(
		`{"id":%d,"is_multi_key":true}`,
		channel.Id,
	))

	require.False(t, result.success)
	stored := reloadChannelForUpdateTest(t, channel.Id)
	assert.False(t, stored.ChannelInfo.IsMultiKey)
}
