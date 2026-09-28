package minimax

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMiniMaxUploadRoutingModelByPurpose(t *testing.T) {
	for purpose, want := range map[string]string{
		"voice_clone":            "minimax-voice-clone",
		"prompt_audio":           "minimax-voice-clone",
		"video_generation_input": "MiniMax-H3",
		"video_generation":       "MiniMax-H3",
		"t2a_async_input":        "minimax-files-upload",
		"video_understanding":    "minimax-files-upload",
		"":                       "minimax-files-upload",
	} {
		assert.Equal(t, want, UploadRoutingModel(purpose), purpose)
	}
}

func TestMiniMaxUploadPurposeAndSizeLimits(t *testing.T) {
	previousBodyLimit := constant.MaxRequestBodyMB
	constant.MaxRequestBodyMB = 128
	t.Cleanup(func() { constant.MaxRequestBodyMB = previousBodyLimit })
	const mb = int64(1 << 20)
	tests := []struct {
		purpose, contentType, filename string
		want                           int64
		wantErr                        bool
	}{
		{"voice_clone", "audio/wav", "a.wav", 20 * mb, false},
		{"prompt_audio", "audio/mpeg", "a.mp3", 20 * mb, false},
		{"t2a_async_input", "text/plain", "a.txt", 20 * mb, false},
		{"video_understanding", "video/mp4", "a.mp4", 20 * mb, false},
		{"video_generation_input", "video/mp4", "a.mp4", 50 * mb, false},
		{"video_generation_input", "image/png", "a.png", 30 * mb, false},
		{"video_generation_input", "audio/mpeg", "a.mp3", 15 * mb, false},
		{"video_generation_input", "application/octet-stream", "clip.MP4", 50 * mb, false},
		{"video_generation_input", "application/octet-stream", "notes.txt", 0, true},
		{"video_generation", "image/jpeg", "a.jpg", 30 * mb, false},
		{"unknown", "audio/wav", "a.wav", 0, true},
		{"", "audio/wav", "a.wav", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.purpose+"/"+tt.filename, func(t *testing.T) {
			header := &multipart.FileHeader{Filename: tt.filename, Header: textproto.MIMEHeader{"Content-Type": {tt.contentType}}}
			limit, err := miniMaxUploadLimit(tt.purpose, header)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, limit)
		})
	}
	constant.MaxRequestBodyMB = 32
	limit, err := miniMaxUploadLimit("video_generation_input", &multipart.FileHeader{Filename: "a.mp4", Header: textproto.MIMEHeader{"Content-Type": {"video/mp4"}}})
	require.NoError(t, err)
	assert.Equal(t, 32*mb, limit)
}

func miniMaxUploadRequest(t *testing.T, purpose, contentType string, size int) *gin.Context {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("purpose", purpose))
	part, err := writer.CreatePart(textproto.MIMEHeader{
		"Content-Disposition": {`form-data; name="file"; filename="input"`},
		"Content-Type":        {contentType},
	})
	require.NoError(t, err)
	_, err = part.Write(bytes.Repeat([]byte{1}, size))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/files/upload", &body)
	ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())
	return ctx
}

func TestMiniMaxVideoGenerationInputUploadIsAccepted(t *testing.T) {
	miniMaxFixtureDB(t)
	service.InitHttpClient()
	client := service.GetHttpClient()
	original := client.Transport
	t.Cleanup(func() { client.Transport = original })
	client.Transport = miniMaxTransport(func(req *http.Request) (*http.Response, error) {
		require.NoError(t, req.ParseMultipartForm(1<<20))
		assert.Equal(t, "video_generation_input", req.FormValue("purpose"))
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"file":{"file_id":42},"base_resp":{"status_code":0}}`)), Header: make(http.Header)}, nil
	})
	info := miniMaxFixtureRoute()
	_, apiErr := RelayMiniMaxFilesUpload(miniMaxUploadRequest(t, "video_generation_input", "image/png", 16), info)
	require.Nil(t, apiErr)
	require.NoError(t, CheckResourceOwnership(info, "file", "42"))

	// Audio inputs keep the 15MB official limit even for video generation.
	_, apiErr = RelayMiniMaxFilesUpload(miniMaxUploadRequest(t, "video_generation_input", "audio/mpeg", 15<<20+1), info)
	require.NotNil(t, apiErr)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
}

func TestMiniMaxCloneFileIDAcceptsNumberAndString(t *testing.T) {
	tests := []struct {
		raw     string
		want    MiniMaxFileID
		wantErr bool
	}{
		{`123456789012345678`, 123456789012345678, false},
		{`"123456789012345678"`, 123456789012345678, false},
		{`9223372036854775807`, 9223372036854775807, false},
		{`1.5`, 0, true},
		{`"1.5"`, 0, true},
		{`-3`, 0, true},
		{`0`, 0, true},
		{`9223372036854775808`, 0, true},
		{`"abc"`, 0, true},
		{`null`, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			var request MiniMaxVoiceCloneRequest
			err := common.Unmarshal([]byte(`{"file_id":`+tt.raw+`}`), &request)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, request.FileID)
			raw, err := common.Marshal(request)
			require.NoError(t, err)
			assert.Contains(t, string(raw), fmt.Sprintf(`"file_id":%d`, tt.want))
		})
	}
}

func runMiniMaxClone(t *testing.T, info *relaycommon.RelayInfo, body string, reply func() (*http.Response, error)) (map[string]any, *hostAPIError) {
	t.Helper()
	service.InitHttpClient()
	client := service.GetHttpClient()
	original := client.Transport
	t.Cleanup(func() { client.Transport = original })
	var sent map[string]any
	client.Transport = miniMaxTransport(func(req *http.Request) (*http.Response, error) {
		raw, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.NoError(t, common.Unmarshal(raw, &sent))
		return reply()
	})
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/voice_clone", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	_, apiErr := RelayMiniMaxVoiceClone(ctx, info)
	if apiErr == nil {
		return sent, nil
	}
	return sent, &hostAPIError{status: apiErr.StatusCode, message: apiErr.Error()}
}

type hostAPIError struct {
	status  int
	message string
}

func okResponse(body string) func() (*http.Response, error) {
	return func() (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	}
}

func TestMiniMaxCloneFieldsMatchOfficialContract(t *testing.T) {
	miniMaxFixtureDB(t)
	info := miniMaxFixtureRoute()
	require.NoError(t, BindResource(info, "file", "101"))
	require.NoError(t, BindResource(info, "file", "202"))

	sent, apiErr := runMiniMaxClone(t, info, `{"file_id":101,"voice_id":"clone_voice1","need_noise_reduce":true,"accuracy":0.7,"text_validation":"hello","language_boost":"Chinese","clone_prompt":{"prompt_audio":"202","prompt_text":"hi"}}`, okResponse(`{"base_resp":{"status_code":0}}`))
	require.Nil(t, apiErr)
	assert.Equal(t, true, sent["need_noise_reduction"])
	assert.NotContains(t, sent, "need_noise_reduce")
	assert.Equal(t, 0.7, sent["accuracy"])
	assert.Equal(t, "hello", sent["text_validation"])
	assert.Equal(t, "Chinese", sent["language_boost"])
	assert.Equal(t, map[string]any{"prompt_audio": float64(202), "prompt_text": "hi"}, sent["clone_prompt"])

	// The official name wins over the legacy alias.
	sent, apiErr = runMiniMaxClone(t, info, `{"file_id":"101","voice_id":"clone_voice2","need_noise_reduction":false,"need_noise_reduce":true}`, okResponse(`{"base_resp":{"status_code":0}}`))
	require.Nil(t, apiErr)
	assert.Equal(t, false, sent["need_noise_reduction"])

	rejected := []struct{ name, body string }{
		{"accuracy above 1", `{"file_id":101,"voice_id":"clone_voice3","accuracy":70}`},
		{"accuracy negative", `{"file_id":101,"voice_id":"clone_voice3","accuracy":-0.1}`},
		{"voice_id trailing dash", `{"file_id":101,"voice_id":"clone_voice-"}`},
		{"voice_id trailing underscore", `{"file_id":101,"voice_id":"clone_voice_"}`},
		{"voice_id too short", `{"file_id":101,"voice_id":"abc1234"}`},
		{"preview text too long", `{"file_id":101,"voice_id":"clone_voice3","model":"speech-02-hd","text":"` + strings.Repeat("a", MaxClonePreviewCharacters+1) + `"}`},
	}
	for _, tt := range rejected {
		t.Run(tt.name, func(t *testing.T) {
			_, apiErr := runMiniMaxClone(t, info, tt.body, okResponse(`{"base_resp":{"status_code":0}}`))
			require.NotNil(t, apiErr)
			assert.Equal(t, http.StatusBadRequest, apiErr.status)
		})
	}
	sent, apiErr = runMiniMaxClone(t, info, `{"file_id":101,"voice_id":"clone_voice3","model":"speech-02-hd","text":"`+strings.Repeat("声", MaxClonePreviewCharacters)+`"}`, okResponse(`{"base_resp":{"status_code":0}}`))
	require.Nil(t, apiErr)
	assert.Equal(t, "speech-02-hd", sent["model"])

	// prompt_audio is an account resource: foreign or unknown files are refused before upstream.
	require.NoError(t, model.BindMiniMaxResource(8, "file", "303", 49, info.ChannelBaseUrl, info.ApiKey, "ready"))
	for _, promptAudio := range []string{"303", "404"} {
		_, apiErr = runMiniMaxClone(t, info, `{"file_id":101,"voice_id":"clone_voice4","clone_prompt":{"prompt_audio":`+promptAudio+`}}`, func() (*http.Response, error) {
			t.Fatal("upstream must not be called")
			return nil, nil
		})
		require.NotNil(t, apiErr)
		assert.Equal(t, http.StatusConflict, apiErr.status)
	}
}

func TestMiniMaxCloneRejectionReleasesVoiceID(t *testing.T) {
	miniMaxFixtureDB(t)
	info := miniMaxFixtureRoute()
	require.NoError(t, BindResource(info, "file", "101"))
	body := `{"file_id":101,"voice_id":"retry_voice1"}`

	for _, reply := range []func() (*http.Response, error){
		okResponse(`{"base_resp":{"status_code":2013,"status_msg":"invalid params"}}`),
		func() (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusBadRequest, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
		},
	} {
		_, apiErr := runMiniMaxClone(t, info, body, reply)
		require.NotNil(t, apiErr)
		_, err := model.LookupMiniMaxResourceBinding(info.UserId, "voice", "retry_voice1")
		require.Error(t, err, "rejected clone must not keep the voice_id reserved")
	}

	// Unknown outcomes keep the reservation so the clone is not paid twice.
	for index, reply := range []func() (*http.Response, error){
		func() (*http.Response, error) { return nil, fmt.Errorf("connection reset") },
		func() (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
		},
		okResponse(`not json`),
	} {
		voiceID := fmt.Sprintf("unknown_voice%d", index)
		_, apiErr := runMiniMaxClone(t, info, `{"file_id":101,"voice_id":"`+voiceID+`"}`, reply)
		require.NotNil(t, apiErr)
		binding, err := model.LookupMiniMaxResourceBinding(info.UserId, "voice", voiceID)
		require.NoError(t, err)
		assert.Equal(t, "pending", binding.State)
	}

	_, apiErr := runMiniMaxClone(t, info, body, okResponse(`{"base_resp":{"status_code":0}}`))
	require.Nil(t, apiErr)
	require.NoError(t, CheckVoiceOwnership(info, "retry_voice1"))
}

func TestMiniMaxReleasePendingVoiceKeepsReadyAndForeignBindings(t *testing.T) {
	miniMaxFixtureDB(t)
	info := miniMaxFixtureRoute()
	require.NoError(t, model.BindMiniMaxResource(7, "voice", "ready_voice1", 49, info.ChannelBaseUrl, info.ApiKey, "pending"))
	require.NoError(t, model.CompleteMiniMaxVoice(7, "ready_voice1", 49, info.ChannelBaseUrl, info.ApiKey))
	require.NoError(t, model.ReleasePendingMiniMaxVoice(7, "ready_voice1", 49, info.ChannelBaseUrl, info.ApiKey))
	require.NoError(t, CheckVoiceOwnership(info, "ready_voice1"))

	require.NoError(t, model.BindMiniMaxResource(7, "voice", "pending_voice1", 49, info.ChannelBaseUrl, info.ApiKey, "pending"))
	require.NoError(t, model.ReleasePendingMiniMaxVoice(8, "pending_voice1", 49, info.ChannelBaseUrl, info.ApiKey))
	require.NoError(t, model.ReleasePendingMiniMaxVoice(7, "pending_voice1", 49, info.ChannelBaseUrl, "other-account"))
	_, err := model.LookupMiniMaxResourceBinding(7, "voice", "pending_voice1")
	require.NoError(t, err)
}

func miniMaxTTSResponse(c *gin.Context, info *relaycommon.RelayInfo, localCharacters int, usage string) (*dto.Usage, *hostAPIError) {
	c.Set("minimax_audio_format", "wav")
	c.Set(miniMaxTTSBillingCharactersContextKey, localCharacters)
	extra := ""
	if usage != "" {
		extra = `,"extra_info":{"usage_characters":` + usage + `}`
	}
	response := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":{"audio":"52494646"},"base_resp":{"status_code":0}` + extra + `}`))}
	result, apiErr := handleTTSResponse(c, response, info)
	if apiErr != nil {
		return nil, &hostAPIError{status: apiErr.StatusCode, message: apiErr.Error()}
	}
	return result.(*dto.Usage), nil
}

func TestMiniMaxTTSBillingCharactersBoundUpstreamUsage(t *testing.T) {
	longChinese := strings.Repeat("声", 5001)
	assert.Equal(t, 10002, miniMaxBillingCharacters(longChinese))
	assert.Equal(t, 7, miniMaxBillingCharacters("测试 ok"))

	tests := []struct {
		name       string
		local      int
		usage      string
		wantTokens int
		wantClamp  bool
	}{
		{"over 5000 Han characters uses upstream count", 10002, "10002", 10002, false},
		{"markup within tolerance", 10, "26", 26, false},
		{"missing usage falls back to weighted count", 10002, "", 10002, true},
		{"zero usage falls back to weighted count", 7, "0", 7, true},
		{"negative usage falls back to weighted count", 7, "-5", 7, true},
		{"oversized usage is clamped to weighted count", 7, "999999", 7, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			info := miniMaxFixtureRoute()
			usage, apiErr := miniMaxTTSResponse(c, info, tt.local, tt.usage)
			require.Nil(t, apiErr)
			assert.Equal(t, tt.wantTokens, usage.PromptTokens)
			assert.Equal(t, tt.wantTokens, usage.TotalTokens)
			assert.Equal(t, "RIFF", recorder.Body.String())
			if !tt.wantClamp {
				assert.Nil(t, info.QuotaClamp)
				return
			}
			require.NotNil(t, info.QuotaClamp)
			assert.Equal(t, "MiniMaxUsageCharacters", info.QuotaClamp.Op)
			assert.Equal(t, tt.local, info.QuotaClamp.Clamped)
		})
	}
}

func TestMiniMaxTTSReservesWeightedCharacters(t *testing.T) {
	miniMaxFixtureDB(t)
	info := miniMaxFixtureRoute()
	info.RelayMode = relayconstant.RelayModeAudioSpeech
	info.OriginModelName = "speech-02-hd"
	billing := &ttsBillingRecorder{}
	info.Billing = billing
	info.PriceData = hosttypes.PriceData{ModelRatio: 175, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1.2}}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	input := strings.Repeat("声", 5001)
	_, err := (&Adaptor{}).ConvertAudioRequest(c, info, dto.AudioRequest{Input: input, Voice: "male-qn-qingse"})
	require.NoError(t, err)
	assert.Equal(t, 10002, c.GetInt(miniMaxTTSBillingCharactersContextKey))
	assert.Equal(t, common.QuotaFromFloat(10002*175*1.2), billing.target)
	assert.Equal(t, billing.target, info.PriceData.QuotaToPreConsume)

	// Per-call pricing is untouched by the character estimate.
	priced := miniMaxFixtureRoute()
	priced.RelayMode = relayconstant.RelayModeAudioSpeech
	priced.Billing = &ttsBillingRecorder{}
	priced.PriceData = hosttypes.PriceData{UsePrice: true, ModelPrice: 1, QuotaToPreConsume: 123}
	_, err = (&Adaptor{}).ConvertAudioRequest(c, priced, dto.AudioRequest{Input: input, Voice: "male-qn-qingse"})
	require.NoError(t, err)
	assert.Equal(t, 123, priced.PriceData.QuotaToPreConsume)
	assert.Zero(t, priced.Billing.(*ttsBillingRecorder).target)
}

type ttsBillingRecorder struct{ target int }

func (r *ttsBillingRecorder) Settle(int) error         { return nil }
func (r *ttsBillingRecorder) Refund(*gin.Context)      {}
func (r *ttsBillingRecorder) NeedsRefund() bool        { return false }
func (r *ttsBillingRecorder) GetPreConsumedQuota() int { return r.target }
func (r *ttsBillingRecorder) Reserve(target int) error {
	if target > r.target {
		r.target = target
	}
	return nil
}
