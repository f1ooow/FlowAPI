package sora

import (
	"bytes"
	relaykitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWanChannelOfficialToRolldek(t *testing.T) {
	for _, name := range []string{"wan3.0-video", "wan3.0-video-prime"} {
		for _, resolution := range []string{"480P", "720P", "1080P"} {
			t.Run(name+resolution, func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				data := []byte(`{"model":"` + name + `","input":{"media":[{"type":"first_frame","url":"https://example.com/a.png"},{"type":"last_frame","url":"https://example.com/a.png"}]},"parameters":{"duration":2,"resolution":"` + resolution + `"}}`)
				c.Request = httptest.NewRequest(http.MethodPost, dto.WanCreatePath, bytes.NewReader(data))
				c.Request.Header.Set("Content-Type", "application/json")
				c.Request.Header.Set("X-DashScope-Async", "enable")
				info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}, ChannelMeta: &relaycommon.ChannelMeta{ChannelOtherSettings: relaykitdto.ChannelOtherSettings{VideoProtocol: dto.WanProtocolRolldek}}, OriginModelName: name}
				a := &TaskAdaptor{}
				a.Init(info)
				require.Nil(t, a.ValidateRequestAndSetAction(c, info))
				reader, err := a.BuildRequestBody(c, info)
				require.NoError(t, err)
				b, err := io.ReadAll(reader)
				require.NoError(t, err)
				var body map[string]any
				require.NoError(t, common.Unmarshal(b, &body))
				assert.Equal(t, name+"-"+map[string]string{"480P": "480p", "720P": "720p", "1080P": "1080p"}[resolution], body["model"])
				assert.Equal(t, "2", body["seconds"])
				assert.Equal(t, "adaptive", body["aspect_ratio"])
				assert.Equal(t, false, body["watermark"], "Rolldek wire must explicitly request no watermark")
				media := body["input"].(map[string]any)["media"].([]any)
				assert.Len(t, media, 2)
				assert.Equal(t, name, info.OriginModelName)
				assert.Equal(t, map[string]float64{"seconds": 2, "resolution": map[string]float64{"480P": .5, "720P": 1, "1080P": 2}[resolution]}, a.EstimateBilling(c, info))
			})
		}
	}
}

func TestWanAndSoraResultIsolation(t *testing.T) {
	data := []byte(`{"id":"upstream","status":"completed","metadata":{"url":"https://example.com/video.mp4"},"usage":{"output_video_duration":2}}`)
	a := &TaskAdaptor{wanProtocol: true}
	result, err := a.ParseTaskResult(data)
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, result.Status)
	assert.Equal(t, "https://example.com/video.mp4", result.Url)
	assert.Equal(t, 2.0, result.OutputVideoDuration)
	a.wanProtocol = false
	result, err = a.ParseTaskResult(data)
	require.NoError(t, err)
	assert.Empty(t, result.Url)
	result, err = parseWanResult([]byte(`{"id":"upstream","status":"queued","error_code":"invalid_ratio","error_message":"not supported"}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusFailure, result.Status)
	result, err = parseWanResult([]byte(`{"id":"upstream","status":"unknown"}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusUnknown, result.Status)
}

func TestWanUnverifiedControlsFailExplicitly(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", dto.WanCreatePath, bytes.NewBufferString(`{"model":"wan3.0-video","input":{"prompt":"test"},"parameters":{"seed":0,"audio":false}}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("X-DashScope-Async", "enable")
	e := (&TaskAdaptor{wanProtocol: true}).ValidateRequestAndSetAction(c, &relaycommon.RelayInfo{})
	require.NotNil(t, e)
	assert.Equal(t, "unsupported_channel_capability", e.Code)
}
