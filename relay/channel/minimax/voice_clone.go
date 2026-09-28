package minimax

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

// MaxClonePreviewCharacters is MiniMax's limit for the clone preview text.
const MaxClonePreviewCharacters = 1000

// MiniMaxFileID accepts the provider's integer form and this gateway's string
// form (upload responses return strings to survive JavaScript numbers). It is
// always encoded upstream as an exact JSON integer.
type MiniMaxFileID int64

func (id *MiniMaxFileID) UnmarshalJSON(data []byte) error {
	text := strings.TrimSpace(string(data))
	if strings.HasPrefix(text, `"`) {
		if err := common.Unmarshal(data, &text); err != nil {
			return err
		}
		text = strings.TrimSpace(text)
	}
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil || value <= 0 {
		return fmt.Errorf("file_id must be a positive 64-bit integer")
	}
	*id = MiniMaxFileID(value)
	return nil
}

type MiniMaxClonePrompt struct {
	PromptAudio MiniMaxFileID `json:"prompt_audio"`
	PromptText  string        `json:"prompt_text,omitempty"`
}

type MiniMaxVoiceCloneRequest struct {
	FileID                MiniMaxFileID       `json:"file_id"`
	VoiceID               string              `json:"voice_id"`
	ClonePrompt           *MiniMaxClonePrompt `json:"clone_prompt,omitempty"`
	Text                  string              `json:"text,omitempty"`
	Model                 string              `json:"model,omitempty"`
	TextValidation        *string             `json:"text_validation,omitempty"`
	LanguageBoost         *string             `json:"language_boost,omitempty"`
	NeedNoiseReduction    *bool               `json:"need_noise_reduction,omitempty"`
	LegacyNeedNoiseReduce *bool               `json:"need_noise_reduce,omitempty"`
	NeedVolumeNormal      *bool               `json:"need_volume_normalization,omitempty"`
	Accuracy              *float64            `json:"accuracy,omitempty"`
	AigcWatermark         *bool               `json:"aigc_watermark,omitempty"`
}

// MiniMax rejects voice IDs ending in '-' or '_'.
var voiceIDPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{6,254}[A-Za-z0-9]$`)

func RelayMiniMaxVoiceClone(c *gin.Context, info *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	var request MiniMaxVoiceCloneRequest
	if err := common.UnmarshalBodyReusable(c, &request); err != nil {
		return nil, miniMaxError(err, http.StatusBadRequest)
	}
	if request.FileID <= 0 || !voiceIDPattern.MatchString(request.VoiceID) || utf8.RuneCountInString(request.Text) > MaxClonePreviewCharacters {
		return nil, miniMaxError(fmt.Errorf("invalid file_id, voice_id or text length (maximum %d characters)", MaxClonePreviewCharacters), http.StatusBadRequest)
	}
	if request.Accuracy != nil && (*request.Accuracy < 0 || *request.Accuracy > 1) {
		return nil, miniMaxError(fmt.Errorf("accuracy must be 0..1"), http.StatusBadRequest)
	}
	if err := CheckResourceOwnership(info, "file", strconv.FormatInt(int64(request.FileID), 10)); err != nil {
		return nil, miniMaxError(err, http.StatusConflict)
	}
	if request.ClonePrompt != nil {
		if request.ClonePrompt.PromptAudio <= 0 {
			return nil, miniMaxError(fmt.Errorf("clone_prompt.prompt_audio is required"), http.StatusBadRequest)
		}
		if err := CheckResourceOwnership(info, "file", strconv.FormatInt(int64(request.ClonePrompt.PromptAudio), 10)); err != nil {
			return nil, miniMaxError(err, http.StatusConflict)
		}
	}
	if request.NeedNoiseReduction == nil {
		request.NeedNoiseReduction = request.LegacyNeedNoiseReduce
	}
	request.LegacyNeedNoiseReduce = nil
	if request.Text == "" {
		request.Model = ""
	} else if request.Model == "minimax-voice-clone" {
		return nil, miniMaxError(fmt.Errorf("preview text requires a speech model"), http.StatusBadRequest)
	}
	raw, err := common.Marshal(request)
	if err != nil {
		return nil, miniMaxError(err, http.StatusBadRequest)
	}
	// A duplicate/pending clone is never re-sent automatically.
	if err := model.BindMiniMaxResource(info.UserId, "voice", request.VoiceID, info.ChannelId, info.ChannelBaseUrl, info.ApiKey, "pending"); err != nil {
		return nil, miniMaxError(err, http.StatusConflict)
	}

	response, apiErr := callMiniMaxExtended(c, info, "/v1/voice_clone", "application/json", bytes.NewReader(raw))
	if apiErr != nil {
		// Only a definitive refusal frees the name; an unknown outcome keeps the
		// reservation so a possibly created voice is not cloned and paid twice.
		var rejection miniMaxRejection
		if errors.As(apiErr, &rejection) {
			if err := model.ReleasePendingMiniMaxVoice(info.UserId, request.VoiceID, info.ChannelId, info.ChannelBaseUrl, info.ApiKey); err != nil {
				common.SysError("failed to release rejected MiniMax voice_id " + request.VoiceID + ": " + err.Error())
			}
		}
		return nil, apiErr
	}
	if err := model.CompleteMiniMaxVoice(info.UserId, request.VoiceID, info.ChannelId, info.ChannelBaseUrl, info.ApiKey); err != nil {
		return nil, miniMaxError(err, http.StatusInternalServerError)
	}
	c.Data(http.StatusOK, "application/json", response)
	// Clone is per call. Upstream usage_characters never changes that contract.
	return &dto.Usage{}, nil
}
