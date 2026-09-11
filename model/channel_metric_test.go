package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MySQL is the dialect that needs FLOOR: its "/" operator returns a DECIMAL, so
// the plain expression would divide and multiply back to the original second
// and every storage bucket would stay its own display bucket. PostgreSQL and
// SQLite truncate integer division, which is what the rollup wants.
func TestChannelMetricBucketExprPerDialect(t *testing.T) {
	previous := common.MainDatabaseType()
	t.Cleanup(func() { common.SetMainDatabaseType(previous) })

	tests := []struct {
		databaseType common.DatabaseType
		want         string
	}{
		{databaseType: common.DatabaseTypeMySQL, want: "FLOOR(bucket_ts / 1800) * 1800"},
		{databaseType: common.DatabaseTypePostgreSQL, want: "(bucket_ts / 1800) * 1800"},
		{databaseType: common.DatabaseTypeSQLite, want: "(bucket_ts / 1800) * 1800"},
	}

	for _, test := range tests {
		t.Run(string(test.databaseType), func(t *testing.T) {
			common.SetMainDatabaseType(test.databaseType)
			assert.Equal(t, test.want, channelMetricBucketExpr(1800))
		})
	}
}

// The 7d range rolls 2016 storage buckets per channel into 84 display buckets,
// so the rollup has to happen in SQL. Counters must be summed per channel, and
// the returned timestamp must be the display bucket start.
func TestGetChannelMetricBucketsRollsStorageBucketsUpPerChannel(t *testing.T) {
	t.Cleanup(func() { DB.Exec("DELETE FROM channel_metrics") })

	rows := []ChannelMetric{
		// Channel 11: three 5min buckets inside the same 30min slot.
		{ChannelId: 11, BucketTs: 1800, AttemptCount: 4, SuccessCount: 4, TotalLatencyMs: 400},
		{ChannelId: 11, BucketTs: 2100, AttemptCount: 6, SuccessCount: 3, TotalLatencyMs: 1200},
		{ChannelId: 11, BucketTs: 3300, AttemptCount: 2, SuccessCount: 0, TotalLatencyMs: 200},
		// Channel 11 in the next slot.
		{ChannelId: 11, BucketTs: 3600, AttemptCount: 5, SuccessCount: 5, TotalLatencyMs: 500},
		// Channel 22 shares the first slot and must stay a separate row.
		{ChannelId: 22, BucketTs: 2400, AttemptCount: 1, SuccessCount: 1, TotalLatencyMs: 90},
		// Outside the queried range.
		{ChannelId: 11, BucketTs: 300, AttemptCount: 99, SuccessCount: 0, TotalLatencyMs: 9900},
	}
	for index := range rows {
		require.NoError(t, UpsertChannelMetric(&rows[index]))
	}

	buckets, err := GetChannelMetricBuckets(1800, 5399, 1800)
	require.NoError(t, err)
	require.Len(t, buckets, 3)

	type key struct {
		channelId int
		bucketTs  int64
	}
	byKey := make(map[key]ChannelMetricBucket, len(buckets))
	for _, bucket := range buckets {
		byKey[key{channelId: bucket.ChannelId, bucketTs: bucket.BucketTs}] = bucket
	}

	first := byKey[key{channelId: 11, bucketTs: 1800}]
	assert.EqualValues(t, 12, first.AttemptCount)
	assert.EqualValues(t, 7, first.SuccessCount)
	assert.EqualValues(t, 1800, first.TotalLatencyMs)

	second := byKey[key{channelId: 11, bucketTs: 3600}]
	assert.EqualValues(t, 5, second.AttemptCount)

	other := byKey[key{channelId: 22, bucketTs: 1800}]
	assert.EqualValues(t, 1, other.AttemptCount, "a second channel in the same slot must not be merged in")
}

func TestUpsertChannelMetricIgnoresEmptySample(t *testing.T) {
	t.Cleanup(func() { DB.Exec("DELETE FROM channel_metrics") })

	require.NoError(t, UpsertChannelMetric(nil))
	require.NoError(t, UpsertChannelMetric(&ChannelMetric{ChannelId: 1, BucketTs: 300}))

	buckets, err := GetChannelMetricBuckets(0, 3600, 300)
	require.NoError(t, err)
	assert.Empty(t, buckets)
}
