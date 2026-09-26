package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelTodayQuotaTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainDatabaseType, previousLogDatabaseType := common.MainDatabaseType(), common.LogDatabaseType()
	previousLogConsumeEnabled := common.LogConsumeEnabled
	previousRedisEnabled := common.RedisEnabled
	db := setupModelListControllerTestDB(t)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		common.LogConsumeEnabled = previousLogConsumeEnabled
		common.RedisEnabled = previousRedisEnabled
	})
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	common.LogConsumeEnabled = true
	return db
}

func TestChannelTodayTimeRangeUsesUTC8DayBoundary(t *testing.T) {
	tests := []struct {
		name      string
		now       time.Time
		wantStart time.Time
	}{
		{
			name:      "before UTC date rolls into next Beijing day",
			now:       time.Date(2026, time.September, 14, 15, 59, 59, 0, time.UTC),
			wantStart: time.Date(2026, time.September, 13, 16, 0, 0, 0, time.UTC),
		},
		{
			name:      "after Beijing midnight",
			now:       time.Date(2026, time.September, 14, 16, 30, 0, 0, time.UTC),
			wantStart: time.Date(2026, time.September, 14, 16, 0, 0, 0, time.UTC),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			startTimestamp, endTimestamp := channelTodayTimeRange(test.now)

			assert.Equal(t, test.wantStart.Unix(), startTimestamp)
			assert.Equal(t, test.now.Unix(), endTimestamp)
		})
	}
}

// Cost is the official-price amount of the channel's requests multiplied by
// the channel's current cost ratio, for both today and all time.
func TestEnrichChannelsWithQuotaSummary(t *testing.T) {
	db := setupChannelTodayQuotaTestDB(t)
	now := time.Date(2026, time.September, 15, 0, 30, 0, 0, time.FixedZone("test-utc-plus-eight", 8*60*60))
	startTimestamp, _ := channelTodayTimeRange(now)
	logs := []model.Log{
		{ChannelId: 21, CreatedAt: startTimestamp, Type: model.LogTypeConsume, Quota: 9, BaseQuota: 6},
		{ChannelId: 21, CreatedAt: now.Unix(), Type: model.LogTypeConsume, Quota: 6, BaseQuota: 4},
		{ChannelId: 21, CreatedAt: startTimestamp - 1, Type: model.LogTypeConsume, Quota: 100, BaseQuota: 50},
		{ChannelId: 21, CreatedAt: now.Unix(), Type: model.LogTypeRefund, Quota: 5, BaseQuota: 5},
	}
	require.NoError(t, db.Create(&logs).Error)
	costRatio := 0.5
	channels := []*model.Channel{{Id: 21, CostRatio: &costRatio}, {Id: 22}}

	enrichChannelsWithQuotaSummary(context.Background(), channels, now)

	busy := channels[0]
	require.NotNil(t, busy.TodayUsedQuota)
	require.NotNil(t, busy.TodayCostQuota)
	require.NotNil(t, busy.TotalUsedQuota)
	require.NotNil(t, busy.TotalCostQuota)
	assert.EqualValues(t, 15, *busy.TodayUsedQuota)
	assert.EqualValues(t, 5, *busy.TodayCostQuota)
	assert.EqualValues(t, 115, *busy.TotalUsedQuota)
	assert.EqualValues(t, 30, *busy.TotalCostQuota)
	idle := channels[1]
	for _, value := range []*int64{idle.TodayUsedQuota, idle.TodayCostQuota, idle.TotalUsedQuota, idle.TotalCostQuota} {
		require.NotNil(t, value)
		assert.Zero(t, *value)
	}

	common.LogConsumeEnabled = false
	enrichChannelsWithQuotaSummary(context.Background(), channels, now)
	for _, channel := range channels {
		assert.Nil(t, channel.TodayUsedQuota)
		assert.Nil(t, channel.TodayCostQuota)
		assert.Nil(t, channel.TotalUsedQuota)
		assert.Nil(t, channel.TotalCostQuota)
	}
}

func TestChannelListEndpointsPreserveTodayQuotaAvailability(t *testing.T) {
	db := setupChannelTodayQuotaTestDB(t)
	channel := model.Channel{Name: "today-quota-channel", Key: "test-key", Models: "gpt-test", Group: "default"}
	require.NoError(t, db.Create(&channel).Error)

	type endpoint struct {
		name    string
		url     string
		handler func(*gin.Context)
	}
	endpoints := []endpoint{
		{name: "list", url: "/api/channel?p=1&page_size=20", handler: GetAllChannels},
		{name: "search", url: "/api/channel/search?keyword=today-quota-channel&p=1&page_size=20", handler: SearchChannels},
	}

	assertValue := func(t *testing.T, wantAvailable bool) {
		t.Helper()
		for _, endpoint := range endpoints {
			t.Run(endpoint.name, func(t *testing.T) {
				recorder := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(recorder)
				ctx.Request = httptest.NewRequest(http.MethodGet, endpoint.url, nil)

				endpoint.handler(ctx)

				var response struct {
					Success bool `json:"success"`
					Data    struct {
						Items []*model.Channel `json:"items"`
					} `json:"data"`
				}
				require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
				require.True(t, response.Success)
				require.Len(t, response.Data.Items, 1)
				item := response.Data.Items[0]
				summary := []*int64{item.TodayUsedQuota, item.TodayCostQuota, item.TotalUsedQuota, item.TotalCostQuota}
				for _, value := range summary {
					if wantAvailable {
						require.NotNil(t, value)
						assert.Zero(t, *value)
						continue
					}
					assert.Nil(t, value)
				}
			})
		}
	}

	t.Run("available with zero consumption", func(t *testing.T) {
		common.LogConsumeEnabled = true
		model.LOG_DB = db
		assertValue(t, true)
	})

	t.Run("consume logging disabled", func(t *testing.T) {
		common.LogConsumeEnabled = false
		model.LOG_DB = nil
		assertValue(t, false)
	})

	t.Run("log query failure", func(t *testing.T) {
		common.LogConsumeEnabled = true
		dsn := fmt.Sprintf("file:%s_error?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
		failedLogDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
		require.NoError(t, err)
		sqlDB, err := failedLogDB.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
		model.LOG_DB = failedLogDB
		assertValue(t, false)
	})
}
