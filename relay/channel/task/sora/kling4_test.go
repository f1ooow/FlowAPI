package sora

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relaykitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKling4ReferenceSecondsMatchSubmittedMedia(t *testing.T) {
	settings := system_setting.GetFetchSetting()
	original := *settings
	t.Cleanup(func() { *settings = original })
	settings.EnableSSRFProtection = false
	service.InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(wanTestMP4(4250))
	}))
	t.Cleanup(server.Close)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	t.Cleanup(func() { common.CleanupBodyStorage(c) })
	c.Request = httptest.NewRequest("POST", "/v1/videos", bytes.NewBufferString(`{"model":"kling-4.0","prompt":"test","duration":10,"resolution":"1080p","videos":["`+server.URL+`/ref.mp4","`+server.URL+`/ref.mp4"]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{
		OriginModelName: "kling-4.0", TaskRelayInfo: &relaycommon.TaskRelayInfo{},
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "kling-4.0", ChannelOtherSettings: relaykitdto.ChannelOtherSettings{VideoProtocol: "rolldek-kling4"}},
	}
	a := &TaskAdaptor{}
	a.Init(info)
	require.Nil(t, a.ValidateRequestAndSetAction(c, info))
	assert.Equal(t, map[string]float64{"seconds": 18.5, "resolution": 1.4}, a.EstimateBilling(c, info))
	reader, err := a.BuildRequestBody(c, info)
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	var wire map[string]any
	require.NoError(t, common.Unmarshal(body, &wire))
	assert.Equal(t, 10.0, wire["duration"])
	assert.Equal(t, "1080p", wire["resolution"])
	assert.Len(t, wire["videos"], 2, "every submitted reference contributes its actual duration")
	assert.NotContains(t, wire, "seconds")
}

func TestKling4RejectsBillingBypassesBeforeFetchingMedia(t *testing.T) {
	for _, extra := range []string{
		`,"seconds":"1"`, `,"size":"720p"`, `,"n":2`,
		`,"metadata":{"duration":30}`, `,"parameters":{"duration":30}`,
		`,"input":{"media":[]}`, `,"generateAudio":true`,
	} {
		t.Run(extra, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			t.Cleanup(func() { common.CleanupBodyStorage(c) })
			c.Request = httptest.NewRequest("POST", "/v1/videos", bytes.NewBufferString(`{"model":"kling-4.0","prompt":"test","duration":30,"resolution":"1080p","videos":["https://invalid.example/reference.mp4"]`+extra+`}`))
			c.Request.Header.Set("Content-Type", "application/json")
			err := (&TaskAdaptor{kling4Protocol: true}).ValidateRequestAndSetAction(c, &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}})
			require.NotNil(t, err)
			assert.Equal(t, 400, err.StatusCode)
			assert.Equal(t, "invalid_request", err.Code, "must reject before probing the invalid URL")
		})
	}
}

func TestKling4RequestBillingBoundaries(t *testing.T) {
	for _, tc := range []struct {
		duration   int
		resolution string
		ratio      float64
	}{{3, "720p", 1}, {30, "1080p", 1.4}} {
		t.Run(strconv.Itoa(tc.duration)+tc.resolution, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			t.Cleanup(func() { common.CleanupBodyStorage(c) })
			c.Request = httptest.NewRequest("POST", "/v1/videos", bytes.NewBufferString(`{"model":"kling-4.0","prompt":"test","duration":`+strconv.Itoa(tc.duration)+`,"resolution":"`+tc.resolution+`"}`))
			c.Request.Header.Set("Content-Type", "application/json; charset=utf-8")
			info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
			a := &TaskAdaptor{kling4Protocol: true}
			require.Nil(t, a.ValidateRequestAndSetAction(c, info))
			assert.Equal(t, map[string]float64{"seconds": float64(tc.duration), "resolution": tc.ratio}, a.EstimateBilling(c, info))
		})
	}
	for _, tc := range []struct{ path, contentType, action string }{
		{"/v1/videos/task_existing/remix", "application/json", constant.TaskActionRemix},
		{"/v1/videos", "multipart/form-data; boundary=fixture", ""},
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", tc.path, bytes.NewBufferString(`{}`))
		c.Request.Header.Set("Content-Type", tc.contentType)
		err := (&TaskAdaptor{kling4Protocol: true}).ValidateRequestAndSetAction(c, &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{Action: tc.action}})
		require.NotNil(t, err)
		assert.Equal(t, 400, err.StatusCode)
	}
}

func TestKling4MaximumReferenceDuration(t *testing.T) {
	settings := system_setting.GetFetchSetting()
	original := *settings
	t.Cleanup(func() { *settings = original })
	settings.EnableSSRFProtection = false
	service.InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(wanTestMP4(30000))
	}))
	t.Cleanup(server.Close)
	data, err := common.Marshal(map[string]any{"model": "kling-4.0", "prompt": "test", "duration": 30, "resolution": "1080p", "videos": []string{server.URL, server.URL, server.URL, server.URL, server.URL}})
	require.NoError(t, err)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	t.Cleanup(func() { common.CleanupBodyStorage(c) })
	c.Request = httptest.NewRequest("POST", "/v1/videos", bytes.NewReader(data))
	c.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
	a := &TaskAdaptor{kling4Protocol: true}
	require.Nil(t, a.ValidateRequestAndSetAction(c, info))
	assert.Equal(t, map[string]float64{"seconds": 180, "resolution": 1.4}, a.EstimateBilling(c, info))
}
