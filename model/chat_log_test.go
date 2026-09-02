package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatSession_CreateAndQuery(t *testing.T) {
	truncateTables(t)
	if !ChatLogDBEnabled() {
		t.Skip("CHATLOG_DB not configured")
	}
	s := &ChatSession{TokenId: 1, UserId: 2, ModelName: "glm-5.3", PrefixHash: "a", MessageCount: 3, System: `"sys"`}
	require.NoError(t, s.Insert())
	require.NoError(t, (&ChatTurn{SessionId: s.Id, TurnIndex: 0, RequestId: "r1", NewMessages: `[{"role":"user","content":"hi"}]`, ResponseBody: `{}`}).Insert())
	require.NoError(t, (&ChatTurn{SessionId: s.Id, TurnIndex: 1, RequestId: "r2", NewMessages: `[{"role":"user","content":"again"}]`, ResponseBody: `{}`}).Insert())

	got, err := GetChatSessionById(s.Id)
	require.NoError(t, err)
	assert.Equal(t, 3, got.MessageCount)

	turns, err := GetChatTurnsBySessionId(s.Id)
	require.NoError(t, err)
	require.Len(t, turns, 2)
	assert.Equal(t, 0, turns[0].TurnIndex)
	assert.Equal(t, 1, turns[1].TurnIndex)
}

func TestFindChatSessionByPrefixHashes_PrefersLongestMatch(t *testing.T) {
	truncateTables(t)
	if !ChatLogDBEnabled() {
		t.Skip("CHATLOG_DB not configured")
	}
	require.NoError(t, (&ChatSession{TokenId: 1, PrefixHash: "h2", MessageCount: 2}).Insert())
	longer := &ChatSession{TokenId: 1, PrefixHash: "h5", MessageCount: 5}
	require.NoError(t, longer.Insert())
	require.NoError(t, (&ChatSession{TokenId: 2, PrefixHash: "h5", MessageCount: 9}).Insert()) // other token

	got, err := FindChatSessionByPrefixHashes(1, []string{"h2", "h5", "h9"})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 5, got.MessageCount)
}

func TestChatSession_Advance(t *testing.T) {
	truncateTables(t)
	if !ChatLogDBEnabled() {
		t.Skip("CHATLOG_DB not configured")
	}
	s := &ChatSession{TokenId: 1, PrefixHash: "h2", MessageCount: 2}
	require.NoError(t, s.Insert())
	s.MessageCount = 4
	s.PrefixHash = "h4"
	require.NoError(t, s.Advance("glm-5.3", 123))
	got, _ := GetChatSessionById(s.Id)
	assert.Equal(t, 4, got.MessageCount)
	assert.Equal(t, "h4", got.PrefixHash)
	assert.Equal(t, 1, got.TurnCount)
	assert.Equal(t, "glm-5.3", got.ModelName)
	assert.Equal(t, int64(123), got.LastActiveAt)
}

func TestSearchChatSessions(t *testing.T) {
	truncateTables(t)
	if !ChatLogDBEnabled() {
		t.Skip("CHATLOG_DB not configured")
	}
	require.NoError(t, (&ChatSession{TokenId: 1, UserId: 1, ModelName: "gpt-4", PrefixHash: "p1", MessageCount: 1, CreatedAt: 100, LastActiveAt: 100}).Insert())
	require.NoError(t, (&ChatSession{TokenId: 1, UserId: 2, ModelName: "claude", PrefixHash: "p2", MessageCount: 2, CreatedAt: 200, LastActiveAt: 200}).Insert())
	require.NoError(t, (&ChatSession{TokenId: 2, UserId: 1, ModelName: "gpt-4", PrefixHash: "p3", MessageCount: 3, CreatedAt: 300, LastActiveAt: 300}).Insert())

	list, total, err := SearchChatSessions(1, 0, "", 1, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, list, 2)

	list, total, err = SearchChatSessions(0, 1, "", 1, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)

	list, total, err = SearchChatSessions(0, 0, "gpt-4", 1, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)

	list, total, err = SearchChatSessions(0, 0, "", 1, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	require.Len(t, list, 1)
	assert.Equal(t, int64(300), list[0].LastActiveAt, "ordered by last_active_at desc")

	list, total, err = SearchChatSessions(0, 0, "", 2, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Empty(t, list, "page 2 of 3 items is empty")
}
