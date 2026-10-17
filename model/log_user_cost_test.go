package model

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSumUserCostQuotaUsesHistoricalSnapshotsByUser(t *testing.T) {
	db := useIsolatedLogDB(t)
	require.NoError(t, db.Create(&[]Log{
		{UserId: 11, Username: "old-name", ChannelId: 991, Type: LogTypeConsume, Quota: 90, BaseQuota: 1, ChannelCostRatio: 5_000},
		{UserId: 11, Username: "new-name", ChannelId: 991, Type: LogTypeConsume, Quota: 80, BaseQuota: 1, ChannelCostRatio: 5_000},
		{UserId: 11, ChannelId: 991, Type: LogTypeConsume, Quota: 70, BaseQuota: 20, ChannelCostRatio: 15_000},
		{UserId: 12, ChannelId: 992, Type: LogTypeConsume, Quota: 60, BaseQuota: 40, ChannelCostRatio: 2_500},
		{UserId: 13, ChannelId: 991, Type: LogTypeConsume, Quota: 50, BaseQuota: 100, ChannelCostRatio: 10_000},
		{UserId: 11, ChannelId: 991, Type: LogTypeRefund, Quota: 40, BaseQuota: 100, ChannelCostRatio: 10_000},
	}).Error)

	usage, err := SumUserCostQuota(context.Background(), []int{11, 12, 14, 11, 0, -1})
	require.NoError(t, err)
	assert.Equal(t, map[int]UserCostQuota{
		11: {CostQuota: 31}, // Two half-quota rows round once, plus 20 at 1.5.
		12: {CostQuota: 10},
	}, usage)

	// Adding, renaming, repricing, and deleting channels must not restate costs.
	require.NoError(t, DB.Create(&Channel{Id: 991, Name: "new-channel", CostRatio: common.GetPointer(9.0)}).Error)
	t.Cleanup(func() { DB.Delete(&Channel{}, 991) })
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", 991).Updates(map[string]interface{}{
		"name": "renamed", "cost_ratio": 3.0,
	}).Error)
	afterEdit, err := SumUserCostQuota(context.Background(), []int{11, 12})
	require.NoError(t, err)
	assert.Equal(t, usage, afterEdit)
	require.NoError(t, DB.Delete(&Channel{}, 991).Error)
	afterDelete, err := SumUserCostQuota(context.Background(), []int{11, 12})
	require.NoError(t, err)
	assert.Equal(t, usage, afterDelete)
}

func TestSumUserCostQuotaExcludesUnverifiableHistory(t *testing.T) {
	db := useIsolatedLogDB(t)
	tests := []struct {
		name      string
		base      interface{}
		ratio     interface{}
		estimated interface{}
		quota     int
		want      UserCostQuota
	}{
		{"observed", int64(20), int64(15_000), false, 60, UserCostQuota{CostQuota: 30}},
		{"estimated", int64(20), int64(10_000), true, 60, UserCostQuota{CostIncomplete: true}},
		{"missing snapshot", int64(20), int64(0), false, 60, UserCostQuota{CostIncomplete: true}},
		{"negative snapshot", int64(20), int64(-1), false, 60, UserCostQuota{CostIncomplete: true}},
		{"missing base", int64(0), int64(10_000), false, 60, UserCostQuota{CostIncomplete: true}},
		{"free group has no recoverable base", int64(0), int64(10_000), false, 0, UserCostQuota{CostIncomplete: true}},
		{"negative base", int64(-10), int64(10_000), false, 60, UserCostQuota{CostIncomplete: true}},
		{"null snapshot", int64(20), nil, false, 60, UserCostQuota{CostIncomplete: true}},
		{"null base", nil, int64(10_000), false, 60, UserCostQuota{CostIncomplete: true}},
		{"null provenance", int64(20), int64(10_000), nil, 60, UserCostQuota{CostIncomplete: true}},
		{"zero charge with recorded positive base", int64(20), int64(10_000), false, 0, UserCostQuota{CostQuota: 20}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, db.Table("logs").Create(map[string]interface{}{
				"user_id": 21, "type": LogTypeConsume, "quota": test.quota,
				"base_quota": test.base, "channel_cost_ratio": test.ratio,
				"channel_cost_ratio_estimated": test.estimated,
			}).Error)
			usage, err := SumUserCostQuota(context.Background(), []int{21})
			require.NoError(t, err)
			assert.Equal(t, test.want, usage[21])
			require.NoError(t, db.Where("user_id = ?", 21).Delete(&Log{}).Error)
		})
	}

	require.NoError(t, db.Create(&[]Log{
		{UserId: 22, Type: LogTypeConsume, Quota: 60, BaseQuota: 20, ChannelCostRatio: 10_000},
		{UserId: 22, Type: LogTypeConsume, Quota: 60, BaseQuota: 100, ChannelCostRatio: 10_000, ChannelCostRatioEstimated: true},
		{UserId: 22, Type: LogTypeConsume, Quota: 60, ChannelCostRatio: 10_000},
	}).Error)
	usage, err := SumUserCostQuota(context.Background(), []int{22})
	require.NoError(t, err)
	assert.Equal(t, UserCostQuota{CostQuota: 20, CostIncomplete: true}, usage[22])
}

func TestSumUserCostQuotaPreservesLargeTotalsAndSaturatesAcrossRatios(t *testing.T) {
	db := useIsolatedLogDB(t)
	require.NoError(t, db.Create(&[]Log{
		{UserId: 31, Type: LogTypeConsume, BaseQuota: 3_000_000_000, ChannelCostRatio: 10_000},
		{UserId: 32, Type: LogTypeConsume, BaseQuota: 6_000_000_000_000, ChannelCostRatio: 10_000_000},
		{UserId: 32, Type: LogTypeConsume, BaseQuota: 6_000_000_000_000, ChannelCostRatio: 9_000_000},
	}).Error)

	usage, err := SumUserCostQuota(context.Background(), []int{31, 32})
	require.NoError(t, err)
	assert.EqualValues(t, 3_000_000_000, usage[31].CostQuota, "aggregate costs must not use the single-request int32 cap")
	assert.EqualValues(t, common.MaxWalletQuota, usage[32].CostQuota, "the sum of ratio buckets must remain JavaScript-safe")
}
