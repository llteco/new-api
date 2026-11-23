package channel

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupAttemptTimeoutTest(t *testing.T, streamHeaderTimeout, nonStreamTimeout int) {
	t.Helper()
	service.InitHttpClient()
	originalHeader, originalNonStream := common.RelayStreamHeaderTimeout, common.RelayNonStreamTimeout
	common.RelayStreamHeaderTimeout, common.RelayNonStreamTimeout = streamHeaderTimeout, nonStreamTimeout
	t.Cleanup(func() {
		common.RelayStreamHeaderTimeout, common.RelayNonStreamTimeout = originalHeader, originalNonStream
	})
	gin.SetMode(gin.TestMode)
}

func newTimeoutTestContext(t *testing.T, target string) (*gin.Context, *http.Request) {
	t.Helper()
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/relay", nil)
	req, err := http.NewRequest(http.MethodPost, target, bytes.NewReader([]byte("{}")))
	require.NoError(t, err)
	return ctx, req
}

func newTimeoutTestRelayInfo(isStream bool) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}, IsStream: isStream}
}

// blockingServer accepts requests and never sends a response until the test
// releases it or the client goes away.
func blockingServer(t *testing.T) *httptest.Server {
	t.Helper()
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(server.Close)
	// Registered after Close so it runs first (cleanups are LIFO); otherwise
	// server.Close would wait on a handler that is still blocked on release.
	t.Cleanup(func() { close(release) })
	return server
}

// TestDoRequestStreamHeaderTimeout protects failover latency: a streaming
// upstream that never sends response headers must be abandoned after the
// configured header timeout instead of blocking the request indefinitely.
func TestDoRequestStreamHeaderTimeout(t *testing.T) {
	setupAttemptTimeoutTest(t, 1, 0)
	server := blockingServer(t)
	ctx, req := newTimeoutTestContext(t, server.URL)

	start := time.Now()
	resp, err := doRequest(ctx, req, newTimeoutTestRelayInfo(true))
	elapsed := time.Since(start)

	require.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "do request failed")
	assert.Less(t, elapsed, 5*time.Second, "streaming attempt must not block past the header timeout")
}

// TestDoRequestNonStreamTimeout protects failover latency for non-streaming
// requests whose upstream never completes the response.
func TestDoRequestNonStreamTimeout(t *testing.T) {
	setupAttemptTimeoutTest(t, 0, 1)
	server := blockingServer(t)
	ctx, req := newTimeoutTestContext(t, server.URL)

	start := time.Now()
	resp, err := doRequest(ctx, req, newTimeoutTestRelayInfo(false))
	elapsed := time.Since(start)

	require.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "do request failed")
	assert.Less(t, elapsed, 5*time.Second, "non-streaming attempt must not block past the response timeout")
}

// TestDoRequestNonStreamBodyDeadline protects the non-streaming bound when
// headers arrive but the body stalls: the deadline must still abort the read.
func TestDoRequestNonStreamBodyDeadline(t *testing.T) {
	setupAttemptTimeoutTest(t, 0, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "10")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "12345")
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(server.Close)
	// Registered after Close so it runs first (cleanups are LIFO).
	t.Cleanup(func() { close(release) })
	ctx, req := newTimeoutTestContext(t, server.URL)

	resp, err := doRequest(ctx, req, newTimeoutTestRelayInfo(false))
	require.NoError(t, err)
	_, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Error(t, readErr, "a stalled non-streaming body must hit the attempt deadline")
}

// TestDoRequestAttemptTimeoutDisabled protects the opt-out: 0 disables the
// per-attempt bound, so a slow but healthy upstream still succeeds.
func TestDoRequestAttemptTimeoutDisabled(t *testing.T) {
	setupAttemptTimeoutTest(t, 0, 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = io.WriteString(w, "ok")
	}))
	t.Cleanup(server.Close)
	ctx, req := newTimeoutTestContext(t, server.URL)

	resp, err := doRequest(ctx, req, newTimeoutTestRelayInfo(false))
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "ok", string(body))
}
