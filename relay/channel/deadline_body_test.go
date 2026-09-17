package channel

import (
	"context"
	"fmt"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNonStreamDeadlineContinuesAfterResponseHeaders(t *testing.T) {
	cancelled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(cancelled)
	}))
	defer server.Close()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	seconds := 1
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{Reliability: &dto.ChannelReliabilitySettings{NonStreamingTimeoutSeconds: &seconds}}}}
	req, err := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(`{}`))
	require.NoError(t, err)
	resp, err := doRequest(c, req, info)
	require.NoError(t, err)
	defer resp.Body.Close()
	_, err = io.ReadAll(resp.Body)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("deadline did not reach upstream")
	}
}

func TestHedgeDrainByteLimitAppliesOnlyAfterDetachment(t *testing.T) {
	a := &relaycommon.HedgeAttempt{}
	body := &hedgeResponseBody{ReadCloser: io.NopCloser(strings.NewReader("beforeafter")), attempt: a, drained: maxHedgeDrainBytes}
	buf := make([]byte, 6)
	n, err := body.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, "before", string(buf[:n]))
	a.Detached.Store(true)
	_, err = body.Read(buf)
	require.ErrorContains(t, err, "byte limit")
}

type deadlineTestTransport struct{ received chan context.Context }

func (t deadlineTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	t.received <- r.Context()
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: deadlineTestBody{ctx: r.Context()}, Request: r}, nil
}

type deadlineTestBody struct{ ctx context.Context }

func (b deadlineTestBody) Read([]byte) (int, error) { <-b.ctx.Done(); return 0, b.ctx.Err() }
func (b deadlineTestBody) Close() error             { return nil }

func TestNonStreamDeadlineInMemoryTransport(t *testing.T) {
	service.InitHttpClient()
	client := service.GetHttpClient()
	originalTransport, originalTimeout := client.Transport, client.Timeout
	t.Cleanup(func() { client.Transport, client.Timeout = originalTransport, originalTimeout })
	for _, channelTimer := range []bool{true, false} {
		t.Run(fmt.Sprint("channel_timer_", channelTimer), func(t *testing.T) {
			received := make(chan context.Context, 1)
			client.Transport = deadlineTestTransport{received: received}
			client.Timeout = 0
			seconds := 1
			settings := dto.ChannelSettings{Reliability: &dto.ChannelReliabilitySettings{}}
			if channelTimer {
				settings.Reliability.NonStreamingTimeoutSeconds = &seconds
			} else {
				client.Timeout = time.Millisecond
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			req, err := http.NewRequest(http.MethodPost, "http://deadline.test", strings.NewReader(`{}`))
			require.NoError(t, err)
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelSetting: settings}}
			resp, err := doRequest(c, req, info)
			require.NoError(t, err)
			defer resp.Body.Close()
			upstreamCtx := <-received
			_, hasDeadline := upstreamCtx.Deadline()
			require.True(t, hasDeadline)
			_, err = io.ReadAll(resp.Body)
			require.ErrorIs(t, err, context.DeadlineExceeded)
		})
	}
}
