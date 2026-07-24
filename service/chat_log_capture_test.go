package service

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCaptureTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	return c, rec
}

func TestChatLogCaptureWriter_BuffersBytes(t *testing.T) {
	c, rec := newCaptureTestContext()
	cap := wrapWithChatLogCapture(c, c.Writer, 1024).(*chatLogCaptureWriter)

	n, err := cap.Write([]byte(`{"hello":"world"}`))
	require.NoError(t, err)
	assert.Equal(t, 17, n)

	got, truncated := cap.capturedBytes()
	assert.False(t, truncated)
	assert.Equal(t, `{"hello":"world"}`, got)
	assert.Equal(t, `{"hello":"world"}`, rec.Body.String(), "underlying writer must still receive data")
}

func TestChatLogCaptureWriter_TruncatesAtCap(t *testing.T) {
	c, _ := newCaptureTestContext()
	cap := wrapWithChatLogCapture(c, c.Writer, 4).(*chatLogCaptureWriter)

	n, err := cap.Write([]byte(`1234567890`))
	require.NoError(t, err)
	assert.Equal(t, 10, n, "all bytes written to underlying writer")

	got, truncated := cap.capturedBytes()
	require.True(t, truncated)
	assert.Equal(t, `1234`, got)
}

func TestChatLogCaptureWriter_NoBufferWhenMaxZero(t *testing.T) {
	c, _ := newCaptureTestContext()
	w := wrapWithChatLogCapture(c, c.Writer, 0)
	_, ok := w.(*chatLogCaptureWriter)
	assert.False(t, ok, "maxBytes<=0 returns the original writer unwrapped")
}

func TestChatLogCaptureWriter_MultiWriteAccumulates(t *testing.T) {
	c, _ := newCaptureTestContext()
	cap := wrapWithChatLogCapture(c, c.Writer, 100).(*chatLogCaptureWriter)

	_, _ = cap.Write([]byte(`abc`))
	_, _ = cap.Write([]byte(`def`))
	got, truncated := cap.capturedBytes()
	assert.False(t, truncated)
	assert.Equal(t, `abcdef`, got)
}

func TestChatLogCaptureWriter_PreservesStatusCode(t *testing.T) {
	c, rec := newCaptureTestContext()
	cap := wrapWithChatLogCapture(c, c.Writer, 100).(*chatLogCaptureWriter)
	cap.WriteHeader(207)
	_, _ = cap.Write([]byte(`x`))
	assert.Equal(t, 207, rec.Code, "embedded ResponseWriter must pass status through")
}
