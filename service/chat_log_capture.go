package service

import (
	"bytes"
	"sync"

	"github.com/gin-gonic/gin"
)

type chatLogCaptureWriter struct {
	gin.ResponseWriter
	buffer bytes.Buffer
	mu     sync.Mutex
}

func wrapWithChatLogCapture(c *gin.Context, original gin.ResponseWriter) gin.ResponseWriter {
	w := &chatLogCaptureWriter{
		ResponseWriter: original,
	}
	c.Writer = w
	return w
}

// ponytail: response is buffered fully in memory per request; chat-log is
// opt-in per token, add a cap again if this ever becomes a memory problem.
func (w *chatLogCaptureWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	if err != nil {
		return n, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buffer.Write(data)
	return n, nil
}

func (w *chatLogCaptureWriter) capturedBytes() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buffer.String()
}
