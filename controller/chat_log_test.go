package controller

import (
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
	require.NoError(t, db.AutoMigrate(&model.ChatLog{}))
	model.CHATLOG_DB = db
	common.SetChatLogDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() { model.CHATLOG_DB = nil })
}

func TestAdminGetChatLogs_ListAndDetail(t *testing.T) {
	setupChatLogTestDB(t)
	cl := &model.ChatLog{
		TokenId: 1, UserId: 1, ChannelId: 5, ModelName: "gpt-4",
		RequestId: "req-x", RequestBody: `{"q":1}`, ResponseBody: `{"a":2}`,
	}
	require.NoError(t, cl.Insert())

	authMW := func(role int) gin.HandlerFunc {
		return func(c *gin.Context) { c.Set("role", role); c.Next() }
	}

	// List
	rec := httptest.NewRecorder()
	r := gin.New()
	r.GET("/chat_logs", authMW(common.RoleAdminUser), AdminGetChatLogs)
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/chat_logs?page=1&page_size=10", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	resp := struct {
		Success bool  `json:"success"`
		Total   int64 `json:"total"`
	}{}
	require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.Equal(t, int64(1), resp.Total)

	// Detail
	rec2 := httptest.NewRecorder()
	r2 := gin.New()
	r2.GET("/chat_logs/:id", authMW(common.RoleAdminUser), AdminGetChatLogDetail)
	r2.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/chat_logs/"+strconv.Itoa(cl.Id), nil))
	require.Equal(t, http.StatusOK, rec2.Code)
	detail := struct {
		Success bool           `json:"success"`
		Data    *model.ChatLog `json:"data"`
	}{}
	require.NoError(t, common.Unmarshal(rec2.Body.Bytes(), &detail))
	require.True(t, detail.Success)
	assert.Equal(t, `{"q":1}`, detail.Data.RequestBody)
}
