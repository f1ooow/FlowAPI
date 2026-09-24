package volcengine

import (
	"io"
	"net/http"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
)

func TestVolcengineAuthAndEncodingContract(t *testing.T) {
	app, token, err := parseVolcengineAuth("app|token")
	require.NoError(t, err)
	assert.Equal(t, "app", app)
	assert.Equal(t, "token", token)
	_, _, err = parseVolcengineAuth("app|token|extra")
	assert.Error(t, err)
	assert.Equal(t, "audio/mpeg", getContentTypeByEncoding("mp3"))
	assert.Equal(t, "audio/pcm", getContentTypeByEncoding("pcm"))
}

func TestVolcengineTTSBinaryResponseRejectsProtocolErrors(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	info := &relaycommon.RelayInfo{InputAudioFormat: "", OutputAudioFormat: ""}
	response := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{\"code\":1,\"message\":\"bad\"}"))}
	_, apiErr := handleTTSResponse(c, response, info, "mp3")
	require.Error(t, apiErr)
	assert.False(t, recorder.Flushed)

	recorder = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(recorder)
	response = &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{\"code\":3000,\"data\":\"aGVsbG8=\",\"sequence\":-1}"))}
	_, apiErr = handleTTSResponse(c, response, info, "mp3")
	assert.Nil(t, apiErr)
	assert.Equal(t, "audio/mpeg", recorder.Header().Get("Content-Type"))
	assert.Equal(t, "hello", recorder.Body.String())
}

func TestSeedTTSHeaderAndASRResultBounds(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ApiKey: "app|token"}}
	header := http.Header{}
	require.NoError(t, setupSeedTTSV3Headers(&header, info))
	assert.Equal(t, "app", header.Get("X-Api-App-Id"))
	assert.Equal(t, "token", header.Get("X-Api-Access-Key"))
	assert.Equal(t, "seed-tts-2.0", header.Get("X-Api-Resource-Id"))
	assert.Equal(t, "application/json", header.Get("Content-Type"))

	valid := &asrQueryResponse{}
	valid.AudioInfo.Duration = 1000
	valid.Result.Utterances = []asrUtterance{{StartTime: 0, EndTime: 1000, Text: "x"}}
	require.NoError(t, validateASRResult(valid))
	valid.Result.Utterances[0].EndTime = 2000
	assert.Error(t, validateASRResult(valid))
	valid.Result.Utterances[0].EndTime = 1000
	valid.AudioInfo.Duration = 0
	assert.Error(t, validateASRResult(valid))

	request := dto.AudioRequest{Input: "hello", Voice: "speaker", ResponseFormat: "mp3"}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	reader, err := convertSeedTTSV3AudioRequest(ctx, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "doubao-tts-2.0"}}, request)
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"enable_subtitle":false`)
}
