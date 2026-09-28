package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSumChannelQuotaUsage(t *testing.T) {
	db := useIsolatedLogDB(t)

	const (
		ratioOne   = int64(10_000) // 1.0
		ratioOnePt5 = int64(15_000) // 1.5
	)
	logs := []Log{
		// Two rows at 1.5 and one at 1.0 on channel 11: the summed base quota is
		// priced per ratio group, so cost is 20*1.5 + 40*1.5 + ... rather than
		// the whole channel at a single multiplier.
		{ChannelId: 11, CreatedAt: 100, Type: LogTypeConsume, Quota: 10, BaseQuota: 20, ChannelCostRatio: ratioOnePt5},
		{ChannelId: 11, CreatedAt: 150, Type: LogTypeConsume, Quota: 20, BaseQuota: 40, ChannelCostRatio: ratioOnePt5},
		{ChannelId: 12, CreatedAt: 200, Type: LogTypeConsume, Quota: 7, BaseQuota: 7, ChannelCostRatio: ratioOne},
		{ChannelId: 11, CreatedAt: 99, Type: LogTypeConsume, Quota: 100, BaseQuota: 100, ChannelCostRatio: ratioOnePt5},
		{ChannelId: 11, CreatedAt: 201, Type: LogTypeConsume, Quota: 100, BaseQuota: 100, ChannelCostRatio: ratioOnePt5},
		{ChannelId: 11, CreatedAt: 150, Type: LogTypeRefund, Quota: 50},
		{ChannelId: 13, CreatedAt: 150, Type: LogTypeConsume, Quota: 80, BaseQuota: 80, ChannelCostRatio: ratioOnePt5},
	}
	require.NoError(t, db.Create(&logs).Error)

	usageByChannel, err := SumChannelQuotaUsage(context.Background(), []int{11, 12, 14, 11, 0, -1}, 100, 200)

	require.NoError(t, err)
	// Channel 11 in [100, 200]: base 20 + 40 = 60, all at 1.5, so cost 90.
	assert.Equal(t, map[int]ChannelQuotaUsage{
		11: {UsedQuota: 30, BaseQuota: 60, CostQuota: 90},
		12: {UsedQuota: 7, BaseQuota: 7, CostQuota: 7},
	}, usageByChannel)
}

// A log with no recorded cost ratio is priced at the neutral ratio and flagged
// as an estimate, so a total built from inferred values is identifiable rather
// than silently presented as settled.
func TestSumChannelQuotaUsageMarksRowsWithoutSnapshotAsEstimate(t *testing.T) {
	db := useIsolatedLogDB(t)

	logs := []Log{
		{ChannelId: 21, CreatedAt: 100, Type: LogTypeConsume, Quota: 10, BaseQuota: 30, ChannelCostRatio: 10_000},
		{ChannelId: 22, CreatedAt: 100, Type: LogTypeConsume, Quota: 10, BaseQuota: 30},
	}
	require.NoError(t, db.Create(&logs).Error)

	usageByChannel, err := SumChannelQuotaUsage(context.Background(), []int{21, 22}, 0, 1000)
	require.NoError(t, err)

	observed := usageByChannel[21]
	assert.EqualValues(t, 30, observed.CostQuota)
	assert.False(t, observed.CostHasEstimate, "a snapshot was recorded, so the cost is not an estimate")

	inferred := usageByChannel[22]
	assert.EqualValues(t, 30, inferred.CostQuota, "a missing snapshot prices at the neutral ratio")
	assert.True(t, inferred.CostHasEstimate, "a missing snapshot makes the cost an estimate")
}
