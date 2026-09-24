package minimax

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type miniMaxTransport func(*http.Request) (*http.Response, error)

func (fn miniMaxTransport) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }

func miniMaxFixtureDB(t *testing.T) {
	t.Helper()
	old := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, MigrateResourceBindings(db))
	require.NoError(t, MigrateResourceBindings(db))
	model.DB = db
	t.Cleanup(func() {
		model.DB = old
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
}
func miniMaxFixtureRoute() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{UserId: 7, ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 49, ApiKey: "account-a", ChannelBaseUrl: "https://api.minimax.chat", UpstreamModelName: "speech-02-hd"}}
}

func TestMiniMaxResourceAccountIsolation(t *testing.T) {
	miniMaxFixtureDB(t)
	info := miniMaxFixtureRoute()
	require.NoError(t, BindResource(info, "file", "9223372036854775807"))
	require.NoError(t, CheckResourceOwnership(info, "file", "9223372036854775807"))
	require.NoError(t, BindResource(info, "file", "9223372036854775807"))
	info.ApiKey = "account-b"
	require.Error(t, CheckResourceOwnership(info, "file", "9223372036854775807"))
	require.Error(t, BindResource(info, "file", "9223372036854775807"))
	info.ApiKey = "account-a"
	info.ChannelId = 50
	require.Error(t, CheckResourceOwnership(info, "file", "9223372036854775807"))
	info.ChannelId = 49
	info.UserId = 8
	require.Error(t, CheckResourceOwnership(info, "file", "9223372036854775807"))
	info.UserId = 7
	require.NoError(t, model.BindMiniMaxResource(7, "voice", "custom_voice", 49, info.ChannelBaseUrl, info.ApiKey, "pending"))
	require.Error(t, CheckVoiceOwnership(info, "custom_voice"))
	require.Error(t, model.BindMiniMaxResource(7, "voice", "custom_voice", 49, info.ChannelBaseUrl, info.ApiKey, "pending"))
	require.NoError(t, model.CompleteMiniMaxVoice(7, "custom_voice", 49, info.ChannelBaseUrl, info.ApiKey))
	require.NoError(t, CheckVoiceOwnership(info, "custom_voice"))
	info.UserId = 8
	require.Error(t, CheckVoiceOwnership(info, "custom_voice"))
	require.Error(t, model.BindMiniMaxResource(8, "voice", "custom_voice", 49, info.ChannelBaseUrl, info.ApiKey, "ready"))
	require.NoError(t, CheckVoiceOwnership(info, "provider-managed-voice"))
	require.NoError(t, CheckVoiceOwnership(info, "male-qn-qingse"))
}

func TestMiniMaxUploadAndCloneWireContract(t *testing.T) {
	miniMaxFixtureDB(t)
	service.InitHttpClient()
	client := service.GetHttpClient()
	original := client.Transport
	t.Cleanup(func() { client.Transport = original })
	info := miniMaxFixtureRoute()
	step := 0
	client.Transport = miniMaxTransport(func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "Bearer account-a", req.Header.Get("Authorization"))
		switch step {
		case 0:
			assert.Equal(t, "/v1/files/upload", req.URL.Path)
			require.NoError(t, req.ParseMultipartForm(MaxUploadBytes))
			assert.Equal(t, "voice_clone", req.FormValue("purpose"))
			assert.Empty(t, req.FormValue("model"))
			step++
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"file":{"file_id":9223372036854775807},"base_resp":{"status_code":0}}`)), Header: make(http.Header)}, nil
		default:
			body, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			assert.Contains(t, string(body), `"file_id":9223372036854775807`)
			assert.Contains(t, string(body), `"need_noise_reduce":false`)
			assert.NotContains(t, string(body), `"model"`)
			step++
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"base_resp":{"status_code":0},"extra_info":{"usage_characters":12345}}`)), Header: make(http.Header)}, nil
		}
	})
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "minimax-files-upload"))
	require.NoError(t, writer.WriteField("purpose", "voice_clone"))
	part, err := writer.CreateFormFile("file", "voice.wav")
	require.NoError(t, err)
	_, err = part.Write([]byte("audio fixture"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/v1/files/upload", &body)
	ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())
	usage, apiErr := RelayMiniMaxFilesUpload(ctx, info)
	require.Nil(t, apiErr)
	assert.Zero(t, usage.(*dto.Usage).TotalTokens)
	assert.Contains(t, recorder.Body.String(), `"file_id":"9223372036854775807"`)
	recorder = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/v1/voice_clone", strings.NewReader(`{"file_id":"9223372036854775807","voice_id":"custom_voice","need_noise_reduce":false}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	usage, apiErr = RelayMiniMaxVoiceClone(ctx, info)
	require.Nil(t, apiErr)
	assert.Zero(t, usage.(*dto.Usage).TotalTokens)
	require.NoError(t, CheckVoiceOwnership(info, "custom_voice"))
	assert.Equal(t, 2, step)
}

func TestMiniMaxTTSMappingExplicitZerosAndUsage(t *testing.T) {
	miniMaxFixtureDB(t)
	info := miniMaxFixtureRoute()
	info.RelayMode = relayconstant.RelayModeAudioSpeech
	info.OriginModelName = "customer-speech"
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	reader, err := (&Adaptor{}).ConvertAudioRequest(ctx, info, dto.AudioRequest{Input: "测试", Voice: "male-qn-qingse", ResponseFormat: "wav", Metadata: []byte(`{"model":"unpriced-model","text":"unpriced text","voice_setting":{"pitch":0,"text_normalization":false},"aigc_watermark":false}`)})
	require.NoError(t, err)
	raw, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"model":"speech-02-hd"`)
	assert.Contains(t, string(raw), `"text":"测试"`)
	assert.Contains(t, string(raw), `"pitch":0`)
	assert.Contains(t, string(raw), `"text_normalization":false`)
	assert.Contains(t, string(raw), `"aigc_watermark":false`)
	for _, characters := range []int{2, 0, -1, 10001} {
		t.Run(fmt.Sprint(characters), func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Set("minimax_audio_format", "wav")
			response := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"data":{"audio":"52494646"},"base_resp":{"status_code":0},"extra_info":{"usage_characters":%d}}`, characters)))}
			usage, apiErr := handleTTSResponse(c, response, info)
			if characters != 2 {
				require.NotNil(t, apiErr)
				assert.False(t, c.Writer.Written())
				return
			}
			require.Nil(t, apiErr)
			assert.Equal(t, 2, usage.(*dto.Usage).PromptTokens)
			assert.Equal(t, "audio/wav", recorder.Header().Get("Content-Type"))
			assert.Equal(t, "RIFF", recorder.Body.String())
		})
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	response := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":{},"base_resp":{"status_code":1004,"status_msg":"invalid voice"},"extra_info":{"usage_characters":0}}`))}
	_, apiErr := handleTTSResponse(c, response, info)
	require.NotNil(t, apiErr)
	assert.Contains(t, apiErr.Error(), "invalid voice")
}

func TestMiniMaxFileIDRejectsInvalidAndPreservesInt64(t *testing.T) {
	for _, raw := range []string{`9223372036854775807`, `"9223372036854775807"`} {
		var file MiniMaxFileObject
		require.NoError(t, common.Unmarshal([]byte(`{"file_id":`+raw+`}`), &file))
		assert.Equal(t, "9223372036854775807", file.FileID)
	}
	var file MiniMaxFileObject
	require.Error(t, common.Unmarshal([]byte(`{"file_id":1.5}`), &file))
}
