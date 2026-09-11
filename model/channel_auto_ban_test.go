package model

import (
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createAutoBanTestChannel(t *testing.T, threshold int, status int) *Channel {
	t.Helper()
	setupChannelStatusTest(t)

	autoBan := 1
	channel := &Channel{
		Name:    "automatic-ban-state",
		Key:     "test-key",
		Status:  status,
		Models:  "test-model",
		Group:   "default",
		AutoBan: &autoBan,
	}
	channel.SetSetting(dto.ChannelSettings{
		Reliability: &dto.ChannelReliabilitySettings{
			MaxAttempts:         2,
			AutoBanThreshold:    threshold,
			AutoBanDurationSecs: 60,
		},
	})
	require.NoError(t, DB.Create(channel).Error)
	return channel
}

func TestRecordChannelAutoBanFailureBansAtConfiguredThreshold(t *testing.T) {
	channel := createAutoBanTestChannel(t, 2, common.ChannelStatusEnabled)

	first, err := RecordChannelAutoBanFailure(channel.Id, "upstream 502")
	require.NoError(t, err)
	assert.Equal(t, 1, first.ConsecutiveFailures)
	assert.False(t, first.Banned)

	beforeBan := time.Now().Unix()
	second, err := RecordChannelAutoBanFailure(channel.Id, "upstream 502")
	require.NoError(t, err)
	assert.Equal(t, 2, second.ConsecutiveFailures)
	assert.True(t, second.Banned)
	assert.GreaterOrEqual(t, second.BanUntil, beforeBan+60)

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusAutoDisabled, stored.Status)
	assert.Equal(t, float64(2), stored.GetOtherInfo()["auto_ban_failures"])
	assert.Equal(t, second.BanUntil, stored.GetAutoBanUntil())
}

func TestClearChannelAutoBanFailuresResetsEnabledChannel(t *testing.T) {
	channel := createAutoBanTestChannel(t, 5, common.ChannelStatusEnabled)
	_, err := RecordChannelAutoBanFailure(channel.Id, "upstream 502")
	require.NoError(t, err)

	cleared, err := ClearChannelAutoBanFailures(channel.Id)
	require.NoError(t, err)
	assert.True(t, cleared)

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	_, exists := stored.GetOtherInfo()["auto_ban_failures"]
	assert.False(t, exists)
}

func TestClearChannelAutoBanFailuresDoesNotChangeBannedChannel(t *testing.T) {
	channel := createAutoBanTestChannel(t, 5, common.ChannelStatusAutoDisabled)
	info := channel.GetOtherInfo()
	info[autoBanFailuresInfoKey] = 5
	info[autoBanUntilInfoKey] = time.Now().Add(30 * time.Minute).Unix()
	channel.SetOtherInfo(info)
	require.NoError(t, channel.saveStatusState())

	cleared, err := ClearChannelAutoBanFailures(channel.Id)
	require.NoError(t, err)
	assert.False(t, cleared)

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusAutoDisabled, stored.Status)
	assert.Equal(t, 5, stored.GetAutoBanFailureCount())
}

func TestRecordChannelAutoBanFailureIgnoresOptedOutChannel(t *testing.T) {
	channel := createAutoBanTestChannel(t, 5, common.ChannelStatusEnabled)
	disabled := 0
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", channel.Id).Update("auto_ban", disabled).Error)

	result, err := RecordChannelAutoBanFailure(channel.Id, "upstream 502")
	require.NoError(t, err)
	assert.Zero(t, result.ConsecutiveFailures)

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	assert.Zero(t, stored.GetAutoBanFailureCount())
}

func TestConcurrentChannelFailuresDoNotLoseIncrements(t *testing.T) {
	channel := createAutoBanTestChannel(t, 10, common.ChannelStatusEnabled)

	var wait sync.WaitGroup
	errs := make(chan error, 5)
	for range 5 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := RecordChannelAutoBanFailure(channel.Id, "upstream 502")
			errs <- err
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	assert.Equal(t, float64(5), stored.GetOtherInfo()["auto_ban_failures"])
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
}

func TestManualDisableNeverRestoresFromAutomaticBanMetadata(t *testing.T) {
	channel := createAutoBanTestChannel(t, 5, common.ChannelStatusAutoDisabled)
	info := channel.GetOtherInfo()
	info[autoBanUntilInfoKey] = time.Now().Add(-time.Minute).Unix()
	info[autoBanFailuresInfoKey] = 5
	channel.SetOtherInfo(info)
	require.NoError(t, channel.saveStatusState())
	require.True(t, UpdateChannelStatus(channel.Id, "", common.ChannelStatusManuallyDisabled, "manual operation"))

	assert.False(t, MaybeRestoreAutoBannedChannel(channel))

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, stored.Status)
	assert.Zero(t, stored.GetAutoBanUntil())
	assert.Zero(t, stored.GetAutoBanFailureCount())
}
