package model

import (
	"math"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"

	"github.com/gin-gonic/gin"
)

// channelCostRatioScale is the fixed-point scale of Log.ChannelCostRatio.
//
// The ratio is stored as an integer count of 1/10000 so that a snapshot is
// byte-identical across SQLite, MySQL and PostgreSQL. A float column would
// round-trip through each driver's own text encoding, which makes an exact
// audit comparison ("is this row still the ratio we wrote?") dialect
// dependent. Validation already refuses more than two decimal places
// (ValidateChannelCostRatio), so 1e4 leaves two spare digits of headroom
// rather than truncating anything reachable through the admin API.
const channelCostRatioScale = 10_000

// EncodeChannelCostRatio converts a ratio to its fixed-point snapshot form.
// It returns 0 for anything not representable: a snapshot of 0 is the "not
// recorded" sentinel, so an unusable input must not be stored as a
// plausible-looking ratio.
func EncodeChannelCostRatio(ratio float64) int64 {
	if !isValidChannelCostRatio(ratio) {
		return 0
	}
	return int64(math.Round(ratio * channelCostRatioScale))
}

// DecodeChannelCostRatio restores a snapshot to the float64 the quota helpers
// consume. The scaled value never exceeds 10_000_000, so the division is exact
// in binary floating point and introduces no second rounding step before
// common.AggregateQuotaRound. Returns 1 for the 0 sentinel: aggregating a row
// with no snapshot at the neutral ratio is the least surprising behaviour, and
// every caller that cares about the distinction checks the stored value first.
func DecodeChannelCostRatio(scaled int64) float64 {
	if scaled <= 0 {
		return 1
	}
	return float64(scaled) / channelCostRatioScale
}

// isValidChannelCostRatio reports whether a ratio is a finite positive number
// in the accepted range. NaN and +Inf must be rejected explicitly: both defeat
// a plain bounds comparison and would poison a cost total.
func isValidChannelCostRatio(ratio float64) bool {
	if math.IsNaN(ratio) || math.IsInf(ratio, 0) {
		return false
	}
	if ratio < constant.MinChannelCostRatio || ratio > constant.MaxChannelCostRatio {
		return false
	}
	return true
}

// ParseSnapshotChannelCostRatio extracts the channel cost ratio a billing
// snapshot recorded for a single request.
//
// It reads other.admin_info.billing_ratios.channel_ratio, which the billing
// path writes from the channel cost ratio resolved at route selection
// (service.attachBillingRatioInfo). That value is the ratio the request
// actually settled at, which is exactly what a historical snapshot should
// preserve.
//
// A stored 1 is taken at face value: a channel priced at 1.0 is the common
// case and is indistinguishable from a defaulted field, so the value is
// trusted whenever the field is present and sane. Only an absent, malformed,
// non-numeric, or out-of-range field is treated as "no snapshot", which makes
// the caller fall back to the channel's current ratio and flag the row as
// estimated. Over-reporting an estimate is the safe direction; passing an
// inferred ratio off as an observed one is not.
func ParseSnapshotChannelCostRatio(other map[string]interface{}) (float64, bool) {
	if other == nil {
		return 0, false
	}
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	if !ok || adminInfo == nil {
		return 0, false
	}
	billingRatios, ok := adminInfo["billing_ratios"].(map[string]interface{})
	if !ok || billingRatios == nil {
		return 0, false
	}
	raw, ok := billingRatios["channel_ratio"]
	if !ok || raw == nil {
		return 0, false
	}
	ratio, ok := raw.(float64)
	if !ok {
		// common.StrToMap decodes JSON numbers as float64; anything else here
		// means the field was written in an unexpected shape, so treat it as
		// absent rather than guessing a conversion.
		return 0, false
	}
	if !isValidChannelCostRatio(ratio) {
		return 0, false
	}
	return ratio, true
}

// SnapshotChannelCostRatioForLog returns the ChannelCostRatio and
// ChannelCostRatioEstimated pair to persist for a consume log.
//
// The ratio is read from the log's own billing snapshot first, so a log
// created through a path that never resolved a channel ratio still records the
// ratio the request ran at rather than nothing. fallbackRatio is the channel's
// current configuration and is used only when the snapshot carries no usable
// value, in which case the row is marked estimated.
func SnapshotChannelCostRatioForLog(other map[string]interface{}, fallbackRatio float64) (int64, bool) {
	if ratio, ok := ParseSnapshotChannelCostRatio(other); ok {
		if scaled := EncodeChannelCostRatio(ratio); scaled != 0 {
			return scaled, false
		}
	}
	if scaled := EncodeChannelCostRatio(fallbackRatio); scaled != 0 {
		return scaled, true
	}
	// Neither source yielded a usable ratio. Record the neutral ratio so cost
	// aggregation has a well-defined multiplier, and mark it estimated because
	// it is asserted rather than observed.
	return channelCostRatioScale, true
}

// ChannelCostRatioFromContext reads the channel cost ratio the distributor
// resolved for this request. It is the same context key the billing snapshot
// is built from, so a log written here and the snapshot agree by construction.
// Reports false when no ratio was resolved, letting the caller fall back to an
// estimated value instead of asserting a default.
func ChannelCostRatioFromContext(c *gin.Context) (float64, bool) {
	if c == nil {
		return 0, false
	}
	ratio, ok := common.GetContextKeyType[float64](c, constant.ContextKeyChannelCostRatio)
	if !ok || !isValidChannelCostRatio(ratio) {
		return 0, false
	}
	return ratio, true
}

// ResolveLogChannelCostRatio is the snapshot decision for a billing path that
// has a request context: prefer the ratio the request actually ran at, fall
// back to the channel's current configuration when the context carries none.
func ResolveLogChannelCostRatio(c *gin.Context, channelId int, other map[string]interface{}) (int64, bool) {
	if ratio, ok := ChannelCostRatioFromContext(c); ok {
		if scaled := EncodeChannelCostRatio(ratio); scaled != 0 {
			return scaled, false
		}
	}
	return SnapshotChannelCostRatioForLog(other, currentChannelCostRatio(channelId))
}

// currentChannelCostRatio reads a channel's configured cost ratio for use as a
// last-resort fallback. A missing channel (deleted, or an id that never
// existed) yields the neutral ratio, matching Channel.GetCostRatio on a nil
// receiver.
func currentChannelCostRatio(channelId int) float64 {
	if channelId <= 0 {
		return 1
	}
	channel, err := GetChannelById(channelId, true)
	if err != nil || channel == nil {
		return 1
	}
	return channel.GetCostRatio()
}

// ResolveTaskLogChannelCostRatio is ResolveLogChannelCostRatio for the
// asynchronous task billing path, which has no gin context to read the ratio
// from. It falls back to the channel row directly, so a task consume log still
// records a usable snapshot.
func ResolveTaskLogChannelCostRatio(other map[string]interface{}, channelId int) (int64, bool) {
	return SnapshotChannelCostRatioForLog(other, currentChannelCostRatio(channelId))
}

// ChannelCostRatioBucket groups stored snapshots by ratio for aggregation.
// Rows sharing a ratio are summed once and multiplied once, which keeps a
// per-row float multiply out of the aggregation while preserving the
// distinction between observed and estimated rows.
type ChannelCostRatioBucket struct {
	Scaled    int64
	Estimated bool
	Ratio     float64
}

func NewChannelCostRatioBucket(scaled int64, estimated bool) ChannelCostRatioBucket {
	return ChannelCostRatioBucket{
		Scaled:    scaled,
		Estimated: estimated,
		Ratio:     DecodeChannelCostRatio(scaled),
	}
}

// IsEstimate reports whether cost derived from this bucket is an approximation.
// A row with no stored snapshot is always an estimate, whatever its flag says:
// its ratio is unknown rather than merely inferred.
func (bucket ChannelCostRatioBucket) IsEstimate() bool {
	return bucket.Estimated || bucket.Scaled <= 0
}

// CostQuota applies the bucket's ratio to a summed base quota. Conversion goes
// through common.AggregateQuotaRound so that a total which legitimately
// outgrows the single-request int32 bound saturates at the JavaScript-safe
// boundary instead of wrapping into a credit.
func (bucket ChannelCostRatioBucket) CostQuota(baseQuota int64) int64 {
	return common.AggregateQuotaRound(float64(baseQuota) * bucket.Ratio)
}
