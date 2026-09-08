package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupChatLogTestDB(t *testing.T) {
	t.Helper()
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.ChatSession{}, &model.ChatTurn{}))
	model.CHATLOG_DB = db
	common.SetChatLogDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() { model.CHATLOG_DB = nil })
}

func serveChatLog(t *testing.T, route string, handler gin.HandlerFunc, url string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	r := gin.New()
	r.GET(route, handler)
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
	return rec
}

type chatSessionListResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Items      []map[string]any `json:"items"`
		HasMore    bool             `json:"has_more"`
		NextCursor any              `json:"next_cursor"`
	} `json:"data"`
}

func listChatSessionsFor(t *testing.T, query string) chatSessionListResponse {
	t.Helper()
	rec := serveChatLog(t, "/chat_logs/sessions", AdminGetChatSessions, "/chat_logs/sessions"+query)
	require.Equal(t, http.StatusOK, rec.Code)
	var resp chatSessionListResponse
	require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	return resp
}

func TestAdminGetChatSessions_ListMetaShape(t *testing.T) {
	setupChatLogTestDB(t)
	s := &model.ChatSession{
		TokenId: 1, UserId: 1, ModelName: "gpt-4", System: "secret-system",
		TurnCount: 2, MessageCount: 4, PrefixHash: "h1",
	}
	require.NoError(t, s.Insert())

	resp := listChatSessionsFor(t, "?limit=10")
	require.Len(t, resp.Data.Items, 1)
	assert.False(t, resp.Data.HasMore)
	assert.Nil(t, resp.Data.NextCursor)

	meta := resp.Data.Items[0]
	for _, key := range []string{"id", "token_id", "user_id", "model_name", "turn_count", "message_count", "created_at", "last_active_at"} {
		assert.Contains(t, meta, key)
	}
	for _, key := range []string{"system", "prefix_hash", "new_messages", "response_body"} {
		assert.NotContains(t, meta, key)
	}
	assert.Equal(t, "gpt-4", meta["model_name"])
	assert.Equal(t, float64(2), meta["turn_count"])
}

func TestNormalizeChatLogPageLimit(t *testing.T) {
	assert.Equal(t, 20, normalizeChatLogPageLimit(""))
	assert.Equal(t, 20, normalizeChatLogPageLimit("0"))
	assert.Equal(t, 20, normalizeChatLogPageLimit("-3"))
	assert.Equal(t, 20, normalizeChatLogPageLimit("abc"))
	assert.Equal(t, 1, normalizeChatLogPageLimit("1"))
	assert.Equal(t, 50, normalizeChatLogPageLimit("50"))
	assert.Equal(t, 100, normalizeChatLogPageLimit("100"))
	assert.Equal(t, 100, normalizeChatLogPageLimit("5000"))
}

// TestAdminGetChatSessions_DefaultLimitOnHotPath pins the fix for the missing
// limit param: the hot path must see the same default (20) as the DB path
// instead of clamping to a single item.
func TestAdminGetChatSessions_DefaultLimitOnHotPath(t *testing.T) {
	setupChatLogTestDB(t)
	t.Setenv("CHAT_LOG_HOT_CACHE_ENABLED", "true")
	model.InitChatLogHotCache()
	t.Cleanup(func() {
		os.Setenv("CHAT_LOG_HOT_CACHE_ENABLED", "false")
		model.InitChatLogHotCache()
	})

	for i := 0; i < 3; i++ {
		s := &model.ChatSession{TokenId: i + 1, ModelName: "gpt-4", PrefixHash: fmt.Sprintf("h%d", i)}
		require.NoError(t, s.Insert())
	}

	// no limit param at all: default page of 20, all three sessions returned
	resp := listChatSessionsFor(t, "")
	require.Len(t, resp.Data.Items, 3)
}

func TestAdminGetChatSessions_FiltersAndCursorPaging(t *testing.T) {
	setupChatLogTestDB(t)
	now := common.GetTimestamp()
	past := now - 3600
	for i, m := range []string{"gpt-4", "gpt-4", "claude-3"} {
		s := &model.ChatSession{TokenId: i + 1, UserId: 1, ModelName: m, PrefixHash: fmt.Sprintf("h%d", i), LastActiveAt: past}
		require.NoError(t, s.Insert())
	}

	// first page with cursor pagination
	resp := listChatSessionsFor(t, "?limit=2")
	assert.Len(t, resp.Data.Items, 2)
	assert.True(t, resp.Data.HasMore)
	cursor, ok := resp.Data.NextCursor.(string)
	require.True(t, ok)
	require.NotEmpty(t, cursor)

	// walk to the older page
	resp = listChatSessionsFor(t, "?limit=2&cursor="+cursor)
	assert.Len(t, resp.Data.Items, 1)
	assert.False(t, resp.Data.HasMore)
	assert.Nil(t, resp.Data.NextCursor)

	// filters
	resp = listChatSessionsFor(t, "?limit=10&model_name=gpt-4")
	require.Len(t, resp.Data.Items, 2)
	assert.Equal(t, "gpt-4", resp.Data.Items[0]["model_name"])
	assert.Equal(t, "gpt-4", resp.Data.Items[1]["model_name"])

	resp = listChatSessionsFor(t, "?limit=10&token_id=3")
	require.Len(t, resp.Data.Items, 1)
	assert.Equal(t, float64(3), resp.Data.Items[0]["token_id"])

	resp = listChatSessionsFor(t, "?limit=10&model_name=claude-3&token_id=3")
	assert.Len(t, resp.Data.Items, 1)

	resp = listChatSessionsFor(t, "?limit=10&model_name=none")
	assert.Empty(t, resp.Data.Items)
	assert.False(t, resp.Data.HasMore)

	// time range filter (inclusive on last_active_at)
	newer := &model.ChatSession{TokenId: 9, ModelName: "gpt-4", PrefixHash: "h9", LastActiveAt: now}
	require.NoError(t, newer.Insert())
	resp = listChatSessionsFor(t, fmt.Sprintf("?limit=10&start_ts=%d", now))
	require.Len(t, resp.Data.Items, 1)
	assert.Equal(t, float64(9), resp.Data.Items[0]["token_id"])
	resp = listChatSessionsFor(t, fmt.Sprintf("?limit=10&end_ts=%d", now-1))
	assert.Len(t, resp.Data.Items, 3)

	// invalid cursor is rejected ("Zm9vYmFy" decodes to "foobar", wrong shape)
	rec := serveChatLog(t, "/chat_logs/sessions", AdminGetChatSessions, "/chat_logs/sessions?cursor=Zm9vYmFy")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAdminGetChatSessionDetail_TurnsPagedAndOrdered(t *testing.T) {
	setupChatLogTestDB(t)
	s := &model.ChatSession{TokenId: 7, UserId: 1, ModelName: "gpt-4", PrefixHash: "h7"}
	require.NoError(t, s.Insert())
	turn1 := &model.ChatTurn{SessionId: s.Id, TurnIndex: 1, RequestId: "r1", ModelName: "gpt-4", NewMessages: `[{"role":"user"}]`, ResponseBody: `{"a":1}`}
	require.NoError(t, turn1.Insert())
	turn2 := &model.ChatTurn{SessionId: s.Id, TurnIndex: 2, RequestId: "r2", ModelName: "gpt-4", NewMessages: `[{"role":"assistant"}]`, ResponseBody: `{"b":2}`}
	require.NoError(t, turn2.Insert())

	fetch := func(query string) (session *model.ChatSession, turns []*model.ChatTurn, hasMore bool, nextTurnId any) {
		rec := serveChatLog(t, "/chat_logs/sessions/:id", AdminGetChatSessionDetail, "/chat_logs/sessions/"+strconv.Itoa(s.Id)+query)
		require.Equal(t, http.StatusOK, rec.Code)
		var resp struct {
			Success bool `json:"success"`
			Data    struct {
				Session    *model.ChatSession `json:"session"`
				Turns      []*model.ChatTurn  `json:"turns"`
				HasMore    bool               `json:"has_more"`
				NextTurnId any                `json:"next_turn_id"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &resp))
		require.True(t, resp.Success)
		return resp.Data.Session, resp.Data.Turns, resp.Data.HasMore, resp.Data.NextTurnId
	}

	// default view: all turns ascending by insertion id
	session, turns, hasMore, nextTurnId := fetch("")
	require.NotNil(t, session)
	assert.Equal(t, s.Id, session.Id)
	require.Len(t, turns, 2)
	assert.False(t, hasMore)
	assert.Nil(t, nextTurnId)
	assert.Equal(t, "r1", turns[0].RequestId)
	assert.Equal(t, `{"a":1}`, turns[0].ResponseBody)
	assert.Equal(t, "r2", turns[1].RequestId)

	// newest page only; next_turn_id points at the oldest turn of the page
	_, turns, hasMore, nextTurnId = fetch("?limit=1")
	require.Len(t, turns, 1)
	assert.Equal(t, "r2", turns[0].RequestId)
	assert.True(t, hasMore)
	require.Equal(t, float64(turn2.Id), nextTurnId)

	// older page via before_id
	_, turns, hasMore, nextTurnId = fetch("?limit=1&before_id=" + strconv.Itoa(turn2.Id))
	require.Len(t, turns, 1)
	assert.Equal(t, "r1", turns[0].RequestId)
	assert.False(t, hasMore)
	assert.Nil(t, nextTurnId)

	// nonexistent id -> 404
	rec404 := serveChatLog(t, "/chat_logs/sessions/:id", AdminGetChatSessionDetail, "/chat_logs/sessions/99999")
	assert.Equal(t, http.StatusNotFound, rec404.Code)
	// invalid id -> 400
	rec400 := serveChatLog(t, "/chat_logs/sessions/:id", AdminGetChatSessionDetail, "/chat_logs/sessions/abc")
	assert.Equal(t, http.StatusBadRequest, rec400.Code)
}
