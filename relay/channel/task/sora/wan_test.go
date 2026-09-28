package sora

import (
	"bytes"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	relaykitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/system_setting"

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

// wanTestMP4 builds a minimal MP4 whose video track lasts millis/1000 seconds.
func wanTestMP4(millis uint32) []byte {
	box := func(name string, data []byte) []byte {
		b := make([]byte, 8)
		binary.BigEndian.PutUint32(b, uint32(8+len(data)))
		copy(b[4:], name)
		return append(b, data...)
	}
	mdhd := make([]byte, 24)
	binary.BigEndian.PutUint32(mdhd[12:], 1000)
	binary.BigEndian.PutUint32(mdhd[16:], millis)
	hdlr := make([]byte, 25)
	copy(hdlr[8:], "vide")
	return box("moov", box("trak", box("mdia", append(box("mdhd", mdhd), box("hdlr", hdlr)...))))
}

func TestWanReferenceVideoBillingDedupesAndRoundsUp(t *testing.T) {
	fetchSetting := system_setting.GetFetchSetting()
	original := *fetchSetting
	t.Cleanup(func() { *fetchSetting = original })
	fetchSetting.EnableSSRFProtection = false

	var hits sync.Map
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count, _ := hits.LoadOrStore(r.URL.Path, new(atomic.Int32))
		count.(*atomic.Int32).Add(1)
		millis := map[string]uint32{"/a.mp4": 4200, "/b.mp4": 3000}[r.URL.Path]
		_, _ = w.Write(wanTestMP4(millis))
	}))
	t.Cleanup(server.Close)

	tests := []struct {
		name         string
		media        string
		duration     int
		wantInput    float64
		wantHits     map[string]int32
		wantRejected bool
	}{
		{name: "no reference video", media: `[{"type":"reference_image","url":"https://example.com/a.png"}]`, duration: 5, wantInput: 0, wantHits: map[string]int32{}},
		{name: "same URL probed and billed once, rounded up", media: `[{"type":"reference_video","url":"` + server.URL + `/a.mp4"},{"type":"reference_video","url":"` + server.URL + `/a.mp4"}]`, duration: 5, wantInput: 5, wantHits: map[string]int32{"/a.mp4": 1}},
		{name: "distinct URLs sum rounded seconds", media: `[{"type":"reference_video","url":"` + server.URL + `/a.mp4"},{"type":"reference_video","url":"` + server.URL + `/b.mp4"}]`, duration: 5, wantInput: 8, wantHits: map[string]int32{"/a.mp4": 1, "/b.mp4": 1}},
		{name: "30s bound uses deduplicated duration", media: `[{"type":"reference_video","url":"` + server.URL + `/a.mp4"},{"type":"reference_video","url":"` + server.URL + `/a.mp4"},{"type":"reference_video","url":"` + server.URL + `/a.mp4"},{"type":"reference_video","url":"` + server.URL + `/a.mp4"}]`, duration: 25, wantInput: 5, wantHits: map[string]int32{"/a.mp4": 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hits.Range(func(key, _ any) bool { hits.Delete(key); return true })
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			body := `{"model":"wan3.0-video","input":{"prompt":"test","media":` + tt.media + `},"parameters":{"duration":` + strconv.Itoa(tt.duration) + `,"resolution":"480P"}}`
			c.Request = httptest.NewRequest(http.MethodPost, dto.WanCreatePath, bytes.NewBufferString(body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Request.Header.Set("X-DashScope-Async", "enable")
			info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}, ChannelMeta: &relaycommon.ChannelMeta{ChannelOtherSettings: relaykitdto.ChannelOtherSettings{VideoProtocol: dto.WanProtocolRolldek}}, OriginModelName: "wan3.0-video"}
			a := &TaskAdaptor{}
			a.Init(info)
			require.Nil(t, a.ValidateRequestAndSetAction(c, info))
			assert.Equal(t, tt.wantInput, c.GetFloat64(relaycommon.TaskInputVideoSecondsKey))
			assert.Equal(t, map[string]float64{"seconds": float64(tt.duration) + tt.wantInput, "resolution": .5}, a.EstimateBilling(c, info))
			got := map[string]int32{}
			hits.Range(func(key, value any) bool { got[key.(string)] = value.(*atomic.Int32).Load(); return true })
			assert.Equal(t, tt.wantHits, got)
		})
	}
}
