package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// GetPerfMetricGroupBuckets is the only perf metrics query that keeps the group
// dimension and the TTFT counters, and it quotes the reserved "group" column by
// hand, so it needs to run against a real database.
func TestGetPerfMetricGroupBucketsKeepsGroupAndTtftCounters(t *testing.T) {
	t.Cleanup(func() { DB.Exec("DELETE FROM perf_metrics") })

	rows := []PerfMetric{
		{ModelName: "gpt-4o-mini", Group: "default", BucketTs: 300, RequestCount: 10, SuccessCount: 9, TotalLatencyMs: 2000, TtftSumMs: 500, TtftCount: 10},
		{ModelName: "gpt-4o-mini", Group: "vip", BucketTs: 300, RequestCount: 4, SuccessCount: 4, TotalLatencyMs: 400, TtftSumMs: 80, TtftCount: 4},
		{ModelName: "gpt-image-2", Group: "default", BucketTs: 600, RequestCount: 3, SuccessCount: 2, TotalLatencyMs: 300000},
		{ModelName: "gpt-4o-mini", Group: "hidden", BucketTs: 300, RequestCount: 7, SuccessCount: 7, TotalLatencyMs: 700},
		{ModelName: "unmonitored", Group: "default", BucketTs: 300, RequestCount: 5, SuccessCount: 5, TotalLatencyMs: 500},
		{ModelName: "gpt-4o-mini", Group: "default", BucketTs: 9000, RequestCount: 6, SuccessCount: 0, TotalLatencyMs: 600},
	}
	for index := range rows {
		require.NoError(t, UpsertPerfMetric(&rows[index]))
	}

	buckets, err := GetPerfMetricGroupBuckets([]string{"default", "vip"}, []string{"gpt-4o-mini", "gpt-image-2"}, 0, 3600)
	require.NoError(t, err)
	require.Len(t, buckets, 3)

	byKey := make(map[string]PerfMetricGroupBucket, len(buckets))
	for _, bucket := range buckets {
		byKey[bucket.Group+"/"+bucket.ModelName] = bucket
	}

	chat := byKey["default/gpt-4o-mini"]
	assert.EqualValues(t, 300, chat.BucketTs)
	assert.EqualValues(t, 10, chat.RequestCount)
	assert.EqualValues(t, 9, chat.SuccessCount)
	assert.EqualValues(t, 2000, chat.TotalLatencyMs)
	assert.EqualValues(t, 500, chat.TtftSumMs)
	assert.EqualValues(t, 10, chat.TtftCount)

	vip := byKey["vip/gpt-4o-mini"]
	assert.EqualValues(t, 4, vip.RequestCount, "the same model in another group must stay a separate row")

	image := byKey["default/gpt-image-2"]
	assert.EqualValues(t, 3, image.RequestCount)
	assert.EqualValues(t, 0, image.TtftCount, "non-streaming models report no TTFT samples")
}

func TestGetPerfMetricGroupBucketsWithoutScopeReturnsNothing(t *testing.T) {
	t.Cleanup(func() { DB.Exec("DELETE FROM perf_metrics") })

	row := PerfMetric{ModelName: "gpt-4o-mini", Group: "default", BucketTs: 300, RequestCount: 1, SuccessCount: 1}
	require.NoError(t, UpsertPerfMetric(&row))

	buckets, err := GetPerfMetricGroupBuckets(nil, []string{"gpt-4o-mini"}, 0, 3600)
	require.NoError(t, err)
	assert.Empty(t, buckets)

	buckets, err = GetPerfMetricGroupBuckets([]string{"default"}, nil, 0, 3600)
	require.NoError(t, err)
	assert.Empty(t, buckets)
}
