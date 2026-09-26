package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSumChannelQuotaUsage(t *testing.T) {
	db := useIsolatedLogDB(t)

	logs := []Log{
		{ChannelId: 11, CreatedAt: 100, Type: LogTypeConsume, Quota: 10, BaseQuota: 20},
		{ChannelId: 11, CreatedAt: 150, Type: LogTypeConsume, Quota: 20, BaseQuota: 40},
		{ChannelId: 12, CreatedAt: 200, Type: LogTypeConsume, Quota: 7, BaseQuota: 7},
		{ChannelId: 11, CreatedAt: 99, Type: LogTypeConsume, Quota: 100, BaseQuota: 100},
		{ChannelId: 11, CreatedAt: 201, Type: LogTypeConsume, Quota: 100, BaseQuota: 100},
		{ChannelId: 11, CreatedAt: 150, Type: LogTypeRefund, Quota: 50},
		{ChannelId: 13, CreatedAt: 150, Type: LogTypeConsume, Quota: 80, BaseQuota: 80},
	}
	require.NoError(t, db.Create(&logs).Error)

	usageByChannel, err := SumChannelQuotaUsage(context.Background(), []int{11, 12, 14, 11, 0, -1}, 100, 200)

	require.NoError(t, err)
	assert.Equal(t, map[int]ChannelQuotaUsage{
		11: {UsedQuota: 30, BaseQuota: 60},
		12: {UsedQuota: 7, BaseQuota: 7},
	}, usageByChannel)
}
