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

	metas := map[string]groupMonitoringModelMeta{
		"gpt-4o-mini": {Icon: "OpenAI", VendorName: "OpenAI", VendorIcon: "OpenAI.Color"},
	}

	// 15 minute display buckets over a one hour window. The latency window
	// covers the whole series here, so latency and availability read the same
	// samples; the narrower latency window is exercised separately below.
	summaries := buildGroupMonitoringGroups(rows, groups, metas, 0, 4, 900, 0)
	require.Len(t, summaries, 2)

	assert.Equal(t, "default", summaries[0].GroupName)
	assert.Equal(t, "shared pool", summaries[0].Description)
	require.Len(t, summaries[0].Models, 3)

	chat := summaries[0].Models[0]
	assert.Equal(t, "gpt-4o-mini", chat.ModelName)
	assert.Equal(t, "OpenAI", chat.Icon)
	assert.Equal(t, "OpenAI", chat.VendorName)
	assert.Equal(t, "OpenAI.Color", chat.VendorIcon)
	assert.True(t, chat.HasData)
	// 19 successes over 22 requests. Averaging the three storage bucket rates
	// (100%, 0%, 100%) would produce 66.67 instead.
	assert.EqualValues(t, 22, chat.RequestCount)
	assert.Equal(t, 86.36, chat.AvailabilityRate)
	assert.Equal(t, groupMonitoringStateDegraded, chat.State)
	require.NotNil(t, chat.AvgLatencyMs)
	assert.EqualValues(t, 109, *chat.AvgLatencyMs)
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
	assert.Empty(t, image.VendorName, "a model missing from the pricing catalog must not fabricate a vendor")
	assert.True(t, image.HasData)
	assert.Equal(t, groupMonitoringStateHealthy, image.State)
	require.NotNil(t, image.AvgLatencyMs)
	assert.EqualValues(t, 120000, *image.AvgLatencyMs)
	assert.Nil(t, image.AvgTtftMs, "non-streaming models must report no TTFT sample instead of 0 ms")
	assert.EqualValues(t, 0, image.TtftSampleCount)

	unused := summaries[0].Models[2]
	assert.False(t, unused.HasData)
	assert.Equal(t, groupMonitoringStateNoData, unused.State)
	assert.EqualValues(t, 0, unused.RequestCount)
	assert.Nil(t, unused.AvgLatencyMs)
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
			summaries := buildGroupMonitoringGroups(rows, groups, nil, 0, 1, 300, 0)
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

func TestGroupMonitoringVisibleGroups(t *testing.T) {
	groups := []operation_setting.GroupMonitoringGroup{
		// Configured before the allow list existed: a nil list must stay visible
		// to everyone, otherwise an upgrade blanks the page for every non-admin.
		{Group: "legacy", Models: []string{"gpt-4o-mini"}},
		{Group: "admin-only", VisibleToGroups: []string{}, Models: []string{"gpt-4o-mini"}},
		{Group: "codex-pro", VisibleToGroups: []string{"vip", "internal"}, Models: []string{"gpt-4o-mini"}},
		{Group: "vip-only", VisibleToGroups: []string{"vip"}, Models: []string{"gpt-4o-mini"}},
	}

	tests := []struct {
		name      string
		isAdmin   bool
		userGroup string
		want      []string
	}{
		{
			name:    "administrators ignore the allow list",
			isAdmin: true,
			// An admin whose own group is listed nowhere still sees everything.
			userGroup: "default",
			want:      []string{"legacy", "admin-only", "codex-pro", "vip-only"},
		},
		{
			name:      "listed user group sees the groups it is allowed in",
			userGroup: "vip",
			want:      []string{"legacy", "codex-pro", "vip-only"},
		},
		{
			name:      "another listed group sees only its own entry",
			userGroup: "internal",
			want:      []string{"legacy", "codex-pro"},
		},
		{
			name:      "unlisted user group sees only the unrestricted groups",
			userGroup: "default",
			want:      []string{"legacy"},
		},
		{
			name:      "empty user group never matches a non-empty allow list",
			userGroup: "",
			want:      []string{"legacy"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			visible := groupMonitoringVisibleGroups(groups, test.isAdmin, test.userGroup)
			assert.Equal(t, test.want, groupMonitoringGroupNames(visible))
		})
	}
}

func TestGroupMonitoringLatencyStart(t *testing.T) {
	// Aligned to a 5 minute grid so every storage width divides it evenly.
	const seriesEnd = int64(1_700_000_100)

	tests := []struct {
		name           string
		storageBucket  string
		storageSeconds int64
	}{
		{name: "5min storage covers twelve buckets", storageBucket: "5min", storageSeconds: 300},
		{name: "minute storage covers sixty buckets", storageBucket: "minute", storageSeconds: 60},
		{name: "hourly storage covers the single bucket it can offer", storageBucket: "hour", storageSeconds: 3600},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			overridePerfMetricsSetting(t, test.storageBucket, 5)
			start := groupMonitoringLatencyStart(seriesEnd)
			assert.Equal(t, seriesEnd-groupMonitoringLatencyWindowSeconds, start)
			assert.EqualValues(t, 0, (seriesEnd-start)%test.storageSeconds, "the window must cover whole storage buckets")
		})
	}
}

func TestBuildGroupMonitoringGroupsLatencyUsesRecentWindowOnly(t *testing.T) {
	groups := []operation_setting.GroupMonitoringGroup{{
		Group:  "default",
		Models: []string{"streaming-model", "image-model", "quiet-model", "idle-model"},
	}}
	// Two hours of 5 minute storage buckets rendered as 30 minute display
	// buckets; the latency window is the last hour, so it starts at 3600.
	rows := []model.PerfMetricGroupBucket{
		// Older half: slow and failing, must move the 24h availability but must
		// not touch either latency average.
		{Group: "default", ModelName: "streaming-model", BucketTs: 0, RequestCount: 10, SuccessCount: 5, TotalLatencyMs: 200_000, TtftSumMs: 100_000, TtftCount: 10},
		// Recent half.
		{Group: "default", ModelName: "streaming-model", BucketTs: 3600, RequestCount: 6, SuccessCount: 6, TotalLatencyMs: 3_000, TtftSumMs: 1_200, TtftCount: 6},
		{Group: "default", ModelName: "streaming-model", BucketTs: 6900, RequestCount: 4, SuccessCount: 4, TotalLatencyMs: 1_000, TtftSumMs: 600, TtftCount: 2},
		// Non-streaming: recent requests but no TTFT sample at all.
		{Group: "default", ModelName: "image-model", BucketTs: 3600, RequestCount: 4, SuccessCount: 4, TotalLatencyMs: 480_000},
		// Streamed earlier in the day, then a quiet hour that only served a
		// non-streamed request: the 24h TTFT must survive as the fallback.
		{Group: "default", ModelName: "quiet-model", BucketTs: 0, RequestCount: 10, SuccessCount: 10, TotalLatencyMs: 100_000, TtftSumMs: 20_000, TtftCount: 10},
		{Group: "default", ModelName: "quiet-model", BucketTs: 3600, RequestCount: 1, SuccessCount: 1, TotalLatencyMs: 30_000},
		// Served traffic today but nothing in the latency window.
		{Group: "default", ModelName: "idle-model", BucketTs: 0, RequestCount: 20, SuccessCount: 20, TotalLatencyMs: 40_000, TtftSumMs: 20_000, TtftCount: 20},
	}

	summaries := buildGroupMonitoringGroups(rows, groups, nil, 0, 4, 1800, 3600)
	require.Len(t, summaries, 1)
	require.Len(t, summaries[0].Models, 4)

	streaming := summaries[0].Models[0]
	assert.EqualValues(t, 20, streaming.RequestCount, "the request count stays on the full window")
	assert.Equal(t, 75.0, streaming.AvailabilityRate, "availability stays on the full window")
	assert.Equal(t, groupMonitoringStateDown, streaming.State)
	require.NotNil(t, streaming.AvgTtftMs)
	// 1800ms over 8 samples, not the 5655ms the whole window would yield.
	assert.EqualValues(t, 225, *streaming.AvgTtftMs)
	assert.EqualValues(t, 8, streaming.TtftSampleCount, "only the samples inside the latency window count")
	require.NotNil(t, streaming.AvgLatencyMs)
	assert.EqualValues(t, 400, *streaming.AvgLatencyMs)
	// Tier 1: the last hour holds streamed samples, so the card reads the 1h
	// mean. The day mean is carried alongside and is much slower (5655 vs 225
	// ms), which is exactly why the card must not pick it on its own.
	require.NotNil(t, streaming.AvgTtftMs24h)
	assert.EqualValues(t, 5655, *streaming.AvgTtftMs24h)
	assert.EqualValues(t, 18, streaming.TtftSampleCount24h)

	image := summaries[0].Models[1]
	assert.Nil(t, image.AvgTtftMs)
	assert.Nil(t, image.AvgTtftMs24h, "a model that never streams has no 24h TTFT either")
	assert.EqualValues(t, 0, image.TtftSampleCount24h)
	require.NotNil(t, image.AvgLatencyMs)
	// Same window as the streaming card, so the two latency figures compare.
	assert.EqualValues(t, 120_000, *image.AvgLatencyMs)

	// Tier 2: the last hour produced only a non-streamed request, so the card
	// falls back to the 24h first-token mean rather than to total latency.
	quiet := summaries[0].Models[2]
	assert.Nil(t, quiet.AvgTtftMs)
	require.NotNil(t, quiet.AvgTtftMs24h)
	assert.EqualValues(t, 2000, *quiet.AvgTtftMs24h)
	assert.EqualValues(t, 10, quiet.TtftSampleCount24h)
	require.NotNil(t, quiet.AvgLatencyMs)
	assert.EqualValues(t, 30_000, *quiet.AvgLatencyMs)

	idle := summaries[0].Models[3]
	assert.True(t, idle.HasData, "the model did serve traffic inside the availability window")
	assert.Equal(t, groupMonitoringStateHealthy, idle.State, "the badge must not follow the latency window")
	assert.EqualValues(t, 20, idle.RequestCount)
	assert.Nil(t, idle.AvgLatencyMs, "no request in the latency window must not fall back to the 24h mean")
	assert.Nil(t, idle.AvgTtftMs)
	assert.EqualValues(t, 0, idle.TtftSampleCount)
}

func groupMonitoringGroupNames(groups []operation_setting.GroupMonitoringGroup) []string {
	names := make([]string, 0, len(groups))
	for _, group := range groups {
		names = append(names, group.Group)
	}
	return names
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
