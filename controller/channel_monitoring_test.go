package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type channelMonitoringResponse struct {
	Success bool                             `json:"success"`
	Message string                           `json:"message"`
	Data    channelMonitoringSummaryResponse `json:"data"`
}

func channelSummaryById(t *testing.T, summaries []channelMonitoringChannelSummary, channelId int) channelMonitoringChannelSummary {
	t.Helper()
	for _, summary := range summaries {
		if summary.ChannelId == channelId {
			return summary
		}
	}
	require.FailNowf(t, "missing channel", "channel %d is not in the response", channelId)
	return channelMonitoringChannelSummary{}
}

// The page exists to expose the channel that a failover hid. One user request
// that hits channel 11 twice with a 500 and then succeeds on channel 22 must
// show 11 as down and 22 as healthy, never a single 100% success row.
func TestBuildChannelMonitoringChannelsSurfacesFailoverVictim(t *testing.T) {
	const step = int64(1800)
	const seriesStart = int64(0)
	const bucketCount = 4

	rows := []model.ChannelMetricBucket{
		{ChannelId: 11, BucketTs: 0, AttemptCount: 2, SuccessCount: 0, TotalLatencyMs: 400},
		{ChannelId: 22, BucketTs: 0, AttemptCount: 1, SuccessCount: 1, TotalLatencyMs: 300},
		{ChannelId: 22, BucketTs: 1800, AttemptCount: 9, SuccessCount: 9, TotalLatencyMs: 900},
		// A channel row that outlived its channel row in `channels`.
		{ChannelId: 99, BucketTs: 3600, AttemptCount: 10, SuccessCount: 9, TotalLatencyMs: 800},
		// Outside the rendered window.
		{ChannelId: 11, BucketTs: 7200, AttemptCount: 50, SuccessCount: 50, TotalLatencyMs: 5000},
	}
	identities := []model.ChannelIdentity{
		{Id: 11, Name: "flaky"},
		{Id: 22, Name: "backup"},
		{Id: 33, Name: "idle"},
	}

	channels, overall := buildChannelMonitoringChannels(rows, identities, seriesStart, bucketCount, step)
	require.Len(t, channels, 4)

	flaky := channelSummaryById(t, channels, 11)
	assert.True(t, flaky.HasData)
	assert.Equal(t, channelMonitoringStateDown, flaky.State)
	assert.EqualValues(t, 0, flaky.AvailabilityRate)
	assert.EqualValues(t, 100, flaky.ErrorRate)
	assert.EqualValues(t, 2, flaky.AttemptCount, "both failed attempts of the one request belong to this channel")
	assert.EqualValues(t, 200, flaky.AvgLatencyMs)

	backup := channelSummaryById(t, channels, 22)
	assert.Equal(t, channelMonitoringStateHealthy, backup.State)
	assert.EqualValues(t, 100, backup.AvailabilityRate)
	assert.EqualValues(t, 10, backup.AttemptCount)

	deleted := channelSummaryById(t, channels, 99)
	assert.Empty(t, deleted.ChannelName, "a deleted channel keeps its history under the bare id")
	assert.Equal(t, channelMonitoringStateDegraded, deleted.State)
	assert.EqualValues(t, 90, deleted.AvailabilityRate)

	idle := channelSummaryById(t, channels, 33)
	assert.False(t, idle.HasData, "a channel without traffic is no-data, not 0% available")
	assert.Equal(t, channelMonitoringStateNoData, idle.State)
	assert.EqualValues(t, 0, idle.AttemptCount)

	assert.Equal(t, []int{11, 99, 22, 33}, []int{channels[0].ChannelId, channels[1].ChannelId, channels[2].ChannelId, channels[3].ChannelId},
		"worst availability first, channels without data last")

	require.Len(t, backup.Buckets, bucketCount)
	assert.Equal(t, channelMonitoringStateHealthy, backup.Buckets[0].State)
	assert.Equal(t, channelMonitoringStateNoData, backup.Buckets[2].State)
	assert.EqualValues(t, 3600, backup.Buckets[2].Ts)

	assert.EqualValues(t, 22, overall.attemptCount)
	assert.EqualValues(t, 19, overall.successCount)
}

// Rates must come from summed counters. Averaging the two buckets below would
// report (100 + 1) / 2 = 50.5% for a channel that actually served 2 of 101.
func TestBuildChannelMonitoringChannelsSumsCountersBeforeDividing(t *testing.T) {
	rows := []model.ChannelMetricBucket{
		{ChannelId: 5, BucketTs: 0, AttemptCount: 1, SuccessCount: 1, TotalLatencyMs: 100},
		{ChannelId: 5, BucketTs: 300, AttemptCount: 100, SuccessCount: 1, TotalLatencyMs: 10000},
	}

	channels, overall := buildChannelMonitoringChannels(rows, []model.ChannelIdentity{{Id: 5, Name: "noisy"}}, 0, 2, 300)
	require.Len(t, channels, 1)
	assert.EqualValues(t, 1.98, channels[0].AvailabilityRate)
	assert.EqualValues(t, 98.02, channels[0].ErrorRate)
	assert.Equal(t, channelMonitoringStateDown, channels[0].State)
	assert.EqualValues(t, 101, overall.attemptCount)
}

// The cache metrics have their own denominators and must be summed before
// dividing, exactly like availability. Averaging the two buckets below would
// report (80 + 1) / 2 = 40.5% for a channel that actually read 810 of 9000
// input tokens.
func TestBuildChannelMonitoringChannelsDerivesCacheRatesFromSummedTokens(t *testing.T) {
	rows := []model.ChannelMetricBucket{
		{
			ChannelId: 5, BucketTs: 0, AttemptCount: 1, SuccessCount: 1, TotalLatencyMs: 100,
			CacheRequestCount: 1, CacheSignalCount: 1, CacheReadTokens: 800, CacheWriteTokens: 0, CacheInputTokens: 1000,
		},
		{
			ChannelId: 5, BucketTs: 300, AttemptCount: 40, SuccessCount: 40, TotalLatencyMs: 4000,
			CacheRequestCount: 40, CacheSignalCount: 9, CacheReadTokens: 10, CacheWriteTokens: 300, CacheInputTokens: 8000,
		},
	}

	channels, overall := buildChannelMonitoringChannels(rows, []model.ChannelIdentity{{Id: 5, Name: "claude"}}, 0, 2, 300)
	require.Len(t, channels, 1)

	// 810 / 9000 = 9%, not the mean of the two per-bucket rates.
	assert.True(t, channels[0].CacheHasData)
	assert.EqualValues(t, 9, channels[0].CacheHitRate)
	// 10 of 41 settled requests carried a cache signal.
	assert.EqualValues(t, 24.39, channels[0].CacheEngagementRate)
	assert.EqualValues(t, 41, channels[0].CacheRequestCount)
	assert.EqualValues(t, 10, channels[0].CacheSignalCount)
	assert.EqualValues(t, 810, channels[0].CacheReadTokens)
	assert.EqualValues(t, 300, channels[0].CacheWriteTokens)
	assert.EqualValues(t, 9000, channels[0].CacheInputTokens)
	assert.EqualValues(t, 810, overall.cacheReadTokens)

	// The cache denominator is never attempt_count: the two calibers differ
	// because a failed attempt has no usage.
	assert.EqualValues(t, 41, channels[0].AttemptCount)
	assert.NotEqual(t, channels[0].CacheSignalCount, channels[0].AttemptCount)
}

// A channel that received traffic but never a single cache number must report
// "no cache data" rather than a 0% hit rate: the operator would read 0% as
// "caching is broken here" when the provider simply does not report it.
func TestBuildChannelMonitoringChannelsSeparatesCacheDataFromAttemptData(t *testing.T) {
	rows := []model.ChannelMetricBucket{
		{
			ChannelId: 5, BucketTs: 0, AttemptCount: 10, SuccessCount: 10, TotalLatencyMs: 1000,
			CacheRequestCount: 10, CacheSignalCount: 0, CacheInputTokens: 0,
		},
		// Cache samples with no attempt in the window: the settlement of a
		// request whose attempt was recorded in the previous bucket.
		{
			ChannelId: 6, BucketTs: 0, AttemptCount: 0, SuccessCount: 0,
			CacheRequestCount: 2, CacheSignalCount: 2, CacheReadTokens: 500, CacheInputTokens: 1000,
		},
	}

	channels, _ := buildChannelMonitoringChannels(rows, []model.ChannelIdentity{{Id: 5, Name: "no-cache"}, {Id: 6, Name: "cache-only"}}, 0, 1, 300)

	noCache := channelSummaryById(t, channels, 5)
	assert.True(t, noCache.HasData, "it did serve attempts")
	assert.False(t, noCache.CacheHasData, "but nothing passed the cache pre-filter")
	assert.EqualValues(t, 0, noCache.CacheHitRate)
	assert.EqualValues(t, 0, noCache.CacheEngagementRate)
	assert.EqualValues(t, 10, noCache.CacheRequestCount)

	cacheOnly := channelSummaryById(t, channels, 6)
	assert.False(t, cacheOnly.HasData, "no attempt landed in this window")
	assert.True(t, cacheOnly.CacheHasData, "the cache sample must survive anyway")
	assert.EqualValues(t, 50, cacheOnly.CacheHitRate)
	assert.EqualValues(t, 100, cacheOnly.CacheEngagementRate)
}

// OpenAI reports unadjusted cache-write prefix counts, so cached_tokens can
// exceed the normalized input on a single request. The ratio has to stay a
// percentage instead of rendering something like 137%.
func TestChannelMonitoringCacheHitRateClampsToPercentRange(t *testing.T) {
	assert.EqualValues(t, 100, channelMonitoringCacheHitRate(channelMonitoringCounters{
		cacheSignalCount: 1, cacheReadTokens: 1370, cacheInputTokens: 1000,
	}))
	assert.EqualValues(t, 0, channelMonitoringCacheHitRate(channelMonitoringCounters{
		cacheSignalCount: 1, cacheReadTokens: 400, cacheInputTokens: 0,
	}))
}

func TestChannelMonitoringRangeToStep(t *testing.T) {
	tests := []struct {
		rangeKey      string
		windowSeconds int64
		stepSeconds   int64
	}{
		{rangeKey: "15m", windowSeconds: 900, stepSeconds: 300},
		{rangeKey: "1h", windowSeconds: 3600, stepSeconds: 300},
		{rangeKey: "6h", windowSeconds: 21600, stepSeconds: 900},
		{rangeKey: "24h", windowSeconds: 86400, stepSeconds: 1800},
		{rangeKey: "7d", windowSeconds: 604800, stepSeconds: 7200},
	}

	require.Len(t, channelMonitoringWindows, len(tests))
	for _, test := range tests {
		t.Run(test.rangeKey, func(t *testing.T) {
			window, ok := channelMonitoringWindows[test.rangeKey]
			require.True(t, ok)
			assert.Equal(t, test.windowSeconds, window.windowSeconds)
			assert.Equal(t, test.stepSeconds, window.stepSeconds)
			assert.Contains(t, channelMonitoringRangeKeys, test.rangeKey)
		})
	}
}

// A display slot narrower than the stored bucket cannot be produced, so raising
// the global perf_metrics bucket width has to widen the timeline instead of
// rendering empty slots between the real ones.
func TestChannelMonitoringStepSecondsClampsToStorageWidth(t *testing.T) {
	tests := []struct {
		name          string
		storageBucket string
		stepSeconds   int64
		want          int64
	}{
		{name: "5min storage keeps 5min step", storageBucket: "5min", stepSeconds: 300, want: 300},
		{name: "5min storage keeps 2h step", storageBucket: "5min", stepSeconds: 7200, want: 7200},
		{name: "hourly storage widens 5min step", storageBucket: "hour", stepSeconds: 300, want: 3600},
		{name: "hourly storage keeps 2h step", storageBucket: "hour", stepSeconds: 7200, want: 7200},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			overridePerfMetricsSetting(t, test.storageBucket, 5)
			assert.Equal(t, test.want, channelMonitoringStepSeconds(test.stepSeconds))
		})
	}
}

func setupChannelMonitoringTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupModelListControllerTestDB(t)
	previousLogConsumeEnabled := common.LogConsumeEnabled
	common.LogConsumeEnabled = true
	t.Cleanup(func() {
		common.LogConsumeEnabled = previousLogConsumeEnabled
	})
	require.NoError(t, db.AutoMigrate(&model.ChannelMetric{}, &model.Log{}))
	return db
}

func TestGetChannelMonitoringSummaryRejectsUnknownRange(t *testing.T) {
	setupChannelMonitoringTestDB(t)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/channel-monitoring/summary?range=42h", nil)
	GetChannelMonitoringSummary(c)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	var payload channelMonitoringResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	assert.False(t, payload.Success)
	assert.Contains(t, payload.Message, "42h")
}

func TestGetChannelMonitoringSummaryReadsChannelMetrics(t *testing.T) {
	db := setupChannelMonitoringTestDB(t)
	overridePerfMetricsSetting(t, "5min", 5)

	require.NoError(t, model.DB.Create(&model.Channel{Id: 11, Name: "flaky"}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 33, Name: "idle"}).Error)

	// Land the samples in the middle of the rendered window so the assertions
	// do not depend on where the wall clock sits inside the current bucket.
	step := channelMonitoringStepSeconds(channelMonitoringWindows["24h"].stepSeconds)
	seriesEnd := channelMonitoringSeriesEnd(time.Now().Unix(), step)
	sampleTs := seriesEnd - 4*step
	require.NoError(t, model.UpsertChannelMetric(&model.ChannelMetric{
		ChannelId: 11, BucketTs: sampleTs, AttemptCount: 4, SuccessCount: 1, TotalLatencyMs: 800,
		CacheRequestCount: 4, CacheSignalCount: 2, CacheReadTokens: 600, CacheWriteTokens: 120, CacheInputTokens: 2000,
	}))
	todayStart, _ := channelTodayTimeRange(time.Now())
	require.NoError(t, db.Create(&[]model.Log{
		{ChannelId: 11, CreatedAt: todayStart, Type: model.LogTypeConsume, Quota: 9},
		{ChannelId: 11, CreatedAt: todayStart + 1, Type: model.LogTypeConsume, Quota: 6},
		{ChannelId: 11, CreatedAt: todayStart - 1, Type: model.LogTypeConsume, Quota: 100},
		{ChannelId: 11, CreatedAt: todayStart + 1, Type: model.LogTypeRefund, Quota: 50},
	}).Error)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/channel-monitoring/summary?range=24h", nil)
	GetChannelMonitoringSummary(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload channelMonitoringResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success, payload.Message)
	assert.Equal(t, "24h", payload.Data.Range)
	assert.Equal(t, 30, payload.Data.StepMinutes)
	assert.EqualValues(t, 86400, payload.Data.WindowSeconds)
	require.Len(t, payload.Data.Channels, 2)

	flaky := channelSummaryById(t, payload.Data.Channels, 11)
	assert.True(t, flaky.HasData)
	assert.EqualValues(t, 25, flaky.AvailabilityRate)
	assert.EqualValues(t, 4, flaky.AttemptCount)
	assert.EqualValues(t, 200, flaky.AvgLatencyMs)
	assert.Len(t, flaky.Buckets, 48)
	require.NotNil(t, flaky.TodayUsedQuota)
	assert.EqualValues(t, 15, *flaky.TodayUsedQuota)

	// The cache metrics travel flat in the same JSON object and keep their own
	// caliber: 600 / 2000 read, 2 of 4 settled requests with a signal.
	assert.True(t, flaky.CacheHasData)
	assert.EqualValues(t, 30, flaky.CacheHitRate)
	assert.EqualValues(t, 50, flaky.CacheEngagementRate)
	assert.EqualValues(t, 4, flaky.CacheRequestCount)
	assert.EqualValues(t, 2, flaky.CacheSignalCount)
	assert.EqualValues(t, 120, flaky.CacheWriteTokens)
	assert.EqualValues(t, 2000, flaky.CacheInputTokens)

	idle := channelSummaryById(t, payload.Data.Channels, 33)
	assert.False(t, idle.HasData)
	assert.False(t, idle.CacheHasData)
	require.NotNil(t, idle.TodayUsedQuota)
	assert.Zero(t, *idle.TodayUsedQuota)

	assert.True(t, payload.Data.Overall.HasData)
	assert.EqualValues(t, 25, payload.Data.Overall.AvailabilityRate)
	assert.EqualValues(t, 75, payload.Data.Overall.ErrorRate)
	assert.EqualValues(t, 30, payload.Data.Overall.CacheHitRate)
}

func TestGetChannelMonitoringSummaryLeavesTodayQuotaNullWhenConsumeLoggingIsDisabled(t *testing.T) {
	setupChannelMonitoringTestDB(t)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 11, Name: "primary"}).Error)
	common.LogConsumeEnabled = false

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/channel-monitoring/summary?range=24h", nil)
	GetChannelMonitoringSummary(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload channelMonitoringResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success, payload.Message)
	require.Len(t, payload.Data.Channels, 1)
	assert.Nil(t, payload.Data.Channels[0].TodayUsedQuota)
}
