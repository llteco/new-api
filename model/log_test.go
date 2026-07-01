package model

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppendRequestHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Authorization", "Bearer secret")
	c.Request.Header.Set("Cookie", "session=abc")
	c.Request.Header.Set("X-Custom", "value")
	c.Request.Header.Add("X-Multi", "first")
	c.Request.Header.Add("X-Multi", "second")

	other := make(map[string]interface{})
	appendRequestHeaders(c, other)

	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	headers, ok := adminInfo["request_headers"].(map[string]string)
	require.True(t, ok)

	assert.Equal(t, "application/json", headers["Content-Type"])
	assert.Equal(t, "value", headers["X-Custom"])
	assert.Equal(t, "first", headers["X-Multi"])
	assert.NotContains(t, headers, "Authorization")
	assert.NotContains(t, headers, "Cookie")
	assert.NotContains(t, headers, "X-Multi-Second")
}

func TestAppendRequestHeadersPreservesExistingAdminInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/models", nil)
	c.Request.Header.Set("Accept", "application/json")

	other := map[string]interface{}{
		"admin_info": map[string]interface{}{
			"local_count_tokens": true,
		},
	}
	appendRequestHeaders(c, other)

	adminInfo := other["admin_info"].(map[string]interface{})
	assert.Equal(t, true, adminInfo["local_count_tokens"])
	assert.Equal(t, "application/json", adminInfo["request_headers"].(map[string]string)["Accept"])
}

func TestAppendRequestHeadersSkipsEmptyAndSensitive(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.Header.Set("Authorization", "Bearer secret")
	c.Request.Header.Set("X-Empty", "   ")

	other := make(map[string]interface{})
	appendRequestHeaders(c, other)

	assert.Nil(t, other["admin_info"])
}
