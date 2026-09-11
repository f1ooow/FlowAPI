package perfmetrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recent_success_rates drives the model square status bars. Each bar must cover a
// fixed wall-clock hour so that lowering the configured bucket width does not
// shrink the sampled window, and each rate must come from summed counters rather
// than an average of per-bucket rates.
func TestRecentSuccessRatesRollsBucketsIntoFixedHourWindows(t *testing.T) {
	const hour = int64(3600)
	base := int64(1757000000)
	base -= base % hour

	cases := []struct {
		name     string
		buckets  map[int64]counters
		expected []float64
	}{
		{
			name: "hourly buckets keep one rate per hour",
			buckets: map[int64]counters{
				base:            {requestCount: 100, successCount: 100},
				base + hour:     {requestCount: 100, successCount: 90},
				base + 2*hour:   {requestCount: 100, successCount: 50},
				base + 3*hour:   {requestCount: 100, successCount: 80},
				base - 10*hour:  {requestCount: 100, successCount: 0},
				base - 200*hour: {requestCount: 100, successCount: 0},
			},
			expected: []float64{90, 50, 80},
		},
		{
			name: "5min buckets still span three hours, not fifteen minutes",
			buckets: map[int64]counters{
				// hour 0: healthy
				base:       {requestCount: 60, successCount: 60},
				base + 300: {requestCount: 60, successCount: 60},
				// hour 1: one bad 5min bucket diluted by a busy healthy one
				base + hour:       {requestCount: 2, successCount: 1},
				base + hour + 300: {requestCount: 98, successCount: 98},
				// hour 2: fully down
				base + 2*hour:       {requestCount: 10, successCount: 0},
				base + 2*hour + 300: {requestCount: 10, successCount: 0},
			},
			expected: []float64{100, 99, 0},
		},
		{
			name: "fewer windows than requested returns what exists",
			buckets: map[int64]counters{
				base:       {requestCount: 4, successCount: 3},
				base + 600: {requestCount: 4, successCount: 4},
			},
			expected: []float64{87.5},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rates := recentSuccessRates(tc.buckets, recentSuccessRateWindows)
			require.Len(t, rates, len(tc.expected))
			assert.Equal(t, tc.expected, rates)
		})
	}
}

func TestRecentSuccessRatesEmptyInput(t *testing.T) {
	assert.Nil(t, recentSuccessRates(nil, recentSuccessRateWindows))
	assert.Nil(t, recentSuccessRates(map[int64]counters{1757000000: {requestCount: 1, successCount: 1}}, 0))
}
