package channel

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDoRequestKeepsStreamSilentBeforeCommit covers the full transport path,
// not just the stream reader: doRequest used to publish SSE headers and ping
// comments for every streaming request while waiting on the upstream
// handshake. That pinned Content-Type to text/event-stream (so a later JSON
// error body was served as SSE), flushed the 200 status line (so the terminal
// error body was never written at all) and destroyed pre-commit failover.
func TestDoRequestKeepsStreamSilentBeforeCommit(t *testing.T) {
	service.InitHttpClient()
	gin.SetMode(gin.TestMode)

	generalSettings := operation_setting.GetGeneralSetting()
	originalEnabled := generalSettings.PingIntervalEnabled
	originalSeconds := generalSettings.PingIntervalSeconds
	t.Cleanup(func() {
		generalSettings.PingIntervalEnabled = originalEnabled
		generalSettings.PingIntervalSeconds = originalSeconds
	})
	generalSettings.PingIntervalEnabled = true
	generalSettings.PingIntervalSeconds = 1

	// Upstream stalls past the ping interval, which is exactly when the
	// keepalive used to leak bytes to the client.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(1500 * time.Millisecond)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer upstream.Close()

	t.Run("pre-commit stays silent and leaves the error body writable", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

		req, err := http.NewRequest(http.MethodPost, upstream.URL, strings.NewReader("{}"))
		require.NoError(t, err)
		info := &relaycommon.RelayInfo{IsStream: true, ChannelMeta: &relaycommon.ChannelMeta{}}

		resp, err := doRequest(ctx, req, info)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())

		assert.False(t, ctx.Writer.Written())
		assert.Empty(t, recorder.Body.String())
		assert.Empty(t, ctx.Writer.Header().Get("Content-Type"))

		// The router turns a pre-commit stream failure into a JSON error; it
		// must not inherit the SSE content type.
		ctx.JSON(http.StatusBadGateway, gin.H{"error": "upstream failed"})
		assert.Contains(t, recorder.Header().Get("Content-Type"), "application/json")
		assert.Contains(t, recorder.Body.String(), "upstream failed")
	})

	t.Run("committed response is kept warm", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		ctx.Writer.WriteHeaderNow()
		require.True(t, ctx.Writer.Written())

		req, err := http.NewRequest(http.MethodPost, upstream.URL, strings.NewReader("{}"))
		require.NoError(t, err)
		info := &relaycommon.RelayInfo{IsStream: true, ChannelMeta: &relaycommon.ChannelMeta{}}

		resp, err := doRequest(ctx, req, info)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())

		assert.Contains(t, recorder.Body.String(), ": PING")
	})
}
