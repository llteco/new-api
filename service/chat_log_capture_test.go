package service

import (
	"bytes"
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
	cap := wrapWithChatLogCapture(c, c.Writer).(*chatLogCaptureWriter)

	n, err := cap.Write([]byte(`{"hello":"world"}`))
	require.NoError(t, err)
	assert.Equal(t, 17, n)

	got := cap.capturedBytes()
	assert.Equal(t, `{"hello":"world"}`, got)
	assert.Equal(t, `{"hello":"world"}`, rec.Body.String(), "underlying writer must still receive data")
}

func TestChatLogCaptureWriter_NoCapOnLargeBodies(t *testing.T) {
	c, rec := newCaptureTestContext()
	cap := wrapWithChatLogCapture(c, c.Writer).(*chatLogCaptureWriter)

	big := bytes.Repeat([]byte("x"), 512*1024)
	n, err := cap.Write(big)
	require.NoError(t, err)
	assert.Equal(t, len(big), n, "all bytes written to underlying writer")

	assert.Equal(t, len(big), len(cap.capturedBytes()), "capture must not truncate")
	assert.Equal(t, len(big), rec.Body.Len())
}

func TestChatLogCaptureWriter_MultiWriteAccumulates(t *testing.T) {
	c, _ := newCaptureTestContext()
	cap := wrapWithChatLogCapture(c, c.Writer).(*chatLogCaptureWriter)

	_, _ = cap.Write([]byte(`abc`))
	_, _ = cap.Write([]byte(`def`))
	got := cap.capturedBytes()
	assert.Equal(t, `abcdef`, got)
}

func TestChatLogCaptureWriter_PreservesStatusCode(t *testing.T) {
	c, rec := newCaptureTestContext()
	cap := wrapWithChatLogCapture(c, c.Writer).(*chatLogCaptureWriter)
	cap.WriteHeader(207)
	_, _ = cap.Write([]byte(`x`))
	assert.Equal(t, 207, rec.Code, "embedded ResponseWriter must pass status through")
}
