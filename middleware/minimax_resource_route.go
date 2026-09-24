package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/minimax"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"gorm.io/gorm"
)

// miniMaxRequestBinding identifies user-owned resources before normal channel
// selection. Unknown voices still undergo provider system-voice validation.
func miniMaxRequestBinding(c *gin.Context, modelName string) (*minimax.ResourceBinding, error) {
	if c.Request.Method != http.MethodPost || !strings.Contains(c.Request.Header.Get("Content-Type"), "application/json") {
		return nil, nil
	}
	isSpeech := c.Request.URL.Path == "/v1/audio/speech" && strings.HasPrefix(modelName, "speech-")
	isClone := c.Request.URL.Path == "/v1/voice_clone"
	isVideo := strings.Contains(c.Request.URL.Path, "/video")
	if !isSpeech && !isClone && !isVideo {
		return nil, nil
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, err
	}
	raw, err := storage.Bytes()
	if err != nil {
		return nil, err
	}
	type resource struct {
		kind, id string
		required bool
	}
	var resources []resource
	if isClone {
		fileID := gjson.GetBytes(raw, "file_id").String()
		if fileID != "" {
			resources = append(resources, resource{"file", fileID, true})
		}
	}
	if isSpeech {
		voice := gjson.GetBytes(raw, "metadata.voice_setting.voice_id")
		if !voice.Exists() {
			voice = gjson.GetBytes(raw, "voice")
		}
		if voice.String() != "" {
			resources = append(resources, resource{"voice", voice.String(), gjson.GetBytes(raw, "metadata.voice_source").String() == "custom"})
		}
		for _, weight := range gjson.GetBytes(raw, "metadata.timbre_weights").Array() {
			if id := weight.Get("voice_id").String(); id != "" {
				resources = append(resources, resource{"voice", id, false})
			}
		}
	}
	if isVideo {
		for _, path := range []string{"image", "images", "metadata.first_frame_image", "metadata.last_frame_image", "metadata.reference_images", "metadata.reference_videos", "metadata.reference_audios", "content"} {
			values := []gjson.Result{gjson.GetBytes(raw, path)}
			for len(values) > 0 {
				v := values[0]
				values = values[1:]
				if v.IsArray() {
					values = append(values, v.Array()...)
				} else if v.IsObject() {
					for key, item := range v.Map() {
						if key != "text" {
							values = append(values, item)
						}
					}
				} else if v.Type == gjson.String && strings.HasPrefix(v.String(), "mm_file://") {
					resources = append(resources, resource{"file", strings.TrimPrefix(v.String(), "mm_file://"), true})
				}
			}
		}
	}
	var bound *minimax.ResourceBinding
	for _, r := range resources {
		binding, err := minimax.LookupResourceBinding(c.GetInt("id"), r.kind, r.id)
		if errors.Is(err, gorm.ErrRecordNotFound) && !r.required {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("MiniMax %s is not available to this user", r.kind)
		}
		if bound != nil && (bound.ChannelID != binding.ChannelID || bound.CredentialHash != binding.CredentialHash) {
			return nil, errors.New("MiniMax resources must belong to the same provider account")
		}
		bound = binding
	}
	return bound, nil
}

// selectMiniMaxResourceChannel applies the same model/group permissions as
// ordinary distribution before pinning the channel and its original credential.
func selectMiniMaxResourceChannel(c *gin.Context, binding *minimax.ResourceBinding, modelName, usingGroup string) (*model.Channel, error) {
	if common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled) {
		limits, _ := common.GetContextKey(c, constant.ContextKeyTokenModelLimit)
		allowed, _ := limits.(map[string]bool)
		if !allowed[ratio_setting.FormatMatchingModelName(modelName)] {
			return nil, errors.New("token does not allow this model")
		}
	}
	ch, err := model.GetChannelById(binding.ChannelID, true)
	if err != nil || ch.Status != common.ChannelStatusEnabled || !channelSupportsRequestPath(ch, c.Request.URL.Path, modelName) {
		return nil, errors.New("original MiniMax channel is unavailable")
	}
	groups := []string{usingGroup}
	if usingGroup == "auto" {
		groups = service.GetRequestAutoGroups(c, common.GetContextKeyString(c, constant.ContextKeyUserGroup))
	}
	selected := ""
	for _, group := range groups {
		if model.IsChannelEnabledForGroupModel(group, modelName, ch.Id) {
			selected = group
			break
		}
	}
	if selected == "" {
		return nil, errors.New("original MiniMax channel is not enabled for this group and model")
	}
	if usingGroup == "auto" {
		common.SetContextKey(c, constant.ContextKeyAutoGroup, selected)
	}
	keys := []string{ch.Key}
	if ch.ChannelInfo.IsMultiKey {
		keys = ch.GetKeys()
	}
	for index, key := range keys {
		if ch.ChannelInfo.IsMultiKey {
			if status, ok := ch.ChannelInfo.MultiKeyStatusList[index]; ok && status != common.ChannelStatusEnabled {
				continue
			}
		}
		if minimax.ResourceCredentialHash(ch.GetBaseURL(), key) == binding.CredentialHash {
			c.Set("minimax_resource_channel_id", ch.Id)
			c.Set("minimax_resource_key", key)
			c.Set("minimax_resource_key_index", index)
			return ch, nil
		}
	}
	return nil, errors.New("original MiniMax credential is unavailable")
}
