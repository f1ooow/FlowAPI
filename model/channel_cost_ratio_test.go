package model

import (
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The snapshot column is a fixed-point integer so the same ratio round-trips
// to the same integer on every supported database. Anything outside the
// accepted range encodes to the 0 sentinel rather than to a plausible-looking
// ratio, which would be indistinguishable from an observed value.
func TestChannelCostRatioEncodingRoundTrips(t *testing.T) {
	tests := []struct {
		name  string
		ratio float64
		want  int64
	}{
		{name: "neutral", ratio: 1, want: 10_000},
		{name: "two decimal places", ratio: 1.25, want: 12_500},
		{name: "below one", ratio: 0.5, want: 5_000},
		{name: "minimum", ratio: constant.MinChannelCostRatio, want: 100},
		{name: "maximum", ratio: constant.MaxChannelCostRatio, want: 10_000_000},
		{name: "zero is not representable", ratio: 0, want: 0},
		{name: "negative is not representable", ratio: -1, want: 0},
		{name: "above maximum is not representable", ratio: constant.MaxChannelCostRatio + 1, want: 0},
		{name: "NaN is not representable", ratio: math.NaN(), want: 0},
		{name: "infinity is not representable", ratio: math.Inf(1), want: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scaled := EncodeChannelCostRatio(test.ratio)
			assert.Equal(t, test.want, scaled)
			if test.want == 0 {
				return
			}
			// The decode is a power-of-ten division of a small integer, so it
			// must reproduce the input bit for bit and add no error before the
			// quota helpers see it.
			assert.Equal(t, test.ratio, DecodeChannelCostRatio(scaled))
		})
	}
}

func TestParseSnapshotChannelCostRatio(t *testing.T) {
	tests := []struct {
		name  string
		other map[string]interface{}
		want  float64
		ok    bool
	}{
		{
			name: "recorded ratio",
			other: map[string]interface{}{
				"admin_info": map[string]interface{}{
					"billing_ratios": map[string]interface{}{"channel_ratio": 0.25},
				},
			},
			ok:   true,
			want: 0.25,
		},
		{
			// A channel genuinely priced at 1.0 records 1, and 1 is taken at
			// face value.
			name: "recorded one is trusted",
			other: map[string]interface{}{
				"admin_info": map[string]interface{}{
					"billing_ratios": map[string]interface{}{"channel_ratio": 1.0},
				},
			},
			ok:   true,
			want: 1,
		},
		{
			name:  "no admin info",
			other: map[string]interface{}{"group_ratio": 1.0},
		},
		{
			name: "no billing ratios",
			other: map[string]interface{}{
				"admin_info": map[string]interface{}{"request_path": "/v1/messages"},
			},
		},
		{
			name: "billing ratios without a channel ratio",
			other: map[string]interface{}{
				"admin_info": map[string]interface{}{
					"billing_ratios": map[string]interface{}{"effective_ratio": 1.5},
				},
			},
		},
		{
			name: "channel ratio is not a number",
			other: map[string]interface{}{
				"admin_info": map[string]interface{}{
					"billing_ratios": map[string]interface{}{"channel_ratio": "0.5"},
				},
			},
		},
		{
			name: "channel ratio is zero",
			other: map[string]interface{}{
				"admin_info": map[string]interface{}{
					"billing_ratios": map[string]interface{}{"channel_ratio": 0.0},
				},
			},
		},
		{
			name: "channel ratio is negative",
			other: map[string]interface{}{
				"admin_info": map[string]interface{}{
					"billing_ratios": map[string]interface{}{"channel_ratio": -2.0},
				},
			},
		},
		{
			name: "channel ratio is out of range",
			other: map[string]interface{}{
				"admin_info": map[string]interface{}{
					"billing_ratios": map[string]interface{}{"channel_ratio": constant.MaxChannelCostRatio * 2},
				},
			},
		},
		{name: "nil other"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ratio, ok := ParseSnapshotChannelCostRatio(test.other)
			assert.Equal(t, test.ok, ok)
			if test.ok {
				assert.Equal(t, test.want, ratio)
			}
		})
	}
}

// A log that recorded the ratio it ran at is trusted; a log that recorded
// nothing is priced at the channel's current configuration and flagged, so an
// inferred value can never be read as an observed one.
func TestSnapshotChannelCostRatioForLogMarksOnlyInferredRatios(t *testing.T) {
	recorded := map[string]interface{}{
		"admin_info": map[string]interface{}{
			"billing_ratios": map[string]interface{}{"channel_ratio": 2.5},
		},
	}
	scaled, estimated := SnapshotChannelCostRatioForLog(recorded, 9.0)
	assert.EqualValues(t, 25_000, scaled, "the log's own ratio wins over the fallback")
	assert.False(t, estimated)

	scaled, estimated = SnapshotChannelCostRatioForLog(map[string]interface{}{}, 9.0)
	assert.EqualValues(t, 90_000, scaled, "the fallback fills in for the missing snapshot")
	assert.True(t, estimated)
}

// A path that carries no billing snapshot at all (the violation-fee charge)
// must still produce a usable, flagged snapshot rather than panicking on the
// missing map or asserting the neutral ratio as if it were observed.
func TestSnapshotChannelCostRatioForLogHandlesAbsentBillingRatios(t *testing.T) {
	// The shape service/violation_fee.go writes: no admin_info at all.
	violationFeeOther := map[string]interface{}{
		"violation_fee": true,
		"fee_quota":     100,
		"group_ratio":   1.0,
		"status_code":   400,
	}
	scaled, estimated := SnapshotChannelCostRatioForLog(violationFeeOther, 1.75)
	assert.EqualValues(t, 17_500, scaled)
	assert.True(t, estimated)
}

// The billing snapshot the text/audio/task paths write must be readable by the
// snapshot reader: this is the wire format contract between the writer
// (service.attachBillingRatioInfo) and the historical backfill.
func TestParseSnapshotReadsWrittenBillingRatiosShape(t *testing.T) {
	// Exactly the structure attachBillingRatioInfo stores.
	other := map[string]interface{}{
		"group_ratio": 2.4,
		"admin_info": map[string]interface{}{
			"billing_ratios": map[string]interface{}{
				"base_group_ratio":      2.4,
				"user_group_ratio":      1.0,
				"channel_ratio":         1.5,
				"include_channel_ratio": true,
				"effective_ratio":       2.4,
				"legacy_override":       false,
			},
		},
	}
	encoded, err := common.Marshal(other)
	require.NoError(t, err)
	// Round-trip through JSON the way a stored log does, so the reader is
	// exercised against decoded output rather than a Go literal.
	decoded, err := common.StrToMap(string(encoded))
	require.NoError(t, err)

	ratio, ok := ParseSnapshotChannelCostRatio(decoded)
	require.True(t, ok)
	assert.Equal(t, 1.5, ratio)
}

// A consume log written on the billing path records the ratio the request was
// resolved to, read from the same request context the distributor populates.
func TestRecordConsumeLogSnapshotsContextCostRatio(t *testing.T) {
	db := useIsolatedLogDB(t)
	previousDataExport := common.DataExportEnabled
	common.DataExportEnabled = false
	t.Cleanup(func() { common.DataExportEnabled = previousDataExport })

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Set(common.RequestIdKey, "cost-ratio-consume")
	c.Set(string(constant.ContextKeyChannelCostRatio), 1.75)

	RecordConsumeLog(c, 0, RecordConsumeLogParams{
		ChannelId: 95,
		ModelName: "claude-fable-5",
		Quota:     300,
		Other:     map[string]interface{}{"group_ratio": 1.0},
	})

	var log Log
	require.NoError(t, db.Where("request_id = ?", "cost-ratio-consume").First(&log).Error)
	assert.EqualValues(t, 17_500, log.ChannelCostRatio)
	assert.False(t, log.ChannelCostRatioEstimated, "a ratio read from the request is observed, not inferred")
}

// channel-test builds its own other map through service.GenerateTextOtherInfo;
// this asserts the reader copes with a billing snapshot that exists but was
// written without a channel ratio, which must fall back rather than panic.
func TestSnapshotChannelCostRatioForLogHandlesBillingRatiosWithoutChannelRatio(t *testing.T) {
	other := map[string]interface{}{
		"model_ratio": 1.0,
		"group_ratio": 1.0,
		"admin_info": map[string]interface{}{
			"billing_ratios": map[string]interface{}{
				"effective_ratio": 1.0,
			},
		},
	}
	scaled, estimated := SnapshotChannelCostRatioForLog(other, 0.5)
	assert.EqualValues(t, 5_000, scaled)
	assert.True(t, estimated, "the effective group ratio is not a channel ratio")
}

// A row with no snapshot at all is an estimate at the neutral ratio; its flag
// says nothing, so IsEstimate must still report true.
func TestChannelCostRatioBucketTreatsMissingSnapshotAsEstimate(t *testing.T) {
	recorded := NewChannelCostRatioBucket(25_000, false)
	assert.False(t, recorded.IsEstimate())
	assert.Equal(t, 2.5, recorded.Ratio)
	assert.EqualValues(t, 250, recorded.CostQuota(100))

	// Marked inferred by the backfill.
	inferred := NewChannelCostRatioBucket(20_000, true)
	assert.True(t, inferred.IsEstimate())
	assert.EqualValues(t, 200, inferred.CostQuota(100))

	// Never filled: no ratio, no flag. Unknown is an estimate.
	missing := NewChannelCostRatioBucket(0, false)
	assert.True(t, missing.IsEstimate())
	assert.Equal(t, 1.0, missing.Ratio)
	assert.EqualValues(t, 100, missing.CostQuota(100))
}
