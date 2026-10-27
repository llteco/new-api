package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupRelayErrorLogTest(t *testing.T) *gin.Context {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousEnabled := constant.ErrorLogEnabled
	previousRedis := common.RedisEnabled
	common.RedisEnabled = false
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	// :memory: 下每个连接是独立库，单连接保证所有读写落在同一份数据上
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.User{}))
	model.DB = db
	model.LOG_DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	constant.ErrorLogEnabled = true
	t.Cleanup(func() {
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		constant.ErrorLogEnabled = previousEnabled
		common.RedisEnabled = previousRedis
	})

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set("id", 42)
	c.Set("token_id", 7)
	c.Set("token_name", "error-log-token")
	c.Set("group", "default")
	c.Set("original_model", "gpt-4o")
	return c
}

func errorLogCount(t *testing.T) int64 {
	t.Helper()
	var total int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("type = ?", model.LogTypeError).Count(&total).Error)
	return total
}

// A failure that processChannelError already recorded must not be recorded a
// second time by the request-level fallback in Relay's error defer.
func TestRecordRelayErrorLogSkipsWhenChannelErrorAlreadyRecorded(t *testing.T) {
	c := setupRelayErrorLogTest(t)
	channelErr := types.NewError(errors.New("upstream boom"), types.ErrorCodeBadResponseStatusCode)

	recordChannelErrorLog(c, channelErr)
	assert.EqualValues(t, 1, errorLogCount(t))

	recordRelayErrorLog(c, channelErr)
	assert.EqualValues(t, 1, errorLogCount(t))
}

// Request-stage failures that never reach processChannelError (invalid body,
// pricing errors, no usable channel) must be recorded exactly once by the
// request-level fallback.
func TestRecordRelayErrorLogRecordsRequestStageFailure(t *testing.T) {
	c := setupRelayErrorLogTest(t)

	recordRelayErrorLog(c, types.NewError(errors.New("invalid request body"), types.ErrorCodeInvalidRequest))

	assert.EqualValues(t, 1, errorLogCount(t))
	var log model.Log
	require.NoError(t, model.LOG_DB.Where("type = ?", model.LogTypeError).First(&log).Error)
	assert.Equal(t, 42, log.UserId)
	assert.Equal(t, "gpt-4o", log.ModelName)
	assert.Contains(t, log.Other, `"error_code":"invalid_request"`)
}

// Errors carrying the NoRecordErrorLog opt-out (e.g. insufficient quota) must
// stay unrecorded even through the request-level fallback.
func TestRecordRelayErrorLogHonorsNoRecordErrorLogOptOut(t *testing.T) {
	c := setupRelayErrorLogTest(t)

	recordRelayErrorLog(c, types.NewError(errors.New("quota not enough"), types.ErrorCodeInsufficientUserQuota, types.ErrOptionWithNoRecordErrorLog()))

	assert.EqualValues(t, 0, errorLogCount(t))
}

// Task submissions only log local validation/conversion failures here; upstream
// failures (LocalError=false) are already covered by processChannelError.
func TestRecordTaskErrorLogOnlyRecordsLocalErrors(t *testing.T) {
	c := setupRelayErrorLogTest(t)

	recordTaskErrorLog(c, service.TaskErrorWrapper(errors.New("remote failure"), "fail_to_fetch_task", http.StatusBadGateway))
	assert.EqualValues(t, 0, errorLogCount(t))

	recordTaskErrorLog(c, service.TaskErrorWrapperLocal(errors.New("invalid_relay_mode"), "invalid_relay_mode", http.StatusBadRequest))
	assert.EqualValues(t, 1, errorLogCount(t))

	// 与渠道错误重复时不再补记
	recordTaskErrorLog(c, service.TaskErrorWrapperLocal(errors.New("invalid again"), "invalid_relay_mode", http.StatusBadRequest))
	assert.EqualValues(t, 1, errorLogCount(t))
}
