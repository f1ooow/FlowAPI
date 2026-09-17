package helper

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
	if constant.StreamingTimeout == 0 {
		constant.StreamingTimeout = 30
	}
}

func setupStreamTest(t *testing.T, body io.Reader) (*gin.Context, *http.Response, *relaycommon.RelayInfo) {
	t.Helper()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	resp := &http.Response{
		Body: io.NopCloser(body),
	}

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{},
	}

	return c, resp, info
}

func buildSSEBody(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "data: {\"id\":%d,\"choices\":[{\"delta\":{\"content\":\"token_%d\"}}]}\n", i, i)
	}
	b.WriteString("data: [DONE]\n")
	return b.String()
}

// ---------- Basic correctness ----------

func TestStreamScannerHandler_NilInputs(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}

	StreamScannerHandler(c, nil, info, func(data string, sr *StreamResult) {})
	StreamScannerHandler(c, &http.Response{Body: io.NopCloser(strings.NewReader(""))}, info, nil)
}

func TestNewStreamScanner_AllowsLargeStreamLine(t *testing.T) {
	oldBufferMB := constant.StreamScannerMaxBufferMB
	constant.StreamScannerMaxBufferMB = 1
	t.Cleanup(func() {
		constant.StreamScannerMaxBufferMB = oldBufferMB
	})

	payload := strings.Repeat("x", 128<<10)
	scanner := NewStreamScanner(strings.NewReader("data: " + payload + "\n"))
	scanner.Split(bufio.ScanLines)

	require.True(t, scanner.Scan())
	assert.Equal(t, "data: "+payload, scanner.Text())
	require.NoError(t, scanner.Err())
}

func TestStreamScannerHandler_EmptyBody(t *testing.T) {
	t.Parallel()

	c, resp, info := setupStreamTest(t, strings.NewReader(""))

	var called atomic.Bool
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		called.Store(true)
	})

	assert.False(t, called.Load(), "handler should not be called for empty body")
}

func TestStreamScannerHandler_1000Chunks(t *testing.T) {
	t.Parallel()

	const numChunks = 1000
	body := buildSSEBody(numChunks)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	var count atomic.Int64
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		count.Add(1)
	})

	assert.Equal(t, int64(numChunks), count.Load())
	assert.Equal(t, numChunks, info.ReceivedResponseCount)
}

func TestStreamScannerHandler_OrderPreserved(t *testing.T) {
	t.Parallel()

	const numChunks = 500
	body := buildSSEBody(numChunks)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	var mu sync.Mutex
	received := make([]string, 0, numChunks)

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		mu.Lock()
		received = append(received, data)
		mu.Unlock()
	})

	require.Equal(t, numChunks, len(received))
	for i := 0; i < numChunks; i++ {
		expected := fmt.Sprintf("{\"id\":%d,\"choices\":[{\"delta\":{\"content\":\"token_%d\"}}]}", i, i)
		assert.Equal(t, expected, received[i], "chunk %d out of order", i)
	}
}

func TestStreamScannerHandler_DoneStopsScanner(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(50) + "data: should_not_appear\n"
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	var count atomic.Int64
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		count.Add(1)
	})

	assert.Equal(t, int64(50), count.Load(), "data after [DONE] must not be processed")
}

func TestStreamScannerHandler_StopStopsStream(t *testing.T) {
	t.Parallel()

	const numChunks = 200
	body := buildSSEBody(numChunks)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	const stopAt int64 = 50
	var count atomic.Int64
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		n := count.Add(1)
		if n >= stopAt {
			sr.Stop(fmt.Errorf("fatal at %d", n))
		}
	})

	assert.Equal(t, stopAt, count.Load())
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonHandlerStop, info.StreamStatus.EndReason)
}

func TestStreamScannerHandler_SkipsNonDataLines(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	b.WriteString(": comment line\n")
	b.WriteString("event: message\n")
	b.WriteString("id: 12345\n")
	b.WriteString("retry: 5000\n")
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&b, "data: payload_%d\n", i)
		b.WriteString(": interleaved comment\n")
	}
	b.WriteString("data: [DONE]\n")

	c, resp, info := setupStreamTest(t, strings.NewReader(b.String()))

	var count atomic.Int64
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		count.Add(1)
	})

	assert.Equal(t, int64(100), count.Load())
}

func TestStreamScannerHandler_DataWithExtraSpaces(t *testing.T) {
	t.Parallel()

	body := "data:   {\"trimmed\":true}  \ndata: [DONE]\n"
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	var got string
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		got = data
	})

	assert.Equal(t, "{\"trimmed\":true}", got)
}

func TestStreamScannerHandlerWithGate_PrecommitErrorWritesNothing(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	resp := &http.Response{Body: io.NopCloser(strings.NewReader("data: {\"error\":{\"message\":\"provider failed\"}}\n\n"))}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	err := StreamScannerHandlerWithGate(c, resp, info, StreamProtocolOpenAIChat, func(data string, sr *StreamResult) {
		require.NoError(t, StringData(c, data))
	})

	require.NotNil(t, err)
	assert.Equal(t, http.StatusBadGateway, err.StatusCode)
	assert.False(t, info.StreamStatus.IsCommitted())
	assert.Empty(t, c.Writer.Header().Get("Content-Type"))
	assert.False(t, c.Writer.Written())
	assert.Empty(t, recorder.Body.String())
}

func TestStreamScannerHandlerWithGate_BuffersNeutralPrefixUntilContent(t *testing.T) {
	t.Parallel()

	body := strings.Join([]string{
		`data: {"id":"chat_1","choices":[{"delta":{"role":"assistant"}}]}`,
		`data: {"id":"chat_1","choices":[{"delta":{"content":"hello"}}]}`,
		`data: [DONE]`,
	}, "\n\n")
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(body))}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	err := StreamScannerHandlerWithGate(c, resp, info, StreamProtocolOpenAIChat, func(data string, sr *StreamResult) {
		require.NoError(t, StringData(c, data))
	})

	require.Nil(t, err)
	assert.True(t, info.StreamStatus.IsCommitted())
	assert.Contains(t, c.Writer.Header().Get("Content-Type"), "text/event-stream")
	assert.True(t, c.Writer.Written())
	output := recorder.Body.String()
	assert.Contains(t, output, `"role":"assistant"`)
	assert.Contains(t, output, `"content":"hello"`)
	assert.Less(t, strings.Index(output, `"role":"assistant"`), strings.Index(output, `"content":"hello"`))
}

func TestStreamScannerHandlerWithGate_PostcommitErrorDoesNotRequestReplay(t *testing.T) {
	t.Parallel()

	body := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"hello"}}]}`,
		`data: {"error":{"message":"late failure"}}`,
	}, "\n\n")
	c, resp, info := setupStreamTest(t, strings.NewReader(body))
	err := StreamScannerHandlerWithGate(c, resp, info, StreamProtocolOpenAIChat, func(data string, sr *StreamResult) {
		require.NoError(t, StringData(c, data))
	})

	require.Nil(t, err)
	assert.True(t, info.StreamStatus.IsCommitted())
}

func TestStreamScannerHandlerWithGate_ProtocolFailuresStayPrecommit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		protocol StreamProtocol
		body     string
	}{
		{"anthropic error", StreamProtocolAnthropic, `data: {"type":"error","error":{"message":"unavailable"}}`},
		{"responses failure", StreamProtocolOpenAIResponses, `data: {"type":"response.failed","response":{"error":{"message":"unavailable"}}}`},
		{"gemini blocked", StreamProtocolGemini, `data: {"promptFeedback":{"blockReason":"SAFETY"}}`},
		{"image error", StreamProtocolOpenAIImage, `data: {"type":"upstream_error","error":{"message":"unavailable"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/test", nil)
			resp := &http.Response{Body: io.NopCloser(strings.NewReader(tt.body + "\n\n"))}
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}

			err := StreamScannerHandlerWithGate(c, resp, info, tt.protocol, func(data string, sr *StreamResult) {
				require.NoError(t, StringData(c, data))
			})

			require.NotNil(t, err)
			assert.Equal(t, http.StatusBadGateway, err.StatusCode)
			assert.False(t, info.StreamStatus.IsCommitted())
			assert.Empty(t, recorder.Body.String())
		})
	}
}

// TestStreamScannerHandlerWithGate_ContentFreeCompletionIsReleased pins the
// boundary between "upstream produced nothing" and "upstream legitimately
// finished without content". Treating the latter as a pre-commit failure made
// the router replay content-filtered / immediately-stopped completions across
// every eligible channel and auto-ban each of them.
func TestStreamScannerHandlerWithGate_ContentFreeCompletionIsReleased(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		protocol StreamProtocol
		frames   []string
		expect   []string
	}{
		{
			name:     "openai content filter",
			protocol: StreamProtocolOpenAIChat,
			frames: []string{
				`{"choices":[{"delta":{},"finish_reason":"content_filter"}]}`,
				`[DONE]`,
			},
			expect: []string{`"finish_reason":"content_filter"`},
		},
		{
			name:     "openai usage only tail",
			protocol: StreamProtocolOpenAIChat,
			frames: []string{
				`{"choices":[],"usage":{"total_tokens":7}}`,
				`[DONE]`,
			},
			expect: []string{`"total_tokens":7`},
		},
		{
			name:     "proxy heartbeat then finish",
			protocol: StreamProtocolOpenAIChat,
			frames: []string{
				`ping`,
				`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
				`[DONE]`,
			},
			expect: []string{`"finish_reason":"stop"`},
		},
		{
			name:     "anthropic stop sequence",
			protocol: StreamProtocolAnthropic,
			frames: []string{
				`{"type":"message_start","message":{"id":"m"}}`,
				`{"type":"message_delta","delta":{"stop_reason":"stop_sequence"}}`,
				`{"type":"message_stop"}`,
			},
			expect: []string{`"type":"message_start"`, `"stop_reason":"stop_sequence"`},
		},
		{
			name:     "gemini finish reason without parts",
			protocol: StreamProtocolGemini,
			frames: []string{
				`{"candidates":[{"finishReason":"MAX_TOKENS","content":{"parts":[]}}]}`,
			},
			expect: []string{`"finishReason":"MAX_TOKENS"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := "data: " + strings.Join(tt.frames, "\n\ndata: ") + "\n\n"
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/test", nil)
			resp := &http.Response{Body: io.NopCloser(strings.NewReader(body))}
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}

			err := StreamScannerHandlerWithGate(c, resp, info, tt.protocol, func(data string, sr *StreamResult) {
				require.NoError(t, StringData(c, data))
			})

			require.Nil(t, err)
			assert.True(t, info.StreamStatus.IsCommitted())
			output := recorder.Body.String()
			for _, want := range tt.expect {
				assert.Contains(t, output, want)
			}
		})
	}
}

// TestStreamScannerHandlerWithGate_ImmediateTerminationStaysPrecommit keeps the
// genuinely empty stream retryable: no upstream event at all before the
// termination marker.
func TestStreamScannerHandlerWithGate_ImmediateTerminationStaysPrecommit(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	resp := &http.Response{Body: io.NopCloser(strings.NewReader("data: [DONE]\n\n"))}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}

	err := StreamScannerHandlerWithGate(c, resp, info, StreamProtocolOpenAIChat, func(data string, sr *StreamResult) {
		require.NoError(t, StringData(c, data))
	})

	require.NotNil(t, err)
	assert.Equal(t, http.StatusBadGateway, err.StatusCode)
	assert.False(t, info.StreamStatus.IsCommitted())
	assert.Empty(t, recorder.Body.String())
}

// TestStreamScannerHandlerGateFlagMatchesReader guards the failover decision in
// controller.Relay: IsCommitted() is only meaningful for the gated reader, so a
// straight-through reader must report GateEnabled() == false and let written
// bytes decide, otherwise a half-written SSE response gets replayed.
func TestStreamScannerHandlerGateFlagMatchesReader(t *testing.T) {
	t.Parallel()

	var nilStatus *relaycommon.StreamStatus
	assert.False(t, nilStatus.GateEnabled())

	body := "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: [DONE]\n\n"

	c, resp, info := setupStreamTest(t, strings.NewReader(body))
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		require.NoError(t, StringData(c, data))
	})
	assert.False(t, info.StreamStatus.GateEnabled())
	assert.False(t, info.StreamStatus.IsCommitted())
	assert.True(t, c.Writer.Written())

	gatedC, gatedResp, gatedInfo := setupStreamTest(t, strings.NewReader(body))
	require.Nil(t, StreamScannerHandlerWithGate(gatedC, gatedResp, gatedInfo, StreamProtocolOpenAIChat, func(data string, sr *StreamResult) {
		require.NoError(t, StringData(gatedC, data))
	}))
	assert.True(t, gatedInfo.StreamStatus.GateEnabled())
	assert.True(t, gatedInfo.StreamStatus.IsCommitted())
}

// TestStreamScannerHandlerWithGate_BufferedFramesAreCounted keeps TTFT
// comparable between gated and non-gated channels: events held back by the gate
// must still count as received upstream output, otherwise perf_metrics mixes
// "first upstream event" and "first content event" latencies in one table.
func TestStreamScannerHandlerWithGate_BufferedFramesAreCounted(t *testing.T) {
	t.Parallel()

	body := strings.Join([]string{
		`data: {"choices":[{"delta":{"role":"assistant"}}]}`,
		`data: {"choices":[{"delta":{"content":"hi"}}]}`,
		`data: [DONE]`,
	}, "\n\n")
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	require.Nil(t, StreamScannerHandlerWithGate(c, resp, info, StreamProtocolOpenAIChat, func(data string, sr *StreamResult) {
		require.NoError(t, StringData(c, data))
	}))
	assert.Equal(t, 2, info.ReceivedResponseCount)
}

// Keepalive heartbeats arrive on a wall-clock cadence, so counting them against
// the event cap would fail every request whose first token takes longer than
// cap*ping_interval — a real pattern for reasoning models behind proxies that
// ping every ~10s. JSON events still consume the budget.
func TestStreamScannerHandlerWithGate_HeartbeatsDoNotConsumeEventBudget(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		neutral      string
		wantOverflow bool
	}{
		{name: "non-json heartbeat", neutral: `data: ping`},
		{
			name:         "json neutral frame",
			neutral:      `data: {"choices":[{"delta":{"role":"assistant"}}]}`,
			wantOverflow: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			frames := make([]string, 0, streamGateEventCap+3)
			for i := 0; i < streamGateEventCap+1; i++ {
				frames = append(frames, tc.neutral)
			}
			frames = append(frames, `data: {"choices":[{"delta":{"content":"hi"}}]}`, `data: [DONE]`)

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			resp := &http.Response{Body: io.NopCloser(strings.NewReader(strings.Join(frames, "\n\n")))}
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}

			err := StreamScannerHandlerWithGate(c, resp, info, StreamProtocolOpenAIChat, func(data string, sr *StreamResult) {
				require.NoError(t, StringData(c, data))
			})

			if tc.wantOverflow {
				require.NotNil(t, err)
				assert.Equal(t, http.StatusBadGateway, err.StatusCode)
				assert.False(t, info.StreamStatus.IsCommitted())
				assert.Empty(t, recorder.Body.String())
				return
			}
			require.Nil(t, err)
			assert.True(t, info.StreamStatus.IsCommitted())
			assert.Contains(t, recorder.Body.String(), `"content":"hi"`)
		})
	}
}

func TestClassifyStreamFrameProtocols(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		protocol StreamProtocol
		data     string
		want     streamFrameVerdict
	}{
		{"anthropic metadata", StreamProtocolAnthropic, `{"type":"message_start","message":{"id":"m"}}`, streamFrameNeutral},
		{"anthropic text", StreamProtocolAnthropic, `{"type":"content_block_delta","delta":{"text":"hello"}}`, streamFrameContent},
		{"openai role", StreamProtocolOpenAIChat, `{"choices":[{"delta":{"role":"assistant"}}]}`, streamFrameNeutral},
		{"openai tool arguments", StreamProtocolOpenAIChat, `{"choices":[{"delta":{"tool_calls":[{"function":{"arguments":"{}"}}]}}]}`, streamFrameContent},
		{"responses lifecycle", StreamProtocolOpenAIResponses, `{"type":"response.created","response":{"id":"r"}}`, streamFrameNeutral},
		{"responses delta", StreamProtocolOpenAIResponses, `{"type":"response.output_text.delta","delta":"hello"}`, streamFrameContent},
		{"responses completed content", StreamProtocolOpenAIResponses, `{"type":"response.completed","response":{"output":[{"content":[{"text":"hello"}]}]}}`, streamFrameContent},
		{"gemini text", StreamProtocolGemini, `{"candidates":[{"content":{"parts":[{"text":"hello"}]}}]}`, streamFrameContent},
		{"gemini blocked", StreamProtocolGemini, `{"promptFeedback":{"blockReason":"SAFETY"}}`, streamFrameError},
		{"image content", StreamProtocolOpenAIImage, `{"type":"image_generation.completed","b64_json":"abc"}`, streamFrameContent},
		{"corrupt json", StreamProtocolOpenAIChat, `{"choices":[`, streamFrameMalformed},
		// Third-party proxies inject plain-text heartbeats into the SSE body;
		// they are noise, not a broken upstream.
		{"proxy heartbeat", StreamProtocolOpenAIChat, `ping`, streamFrameNeutral},
		// Explicit completion signals: a generation may legitimately finish
		// without ever emitting content (filtered, immediate stop sequence).
		{"openai finish reason", StreamProtocolOpenAIChat, `{"choices":[{"delta":{},"finish_reason":"content_filter"}]}`, streamFrameCompleted},
		{"openai null finish reason", StreamProtocolOpenAIChat, `{"choices":[{"delta":{},"finish_reason":null}]}`, streamFrameNeutral},
		{"openai usage tail", StreamProtocolOpenAIChat, `{"choices":[],"usage":{"total_tokens":7}}`, streamFrameNeutral},
		{"anthropic stop reason", StreamProtocolAnthropic, `{"type":"message_delta","delta":{"stop_reason":"stop_sequence"}}`, streamFrameCompleted},
		{"responses completed empty", StreamProtocolOpenAIResponses, `{"type":"response.completed","response":{"output":[]}}`, streamFrameCompleted},
		{"gemini finish reason", StreamProtocolGemini, `{"candidates":[{"finishReason":"SAFETY","content":{"parts":[]}}]}`, streamFrameCompleted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, classifyStreamFrame(tt.protocol, tt.data))
		})
	}
}

// TestStreamScannerHandler_ClientCancelAbortsUpstreamAndReturns pins the
// disconnect contract: when the client goes away, the handler must return
// promptly (all goroutines joined, so the gin.Context can never leak into a
// pooled reuse), the upstream body must be closed to stop token generation,
// and no data received after the disconnect may be processed or written.
func TestStreamScannerHandler_ClientCancelAbortsUpstreamAndReturns(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pr, pw := io.Pipe()
	t.Cleanup(func() {
		_ = pr.Close()
		_ = pw.Close()
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)

	resp := &http.Response{Body: pr}
	info := &relaycommon.RelayInfo{
		DisablePing: true,
		ChannelMeta: &relaycommon.ChannelMeta{},
	}

	var count atomic.Int64
	firstHandled := make(chan struct{})
	done := make(chan struct{})
	go func() {
		StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
			count.Add(1)
			_ = StringData(c, data)
			if data == "first" {
				close(firstHandled)
			}
		})
		close(done)
	}()

	_, err := fmt.Fprint(pw, "data: first\n")
	require.NoError(t, err)

	select {
	case <-firstHandled:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first chunk")
	}

	cancel()

	// The handler must return without any further upstream input: cleanup
	// closes resp.Body, which unblocks the scanner goroutine.
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not return after client disconnect")
	}

	// Upstream read side must be closed so the provider stops generating
	// (and billing) for a request nobody is listening to.
	_, err = fmt.Fprint(pw, "data: second\n")
	require.ErrorIs(t, err, io.ErrClosedPipe, "upstream body should be closed after client disconnect")

	assert.Equal(t, int64(1), count.Load(), "no chunk after disconnect should be processed")
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonClientGone, info.StreamStatus.EndReason)

	body := recorder.Body.String()
	assert.Contains(t, body, "first")
	assert.NotContains(t, body, "second")
}

// ---------- Ping tests ----------

func TestStreamScannerHandler_PingSentDuringSlowUpstream(t *testing.T) {
	setting := operation_setting.GetGeneralSetting()
	oldEnabled := setting.PingIntervalEnabled
	oldSeconds := setting.PingIntervalSeconds
	setting.PingIntervalEnabled = true
	setting.PingIntervalSeconds = 1
	t.Cleanup(func() {
		setting.PingIntervalEnabled = oldEnabled
		setting.PingIntervalSeconds = oldSeconds
	})

	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		for i := 0; i < 4; i++ {
			fmt.Fprintf(pw, "data: chunk_%d\n", i)
			time.Sleep(400 * time.Millisecond)
		}
		fmt.Fprint(pw, "data: [DONE]\n")
	}()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	resp := &http.Response{Body: pr}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}

	var count atomic.Int64
	done := make(chan struct{})
	go func() {
		StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
			count.Add(1)
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for stream to finish")
	}

	assert.Equal(t, int64(4), count.Load())

	body := recorder.Body.String()
	pingCount := strings.Count(body, ": PING")
	assert.GreaterOrEqual(t, pingCount, 1,
		"expected at least 1 ping during slow stream with 1s interval; got %d", pingCount)
}

func TestStreamScannerHandler_PingDisabledByRelayInfo(t *testing.T) {
	setting := operation_setting.GetGeneralSetting()
	oldEnabled := setting.PingIntervalEnabled
	oldSeconds := setting.PingIntervalSeconds
	setting.PingIntervalEnabled = true
	setting.PingIntervalSeconds = 1
	t.Cleanup(func() {
		setting.PingIntervalEnabled = oldEnabled
		setting.PingIntervalSeconds = oldSeconds
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	resp := &http.Response{Body: io.NopCloser(strings.NewReader(buildSSEBody(5)))}
	info := &relaycommon.RelayInfo{
		DisablePing: true,
		ChannelMeta: &relaycommon.ChannelMeta{},
	}

	var count atomic.Int64
	done := make(chan struct{})
	go func() {
		StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
			count.Add(1)
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}

	assert.Equal(t, int64(5), count.Load())

	body := recorder.Body.String()
	pingCount := strings.Count(body, ": PING")
	assert.Equal(t, 0, pingCount, "pings should be disabled when DisablePing=true")
}

// ---------- StreamStatus integration ----------

func TestStreamScannerHandler_StreamStatus_DoneReason(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(10)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {})

	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
	assert.Nil(t, info.StreamStatus.EndError)
	assert.True(t, info.StreamStatus.IsNormalEnd())
	assert.False(t, info.StreamStatus.HasErrors())
}

func TestStreamScannerHandler_StreamStatus_EOFWithoutDone(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	for i := 0; i < 5; i++ {
		fmt.Fprintf(&b, "data: {\"id\":%d}\n", i)
	}
	c, resp, info := setupStreamTest(t, strings.NewReader(b.String()))

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {})

	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonEOF, info.StreamStatus.EndReason)
	assert.True(t, info.StreamStatus.IsNormalEnd())
}

func TestStreamScannerHandler_StreamStatus_HandlerStop(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(100)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	var count atomic.Int64
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		n := count.Add(1)
		if n >= 10 {
			sr.Stop(fmt.Errorf("stop at 10"))
		}
	})

	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonHandlerStop, info.StreamStatus.EndReason)
	assert.True(t, info.StreamStatus.HasErrors())
}

func TestStreamScannerHandler_StreamStatus_HandlerDone(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(20)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	var count atomic.Int64
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		n := count.Add(1)
		if n >= 5 {
			sr.Done()
		}
	})

	assert.Equal(t, int64(5), count.Load())
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
	assert.False(t, info.StreamStatus.HasErrors())
}

func TestStreamScannerHandler_StreamStatus_Timeout(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()
	go func() {
		fmt.Fprint(pw, "data: {\"id\":1}\n")
	}()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	resp := &http.Response{Body: pr}
	timeout := 1
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{Reliability: &dto.ChannelReliabilitySettings{StreamingIdleTimeoutSeconds: &timeout}}}}

	done := make(chan struct{})
	go func() {
		StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for stream timeout")
	}

	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonTimeout, info.StreamStatus.EndReason)
	assert.False(t, info.StreamStatus.IsNormalEnd())
}

func TestStreamScannerHandler_StreamStatus_SoftErrors(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(10)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		sr.Error(fmt.Errorf("soft error for chunk"))
	})

	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
	assert.True(t, info.StreamStatus.HasErrors())
	assert.Equal(t, 10, info.StreamStatus.TotalErrorCount())
}

func TestStreamScannerHandler_StreamStatus_MultipleErrorsPerChunk(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(5)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		sr.Error(fmt.Errorf("error A"))
		sr.Error(fmt.Errorf("error B"))
	})

	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
	assert.Equal(t, 10, info.StreamStatus.TotalErrorCount())
}

func TestStreamScannerHandler_StreamStatus_ErrorThenStop(t *testing.T) {
	t.Parallel()

	// Use a large body without [DONE] to avoid race between scanner's [DONE]
	// and handler's Stop on the sync.Once EndReason.
	var b strings.Builder
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&b, "data: {\"id\":%d}\n", i)
	}
	c, resp, info := setupStreamTest(t, strings.NewReader(b.String()))

	var count atomic.Int64
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		count.Add(1)
		sr.Error(fmt.Errorf("soft error"))
		sr.Stop(fmt.Errorf("fatal"))
	})

	assert.Equal(t, int64(1), count.Load())
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonHandlerStop, info.StreamStatus.EndReason)
	assert.Equal(t, 2, info.StreamStatus.TotalErrorCount())
}

func TestStreamScannerHandler_StreamStatus_InitializedIfNil(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(1)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	assert.Nil(t, info.StreamStatus)

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {})

	assert.NotNil(t, info.StreamStatus)
}

func TestStreamScannerHandler_StreamStatus_ReplacesPreInitialized(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(5)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	info.StreamStatus = relaycommon.NewStreamStatus()
	info.StreamStatus.RecordError("pre-existing error")

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {})

	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
	assert.Equal(t, 0, info.StreamStatus.TotalErrorCount())
}
