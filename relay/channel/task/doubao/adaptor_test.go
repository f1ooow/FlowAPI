package doubao

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type seedanceTransport func(*http.Request) (*http.Response, error)

func (f seedanceTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSeedanceRequestUsageAndOriginalAccount(t *testing.T) {
	service.InitHttpClient()
	client := service.GetHttpClient()
	previous := client.Transport
	t.Cleanup(func() { client.Transport = previous })
	client.Transport = seedanceTransport(func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, "/api/v3/contents/generations/tasks/upstream-id", r.URL.Path)
		assert.Equal(t, "Bearer original-account", r.Header.Get("Authorization"))
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"upstream-id","status":"succeeded","content":{"video_url":"https://cdn.example/video.mp4"},"usage":{"completion_tokens":123,"total_tokens":123}}`))}, nil
	})
	for _, name := range []string{"doubao-seedance-2-5-260628", "doubao-seedance-2-0-260128", "doubao-seedance-2-0-fast-260128", "doubao-seedance-2-0-mini-260615"} {
		t.Run(name, func(t *testing.T) {
			a := &TaskAdaptor{}
			info := &relaycommon.RelayInfo{OriginModelName: name, ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://ark.example", ApiKey: "original-account", UpstreamModelName: name}, TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "public-task"}}
			a.Init(info)
			req := relaycommon.TaskSubmitReq{Model: name, Prompt: "hello", Seconds: "5", Metadata: map[string]any{"seed": 0, "watermark": false, "generate_audio": false, "output_format": "mp4", "resolution": "720p", "content": []any{map[string]any{"type": "video_url", "video_url": map[string]any{"url": "https://cdn.example/input.mp4"}}}}}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set("task_request", req)
			wire, err := a.BuildRequestBody(c, info)
			require.NoError(t, err)
			raw, err := io.ReadAll(wire)
			require.NoError(t, err)
			var payload map[string]any
			require.NoError(t, common.Unmarshal(raw, &payload))
			assert.Equal(t, float64(0), payload["seed"])
			assert.Equal(t, false, payload["watermark"])
			assert.Equal(t, false, payload["generate_audio"])
			assert.NotContains(t, a.EstimateBilling(c, info), "seconds")
			resp, err := a.FetchTask("https://ark.example", "original-account", map[string]any{"task_id": "upstream-id"}, "")
			require.NoError(t, err)
			data, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			resp.Body.Close()
			result, err := a.ParseTaskResult(data)
			require.NoError(t, err)
			assert.Equal(t, model.TaskStatusSuccess, result.Status)
			assert.Equal(t, 123, result.TotalTokens)
			assert.Equal(t, "https://cdn.example/video.mp4", result.Url)
			id, _, taskErr := a.DoResponse(c, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"id":"upstream-id"}`))}, info)
			require.Nil(t, taskErr)
			assert.Equal(t, "upstream-id", id)
		})
	}
}

func TestSeedanceRejectsMetadataDurationAndMapsFailure(t *testing.T) {
	a := &TaskAdaptor{}
	payload, err := a.convertToRequestPayload(&relaycommon.TaskSubmitReq{Prompt: "hello", Model: "doubao-seedance-2-5-260628", Duration: 6})
	require.NoError(t, err)
	require.NotNil(t, payload.Duration)
	assert.EqualValues(t, 6, *payload.Duration)
	_, err = a.convertToRequestPayload(&relaycommon.TaskSubmitReq{Prompt: "hello", Model: "doubao-seedance-2-5-260628", Metadata: map[string]any{"duration": relaycommon.MaxTaskDurationSeconds + 1}})
	require.Error(t, err)
	for _, tc := range []struct{ status, want string }{{"queued", model.TaskStatusQueued}, {"running", model.TaskStatusInProgress}, {"failed", model.TaskStatusFailure}, {"UNKNOWN", model.TaskStatusInProgress}} {
		result, err := a.ParseTaskResult([]byte(`{"status":"` + tc.status + `","error":{"message":"provider rejected"}}`))
		require.NoError(t, err)
		assert.Equal(t, tc.want, result.Status)
	}
}
