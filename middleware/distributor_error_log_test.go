package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupDistributorErrorLogDB swaps model.DB/model.LOG_DB for an in-memory
// SQLite database so error logs written by the distributor land in a table the
// test can query, and enables the error log switch for the test's duration.
func setupDistributorErrorLogDB(t *testing.T) {
	t.Helper()
	// Distribute 的 abort 消息经过 i18n.T，未初始化 bundle 会 panic
	require.NoError(t, i18n.Init())
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousEnabled := constant.ErrorLogEnabled
	previousRedis := common.RedisEnabled
	common.RedisEnabled = false
	memoryCacheEnabled := common.MemoryCacheEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	// :memory: 下每个连接是独立库，单连接保证所有读写落在同一份数据上
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.User{}, &model.Channel{}, &model.Ability{}))
	model.DB = db
	model.LOG_DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	constant.ErrorLogEnabled = true
	common.MemoryCacheEnabled = true
	require.NoError(t, db.Exec("DELETE FROM abilities").Error)
	require.NoError(t, db.Exec("DELETE FROM channels").Error)
	require.NoError(t, db.Exec("DELETE FROM logs").Error)
	model.InitChannelCache()
	t.Cleanup(func() {
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		constant.ErrorLogEnabled = previousEnabled
		common.RedisEnabled = previousRedis
		common.MemoryCacheEnabled = memoryCacheEnabled
		model.InitChannelCache()
	})
}

func newDistributeTestContext(t *testing.T, target string, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	return c, recorder
}

func markAuthenticated(c *gin.Context) {
	c.Set("id", 42)
	c.Set("token_id", 7)
	c.Set("token_name", "error-log-token")
	common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
}

func countErrorLogs(t *testing.T) int64 {
	t.Helper()
	var total int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("type = ?", model.LogTypeError).Count(&total).Error)
	return total
}

// An authenticated request with a malformed JSON body must be rejected with
// 400 and leave exactly one error log carrying the caller's identity.
func TestDistributeRecordsErrorLogForAuthorizedInvalidJSON(t *testing.T) {
	setupDistributorErrorLogDB(t)

	c, recorder := newDistributeTestContext(t, "/v1/chat/completions", `{"model": "gpt-4o",`)
	markAuthenticated(c)

	Distribute()(c)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.True(t, c.IsAborted())
	assert.EqualValues(t, 1, countErrorLogs(t))
	var log model.Log
	require.NoError(t, model.LOG_DB.Where("type = ?", model.LogTypeError).First(&log).Error)
	assert.Equal(t, 42, log.UserId)
	assert.Equal(t, 7, log.TokenId)
	assert.Equal(t, "error-log-token", log.TokenName)
	assert.Equal(t, "default", log.Group)
	assert.Contains(t, log.Other, `"error_code":"invalid_request"`)
	assert.Contains(t, log.Other, `"request_path":"/v1/chat/completions"`)
}

// Requests without an authenticated user id must never be logged, even if they
// somehow reach the distributor (defense in depth for the "unauthorized access
// is not recorded" contract).
func TestDistributeSkipsErrorLogForUnauthenticatedCaller(t *testing.T) {
	setupDistributorErrorLogDB(t)

	c, recorder := newDistributeTestContext(t, "/v1/chat/completions", `{"model": "gpt-4o",`)

	Distribute()(c)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.True(t, c.IsAborted())
	assert.EqualValues(t, 0, countErrorLogs(t))
}

// ERROR_LOG_ENABLED=false must keep the distributor silent in the DB.
func TestDistributeSkipsErrorLogWhenDisabled(t *testing.T) {
	setupDistributorErrorLogDB(t)
	constant.ErrorLogEnabled = false

	c, recorder := newDistributeTestContext(t, "/v1/chat/completions", `{"model": "gpt-4o",`)
	markAuthenticated(c)

	Distribute()(c)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.EqualValues(t, 0, countErrorLogs(t))
}

// A well-formed request for a model without any available channel is an
// authenticated failure and must be recorded with the model_not_found code.
func TestDistributeRecordsErrorLogWhenNoChannelAvailable(t *testing.T) {
	setupDistributorErrorLogDB(t)

	c, recorder := newDistributeTestContext(t, "/v1/chat/completions", `{"model": "no-such-model"}`)
	markAuthenticated(c)

	Distribute()(c)

	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.True(t, c.IsAborted())
	assert.EqualValues(t, 1, countErrorLogs(t))
	var log model.Log
	require.NoError(t, model.LOG_DB.Where("type = ?", model.LogTypeError).First(&log).Error)
	assert.Equal(t, 42, log.UserId)
	assert.Equal(t, "no-such-model", log.ModelName)
	assert.Contains(t, log.Other, `"error_code":"model_not_found"`)
}

// abortDistributeWithErrorLog must mirror abortWithOpenAiMessage's response
// payload (message + code) — the log helper must not change the client reply.
func TestAbortDistributeWithErrorLogKeepsOpenAIErrorShape(t *testing.T) {
	setupDistributorErrorLogDB(t)

	c, recorder := newDistributeTestContext(t, "/v1/chat/completions", "{}")
	markAuthenticated(c)

	abortDistributeWithErrorLog(c, http.StatusBadRequest, "bad input", "gpt-4o", 3, types.ErrorCodeInvalidRequest)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	body := recorder.Body.String()
	assert.Contains(t, body, `"type":"new_api_error"`)
	assert.Contains(t, body, `"code":"invalid_request"`)
	assert.Contains(t, body, "bad input")
}
