package model

import (
	"math"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
)

func TestChannelCostRatioValidationAndNormalization(t *testing.T) {
	valid := 1.5
	tooHigh := constant.MaxChannelCostRatio + 1
	zero := 0.0
	nan := math.NaN()

	assert.NoError(t, ValidateChannelCostRatio(nil))
	assert.NoError(t, ValidateChannelCostRatio(&valid))
	assert.Error(t, ValidateChannelCostRatio(&tooHigh))
	assert.Error(t, ValidateChannelCostRatio(&zero))
	assert.Error(t, ValidateChannelCostRatio(&nan))
	assert.Equal(t, 1.0, (&Channel{}).GetCostRatio())
	assert.Equal(t, 1.5, (&Channel{CostRatio: &valid}).GetCostRatio())
	assert.Equal(t, 1.0, (&Channel{CostRatio: &tooHigh}).GetCostRatio())
}

func TestBuildChannelCostRatioRangesUsesEnabledCommaSeparatedGroups(t *testing.T) {
	one := 1.0
	low := 0.8
	high := 1.6
	ranges := buildChannelCostRatioRanges([]*Channel{
		{Status: common.ChannelStatusEnabled, Group: "default, premium", CostRatio: &one},
		{Status: common.ChannelStatusEnabled, Group: "premium", CostRatio: &low},
		{Status: common.ChannelStatusEnabled, Group: "premium", CostRatio: &high},
		{Status: common.ChannelStatusManuallyDisabled, Group: "premium", CostRatio: common.GetPointer(9.0)},
	})

	assert.Equal(t, ChannelCostRatioRange{Min: 1, Max: 1, Available: true}, ranges["default"])
	assert.Equal(t, ChannelCostRatioRange{Min: 0.8, Max: 1.6, Available: true}, ranges["premium"])
	assert.False(t, ranges["missing"].Available)
}

func TestCacheUpdateChannelStatusRefreshesCostRatioRange(t *testing.T) {
	previousMemoryCache := common.MemoryCacheEnabled
	channelSyncLock.Lock()
	previousChannels := channelsIDM
	previousRanges := channelCostRatioRanges
	previousGroupChannels := group2model2channels
	low := 0.8
	high := 1.6
	channelsIDM = map[int]*Channel{
		1: {Id: 1, Status: common.ChannelStatusEnabled, Group: "premium", CostRatio: &low},
		2: {Id: 2, Status: common.ChannelStatusEnabled, Group: "premium", CostRatio: &high},
	}
	channelCostRatioRanges = buildChannelCostRatioRanges([]*Channel{channelsIDM[1], channelsIDM[2]})
	group2model2channels = map[string]map[string][]int{}
	channelSyncLock.Unlock()
	common.MemoryCacheEnabled = true
	t.Cleanup(func() {
		common.MemoryCacheEnabled = previousMemoryCache
		channelSyncLock.Lock()
		channelsIDM = previousChannels
		channelCostRatioRanges = previousRanges
		group2model2channels = previousGroupChannels
		channelSyncLock.Unlock()
	})

	CacheUpdateChannelStatus(2, common.ChannelStatusAutoDisabled)
	assert.Equal(t, ChannelCostRatioRange{Min: 0.8, Max: 0.8, Available: true}, GetChannelCostRatioRange("premium"))

	CacheUpdateChannelStatus(2, common.ChannelStatusEnabled)
	assert.Equal(t, ChannelCostRatioRange{Min: 0.8, Max: 1.6, Available: true}, GetChannelCostRatioRange("premium"))
}
