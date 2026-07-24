package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatLog_CreateAndQuery(t *testing.T) {
	truncateTables(t)
	if !ChatLogDBEnabled() {
		t.Skip("CHATLOG_DB not configured")
	}
	cl := &ChatLog{
		TokenId: 1, UserId: 1, ChannelId: 5,
		ModelName: "gpt-4", RequestId: "req-1",
		RequestBody: `{"messages":[]}`, ResponseBody: `{"choices":[]}`,
		IsStream: true, StatusCode: 200, UseTime: 2,
	}
	require.NoError(t, cl.Insert())

	got, err := GetChatLogById(cl.Id)
	require.NoError(t, err)
	assert.Equal(t, "gpt-4", got.ModelName)
	assert.Equal(t, `{"messages":[]}`, got.RequestBody)

	list, total, err := SearchChatLogs(1, 0, 0, "", "", 1, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, list, 1)
	assert.Equal(t, cl.Id, list[0].Id)
}

func TestChatLog_SearchFilters(t *testing.T) {
	truncateTables(t)
	if !ChatLogDBEnabled() {
		t.Skip("CHATLOG_DB not configured")
	}
	require.NoError(t, (&ChatLog{TokenId: 1, UserId: 1, ChannelId: 5, ModelName: "gpt-4", RequestId: "a", RequestBody: "{}", ResponseBody: "{}"}).Insert())
	require.NoError(t, (&ChatLog{TokenId: 1, UserId: 2, ChannelId: 6, ModelName: "claude", RequestId: "b", RequestBody: "{}", ResponseBody: "{}"}).Insert())
	require.NoError(t, (&ChatLog{TokenId: 2, UserId: 1, ChannelId: 5, ModelName: "gpt-4", RequestId: "c", RequestBody: "{}", ResponseBody: "{}"}).Insert())

	list, total, err := SearchChatLogs(1, 0, 0, "", "", 1, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, list, 2)

	list, total, err = SearchChatLogs(0, 0, 0, "gpt-4", "", 1, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)

	list, total, err = SearchChatLogs(0, 0, 0, "", "b", 1, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "claude", list[0].ModelName)

	list, total, err = SearchChatLogs(0, 0, 0, "", "", 2, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Empty(t, list, "page 2 of 3 items is empty")
}
