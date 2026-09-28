package minimax

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/claude"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"math"
	"strings"
	"unicode/utf8"
)

type Adaptor struct {
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.ClaudeRequest) (any, error) {
	adaptor := claude.Adaptor{}
	return adaptor.ConvertClaudeRequest(c, info, req)
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	if info.RelayMode != constant.RelayModeAudioSpeech {
		return nil, errors.New("unsupported audio relay mode")
	}

	minimaxRequest := MiniMaxTTSRequest{
		Model: info.UpstreamModelName, Text: request.Input,
		VoiceSetting: VoiceSetting{VoiceID: request.Voice, Speed: request.Speed},
		AudioSetting: &AudioSetting{Format: request.ResponseFormat}, OutputFormat: "hex",
	}
	if minimaxRequest.Model == "" {
		minimaxRequest.Model = info.OriginModelName
	}
	if len(request.Metadata) > 0 {
		if err := common.Unmarshal(request.Metadata, &minimaxRequest); err != nil {
			return nil, fmt.Errorf("invalid MiniMax metadata: %w", err)
		}
	}
	// Metadata cannot replace the priced text or mapped model.
	minimaxRequest.Model = info.UpstreamModelName
	if minimaxRequest.Model == "" {
		minimaxRequest.Model = info.OriginModelName
	}
	minimaxRequest.Text = request.Input
	if strings.TrimSpace(minimaxRequest.Text) == "" || utf8.RuneCountInString(minimaxRequest.Text) > MaxSpeechCharacters {
		return nil, fmt.Errorf("input must contain 1..%d characters", MaxSpeechCharacters)
	}
	if minimaxRequest.Stream != nil && *minimaxRequest.Stream {
		return nil, fmt.Errorf("streaming MiniMax speech is not supported")
	}
	if len(minimaxRequest.TimbreWeights) > 4 {
		return nil, fmt.Errorf("at most four timbre weights are supported")
	}
	if minimaxRequest.VoiceSetting.Speed != nil {
		speed := *minimaxRequest.VoiceSetting.Speed
		if math.IsNaN(speed) || math.IsInf(speed, 0) || speed < 0.5 || speed > 2 {
			return nil, fmt.Errorf("speech speed must be 0.5..2")
		}
	}
	if minimaxRequest.VoiceSource != "" && minimaxRequest.VoiceSource != "system" && minimaxRequest.VoiceSource != "custom" {
		return nil, fmt.Errorf("voice_source must be system or custom")
	}
	if minimaxRequest.VoiceSource == "custom" {
		if err := CheckResourceOwnership(info, "voice", minimaxRequest.VoiceSetting.VoiceID); err != nil {
			return nil, err
		}
	}
	minimaxRequest.VoiceSource = ""
	if err := CheckVoiceOwnership(info, minimaxRequest.VoiceSetting.VoiceID); err != nil {
		return nil, err
	}
	for _, timbre := range minimaxRequest.TimbreWeights {
		if timbre.Weight < 1 || timbre.Weight > 100 {
			return nil, fmt.Errorf("timbre weight must be 1..100")
		}
		if err := CheckVoiceOwnership(info, timbre.VoiceID); err != nil {
			return nil, err
		}
	}
	if minimaxRequest.OutputFormat != "url" && minimaxRequest.OutputFormat != "hex" {
		return nil, fmt.Errorf("output_format must be hex or url")
	}
	if minimaxRequest.AudioSetting == nil {
		minimaxRequest.AudioSetting = &AudioSetting{}
	}
	if minimaxRequest.AudioSetting.Format == "" {
		minimaxRequest.AudioSetting.Format = "mp3"
	}
	switch minimaxRequest.AudioSetting.Format {
	case "mp3", "wav", "flac", "aac", "pcm":
	default:
		return nil, fmt.Errorf("unsupported audio format")
	}
	billingCharacters := miniMaxBillingCharacters(minimaxRequest.Text)
	c.Set(miniMaxTTSBillingCharactersContextKey, billingCharacters)
	// The generic estimate counts runes; MiniMax bills Han characters twice.
	// Reserve on the same unit that settlement uses.
	if !info.PriceData.UsePrice && info.TieredBillingSnapshot == nil {
		info.PriceData.PreConsumeQuotaBeforeGroup = float64(max(billingCharacters, common.PreConsumedQuota)) * info.PriceData.ModelRatio
		if apiErr := service.PrepareBillingForSelectedRoute(c, info); apiErr != nil {
			return nil, apiErr
		}
	}
	c.Set("minimax_audio_format", minimaxRequest.AudioSetting.Format)
	c.Set("response_format", minimaxRequest.OutputFormat)
	c.Set(miniMaxTTSSubtitleEnabledContextKey, minimaxRequest.SubtitleEnable != nil && *minimaxRequest.SubtitleEnable)
	jsonData, err := common.Marshal(minimaxRequest)
	if err != nil {
		return nil, err
	}

	return bytes.NewReader(jsonData), nil
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	if info.RelayMode != constant.RelayModeImagesGenerations {
		return nil, fmt.Errorf("unsupported image relay mode: %d", info.RelayMode)
	}
	return oaiImage2MiniMaxImageRequest(request), nil
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return GetRequestURL(info)
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return request, nil
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, errors.New("not implemented")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if info.RelayMode == constant.RelayModeAudioSpeech {
		return handleTTSResponse(c, resp, info)
	}
	if info.RelayMode == constant.RelayModeImagesGenerations {
		return miniMaxImageHandler(c, resp, info)
	}

	switch info.RelayFormat {
	case types.RelayFormatClaude:
		adaptor := claude.Adaptor{}
		return adaptor.DoResponse(c, resp, info)
	default:
		adaptor := openai.Adaptor{}
		return adaptor.DoResponse(c, resp, info)
	}
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
