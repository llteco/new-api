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
			MultiKeyCooldownUntil: map[int]int64{
				1: common.GetTimestamp() + 3600,
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

func TestUpdateChannelMultiKeyToSingleAcceptsExpiredCooldownKey(t *testing.T) {
	setupModelListControllerTestDB(t)

	channel := &model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-a\nsk-b",
		Name:   "multi-to-single-expired-cooldown",
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
			MultiKeyCooldownUntil: map[int]int64{
				1: common.GetTimestamp() - 10,
			},
			MultiKeyMode: constant.MultiKeyModeRandom,
		},
	}
	insertChannelForUpdateTest(t, channel)

	result := performUpdateChannelRequest(t, common.RoleRootUser, fmt.Sprintf(
		`{"id":%d,"is_multi_key":false}`,
		channel.Id,
	))

	require.True(t, result.success, result.message)
	stored := reloadChannelForUpdateTest(t, channel.Id)
	assert.Equal(t, "sk-b", stored.Key)
	assert.False(t, stored.ChannelInfo.IsMultiKey)
	assert.Zero(t, stored.ChannelInfo.MultiKeySize)
	assert.Nil(t, stored.ChannelInfo.MultiKeyStatusList)
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

// prettyVertexCredential 是美化格式的服务账号凭据：含换行，但只是一条逻辑凭据。
const prettyVertexCredential = `{
  "type": "service_account",
  "project_id": "proj-a",
  "private_key": "-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----\n"
}`

func insertVertexChannelForUpdateTest(t *testing.T, name, key string, info model.ChannelInfo) *model.Channel {
	t.Helper()

	channel := &model.Channel{
		Type:          constant.ChannelTypeVertexAi,
		Key:           key,
		Name:          name,
		Models:        "gemini-2.0-flash",
		Group:         "default",
		Status:        common.ChannelStatusEnabled,
		Other:         `{"default":"us-central1"}`,
		OtherSettings: `{"vertex_key_type":"json"}`,
		ChannelInfo:   info,
	}
	insertChannelForUpdateTest(t, channel)
	return channel
}

func assertVertexCredentialProject(t *testing.T, key, project string) {
	t.Helper()

	var credential map[string]any
	require.NoError(t, common.Unmarshal([]byte(key), &credential))
	assert.Equal(t, project, credential["project_id"])
}

func TestUpdateChannelConvertsVertexSingleKeyToMultiKey(t *testing.T) {
	setupModelListControllerTestDB(t)

	t.Run("compacts pretty-printed credential when no key is provided", func(t *testing.T) {
		channel := insertVertexChannelForUpdateTest(t, "vertex-single-to-multi", prettyVertexCredential, model.ChannelInfo{})

		result := performUpdateChannelRequest(t, common.RoleRootUser, fmt.Sprintf(
			`{"id":%d,"is_multi_key":true,"other":"{\"default\":\"us-central1\"}"}`,
			channel.Id,
		))

		require.True(t, result.success, result.message)
		stored := reloadChannelForUpdateTest(t, channel.Id)
		assert.True(t, stored.ChannelInfo.IsMultiKey)
		assert.Equal(t, 1, stored.ChannelInfo.MultiKeySize)
		keys := stored.GetKeys()
		require.Len(t, keys, 1)
		assert.NotContains(t, keys[0], "\n")
		assertVertexCredentialProject(t, keys[0], "proj-a")
	})

	t.Run("appends new credential without splitting the pretty-printed one", func(t *testing.T) {
		channel := insertVertexChannelForUpdateTest(t, "vertex-single-to-multi-append", prettyVertexCredential, model.ChannelInfo{})

		newCredential := `{"type":"service_account","project_id":"proj-b"}`
		keyJSON, err := common.Marshal(newCredential)
		require.NoError(t, err)
		result := performUpdateChannelRequest(t, common.RoleRootUser, fmt.Sprintf(
			`{"id":%d,"is_multi_key":true,"key_mode":"append","other":"{\"default\":\"us-central1\"}","key":%s}`,
			channel.Id, keyJSON,
		))

		require.True(t, result.success, result.message)
		stored := reloadChannelForUpdateTest(t, channel.Id)
		assert.True(t, stored.ChannelInfo.IsMultiKey)
		assert.Equal(t, 2, stored.ChannelInfo.MultiKeySize)
		keys := stored.GetKeys()
		require.Len(t, keys, 2)
		assertVertexCredentialProject(t, keys[0], "proj-a")
		assertVertexCredentialProject(t, keys[1], "proj-b")
	})

	t.Run("compacts pretty-printed string elements in a credential array", func(t *testing.T) {
		channel := insertVertexChannelForUpdateTest(t, "vertex-array-string-element", `{"type":"service_account","project_id":"proj-old"}`, model.ChannelInfo{})

		// 数组元素是"包含美化 JSON 的字符串"，同样不能把换行带进多 key 存储
		arrayKey, err := common.Marshal([]string{prettyVertexCredential})
		require.NoError(t, err)
		keyJSON, err := common.Marshal(string(arrayKey))
		require.NoError(t, err)
		result := performUpdateChannelRequest(t, common.RoleRootUser, fmt.Sprintf(
			`{"id":%d,"is_multi_key":true,"other":"{\"default\":\"us-central1\"}","key":%s}`,
			channel.Id, keyJSON,
		))

		require.True(t, result.success, result.message)
		stored := reloadChannelForUpdateTest(t, channel.Id)
		assert.True(t, stored.ChannelInfo.IsMultiKey)
		assert.Equal(t, 1, stored.ChannelInfo.MultiKeySize)
		keys := stored.GetKeys()
		require.Len(t, keys, 1)
		assert.NotContains(t, keys[0], "\n")
		assertVertexCredentialProject(t, keys[0], "proj-a")
	})
}

func TestUpdateChannelVertexMultiToSingleRequiresOneCredential(t *testing.T) {
	setupModelListControllerTestDB(t)

	multiKeyInfo := model.ChannelInfo{
		IsMultiKey:   true,
		MultiKeySize: 2,
		MultiKeyMode: constant.MultiKeyModeRandom,
	}
	storedKeys := `{"type":"service_account","project_id":"proj-a"}` + "\n" + `{"type":"service_account","project_id":"proj-b"}`

	t.Run("rejects compact array of multiple credentials", func(t *testing.T) {
		channel := insertVertexChannelForUpdateTest(t, "vertex-multi-to-single-array", storedKeys, multiKeyInfo)

		arrayKey := `[{"type":"service_account","project_id":"proj-a"},{"type":"service_account","project_id":"proj-b"}]`
		keyJSON, err := common.Marshal(arrayKey)
		require.NoError(t, err)
		result := performUpdateChannelRequest(t, common.RoleRootUser, fmt.Sprintf(
			`{"id":%d,"is_multi_key":false,"other":"{\"default\":\"us-central1\"}","key":%s}`,
			channel.Id, keyJSON,
		))

		require.False(t, result.success)
		assert.Contains(t, result.message, "只能保留一个密钥")
		stored := reloadChannelForUpdateTest(t, channel.Id)
		assert.True(t, stored.ChannelInfo.IsMultiKey)
	})

	t.Run("accepts one pretty-printed credential", func(t *testing.T) {
		channel := insertVertexChannelForUpdateTest(t, "vertex-multi-to-single-pretty", storedKeys, multiKeyInfo)

		keyJSON, err := common.Marshal(prettyVertexCredential)
		require.NoError(t, err)
		result := performUpdateChannelRequest(t, common.RoleRootUser, fmt.Sprintf(
			`{"id":%d,"is_multi_key":false,"other":"{\"default\":\"us-central1\"}","key":%s}`,
			channel.Id, keyJSON,
		))

		require.True(t, result.success, result.message)
		stored := reloadChannelForUpdateTest(t, channel.Id)
		assert.False(t, stored.ChannelInfo.IsMultiKey)
		assert.NotContains(t, stored.Key, "\n")
		assertVertexCredentialProject(t, stored.Key, "proj-a")
	})
}

func TestUpdateChannelCodexKeyModeConversion(t *testing.T) {
	setupModelListControllerTestDB(t)

	t.Run("rejects converting a Codex channel to multi-key", func(t *testing.T) {
		channel := &model.Channel{
			Type:   constant.ChannelTypeCodex,
			Key:    `{"access_token":"tok","account_id":"acc"}`,
			Name:   "codex-to-multi",
			Models: "gpt-5-codex",
			Group:  "default",
			Status: common.ChannelStatusEnabled,
		}
		insertChannelForUpdateTest(t, channel)

		result := performUpdateChannelRequest(t, common.RoleRootUser, fmt.Sprintf(
			`{"id":%d,"is_multi_key":true}`,
			channel.Id,
		))

		require.False(t, result.success)
		assert.Contains(t, result.message, "Codex")
		stored := reloadChannelForUpdateTest(t, channel.Id)
		assert.False(t, stored.ChannelInfo.IsMultiKey)
		assert.Equal(t, `{"access_token":"tok","account_id":"acc"}`, stored.Key)
	})

	t.Run("allows converting an existing multi-key Codex channel back to single", func(t *testing.T) {
		channel := &model.Channel{
			Type:   constant.ChannelTypeCodex,
			Key:    "{\"access_token\":\"tok-a\",\"account_id\":\"acc\"}\n{\"access_token\":\"tok-b\",\"account_id\":\"acc\"}",
			Name:   "codex-to-single",
			Models: "gpt-5-codex",
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
			`{"id":%d,"is_multi_key":false}`,
			channel.Id,
		))

		require.True(t, result.success, result.message)
		stored := reloadChannelForUpdateTest(t, channel.Id)
		assert.False(t, stored.ChannelInfo.IsMultiKey)
		assert.Equal(t, `{"access_token":"tok-a","account_id":"acc"}`, stored.Key)
	})

	t.Run("accepts a pretty-printed OAuth credential when converting to single key", func(t *testing.T) {
		channel := &model.Channel{
			Type:   constant.ChannelTypeCodex,
			Key:    "{\"access_token\":\"tok-a\",\"account_id\":\"acc\"}\n{\"access_token\":\"tok-b\",\"account_id\":\"acc\"}",
			Name:   "codex-pretty-to-single",
			Models: "gpt-5-codex",
			Group:  "default",
			Status: common.ChannelStatusEnabled,
			ChannelInfo: model.ChannelInfo{
				IsMultiKey:   true,
				MultiKeySize: 2,
				MultiKeyMode: constant.MultiKeyModeRandom,
			},
		}
		insertChannelForUpdateTest(t, channel)

		// Codex 的 OAuth 凭据是单个 JSON 对象，美化格式（含换行）不是多 key
		prettyCredential := "{\n  \"access_token\": \"tok-new\",\n  \"account_id\": \"acc\"\n}"
		keyJSON, err := common.Marshal(prettyCredential)
		require.NoError(t, err)
		result := performUpdateChannelRequest(t, common.RoleRootUser, fmt.Sprintf(
			`{"id":%d,"is_multi_key":false,"key":%s}`,
			channel.Id, keyJSON,
		))

		require.True(t, result.success, result.message)
		stored := reloadChannelForUpdateTest(t, channel.Id)
		assert.False(t, stored.ChannelInfo.IsMultiKey)
		assert.Equal(t, `{"access_token":"tok-new","account_id":"acc"}`, stored.Key)
	})
}

func TestAddChannelRejectsCodexMultiKeyMode(t *testing.T) {
	setupModelListControllerTestDB(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/", bytes.NewBufferString(fmt.Sprintf(
		`{"mode":"multi_to_single","multi_key_mode":"random","channel":{"type":%d,"name":"codex-multi","key":"{\"access_token\":\"tok\",\"account_id\":\"acc\"}","models":"gpt-5-codex","group":"default"}}`,
		constant.ChannelTypeCodex,
	)))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("role", common.RoleRootUser)
	ctx.Set("id", 1)

	AddChannel(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Contains(t, response.Message, "Codex")
}
