package fastaitoken

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

func TestFastAIModelsKeepBillingUnitsSeparate(t *testing.T) {
	for _, tc := range []struct {
		origin, upstream, unit string
		ratio                  float64
	}{
		{"doubao-seedance-2-5-260628", "seedance-2.5-token", "tokens", 42.0 / 70},
		{"doubao-seedance-2-0-mini-260615", "seedance-2.0-mini-token", "tokens", 14.0 / 23},
		{"doubao-seedance-2-0-260128", "seedance-2.0-time", "seconds", 0},
		{"doubao-seedance-2-0-fast-260128", "seedance-2.0-fast-time", "seconds", 0},
	} {
		t.Run(tc.origin, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set("task_request", relaycommon.TaskSubmitReq{Model: tc.origin, Prompt: "waves", Seconds: "6", Metadata: map[string]any{"resolution": "720P", "reference_videos": []string{"https://media.example/input.mp4"}, "watermark": false, "seed": 0}})
			info := &relaycommon.RelayInfo{OriginModelName: tc.origin, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: tc.upstream, IsModelMapped: true}}
			info.PriceData.UsePrice = true
			a := &TaskAdaptor{}
			a.Init(info)
			ratios := a.EstimateBilling(c, info)
			assert.Equal(t, tc.unit, c.GetString("task_billing_unit"))
			if tc.unit == "seconds" {
				assert.Equal(t, map[string]float64{"seconds": 6}, ratios)
			} else {
				assert.InDelta(t, tc.ratio, ratios["video_input"], 1e-9)
				assert.NotContains(t, ratios, "seconds")
			}
			body, err := a.BuildRequestBody(c, info)
			require.NoError(t, err)
			data, err := io.ReadAll(body)
			require.NoError(t, err)
			var wire map[string]any
			require.NoError(t, common.Unmarshal(data, &wire))
			assert.Equal(t, tc.upstream, wire["model"])
			assert.Equal(t, float64(6), wire["duration"])
			assert.Equal(t, "720p", wire["resolution"])
			assert.Equal(t, "omni", wire["mode"])
			assert.Equal(t, false, wire["watermark"])
			assert.Equal(t, float64(0), wire["seed"])
			assert.Equal(t, tc.origin, info.OriginModelName)
			result, err := a.ParseTaskResult([]byte(`{"id":"provider-id","status":"completed","video_url":"https://media.example/out.mp4","usage":{"completion_tokens":100,"total_tokens":100,"output_video_duration":6}}`))
			require.NoError(t, err)
			if tc.unit == "seconds" {
				assert.Zero(t, result.TotalTokens)
			} else {
				assert.Equal(t, 100, result.TotalTokens)
			}
			assert.Equal(t, 6.0, result.OutputVideoDuration)
		})
	}
}

func TestFastAIDoesNotAdvertiseWanModels(t *testing.T) {
	assert.NotContains(t, ModelList, "wan-3.0")
	assert.NotContains(t, ModelList, "wan-3.0-time")
	_, _, ok := ResolveModel("wan-3.0")
	assert.False(t, ok)
	_, _, ok = ResolveModel("wan-3.0-time")
	assert.False(t, ok)
}

func TestFastAIReferenceMappingAndBounds(t *testing.T) {
	req := relaycommon.TaskSubmitReq{Model: "seedance-2.5-token", Prompt: "hello", Seconds: "5", Metadata: map[string]any{"content": []any{
		map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://media.example/a.png"}, "role": "first_frame"},
		map[string]any{"type": "video_url", "video_url": "https://media.example/a.mp4"},
		map[string]any{"type": "audio_url", "audio_url": map[string]any{"url": "https://media.example/a.mp3"}},
	}}}
	payload, err := buildPayload(&req)
	require.NoError(t, err)
	assert.Equal(t, []string{"https://media.example/a.png"}, payload.Images)
	assert.Equal(t, []string{"https://media.example/a.mp4"}, payload.Videos)
	assert.Equal(t, []string{"https://media.example/a.mp3"}, payload.Audio)
	for _, tc := range []relaycommon.TaskSubmitReq{
		{Duration: relaycommon.MaxTaskDurationSeconds + 1}, {Seconds: "18446744073709551615"}, {Duration: 5, Seconds: "-2"},
		{Metadata: map[string]any{"reference_videos": []string{"1", "2", "3", "4"}}},
	} {
		_, err := buildPayload(&tc)
		require.Error(t, err)
	}
}

type fastAITransport func(*http.Request) (*http.Response, error)

func (f fastAITransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestFastAISubmitAndFetchContract(t *testing.T) {
	service.InitHttpClient()
	var paths, keys []string
	client := service.GetHttpClient()
	oldTransport := client.Transport
	client.Transport = fastAITransport(func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.URL.EscapedPath())
		keys = append(keys, r.Header.Get("Authorization"))
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"task_id":"upstream","status":"running"}`))}, nil
	})
	t.Cleanup(func() { client.Transport = oldTransport })
	baseURL := "https://provider.example"
	a := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{OriginModelName: "seedance-2.0-time", ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: baseURL, ApiKey: "saved-key"}, TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "public-id"}}
	a.Init(info)
	endpoint, err := a.BuildRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, baseURL+SubmitEndpoint, endpoint)
	req, err := http.NewRequest("POST", endpoint, strings.NewReader(`{}`))
	require.NoError(t, err)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	require.NoError(t, a.BuildRequestHeader(c, req, info))
	resp, err := client.Do(req)
	require.NoError(t, err)
	id, _, taskErr := a.DoResponse(c, resp, info)
	require.Nil(t, taskErr)
	assert.Equal(t, "upstream", id)
	resp, err = a.FetchTask(baseURL, "original-account", map[string]any{"task_id": "folder/id", "model": "seedance-2.0-time"}, "")
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, []string{SubmitEndpoint, "/v1/videos/generations/folder%2Fid"}, paths)
	assert.Equal(t, []string{"Bearer saved-key", "Bearer original-account"}, keys)
}

func TestFastAIStatusAndErrors(t *testing.T) {
	a := &TaskAdaptor{}
	for _, tc := range []struct{ status, want string }{{"queued", model.TaskStatusQueued}, {"running", model.TaskStatusInProgress}, {"completed", model.TaskStatusSuccess}, {"failed", model.TaskStatusFailure}, {"UNKNOWN", model.TaskStatusUnknown}} {
		result, err := a.ParseTaskResult([]byte(`{"id":"id","status":"` + tc.status + `","download_url":"https://media.example/out.mp4","error":{"message":"failure detail"}}`))
		require.NoError(t, err)
		assert.Equal(t, tc.want, result.Status)
	}
	_, err := a.ParseTaskResult([]byte(`{"error":{"code":"rate_limit","message":"try later"}}`))
	require.ErrorContains(t, err, "try later")
	_, err = a.ParseTaskResult([]byte(`{"status":"completed"}`))
	require.Error(t, err)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	_, _, taskErr := a.DoResponse(c, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"rejected"}}`))}, &relaycommon.RelayInfo{})
	require.NotNil(t, taskErr)
	assert.Equal(t, "upstream_error", taskErr.Code)
}
