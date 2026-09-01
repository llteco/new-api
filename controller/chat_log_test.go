package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
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

func TestAdminGetChatSessions_ListMetaShape(t *testing.T) {
	setupChatLogTestDB(t)
	s := &model.ChatSession{
		TokenId: 1, UserId: 1, ModelName: "gpt-4", System: "secret-system",
		TurnCount: 2, MessageCount: 4, PrefixHash: "h1",
	}
	require.NoError(t, s.Insert())

	rec := serveChatLog(t, "/chat_logs/sessions", AdminGetChatSessions, "/chat_logs/sessions?page=1&page_size=10")
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Success bool             `json:"success"`
		Total   int64            `json:"total"`
		Data    []map[string]any `json:"data"`
	}
	require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Equal(t, int64(1), resp.Total)
	require.Len(t, resp.Data, 1)

	meta := resp.Data[0]
	for _, key := range []string{"id", "token_id", "user_id", "model_name", "turn_count", "message_count", "created_at", "last_active_at"} {
		assert.Contains(t, meta, key)
	}
	for _, key := range []string{"system", "prefix_hash", "new_messages", "response_body"} {
		assert.NotContains(t, meta, key)
	}
	assert.Equal(t, "gpt-4", meta["model_name"])
	assert.Equal(t, float64(2), meta["turn_count"])
}

func TestAdminGetChatSessions_FiltersAndPaging(t *testing.T) {
	setupChatLogTestDB(t)
	for i, m := range []string{"gpt-4", "gpt-4", "claude-3"} {
		s := &model.ChatSession{TokenId: i + 1, UserId: 1, ModelName: m, PrefixHash: fmt.Sprintf("h%d", i)}
		require.NoError(t, s.Insert())
	}

	list := func(query string) (int64, []map[string]any) {
		rec := serveChatLog(t, "/chat_logs/sessions", AdminGetChatSessions, "/chat_logs/sessions"+query)
		require.Equal(t, http.StatusOK, rec.Code)
		var resp struct {
			Success bool             `json:"success"`
			Total   int64            `json:"total"`
			Data    []map[string]any `json:"data"`
		}
		require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &resp))
		require.True(t, resp.Success)
		return resp.Total, resp.Data
	}

	total, data := list("?page=1&page_size=2")
	assert.Equal(t, int64(3), total)
	assert.Len(t, data, 2)

	total, data = list("?page=1&page_size=10&model_name=gpt-4")
	assert.Equal(t, int64(2), total)
	require.Len(t, data, 2)
	assert.Equal(t, "gpt-4", data[0]["model_name"])
	assert.Equal(t, "gpt-4", data[1]["model_name"])

	total, data = list("?page=1&page_size=10&token_id=3")
	assert.Equal(t, int64(1), total)
	require.Len(t, data, 1)
	assert.Equal(t, float64(3), data[0]["token_id"])

	total, data = list("?page=1&page_size=10&model_name=claude-3&token_id=3")
	assert.Equal(t, int64(1), total)
	assert.Len(t, data, 1)

	total, _ = list("?page=1&page_size=10&model_name=none")
	assert.Equal(t, int64(0), total)
}

func TestAdminGetChatSessionDetail_TurnsOrdered(t *testing.T) {
	setupChatLogTestDB(t)
	s := &model.ChatSession{TokenId: 7, UserId: 1, ModelName: "gpt-4", PrefixHash: "h7"}
	require.NoError(t, s.Insert())
	turn2 := &model.ChatTurn{SessionId: s.Id, TurnIndex: 2, RequestId: "r2", ModelName: "gpt-4", NewMessages: `[{"role":"assistant"}]`, ResponseBody: `{"b":2}`}
	require.NoError(t, turn2.Insert())
	turn1 := &model.ChatTurn{SessionId: s.Id, TurnIndex: 1, RequestId: "r1", ModelName: "gpt-4", NewMessages: `[{"role":"user"}]`, ResponseBody: `{"a":1}`}
	require.NoError(t, turn1.Insert())

	rec := serveChatLog(t, "/chat_logs/sessions/:id", AdminGetChatSessionDetail, "/chat_logs/sessions/"+strconv.Itoa(s.Id))
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Session *model.ChatSession `json:"session"`
			Turns   []*model.ChatTurn  `json:"turns"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.NotNil(t, resp.Data.Session)
	assert.Equal(t, s.Id, resp.Data.Session.Id)
	require.Len(t, resp.Data.Turns, 2)
	assert.Equal(t, 1, resp.Data.Turns[0].TurnIndex)
	assert.Equal(t, "r1", resp.Data.Turns[0].RequestId)
	assert.Equal(t, `{"a":1}`, resp.Data.Turns[0].ResponseBody)
	assert.Equal(t, 2, resp.Data.Turns[1].TurnIndex)
	assert.Equal(t, "r2", resp.Data.Turns[1].RequestId)

	// nonexistent id -> 404
	rec404 := serveChatLog(t, "/chat_logs/sessions/:id", AdminGetChatSessionDetail, "/chat_logs/sessions/99999")
	assert.Equal(t, http.StatusNotFound, rec404.Code)
	// invalid id -> 400
	rec400 := serveChatLog(t, "/chat_logs/sessions/:id", AdminGetChatSessionDetail, "/chat_logs/sessions/abc")
	assert.Equal(t, http.StatusBadRequest, rec400.Code)
}
