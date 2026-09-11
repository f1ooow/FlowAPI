package perfmetrics

import (
	"math"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func resetChannelHotBuckets(t *testing.T) {
	t.Helper()
	clear := func() {
		channelHotBuckets.Range(func(key, _ any) bool {
			channelHotBuckets.Delete(key)
			return true
		})
	}
	clear()
	t.Cleanup(clear)
}

func channelBucketCounters(t *testing.T, channelId int) (channelCounters, bool) {
	t.Helper()
	found := false
	total := channelCounters{}
	channelHotBuckets.Range(func(key, value any) bool {
		if key.(channelBucketKey).channelId != channelId {
			return true
		}
		found = true
		bucket := value.(*atomicChannelBucket)
		total.attemptCount += bucket.attemptCount.Load()
		total.successCount += bucket.successCount.Load()
		total.totalLatencyMs += bucket.totalLatencyMs.Load()
		total.cacheRequestCount += bucket.cacheRequestCount.Load()
		total.cacheSignalCount += bucket.cacheSignalCount.Load()
		total.cacheReadTokens += bucket.cacheReadTokens.Load()
		total.cacheWriteTokens += bucket.cacheWriteTokens.Load()
		total.cacheInputTokens += bucket.cacheInputTokens.Load()
		return true
	})
	return total, found
}

// A failover is the reason channel_metrics exists. Channel A answers 500 twice,
// the router gives up on it and channel B serves the same user request: the one
// request must leave a failure record on A and a success record on B. Request
// level sampling would only ever record B's success, so A's availability would
// stay at 100% while it is in fact completely down.
func TestRecordChannelAttemptAttributesEachFailoverAttempt(t *testing.T) {
	resetChannelHotBuckets(t)

	info := &relaycommon.RelayInfo{}
	attemptStart := time.Now().Add(-50 * time.Millisecond)
	RecordChannelAttempt(info, 11, attemptStart, false)
	RecordChannelAttempt(info, 11, attemptStart, false)
	RecordChannelAttempt(info, 22, attemptStart, true)

	broken, ok := channelBucketCounters(t, 11)
	require.True(t, ok, "the failing channel must be recorded even though the request finally succeeded elsewhere")
	assert.EqualValues(t, 2, broken.attemptCount)
	assert.EqualValues(t, 0, broken.successCount)
	assert.Greater(t, broken.totalLatencyMs, int64(0))

	healthy, ok := channelBucketCounters(t, 22)
	require.True(t, ok)
	assert.EqualValues(t, 1, healthy.attemptCount)
	assert.EqualValues(t, 1, healthy.successCount)
}

func TestRecordChannelAttemptSkipsUnattributableTraffic(t *testing.T) {
	tests := []struct {
		name      string
		info      *relaycommon.RelayInfo
		channelId int
	}{
		{
			// getChannel failed before any channel was picked, so RelayInfo has
			// no ChannelMeta at all. Charging this to some channel would be a
			// misattribution, and reading info.ChannelId here would panic.
			name:      "no channel selected yet",
			info:      &relaycommon.RelayInfo{},
			channelId: 0,
		},
		{
			name:      "synthetic channel test traffic",
			info:      &relaycommon.RelayInfo{IsChannelTest: true},
			channelId: 33,
		},
		{
			name:      "missing relay info",
			info:      nil,
			channelId: 33,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resetChannelHotBuckets(t)
			require.NotPanics(t, func() {
				RecordChannelAttempt(test.info, test.channelId, time.Now(), false)
			})
			_, found := channelBucketCounters(t, test.channelId)
			assert.False(t, found)
		})
	}
}

// The CCH pre-filter is what makes the hit rate readable at all: a request the
// upstream answered without any cache number says nothing about cache
// effectiveness, and letting its input tokens into the denominator would drag
// the ratio down for every channel whose provider simply never reports cache.
// Such a request must still count as a request, otherwise the engagement rate
// (how much the pre-filter removed) becomes unknowable.
func TestRecordChannelCacheUsageAppliesThePreFilter(t *testing.T) {
	resetChannelHotBuckets(t)

	info := &relaycommon.RelayInfo{}
	// Two requests with a cache signal and one without.
	RecordChannelCacheUsage(info, 7, ChannelCacheSample{CacheReadTokens: 400, CacheInputTokens: 1000})
	RecordChannelCacheUsage(info, 7, ChannelCacheSample{CacheWriteTokens: 250, CacheInputTokens: 250})
	RecordChannelCacheUsage(info, 7, ChannelCacheSample{CacheInputTokens: 50_000})

	counters, ok := channelBucketCounters(t, 7)
	require.True(t, ok)
	assert.EqualValues(t, 3, counters.cacheRequestCount, "every settled request counts, that is the engagement denominator")
	assert.EqualValues(t, 2, counters.cacheSignalCount)
	assert.EqualValues(t, 400, counters.cacheReadTokens)
	assert.EqualValues(t, 250, counters.cacheWriteTokens)
	assert.EqualValues(t, 1250, counters.cacheInputTokens, "the filtered request must not contribute its 50k input tokens")
	assert.EqualValues(t, 0, counters.attemptCount, "cache sampling must not move the availability counters")

	// A failed flush hands the counters back; losing the cache half there would
	// leave the two families permanently inconsistent.
	bucket := &atomicChannelBucket{}
	bucket.add(120, true)
	bucket.addCacheUsage(ChannelCacheSample{CacheReadTokens: 400, CacheWriteTokens: 10, CacheInputTokens: 1000})
	drained := bucket.drain()
	bucket.addCounters(drained)
	assert.Equal(t, drained, bucket.drain())
}

func TestRecordChannelCacheUsageBoundsUntrustedUpstreamCounts(t *testing.T) {
	resetChannelHotBuckets(t)

	info := &relaycommon.RelayInfo{}
	// Upstream usage counts are never validated in the relay path: a negative
	// value must not shrink the bucket and an absurd one must not ruin it.
	RecordChannelCacheUsage(info, 9, ChannelCacheSample{CacheReadTokens: -500, CacheInputTokens: -500})
	RecordChannelCacheUsage(info, 9, ChannelCacheSample{
		CacheReadTokens:  int64(math.MaxInt32) + 1,
		CacheInputTokens: 1 << 62,
	})

	counters, ok := channelBucketCounters(t, 9)
	require.True(t, ok)
	assert.EqualValues(t, 2, counters.cacheRequestCount)
	assert.EqualValues(t, 1, counters.cacheSignalCount, "the all-negative sample carries no signal")
	assert.EqualValues(t, channelCacheTokenSampleLimit, counters.cacheReadTokens)
	assert.EqualValues(t, channelCacheTokenSampleLimit, counters.cacheInputTokens)
}

func TestRecordChannelCacheUsageSkipsUnattributableTraffic(t *testing.T) {
	tests := []struct {
		name      string
		info      *relaycommon.RelayInfo
		channelId int
	}{
		{name: "no channel selected yet", info: &relaycommon.RelayInfo{}, channelId: 0},
		{name: "synthetic channel test traffic", info: &relaycommon.RelayInfo{IsChannelTest: true}, channelId: 33},
		{name: "missing relay info", info: nil, channelId: 33},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resetChannelHotBuckets(t)
			require.NotPanics(t, func() {
				RecordChannelCacheUsage(test.info, test.channelId, ChannelCacheSample{CacheReadTokens: 10, CacheInputTokens: 20})
			})
			_, found := channelBucketCounters(t, test.channelId)
			assert.False(t, found)
		})
	}
}

func setupChannelMetricTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ChannelMetric{}))
	previous := model.DB
	model.DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB = previous
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
}

// The flush must only drain closed buckets, and re-flushing the same bucket key
// has to accumulate through the unique index rather than insert a second row.
func TestFlushCompletedChannelBucketsAccumulatesClosedBucketsOnly(t *testing.T) {
	resetChannelHotBuckets(t)
	setupChannelMetricTestDB(t)

	bucketSeconds := int64(300)
	closedTs := bucketStart(time.Now().Unix()) - bucketSeconds
	openTs := bucketStart(time.Now().Unix())

	closed := &atomicChannelBucket{}
	closed.add(120, false)
	closed.add(80, true)
	channelHotBuckets.Store(channelBucketKey{channelId: 7, bucketTs: closedTs}, closed)

	open := &atomicChannelBucket{}
	open.add(10, true)
	channelHotBuckets.Store(channelBucketKey{channelId: 7, bucketTs: openTs}, open)

	flushCompletedChannelBuckets()

	rows, err := model.GetChannelMetricBuckets(closedTs, openTs, bucketSeconds)
	require.NoError(t, err)
	require.Len(t, rows, 1, "the still-open bucket must stay in memory")
	assert.EqualValues(t, closedTs, rows[0].BucketTs)
	assert.EqualValues(t, 2, rows[0].AttemptCount)
	assert.EqualValues(t, 1, rows[0].SuccessCount)
	assert.EqualValues(t, 200, rows[0].TotalLatencyMs)
	assert.EqualValues(t, 1, open.attemptCount.Load(), "the open bucket must not be drained")

	// A second node's traffic lands on the same (channel, bucket) key.
	late := &atomicChannelBucket{}
	late.add(40, true)
	channelHotBuckets.Store(channelBucketKey{channelId: 7, bucketTs: closedTs}, late)
	flushCompletedChannelBuckets()

	rows, err = model.GetChannelMetricBuckets(closedTs, openTs, bucketSeconds)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.EqualValues(t, 3, rows[0].AttemptCount)
	assert.EqualValues(t, 2, rows[0].SuccessCount)
	assert.EqualValues(t, 240, rows[0].TotalLatencyMs)
}

// The two counter families are fed from two different call sites, so a bucket
// can hold a cache sample and no attempt at all: the attempt was recorded just
// before the bucket boundary and the settlement landed just after it. Gating
// the flush on attemptCount alone would throw that sample away.
func TestFlushCompletedChannelBucketsKeepsCacheOnlyBuckets(t *testing.T) {
	resetChannelHotBuckets(t)
	setupChannelMetricTestDB(t)

	bucketSeconds := int64(300)
	closedTs := bucketStart(time.Now().Unix()) - bucketSeconds

	cacheOnly := &atomicChannelBucket{}
	cacheOnly.addCacheUsage(ChannelCacheSample{CacheReadTokens: 400, CacheWriteTokens: 20, CacheInputTokens: 1000})
	cacheOnly.addCacheUsage(ChannelCacheSample{CacheInputTokens: 900})
	channelHotBuckets.Store(channelBucketKey{channelId: 7, bucketTs: closedTs}, cacheOnly)

	flushCompletedChannelBuckets()

	rows, err := model.GetChannelMetricBuckets(closedTs, closedTs, bucketSeconds)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.EqualValues(t, 0, rows[0].AttemptCount)
	assert.EqualValues(t, 2, rows[0].CacheRequestCount)
	assert.EqualValues(t, 1, rows[0].CacheSignalCount)
	assert.EqualValues(t, 400, rows[0].CacheReadTokens)
	assert.EqualValues(t, 20, rows[0].CacheWriteTokens)
	assert.EqualValues(t, 1000, rows[0].CacheInputTokens)

	// A second node's cache sample lands on the same (channel, bucket) key and
	// has to accumulate through the unique index, like the attempt counters do.
	late := &atomicChannelBucket{}
	late.add(40, true)
	late.addCacheUsage(ChannelCacheSample{CacheReadTokens: 100, CacheInputTokens: 500})
	channelHotBuckets.Store(channelBucketKey{channelId: 7, bucketTs: closedTs}, late)
	flushCompletedChannelBuckets()

	rows, err = model.GetChannelMetricBuckets(closedTs, closedTs, bucketSeconds)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.EqualValues(t, 1, rows[0].AttemptCount)
	assert.EqualValues(t, 3, rows[0].CacheRequestCount)
	assert.EqualValues(t, 2, rows[0].CacheSignalCount)
	assert.EqualValues(t, 500, rows[0].CacheReadTokens)
	assert.EqualValues(t, 1500, rows[0].CacheInputTokens)
}

func TestCleanupExpiredChannelMetricsHonoursRetention(t *testing.T) {
	setupChannelMetricTestDB(t)

	now := time.Now().Unix()
	expired := now - 9*24*3600
	kept := now - 2*24*3600
	require.NoError(t, model.UpsertChannelMetric(&model.ChannelMetric{ChannelId: 1, BucketTs: expired, AttemptCount: 5, SuccessCount: 5}))
	require.NoError(t, model.UpsertChannelMetric(&model.ChannelMetric{ChannelId: 1, BucketTs: kept, AttemptCount: 3, SuccessCount: 3}))

	// Retention 0 is what perf_metrics shipped with, and it means "never clean".
	cleanupExpiredChannelMetrics(0)
	rows, err := model.GetChannelMetricBuckets(0, now, 300)
	require.NoError(t, err)
	assert.Len(t, rows, 2)

	cleanupExpiredChannelMetrics(7)
	rows, err = model.GetChannelMetricBuckets(0, now, 300)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.EqualValues(t, 3, rows[0].AttemptCount)
}
