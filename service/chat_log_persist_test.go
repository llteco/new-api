package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMaybeInstallChatLogCapture_DisabledReturnsNil(t *testing.T) {
	truncate(t)
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/", nil)
	assert.Nil(t, MaybeInstallChatLogCapture(c, false))
}

func TestMaybeInstallChatLogCapture_EnabledWhenDBOn(t *testing.T) {
	truncate(t)
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/", nil)
	if !model.ChatLogDBEnabled() {
		t.Skip("CHATLOG_DB not configured")
	}
	rec := MaybeInstallChatLogCapture(c, true)
	require.NotNil(t, rec)
	_, ok := c.Writer.(*chatLogCaptureWriter)
	assert.True(t, ok, "c.Writer should be wrapped")
}

func TestChatLogRecorder_PersistInsertsRow(t *testing.T) {
	truncate(t)
	if !model.ChatLogDBEnabled() {
		t.Skip("CHATLOG_DB not configured")
	}
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/chat/completions", nil)
	c.Set("token_id", 7)
	c.Set("id", 3)
	c.Set("channel_id", 11)
	c.Set("original_model", "gpt-4")
	c.Set(common.RequestIdKey, "req-persist")
	c.Set("is_stream", true)

	recorder := MaybeInstallChatLogCapture(c, true)
	require.NotNil(t, recorder)
	recorder.SetRequestBody([]byte(`{"messages":[]}`))
	_, _ = c.Writer.Write([]byte(`{"choices":[]}`))
	recorder.Persist(c)

	require.Eventually(t, func() bool {
		logs, total, _ := model.SearchChatLogs(7, 0, 0, "", "", 1, 10)
		return total == 1 && len(logs) == 1 && logs[0].ResponseBody == `{"choices":[]}` && logs[0].RequestBody == `{"messages":[]}`
	}, 2*time.Second, 20*time.Millisecond)
}
