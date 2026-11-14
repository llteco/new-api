package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupShouldRetryTest pins the default retry status-code rules so the table
// below asserts the shipped failover policy, not whatever a previous test left.
func setupShouldRetryTest(t *testing.T) {
	t.Helper()
	previousRanges := operation_setting.AutomaticRetryStatusCodesToString()
	require.NoError(t, operation_setting.AutomaticRetryStatusCodesFromString("100-199,300-399,401-407,409-499,500-503,505-523,525-599"))
	t.Cleanup(func() {
		require.NoError(t, operation_setting.AutomaticRetryStatusCodesFromString(previousRanges))
	})
	gin.SetMode(gin.TestMode)
}

func newShouldRetryContext() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c
}

func TestShouldRetry(t *testing.T) {
	setupShouldRetryTest(t)

	upstreamErr := func(statusCode int) *types.NewAPIError {
		return types.NewOpenAIError(errors.New("upstream failed"), types.ErrorCodeBadResponseStatusCode, statusCode)
	}

	tests := []struct {
		name     string
		err      *types.NewAPIError
		setup    func(c *gin.Context)
		expected bool
	}{
		{"nil error", nil, nil, false},
		{"channel error always fails over", types.NewError(errors.New("no available key"), types.ErrorCodeChannelNoAvailableKey), nil, true},
		{"skip-retry error never fails over", types.NewError(errors.New("invalid request"), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry()), nil, false},
		{"500 fails over", upstreamErr(http.StatusInternalServerError), nil, true},
		{"429 fails over", upstreamErr(http.StatusTooManyRequests), nil, true},
		{"400 does not fail over by default", upstreamErr(http.StatusBadRequest), nil, false},
		{"504 never fails over", upstreamErr(http.StatusGatewayTimeout), nil, false},
		{"2xx does not fail over", upstreamErr(http.StatusOK), nil, false},
		{"pinned channel does not fail over", upstreamErr(http.StatusInternalServerError), func(c *gin.Context) {
			c.Set("specific_channel_id", "1")
		}, false},
		{"pinned channel does not fail over on channel error", types.NewError(errors.New("no available key"), types.ErrorCodeChannelNoAvailableKey), func(c *gin.Context) {
			c.Set("specific_channel_id", "1")
		}, false},
		{"out-of-range status fails over", upstreamErr(0), nil, true},
		{"affinity skip blocks failover", upstreamErr(http.StatusInternalServerError), func(c *gin.Context) {
			c.Set("channel_affinity_skip_retry_on_failure", true)
		}, false},
		{"written response blocks failover", upstreamErr(http.StatusInternalServerError), func(c *gin.Context) {
			_, _ = c.Writer.Write([]byte("partial response"))
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newShouldRetryContext()
			if tt.setup != nil {
				tt.setup(c)
			}
			assert.Equal(t, tt.expected, shouldRetry(c, tt.err))
		})
	}
}

func TestShouldRetryTaskRelay(t *testing.T) {
	setupShouldRetryTest(t)

	taskErr := func(statusCode int) *taskdto.TaskError {
		return service.TaskErrorWrapperLocal(errors.New("upstream failed"), "upstream_error", statusCode)
	}
	upstreamTaskErr := func(statusCode int) *taskdto.TaskError {
		return service.TaskErrorWrapper(errors.New("upstream failed"), "upstream_error", statusCode)
	}

	tests := []struct {
		name     string
		err      *taskdto.TaskError
		setup    func(c *gin.Context)
		expected bool
	}{
		{"nil error", nil, nil, false},
		{"429 fails over", upstreamTaskErr(http.StatusTooManyRequests), nil, true},
		{"307 fails over", upstreamTaskErr(http.StatusTemporaryRedirect), nil, true},
		{"500 fails over", upstreamTaskErr(http.StatusInternalServerError), nil, true},
		{"504 never fails over", upstreamTaskErr(http.StatusGatewayTimeout), nil, false},
		{"400 does not fail over", upstreamTaskErr(http.StatusBadRequest), nil, false},
		{"408 does not fail over", upstreamTaskErr(http.StatusRequestTimeout), nil, false},
		{"local error does not fail over", taskErr(http.StatusForbidden), nil, false},
		{"local 5xx does not fail over", taskErr(http.StatusInternalServerError), nil, false},
		{"affinity skip blocks failover", upstreamTaskErr(http.StatusInternalServerError), func(c *gin.Context) {
			c.Set("channel_affinity_skip_retry_on_failure", true)
		}, false},
		{"pinned channel does not fail over", upstreamTaskErr(http.StatusInternalServerError), func(c *gin.Context) {
			c.Set("specific_channel_id", "1")
		}, false},
		{"written response blocks failover", upstreamTaskErr(http.StatusInternalServerError), func(c *gin.Context) {
			_, _ = c.Writer.Write([]byte("partial response"))
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newShouldRetryContext()
			if tt.setup != nil {
				tt.setup(c)
			}
			assert.Equal(t, tt.expected, shouldRetryTaskRelay(c, tt.err))
		})
	}
}
