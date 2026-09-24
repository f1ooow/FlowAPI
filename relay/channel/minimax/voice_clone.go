package minimax

import (
	"bytes"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

type MiniMaxVoiceCloneRequest struct {
	FileID           int64  `json:"file_id,string"`
	VoiceID          string `json:"voice_id"`
	Text             string `json:"text,omitempty"`
	Model            string `json:"model,omitempty"`
	NeedNoiseReduce  *bool  `json:"need_noise_reduce,omitempty"`
	NeedVolumeNormal *bool  `json:"need_volume_normalization,omitempty"`
	Accuracy         *int   `json:"accuracy,omitempty"`
	AigcWatermark    *bool  `json:"aigc_watermark,omitempty"`
}

var voiceIDPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{7,255}$`)

func RelayMiniMaxVoiceClone(c *gin.Context, info *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	var request MiniMaxVoiceCloneRequest
	if err := common.UnmarshalBodyReusable(c, &request); err != nil {
		return nil, miniMaxError(err, http.StatusBadRequest)
	}
	if request.FileID <= 0 || !voiceIDPattern.MatchString(request.VoiceID) || utf8.RuneCountInString(request.Text) > MaxSpeechCharacters {
		return nil, miniMaxError(fmt.Errorf("invalid file_id, voice_id or text length"), http.StatusBadRequest)
	}
	if request.Accuracy != nil && (*request.Accuracy < 0 || *request.Accuracy > 100) {
		return nil, miniMaxError(fmt.Errorf("accuracy must be 0..100"), http.StatusBadRequest)
	}
	if err := CheckResourceOwnership(info, "file", strconv.FormatInt(request.FileID, 10)); err != nil {
		return nil, miniMaxError(err, http.StatusConflict)
	}

	raw, err := common.Marshal(request)
	if err != nil {
		return nil, miniMaxError(err, http.StatusBadRequest)
	}
	var upstream map[string]any
	if err := common.Unmarshal(raw, &upstream); err != nil {
		return nil, miniMaxError(err, http.StatusBadRequest)
	}
	upstream["file_id"] = request.FileID // int64 stays exact when encoded to upstream JSON.
	if request.Text == "" {
		delete(upstream, "model")
	} else if request.Model == "minimax-voice-clone" {
		return nil, miniMaxError(fmt.Errorf("preview text requires a speech model"), http.StatusBadRequest)
	}
	raw, err = common.Marshal(upstream)
	if err != nil {
		return nil, miniMaxError(err, http.StatusBadRequest)
	}
	// A duplicate/pending clone is never re-sent automatically.
	if err := model.BindMiniMaxResource(info.UserId, "voice", request.VoiceID, info.ChannelId, info.ChannelBaseUrl, info.ApiKey, "pending"); err != nil {
		return nil, miniMaxError(err, http.StatusConflict)
	}

	response, apiErr := callMiniMaxExtended(c, info, "/v1/voice_clone", "application/json", bytes.NewReader(raw))
	if apiErr != nil {
		return nil, apiErr
	}
	if err := model.CompleteMiniMaxVoice(info.UserId, request.VoiceID, info.ChannelId, info.ChannelBaseUrl, info.ApiKey); err != nil {
		return nil, miniMaxError(err, http.StatusInternalServerError)
	}
	c.Data(http.StatusOK, "application/json", response)
	// Clone is per call. Upstream usage_characters never changes that contract.
	return &dto.Usage{}, nil
}
