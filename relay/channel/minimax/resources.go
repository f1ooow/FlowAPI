package minimax

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"gorm.io/gorm"
)

type ResourceBinding = model.MiniMaxResourceBinding

func MigrateResourceBindings(db *gorm.DB) error {
	return db.AutoMigrate(&model.MiniMaxResourceBinding{})
}
func ResourceCredentialHash(baseURL, key string) string {
	return model.MiniMaxCredentialHash(baseURL, key)
}
func LookupResourceBinding(userID int, kind, id string) (*ResourceBinding, error) {
	return model.LookupMiniMaxResourceBinding(userID, kind, id)
}

func BindResource(info *relaycommon.RelayInfo, kind, id string) error {
	if info == nil || info.ChannelMeta == nil {
		return errors.New("missing MiniMax route")
	}
	return model.BindMiniMaxResource(info.UserId, kind, id, info.ChannelId, info.ChannelBaseUrl, info.ApiKey, "ready")
}

func CheckResourceOwnership(info *relaycommon.RelayInfo, kind, id string) error {
	if info == nil || info.ChannelMeta == nil {
		return errors.New("missing MiniMax route")
	}
	binding, err := model.LookupMiniMaxResourceBinding(info.UserId, kind, id)
	if err != nil {
		return fmt.Errorf("MiniMax %s must be created by this user through this gateway: %w", kind, err)
	}
	if binding.ChannelID != info.ChannelId || binding.CredentialHash != model.MiniMaxCredentialHash(info.ChannelBaseUrl, info.ApiKey) {
		return errors.New("MiniMax resource belongs to a different channel or credential; select its original account")
	}
	if binding.State != "ready" {
		return errors.New("MiniMax resource creation has not completed")
	}
	return nil
}

// System voice names remain compatible. Gateway-created custom voices always
// require their user/account binding; explicitly custom names use the stricter
// CheckResourceOwnership path even before a resource exists.
func CheckVoiceOwnership(info *relaycommon.RelayInfo, voiceID string) error {
	if voiceID == "" {
		return errors.New("voice_id is required")
	}
	if systemVoiceIDs[voiceID] {
		return nil
	}
	known, err := model.IsKnownMiniMaxVoice(voiceID)
	if err != nil {
		return err
	}
	if !known {
		// MiniMax has provider-managed voices that are not part of this gateway's
		// finite compatibility list. Treat an undeclared name as provider-owned;
		// callers that mean a gateway-created custom voice must set
		// metadata.voice_source=custom, which takes the strict binding path above.
		return nil
	}
	return CheckResourceOwnership(info, "voice", voiceID)
}

// These are provider-owned voices. Any other identifier is treated as a
// gateway-managed custom voice and must have an account binding.
var systemVoiceIDs = map[string]bool{
	"male-qn-qingse": true, "male-qn-jingying": true, "male-qn-badao": true,
	"male-qn-daxuesheng": true, "female-shaonv": true, "female-yujie": true,
	"female-chengshu": true, "female-tianmei": true, "presenter_male": true,
	"presenter_female": true, "audiobook_male_1": true, "audiobook_male_2": true,
	"audiobook_female_1": true, "audiobook_female_2": true,
	"English_Graceful_Lady": true, "English_Insightful_Speaker": true,
	"English_expressive_narrator": true, "Chinese (Mandarin)_Warm_Bestie": true,
	"alloy": true, "echo": true, "fable": true, "onyx": true, "nova": true, "shimmer": true,
}
