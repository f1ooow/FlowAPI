package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// overridePerfMetricsSetting rewrites the registered perf metrics config for one
// test. The storage bucket width and the flush interval both feed the display
// bucket resolution, and neither is reachable through a setter.
func overridePerfMetricsSetting(t *testing.T, bucketTime string, flushIntervalMinutes int) {
	t.Helper()
	setting, ok := config.GlobalConfig.Get("perf_metrics_setting").(*perf_metrics_setting.PerfMetricsSetting)
	require.True(t, ok)
	previous := *setting
	t.Cleanup(func() { *setting = previous })
	setting.BucketTime = bucketTime
	setting.FlushInterval = flushIntervalMinutes
}

func TestGroupMonitoringDisplaySeconds(t *testing.T) {
	tests := []struct {
		name          string
		storageBucket string
		bucketMinutes int
		want          int64
	}{
		{name: "5min storage keeps 5min display", storageBucket: "5min", bucketMinutes: 5, want: 300},
		{name: "5min storage keeps 15min display", storageBucket: "5min", bucketMinutes: 15, want: 900},
		{name: "5min storage keeps 60min display", storageBucket: "5min", bucketMinutes: 60, want: 3600},
		{name: "hourly storage degrades finer display", storageBucket: "hour", bucketMinutes: 5, want: 3600},
		{name: "minute storage keeps configured display", storageBucket: "minute", bucketMinutes: 5, want: 300},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			overridePerfMetricsSetting(t, test.storageBucket, 5)
			assert.Equal(t, test.want, groupMonitoringDisplaySeconds(test.bucketMinutes))
		})
	}
}

func TestGroupMonitoringSeriesEndDropsUnflushedBuckets(t *testing.T) {
	// 1_700_000_123 sits 23s into a 5min bucket and 923s into an hourly one.
	const midBucketNow = int64(1_700_000_123)
	// 1_699_999_300 sits 100s into an hourly bucket: the previous hourly bucket
	// closed well within one flush interval and cannot be trusted yet.
	const justAfterHourNow = int64(1_699_999_300)

	tests := []struct {
		name           string
		now            int64
		flushMinutes   int
		displaySeconds int64
		want           int64
	}{
		{
			// The bucket that closed 23s ago may still be unflushed on some
			// nodes even though display and flush widths are equal.
			name:           "equal display and flush widths still drop the just-closed bucket",
			now:            midBucketNow,
			flushMinutes:   5,
			displaySeconds: 300,
			want:           midBucketNow - midBucketNow%300 - 300,
		},
		{
			name:           "display narrower than flush drops every bucket inside the flush interval",
			now:            midBucketNow,
			flushMinutes:   10,
			displaySeconds: 300,
			want:           midBucketNow - midBucketNow%300 - 600,
		},
		{
			// Rewinding by 300s stays inside the in-flight hour, so alignment
			// lands on the same bound: no data is discarded needlessly.
			name:           "wide display bucket keeps the last settled bucket",
			now:            midBucketNow,
			flushMinutes:   5,
			displaySeconds: 3600,
			want:           midBucketNow - midBucketNow%3600,
		},
		{
			name:           "wide display bucket drops the hour that closed within the flush interval",
			now:            justAfterHourNow,
			flushMinutes:   5,
			displaySeconds: 3600,
			want:           justAfterHourNow - justAfterHourNow%3600 - 3600,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			overridePerfMetricsSetting(t, "5min", test.flushMinutes)
			end := groupMonitoringSeriesEnd(test.now, test.displaySeconds)
			assert.Equal(t, test.want, end)
			// end is the exclusive bound, so the last rendered bucket closes at
			// end. It must have been closed for at least one full flush
			// interval, otherwise a node may not have written it out yet.
			assert.LessOrEqual(t, end, test.now-int64(test.flushMinutes)*60)
		})
	}
}

func TestBuildGroupMonitoringGroupsRollsUpStorageBuckets(t *testing.T) {
	groups := []operation_setting.GroupMonitoringGroup{
		{Group: "default", Description: "shared pool", Models: []string{"gpt-4o-mini", "gpt-image-2", "unused-model"}},
		{Group: "vip", Description: "priority pool", Models: []string{"gpt-4o-mini"}},
	}
	rows := []model.PerfMetricGroupBucket{
		{Group: "default", ModelName: "gpt-4o-mini", BucketTs: 0, RequestCount: 10, SuccessCount: 10, TotalLatencyMs: 1000, TtftSumMs: 500, TtftCount: 10},
		{Group: "default", ModelName: "gpt-4o-mini", BucketTs: 300, RequestCount: 2, SuccessCount: 0, TotalLatencyMs: 400},
		{Group: "default", ModelName: "gpt-4o-mini", BucketTs: 600, RequestCount: 8, SuccessCount: 8, TotalLatencyMs: 800, TtftSumMs: 500, TtftCount: 10},
		{Group: "default", ModelName: "gpt-4o-mini", BucketTs: 900, RequestCount: 2, SuccessCount: 1, TotalLatencyMs: 200},
		// Outside the rendered window: must not reach the buckets or the totals.
		{Group: "default", ModelName: "gpt-4o-mini", BucketTs: 3600, RequestCount: 100, SuccessCount: 0, TotalLatencyMs: 100000},
		{Group: "default", ModelName: "gpt-image-2", BucketTs: 0, RequestCount: 5, SuccessCount: 5, TotalLatencyMs: 600000},
		{Group: "vip", ModelName: "gpt-4o-mini", BucketTs: 0, RequestCount: 4, SuccessCount: 1, TotalLatencyMs: 800},
	}

	// 15 minute display buckets over a one hour window.
	summaries := buildGroupMonitoringGroups(rows, groups, 0, 4, 900)
	require.Len(t, summaries, 2)

	assert.Equal(t, "default", summaries[0].GroupName)
	assert.Equal(t, "shared pool", summaries[0].Description)
	require.Len(t, summaries[0].Models, 3)

	chat := summaries[0].Models[0]
	assert.Equal(t, "gpt-4o-mini", chat.ModelName)
	assert.True(t, chat.HasData)
	// 19 successes over 22 requests. Averaging the three storage bucket rates
	// (100%, 0%, 100%) would produce 66.67 instead.
	assert.EqualValues(t, 22, chat.RequestCount)
	assert.Equal(t, 86.36, chat.AvailabilityRate)
	assert.Equal(t, groupMonitoringStateDegraded, chat.State)
	assert.EqualValues(t, 109, chat.AvgLatencyMs)
	require.NotNil(t, chat.AvgTtftMs)
	assert.EqualValues(t, 50, *chat.AvgTtftMs)
	assert.EqualValues(t, 20, chat.TtftSampleCount)

	require.Len(t, chat.Buckets, 4)
	assert.EqualValues(t, 0, chat.Buckets[0].Ts)
	assert.EqualValues(t, 20, chat.Buckets[0].RequestCount)
	assert.EqualValues(t, 18, chat.Buckets[0].SuccessCount)
	assert.Equal(t, groupMonitoringStateDegraded, chat.Buckets[0].State)
	assert.EqualValues(t, 900, chat.Buckets[1].Ts)
	assert.EqualValues(t, 2, chat.Buckets[1].RequestCount)
	assert.Equal(t, groupMonitoringStateNoData, chat.Buckets[1].State, "two requests are below the minimum sample threshold")
	assert.EqualValues(t, 0, chat.Buckets[2].RequestCount)
	assert.Equal(t, groupMonitoringStateNoData, chat.Buckets[2].State)
	assert.EqualValues(t, 2700, chat.Buckets[3].Ts)
	assert.Equal(t, groupMonitoringStateNoData, chat.Buckets[3].State)

	image := summaries[0].Models[1]
	assert.True(t, image.HasData)
	assert.Equal(t, groupMonitoringStateHealthy, image.State)
	assert.EqualValues(t, 120000, image.AvgLatencyMs)
	assert.Nil(t, image.AvgTtftMs, "non-streaming models must report no TTFT sample instead of 0 ms")
	assert.EqualValues(t, 0, image.TtftSampleCount)

	unused := summaries[0].Models[2]
	assert.False(t, unused.HasData)
	assert.Equal(t, groupMonitoringStateNoData, unused.State)
	assert.EqualValues(t, 0, unused.RequestCount)
	assert.Nil(t, unused.AvgTtftMs)
	require.Len(t, unused.Buckets, 4)

	assert.Equal(t, "vip", summaries[1].GroupName)
	require.Len(t, summaries[1].Models, 1)
	vipChat := summaries[1].Models[0]
	assert.EqualValues(t, 4, vipChat.RequestCount, "the same model in another group must stay separate")
	assert.Equal(t, float64(25), vipChat.AvailabilityRate)
	assert.Equal(t, groupMonitoringStateDown, vipChat.State)
}

func TestBuildGroupMonitoringGroupsBucketStateThresholds(t *testing.T) {
	tests := []struct {
		name         string
		requestCount int64
		successCount int64
		want         string
	}{
		{name: "below minimum samples", requestCount: 2, successCount: 2, want: groupMonitoringStateNoData},
		{name: "at minimum samples all succeeded", requestCount: 3, successCount: 3, want: groupMonitoringStateHealthy},
		{name: "at minimum samples one failed", requestCount: 3, successCount: 2, want: groupMonitoringStateDown},
		{name: "exactly at healthy threshold", requestCount: 100, successCount: 99, want: groupMonitoringStateHealthy},
		{name: "just below healthy threshold", requestCount: 100, successCount: 98, want: groupMonitoringStateDegraded},
		{name: "exactly at degraded threshold", requestCount: 100, successCount: 80, want: groupMonitoringStateDegraded},
		{name: "just below degraded threshold", requestCount: 100, successCount: 79, want: groupMonitoringStateDown},
	}

	groups := []operation_setting.GroupMonitoringGroup{{Group: "default", Models: []string{"gpt-4o-mini"}}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rows := []model.PerfMetricGroupBucket{{
				Group:        "default",
				ModelName:    "gpt-4o-mini",
				BucketTs:     0,
				RequestCount: test.requestCount,
				SuccessCount: test.successCount,
			}}
			summaries := buildGroupMonitoringGroups(rows, groups, 0, 1, 300)
			require.Len(t, summaries, 1)
			require.Len(t, summaries[0].Models[0].Buckets, 1)
			assert.Equal(t, test.want, summaries[0].Models[0].Buckets[0].State)
			// The header rate keeps every request, the threshold only gates the
			// per-bucket colour.
			assert.EqualValues(t, test.requestCount, summaries[0].Models[0].RequestCount)
			assert.True(t, summaries[0].Models[0].HasData)
		})
	}
}

func TestCanonicalGroupMonitoringModel(t *testing.T) {
	tests := []struct {
		name       string
		configured string
		available  []string
		want       string
		wantOK     bool
	}{
		{name: "exact", configured: "claude-sonnet-4-6", available: []string{"claude-sonnet-4-6"}, want: "claude-sonnet-4-6", wantOK: true},
		{name: "legacy case mismatch", configured: "Claude-sonnet-4-6", available: []string{"claude-opus-4-6", "claude-sonnet-4-6"}, want: "claude-sonnet-4-6", wantOK: true},
		{name: "unknown model", configured: "claude-sonnet-5", available: []string{"claude-sonnet-4-6"}, wantOK: false},
		{name: "ambiguous case variants", configured: "CLAUDE", available: []string{"claude", "Claude"}, wantOK: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := canonicalGroupMonitoringModel(test.configured, test.available)
			assert.Equal(t, test.wantOK, ok)
			assert.Equal(t, test.want, got)
		})
	}
}
