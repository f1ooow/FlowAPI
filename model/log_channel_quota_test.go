package model

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSumUsedQuotaByChannelIds(t *testing.T) {
	previousLogDB := LOG_DB
	previousLogDatabaseType := common.LogDatabaseType()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	LOG_DB = db
	common.SetLogDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		LOG_DB = previousLogDB
		common.SetLogDatabaseType(previousLogDatabaseType)
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&Log{}))

	logs := []Log{
		{ChannelId: 11, CreatedAt: 100, Type: LogTypeConsume, Quota: 10},
		{ChannelId: 11, CreatedAt: 150, Type: LogTypeConsume, Quota: 20},
		{ChannelId: 12, CreatedAt: 200, Type: LogTypeConsume, Quota: 7},
		{ChannelId: 11, CreatedAt: 99, Type: LogTypeConsume, Quota: 100},
		{ChannelId: 11, CreatedAt: 201, Type: LogTypeConsume, Quota: 100},
		{ChannelId: 11, CreatedAt: 150, Type: LogTypeRefund, Quota: 50},
		{ChannelId: 13, CreatedAt: 150, Type: LogTypeConsume, Quota: 80},
	}
	require.NoError(t, db.Create(&logs).Error)

	quotaByChannel, err := SumUsedQuotaByChannelIds(context.Background(), []int{11, 12, 14, 11, 0, -1}, 100, 200)

	require.NoError(t, err)
	assert.Equal(t, map[int]int64{11: 30, 12: 7}, quotaByChannel)
}
