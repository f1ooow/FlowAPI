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

// The monthly window starts at the first instant of the UTC+8 calendar month,
// so a request made late on the last day of the previous month in UTC belongs
// to the previous month, not to the current one.
func TestChannelMonthTimeRangeUsesUTC8MonthBoundary(t *testing.T) {
	tests := []struct {
		name      string
		now       time.Time
		wantStart time.Time
	}{
		{
			name: "mid month",
			now:  time.Date(2026, time.September, 15, 0, 30, 0, 0, time.UTC),
			// 2026-09-15 00:30 UTC is 08:30 on the 15th in UTC+8, so the month
			// began at 2026-08-31 16:00 UTC.
			wantStart: time.Date(2026, time.August, 31, 16, 0, 0, 0, time.UTC),
		},
		{
			name:      "first instant of the Beijing month",
			now:       time.Date(2026, time.September, 1, 0, 0, 0, 0, time.FixedZone("utc+8", 8*60*60)),
			wantStart: time.Date(2026, time.August, 31, 16, 0, 0, 0, time.UTC),
		},
		{
			name:      "one second before the Beijing month rolls over",
			now:       time.Date(2026, time.August, 31, 15, 59, 59, 0, time.UTC),
			wantStart: time.Date(2026, time.July, 31, 16, 0, 0, 0, time.UTC),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			startTimestamp, endTimestamp := channelMonthTimeRange(test.now)

			assert.Equal(t, test.wantStart.Unix(), startTimestamp)
			assert.Equal(t, test.now.Unix(), endTimestamp,
				"the window ends now, never at a future month end")
		})
	}
}

// The three windows are nested and each is scoped only by time. A request from
// the previous month is excluded from today and from this month but still
// counts toward the all-time total.
func TestSumGlobalQuotaUsageWindowBoundaries(t *testing.T) {
	db := setupChannelTodayQuotaTestDB(t)
	// 08:30 on 15 September 2026 in UTC+8.
	now := time.Date(2026, time.September, 15, 0, 30, 0, 0, time.UTC)
	todayStart, todayEnd := channelTodayTimeRange(now)
	monthStart, monthEnd := channelMonthTimeRange(now)

	require.NoError(t, db.Create(&[]model.Log{
		// Today (and therefore also this month).
		{ChannelId: 61, CreatedAt: todayStart, Type: model.LogTypeConsume, Quota: 10, BaseQuota: 10, ChannelCostRatio: 10_000},
		// Earlier this month but not today.
		{ChannelId: 61, CreatedAt: monthStart, Type: model.LogTypeConsume, Quota: 20, BaseQuota: 20, ChannelCostRatio: 10_000},
		{ChannelId: 61, CreatedAt: todayStart - 1, Type: model.LogTypeConsume, Quota: 40, BaseQuota: 40, ChannelCostRatio: 10_000},
		// Previous month: only the all-time window sees it.
		{ChannelId: 61, CreatedAt: monthStart - 1, Type: model.LogTypeConsume, Quota: 80, BaseQuota: 80, ChannelCostRatio: 10_000},
		// A refund is not consumption and must not enter any window.
		{ChannelId: 61, CreatedAt: todayStart, Type: model.LogTypeRefund, Quota: 5, BaseQuota: 5},
	}).Error)

	today, err := model.SumGlobalQuotaUsage(context.Background(), todayStart, todayEnd)
	require.NoError(t, err)
	assert.EqualValues(t, 10, today.UsedQuota)
	assert.EqualValues(t, 10, today.CostQuota)
	assert.Zero(t, today.Profit)

	thisMonth, err := model.SumGlobalQuotaUsage(context.Background(), monthStart, monthEnd)
	require.NoError(t, err)
	assert.EqualValues(t, 70, thisMonth.UsedQuota, "10 today + 20 earlier this month + 40 yesterday")
	assert.EqualValues(t, 70, thisMonth.CostQuota)

	allTime, err := model.SumGlobalQuotaUsage(context.Background(), 0, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 150, allTime.UsedQuota, "the previous month's 80 is included only here")
	assert.EqualValues(t, 150, allTime.CostQuota)
}

// Cost is the official-price amount of the channel's requests priced at the
// cost ratio each log was billed under, for both today and all time. The
// channel's configured ratio is deliberately unrelated to the snapshots so a
// regression that reads the live configuration instead of the snapshot fails
// here.
func TestEnrichChannelsWithQuotaSummary(t *testing.T) {
	db := setupChannelTodayQuotaTestDB(t)
	now := time.Date(2026, time.September, 15, 0, 30, 0, 0, time.FixedZone("test-utc-plus-eight", 8*60*60))
	startTimestamp, _ := channelTodayTimeRange(now)
	const ratioHalf = int64(5_000) // 0.5
	logs := []model.Log{
		{ChannelId: 21, CreatedAt: startTimestamp, Type: model.LogTypeConsume, Quota: 9, BaseQuota: 6, ChannelCostRatio: ratioHalf},
		{ChannelId: 21, CreatedAt: now.Unix(), Type: model.LogTypeConsume, Quota: 6, BaseQuota: 4, ChannelCostRatio: ratioHalf},
		{ChannelId: 21, CreatedAt: startTimestamp - 1, Type: model.LogTypeConsume, Quota: 100, BaseQuota: 50, ChannelCostRatio: ratioHalf},
		{ChannelId: 21, CreatedAt: now.Unix(), Type: model.LogTypeRefund, Quota: 5, BaseQuota: 5},
	}
	require.NoError(t, db.Create(&logs).Error)
	// A wildly different live ratio: the figures below must ignore it.
	costRatio := 8.0
	channels := []*model.Channel{{Id: 21, CostRatio: &costRatio}, {Id: 22}}

	enrichChannelsWithQuotaSummary(context.Background(), channels, now)

	busy := channels[0]
	require.NotNil(t, busy.TodayUsedQuota)
	require.NotNil(t, busy.TodayCostQuota)
	require.NotNil(t, busy.TotalUsedQuota)
	require.NotNil(t, busy.TotalCostQuota)
	assert.EqualValues(t, 15, *busy.TodayUsedQuota)
	assert.EqualValues(t, 5, *busy.TodayCostQuota, "today's cost uses the logs' 0.5 snapshot, not the channel's 8.0")
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

// Repricing a channel must not restate what its settled requests cost. This is
// the operator-visible promise of the snapshot column: yesterday's profit
// figure is a fact, not a function of today's configuration.
func TestEnrichChannelsWithQuotaSummaryIgnoresLaterCostRatioChange(t *testing.T) {
	db := setupChannelTodayQuotaTestDB(t)
	now := time.Date(2026, time.September, 15, 0, 30, 0, 0, time.FixedZone("test-utc-plus-eight", 8*60*60))
	startTimestamp, _ := channelTodayTimeRange(now)
	require.NoError(t, db.Create(&[]model.Log{
		{ChannelId: 31, CreatedAt: startTimestamp, Type: model.LogTypeConsume, Quota: 100, BaseQuota: 100, ChannelCostRatio: 10_000},
	}).Error)

	cheap := 1.0
	channel := &model.Channel{Id: 31, CostRatio: &cheap}
	channels := []*model.Channel{channel}
	enrichChannelsWithQuotaSummary(context.Background(), channels, now)
	require.NotNil(t, channel.TotalCostQuota)
	assert.EqualValues(t, 100, *channel.TotalCostQuota)

	// The operator triples the channel's cost ratio. History must not move.
	dear := 3.0
	channel.CostRatio = &dear
	enrichChannelsWithQuotaSummary(context.Background(), channels, now)
	require.NotNil(t, channel.TotalCostQuota)
	assert.EqualValues(t, 100, *channel.TotalCostQuota,
		"history stays priced at the 1.0 the request was billed under")
}

// Renaming a channel is bookkeeping, not accounting: the figures are keyed by
// channel id, so a rename cannot move or lose them.
func TestEnrichChannelsWithQuotaSummarySurvivesRename(t *testing.T) {
	db := setupChannelTodayQuotaTestDB(t)
	now := time.Date(2026, time.September, 15, 0, 30, 0, 0, time.FixedZone("test-utc-plus-eight", 8*60*60))
	startTimestamp, _ := channelTodayTimeRange(now)
	require.NoError(t, db.Create(&[]model.Log{
		{ChannelId: 41, CreatedAt: startTimestamp, Type: model.LogTypeConsume, Quota: 100, BaseQuota: 50, ChannelCostRatio: 20_000},
	}).Error)

	ratio := 1.0
	for _, name := range []string{"before-rename", "after-rename"} {
		channel := &model.Channel{Id: 41, Name: name, CostRatio: &ratio}
		enrichChannelsWithQuotaSummary(context.Background(), []*model.Channel{channel}, now)
		require.NotNil(t, channel.TotalUsedQuota)
		assert.EqualValues(t, 100, *channel.TotalUsedQuota, "name %q", name)
		assert.EqualValues(t, 100, *channel.TotalCostQuota, "name %q", name)
	}
}

// Deleting a channel must not delete its history. The fleet-wide totals are
// aggregated from consume logs alone, so a channel id with no row in
// `channels` still contributes.
func TestSumGlobalQuotaUsageCoversDeletedChannels(t *testing.T) {
	db := setupChannelTodayQuotaTestDB(t)
	now := time.Date(2026, time.September, 15, 0, 30, 0, 0, time.FixedZone("test-utc-plus-eight", 8*60*60))
	startTimestamp, _ := channelTodayTimeRange(now)
	// Channel 51 exists and logs; channel 52 logged and was then deleted, so
	// no channel row references it anywhere.
	require.NoError(t, model.LOG_DB.Create(&[]model.Log{
		{ChannelId: 51, CreatedAt: startTimestamp, Type: model.LogTypeConsume, Quota: 100, BaseQuota: 40, ChannelCostRatio: 10_000},
		{ChannelId: 52, CreatedAt: startTimestamp, Type: model.LogTypeConsume, Quota: 70, BaseQuota: 20, ChannelCostRatio: 5_000},
	}).Error)

	usage, err := model.SumGlobalQuotaUsage(context.Background(), startTimestamp, now.Unix())
	require.NoError(t, err)

	assert.EqualValues(t, 170, usage.UsedQuota, "both channels' charges")
	assert.EqualValues(t, 50, usage.CostQuota, "40*1.0 + 20*0.5")
	assert.EqualValues(t, 120, usage.Profit)

	// Adding a brand new channel that has not served anything leaves the
	// totals untouched.
	require.NoError(t, db.Create(&model.Channel{Id: 53, Name: "brand-new", Group: "default"}).Error)
	again, err := model.SumGlobalQuotaUsage(context.Background(), startTimestamp, now.Unix())
	require.NoError(t, err)
	assert.Equal(t, usage, again, "a new channel cannot move existing history")
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
