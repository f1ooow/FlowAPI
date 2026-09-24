package ali

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type aliFixtureTransport func(*http.Request) (*http.Response, error)

func (f aliFixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAliSubmitAndQueryKeepOriginalAccount(t *testing.T) {
	service.InitHttpClient()
	client := service.GetHttpClient()
	old := client.Transport
	t.Cleanup(func() { client.Transport = old })
	client.Transport = aliFixtureTransport(func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, "Bearer original-key", r.Header.Get("Authorization"))
		assert.Equal(t, "/api/v1/tasks/provider-id", r.URL.Path)
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"output":{"task_id":"provider-id","task_status":"RUNNING"}}`))}, nil
	})
	a := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{OriginModelName: "happyhorse-1.1-r2v", TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "public-id"}, ChannelMeta: &relaycommon.ChannelMeta{ApiKey: "original-key", ChannelBaseUrl: "https://zivv.pro", ChannelOtherSettings: dto.ChannelOtherSettings{TaskEndpointOverride: &dto.TaskEndpointOverride{SubmitPath: "/v1/services/aigc/video-generation/video-synthesis", FetchPath: "/api/v1/tasks/{task_id}"}}}}
	a.Init(info)
	endpoint, err := a.BuildRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://zivv.pro/v1/services/aigc/video-generation/video-synthesis", endpoint)
	request, err := http.NewRequest("POST", endpoint, nil)
	require.NoError(t, err)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	require.NoError(t, a.BuildRequestHeader(c, request, info))
	assert.Equal(t, "enable", request.Header.Get("X-DashScope-Async"))
	assert.Equal(t, "Bearer original-key", request.Header.Get("Authorization"))
	id, _, taskErr := a.DoResponse(c, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"output":{"task_id":"provider-id","task_status":"PENDING"}}`))}, info)
	require.Nil(t, taskErr)
	assert.Equal(t, "provider-id", id)
	response, err := a.FetchTask("https://zivv.pro", "original-key", map[string]any{"task_id": "provider-id"}, "")
	require.NoError(t, err)
	response.Body.Close()
	_, _, taskErr = a.DoResponse(c, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"code":"InvalidParameter","message":"bad media"}`))}, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, 502, taskErr.StatusCode)
	assert.Contains(t, taskErr.Message, "bad media")
	_, err = a.ParseTaskResult([]byte(`{"code":"Throttling","message":"later"}`))
	require.ErrorContains(t, err, "Throttling")
}
