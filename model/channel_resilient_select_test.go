package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResilientSelectionExhaustsSamePriorityBeforeFallback(t *testing.T) {
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	channelSyncLock.Lock()
	oldChannels := channelsIDM
	highPriority := int64(10)
	lowPriority := int64(5)
	weight := uint(1)
	channelsIDM = map[int]*Channel{
		1: {Id: 1, Status: common.ChannelStatusEnabled, Models: "model", Group: "default", Priority: &highPriority, Weight: &weight},
		2: {Id: 2, Status: common.ChannelStatusEnabled, Models: "model", Group: "default", Priority: &highPriority, Weight: &weight},
		3: {Id: 3, Status: common.ChannelStatusEnabled, Models: "model", Group: "default", Priority: &lowPriority, Weight: &weight},
	}
	channelSyncLock.Unlock()
	t.Cleanup(func() {
		channelSyncLock.Lock()
		channelsIDM = oldChannels
		channelSyncLock.Unlock()
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
	})

	channel, err := GetRandomSatisfiedChannelWithOptions("default", "model", "/v1/messages", ChannelSelectionOptions{ExcludedChannelIDs: map[int]struct{}{1: {}}})
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 2, channel.Id)

	channel, err = GetRandomSatisfiedChannelWithOptions("default", "model", "/v1/messages", ChannelSelectionOptions{ExcludedChannelIDs: map[int]struct{}{1: {}, 2: {}}})
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 3, channel.Id)
}

func TestResilientSelectionUsesRedirectRuleForEligibility(t *testing.T) {
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	channelSyncLock.Lock()
	oldChannels := channelsIDM
	priority := int64(10)
	weight := uint(1)
	raw := `[{"match_type":"contains","source":"opus","target":"claude-opus-4-6"}]`
	channelsIDM = map[int]*Channel{
		7: {Id: 7, Status: common.ChannelStatusEnabled, Models: "claude-opus-4-6", ModelMapping: &raw, Group: "default", Priority: &priority, Weight: &weight},
	}
	channelSyncLock.Unlock()
	t.Cleanup(func() {
		channelSyncLock.Lock()
		channelsIDM = oldChannels
		channelSyncLock.Unlock()
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
	})

	channel, err := GetRandomSatisfiedChannelWithOptions("default", "claude-opus-5", "/v1/messages", ChannelSelectionOptions{})
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 7, channel.Id)
}

func TestAdvancedCustomPathAcceptsRedirectTargetModel(t *testing.T) {
	raw := `[{"match_type":"contains","source":"opus","target":"claude-opus-4-6"}]`
	channel := &Channel{Type: constant.ChannelTypeAdvancedCustom, ModelMapping: &raw}
	channel.SetOtherSettings(dto.ChannelOtherSettings{AdvancedCustom: &dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{
		{IncomingPath: "/v1/messages", UpstreamPath: "/v1/chat/completions", Models: []string{"claude-opus-4-6"}},
	}}})

	assert.True(t, channel.SupportsRequestPath("/v1/messages", "claude-opus-5"))
	assert.False(t, channel.SupportsRequestPath("/v1/responses", "claude-opus-5"))
}

func TestResilientSelectionRestoresExpiredAutomaticBan(t *testing.T) {
	channel := createAutoBanTestChannel(t, 5, common.ChannelStatusAutoDisabled)
	info := channel.GetOtherInfo()
	info[autoBanFailuresInfoKey] = 5
	info[autoBanUntilInfoKey] = time.Now().Add(-time.Minute).Unix()
	channel.SetOtherInfo(info)
	require.NoError(t, channel.saveStatusState())
	require.NoError(t, DB.Create(&Ability{
		Group:     channel.Group,
		Model:     channel.Models,
		ChannelId: channel.Id,
		Enabled:   false,
	}).Error)

	common.MemoryCacheEnabled = true
	channelSyncLock.Lock()
	previousChannels := channelsIDM
	channelsIDM = map[int]*Channel{channel.Id: channel}
	channelSyncLock.Unlock()
	t.Cleanup(func() {
		channelSyncLock.Lock()
		channelsIDM = previousChannels
		channelSyncLock.Unlock()
	})

	selected, err := GetRandomSatisfiedChannelWithOptions(
		"default",
		"test-model",
		"/v1/messages",
		ChannelSelectionOptions{},
	)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, channel.Id, selected.Id)
	assert.Equal(t, common.ChannelStatusEnabled, selected.Status)
	assert.Zero(t, selected.GetAutoBanFailureCount())
}
