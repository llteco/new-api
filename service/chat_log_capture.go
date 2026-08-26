package service

import (
	"bytes"
	"sync"

	"github.com/gin-gonic/gin"
)

type chatLogCaptureWriter struct {
	gin.ResponseWriter
	buffer    bytes.Buffer
	mu        sync.Mutex
	maxBytes  int
	truncated bool
}

func wrapWithChatLogCapture(c *gin.Context, original gin.ResponseWriter, maxBytes int) gin.ResponseWriter {
	if maxBytes <= 0 {
		return original
	}
	w := &chatLogCaptureWriter{
		ResponseWriter: original,
		maxBytes:       maxBytes,
	}
	c.Writer = w
	return w
}

func (w *chatLogCaptureWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	if err != nil {
		return n, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.truncated {
		return n, nil
	}
	remaining := w.maxBytes - w.buffer.Len()
	if remaining <= 0 {
		w.truncated = true
		return n, nil
	}
	if len(data) <= remaining {
		w.buffer.Write(data)
	} else {
		w.buffer.Write(data[:remaining])
		w.truncated = true
	}
	return n, nil
}

func (w *chatLogCaptureWriter) capturedBytes() (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buffer.String(), w.truncated
}
