package hailuo

import (
	"math"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/shopspring/decimal"
)

// AdjustBillingOnCompleteChecked settles H3 from the provider's task.usage
// against the immutable submission snapshot. It only ever refunds: the
// reservation already covers output plus the full reference-video cap, and a
// charge above it could not be collected safely after the fact. Missing,
// out-of-range or untrustworthy usage keeps the reservation.
func (a *TaskAdaptor) AdjustBillingOnCompleteChecked(task *model.Task, result *relaycommon.TaskInfo) (int, *common.QuotaClamp, bool) {
	if task == nil || result == nil || result.Status != model.TaskStatusSuccess {
		return 0, nil, false
	}
	bc := task.PrivateData.BillingContext
	if bc == nil || (bc.BillingUnit != H3BillingUnit && bc.BillingUnit != H3MediaBillingUnit) {
		return 0, nil, false
	}
	var response H3QueryResponse
	if err := common.Unmarshal(task.Data, &response); err != nil {
		return 0, nil, false
	}
	h3Task := response.GetTask()
	if h3Task == nil || h3Task.Usage == nil || h3Task.Usage.OutputSeconds == nil || h3Task.Usage.InputSeconds == nil {
		return 0, nil, false
	}
	output, input := *h3Task.Usage.OutputSeconds, *h3Task.Usage.InputSeconds
	if math.IsNaN(output) || math.IsNaN(input) || output < 4 || output > 15 || input < 0 || input > H3MaxReferenceVideoSeconds {
		return 0, nil, false
	}
	// A request with reference video cannot have zero billed input seconds;
	// such a report is not evidence of a cheaper task.
	if bc.InputVideoDuration > 0 && input == 0 {
		return 0, nil, false
	}
	reserved := bc.OtherRatios["hailuo-h3-duration"]
	if reserved <= 0 || output+input >= reserved {
		return 0, nil, false
	}

	ratio := bc.GroupRatio
	if bc.EffectiveRatio != nil {
		ratio = *bc.EffectiveRatio
	}
	if math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio < 0 || math.IsNaN(bc.ModelPrice) || math.IsInf(bc.ModelPrice, 0) || bc.ModelPrice < 0 {
		return 0, nil, false
	}
	amount := decimal.NewFromFloat(bc.ModelPrice).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).
		Mul(decimal.NewFromFloat(output + input))
	for name, factor := range bc.OtherRatios {
		if name == "hailuo-h3-duration" {
			continue
		}
		if math.IsNaN(factor) || math.IsInf(factor, 0) || factor <= 0 {
			return 0, nil, false
		}
		amount = amount.Mul(decimal.NewFromFloat(factor))
	}
	if bc.BillingUnit == H3MediaBillingUnit {
		if bc.InputImageCount < 0 || bc.InputImageCount > 4 || math.IsNaN(bc.InputImagePrice) || math.IsInf(bc.InputImagePrice, 0) || bc.InputImagePrice < 0 {
			return 0, nil, false
		}
		amount = amount.Add(decimal.NewFromFloat(bc.InputImagePrice).Mul(decimal.NewFromInt(int64(bc.InputImageCount))).Mul(decimal.NewFromFloat(common.QuotaPerUnit)))
	}
	amount = amount.Mul(decimal.NewFromFloat(ratio))
	// Truncate like the reservation so equal seconds give an equal quota.
	quota, clamp := common.QuotaFromDecimalChecked(amount.Round(6).Truncate(0))
	if quota <= 0 || quota >= task.Quota {
		return 0, clamp, false
	}
	return quota, clamp, true
}
