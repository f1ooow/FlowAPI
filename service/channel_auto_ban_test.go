package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAutoBanServiceTest(t *testing.T) *model.Channel {
	t.Helper()

	originalDB := model.DB
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalAutomaticDisableEnabled := common.AutomaticDisableChannelEnabled

	db, err := gorm.Open(sqlite.Open("file:auto-ban-service?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	model.DB = db
	common.MemoryCacheEnabled = false
	common.AutomaticDisableChannelEnabled = false

	t.Cleanup(func() {
		model.DB = originalDB
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		common.AutomaticDisableChannelEnabled = originalAutomaticDisableEnabled
	})

	autoBan := 1
	channel := &model.Channel{
		Name:    "automatic-ban-test",
		Key:     "test-key",
		Status:  common.ChannelStatusEnabled,
		Models:  "test-model",
		Group:   "default",
		AutoBan: &autoBan,
	}
	channel.SetSetting(dto.ChannelSettings{
		Reliability: &dto.ChannelReliabilitySettings{
			MaxAttempts:         2,
			AutoBanThreshold:    5,
			AutoBanDurationSecs: 1800,
		},
	})
	require.NoError(t, db.Create(channel).Error)
	return channel
}

func TestRecordAutoBanFailureDoesNotDependOnLegacyGlobalSwitch(t *testing.T) {
	channel := setupAutoBanServiceTest(t)

	RecordAutoBanFailure(types.ChannelError{
		ChannelId:   channel.Id,
		ChannelName: channel.Name,
		AutoBan:     true,
	}, "upstream 502")

	var stored model.Channel
	require.NoError(t, model.DB.First(&stored, channel.Id).Error)
	assert.Equal(t, float64(1), stored.GetOtherInfo()["auto_ban_failures"])
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
}

func TestRecordAutoBanSuccessClearsConsecutiveFailures(t *testing.T) {
	channel := setupAutoBanServiceTest(t)
	channelError := types.ChannelError{
		ChannelId:   channel.Id,
		ChannelName: channel.Name,
		AutoBan:     true,
	}
	RecordAutoBanFailure(channelError, "upstream 502")

	RecordAutoBanSuccess(channel.Id)

	var stored model.Channel
	require.NoError(t, model.DB.First(&stored, channel.Id).Error)
	_, hasFailureCount := stored.GetOtherInfo()["auto_ban_failures"]
	assert.False(t, hasFailureCount)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
}
