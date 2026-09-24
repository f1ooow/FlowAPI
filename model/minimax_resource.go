package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type MiniMaxResourceBinding struct {
	ID             string `gorm:"type:varchar(64);primaryKey"`
	UserID         int    `gorm:"index;not null"`
	Kind           string `gorm:"type:varchar(16);index:idx_minimax_resource,priority:1;uniqueIndex:idx_minimax_resource_global,priority:1;not null"`
	ResourceID     string `gorm:"type:varchar(255);index:idx_minimax_resource,priority:2;uniqueIndex:idx_minimax_resource_global,priority:2;not null"`
	ChannelID      int    `gorm:"not null"`
	CredentialHash string `gorm:"type:varchar(64);not null"`
	State          string `gorm:"type:varchar(16);not null"`
}

func (MiniMaxResourceBinding) TableName() string { return "minimax_resource_bindings" }

func MiniMaxCredentialHash(baseURL, key string) string {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.minimax.chat"
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.TrimRight(baseURL, "/")+"\x00"+key)))
}

func miniMaxResourceID(userID int, kind, id string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s\x00%s", userID, kind, id))))
}

func LookupMiniMaxResourceBinding(userID int, kind, id string) (*MiniMaxResourceBinding, error) {
	if DB == nil {
		return nil, errors.New("MiniMax resource store is unavailable")
	}
	var binding MiniMaxResourceBinding
	err := DB.Where("id = ?", miniMaxResourceID(userID, kind, id)).First(&binding).Error
	return &binding, err
}

// IsKnownMiniMaxVoice distinguishes gateway-managed custom voices from system
// voice names without maintaining an incomplete provider system-voice list.
func IsKnownMiniMaxVoice(id string) (bool, error) {
	if DB == nil {
		return false, errors.New("MiniMax resource store is unavailable")
	}
	var count int64
	err := DB.Model(&MiniMaxResourceBinding{}).Where("kind = ? AND resource_id = ?", "voice", id).Count(&count).Error
	return count > 0, err
}

// BindMiniMaxResource never replaces ownership. A pending voice remains tied
// to the account that first sent the clone, even if that response is lost.
func BindMiniMaxResource(userID int, kind, id string, channelID int, baseURL, key, state string) error {
	if DB == nil {
		return errors.New("MiniMax resource store is unavailable")
	}
	if userID <= 0 || channelID <= 0 || key == "" || id == "" || len(id) > 255 || (kind != "file" && kind != "voice") || (state != "pending" && state != "ready") {
		return errors.New("invalid MiniMax resource binding")
	}
	var global MiniMaxResourceBinding
	if err := DB.Where("kind = ? AND resource_id = ?", kind, id).First(&global).Error; err == nil {
		if global.UserID != userID {
			return errors.New("MiniMax resource is already owned by another user")
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	binding := MiniMaxResourceBinding{ID: miniMaxResourceID(userID, kind, id), UserID: userID, Kind: kind, ResourceID: id, ChannelID: channelID, CredentialHash: MiniMaxCredentialHash(baseURL, key), State: state}
	result := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&binding)
	if result.Error != nil {
		return result.Error
	}
	existing, err := LookupMiniMaxResourceBinding(userID, kind, id)
	if err != nil {
		return err
	}
	if existing.ChannelID != channelID || existing.CredentialHash != binding.CredentialHash {
		return errors.New("MiniMax resource belongs to a different channel or credential")
	}
	if state == "pending" && result.RowsAffected == 0 {
		return errors.New("voice_id is already reserved; use a new name or recover the original clone")
	}
	return nil
}

func CompleteMiniMaxVoice(userID int, id string, channelID int, baseURL, key string) error {
	if DB == nil {
		return errors.New("MiniMax resource store is unavailable")
	}
	result := DB.Model(&MiniMaxResourceBinding{}).Where("id = ? AND channel_id = ? AND credential_hash = ? AND state = ?", miniMaxResourceID(userID, "voice", id), channelID, MiniMaxCredentialHash(baseURL, key), "pending").Update("state", "ready")
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
