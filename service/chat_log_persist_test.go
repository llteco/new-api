package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMaybeInstallChatLogCapture_DisabledReturnsNil(t *testing.T) {
	truncate(t)
	if model.ChatLogDBEnabled() {
		t.Skip("CHATLOG_DB configured")
	}
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/", nil)
	assert.Nil(t, MaybeInstallChatLogCapture(c, types.RelayFormatClaude))
}

func TestMaybeInstallChatLogCapture_EnabledWhenDBOn(t *testing.T) {
	truncate(t)
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/", nil)
	if !model.ChatLogDBEnabled() {
		t.Skip("CHATLOG_DB not configured")
	}
	rec := MaybeInstallChatLogCapture(c, types.RelayFormatClaude)
	require.NotNil(t, rec)
	_, ok := c.Writer.(*chatLogCaptureWriter)
	assert.True(t, ok, "c.Writer should be wrapped")
}

func TestChatLogRecorder_PersistChainsTurns(t *testing.T) {
	truncate(t)
	if !model.ChatLogDBEnabled() {
		t.Skip("CHATLOG_DB not configured")
	}
	tokenId := int(time.Now().UnixNano() % 1e6)
	turn1Body := `{"system":"You are helpful.","messages":[{"role":"user","content":"hi"}]}`
	turn2Body := `{"system":"You are helpful.","messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"hello"},{"role":"user","content":"bye"}]}`

	newTurnCtx := func(requestId string) *gin.Context {
		gin.SetMode(gin.TestMode)
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
		c.Set("token_id", tokenId)
		c.Set("id", 3)
		c.Set("channel_id", 11)
		c.Set("original_model", "claude-3-5-sonnet")
		c.Set(common.RequestIdKey, requestId)
		c.Set("is_stream", false)
		return c
	}

	persistTurn := func(requestId, body string) {
		c := newTurnCtx(requestId)
		recorder := MaybeInstallChatLogCapture(c, types.RelayFormatClaude)
		require.NotNil(t, recorder)
		recorder.SetRequestBody([]byte(body))
		_, err := c.Writer.Write([]byte(`{"type":"message"}`))
		require.NoError(t, err)
		recorder.Persist(c)
	}

	persistTurn("req-turn-1", turn1Body)
	require.Eventually(t, func() bool {
		sessions, total, err := model.SearchChatSessions(tokenId, 0, "", 1, 10)
		return err == nil && total == 1 && len(sessions) == 1 &&
			sessions[0].TurnCount == 1 && sessions[0].MessageCount == 2
	}, 2*time.Second, 20*time.Millisecond)

	persistTurn("req-turn-2", turn2Body)
	require.Eventually(t, func() bool {
		sessions, total, err := model.SearchChatSessions(tokenId, 0, "", 1, 10)
		return err == nil && total == 1 && len(sessions) == 1 &&
			sessions[0].TurnCount == 2 && sessions[0].MessageCount == 4
	}, 2*time.Second, 20*time.Millisecond)

	sessions, total, err := model.SearchChatSessions(tokenId, 0, "", 1, 10)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, sessions, 1)
	s := sessions[0]
	assert.Equal(t, 2, s.TurnCount)
	assert.Equal(t, 4, s.MessageCount)
	assert.Equal(t, `"You are helpful."`, s.System)

	turns, err := model.GetChatTurnsBySessionId(s.Id)
	require.NoError(t, err)
	require.Len(t, turns, 2)
	assert.Equal(t, 0, turns[0].TurnIndex)
	assert.JSONEq(t, `[{"role":"user","content":"hi"}]`, turns[0].NewMessages)
	assert.Equal(t, `{"type":"message"}`, turns[0].ResponseBody)
	assert.Equal(t, 1, turns[1].TurnIndex)
	assert.JSONEq(t, `[{"role":"assistant","content":"hello"},{"role":"user","content":"bye"}]`, turns[1].NewMessages)
	assert.Equal(t, "req-turn-2", turns[1].RequestId)
}
