package hailuo

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type hailuoFixtureTransport func(*http.Request) (*http.Response, error)

func (f hailuoFixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestHailuoQueriesUseOriginalModelAndAccount(t *testing.T) {
	service.InitHttpClient()
	client := service.GetHttpClient()
	old := client.Transport
	t.Cleanup(func() { client.Transport = old })
	var paths []string
	client.Transport = hailuoFixtureTransport(func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, "Bearer original-key", r.Header.Get("Authorization"))
		paths = append(paths, r.URL.RequestURI())
		body := `{"task_id":"task","status":"Success","file_id":"9223372036854775807","base_resp":{"status_code":0}}`
		if strings.Contains(r.URL.Path, "/files/retrieve") {
			body = `{"file":{"download_url":"https://cdn.example/video.mp4"},"base_resp":{"status_code":0}}`
		} else if strings.Contains(r.URL.Path, "/v2/") {
			body = `{"task":{"id":"task","status":"completed","content":{"url":"https://cdn.example/h3.mp4"}}}`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	a := &TaskAdaptor{}
	a.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://minimax.example/prefix", ApiKey: "original-key"}})
	for _, name := range []string{"MiniMax-Hailuo-2.3", ModelMiniMaxH3} {
		resp, err := a.FetchTask("https://minimax.example/prefix", "original-key", map[string]any{"task_id": "task", "model": name}, "")
		require.NoError(t, err)
		raw, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		resp.Body.Close()
		result, err := a.ParseTaskResult(raw)
		require.NoError(t, err)
		assert.Equal(t, model.TaskStatusSuccess, result.Status)
		assert.NotEmpty(t, result.Url)
	}
	assert.Equal(t, []string{"/prefix/v1/query/video_generation?task_id=task", "/prefix/v1/files/retrieve?file_id=9223372036854775807", "/prefix/v2/query/video_generation/task"}, paths)
	_, err := a.ParseTaskResult([]byte(`{"base_resp":{"status_code":1002,"status_msg":"rate limited"}}`))
	require.ErrorContains(t, err, "rate limited")
	result, err := a.ParseTaskResult([]byte(`{"status":"UNKNOWN","base_resp":{"status_code":0}}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusInProgress, result.Status)
	result, err = a.ParseTaskResult([]byte(`{"status":"Fail","base_resp":{"status_code":0}}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusFailure, result.Status)
}
