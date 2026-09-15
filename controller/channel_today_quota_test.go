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

func TestEnrichChannelsWithTodayUsedQuota(t *testing.T) {
	db := setupChannelTodayQuotaTestDB(t)
	now := time.Date(2026, time.September, 15, 0, 30, 0, 0, time.FixedZone("test-utc-plus-eight", 8*60*60))
	startTimestamp, _ := channelTodayTimeRange(now)
	logs := []model.Log{
		{ChannelId: 21, CreatedAt: startTimestamp, Type: model.LogTypeConsume, Quota: 9},
		{ChannelId: 21, CreatedAt: now.Unix(), Type: model.LogTypeConsume, Quota: 6},
		{ChannelId: 21, CreatedAt: startTimestamp - 1, Type: model.LogTypeConsume, Quota: 100},
		{ChannelId: 21, CreatedAt: now.Unix(), Type: model.LogTypeRefund, Quota: 5},
	}
	require.NoError(t, db.Create(&logs).Error)
	channels := []*model.Channel{{Id: 21}, {Id: 22}}

	enrichChannelsWithTodayUsedQuota(context.Background(), channels, now)

	require.NotNil(t, channels[0].TodayUsedQuota)
	assert.Equal(t, int64(15), *channels[0].TodayUsedQuota)
	require.NotNil(t, channels[1].TodayUsedQuota)
	assert.Zero(t, *channels[1].TodayUsedQuota)

	common.LogConsumeEnabled = false
	enrichChannelsWithTodayUsedQuota(context.Background(), channels, now)
	assert.Nil(t, channels[0].TodayUsedQuota)
	assert.Nil(t, channels[1].TodayUsedQuota)
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
				if wantAvailable {
					require.NotNil(t, response.Data.Items[0].TodayUsedQuota)
					assert.Zero(t, *response.Data.Items[0].TodayUsedQuota)
					return
				}
				assert.Nil(t, response.Data.Items[0].TodayUsedQuota)
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
