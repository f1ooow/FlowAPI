package volcvod

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/shopspring/decimal"
	"math"
)

// MinuteQuotaChecked settles authoritative media seconds against the immutable
// minute-price and effective group ratio snapshot. Zero is a real free charge.
func MinuteQuotaChecked(task *model.Task, seconds float64) (int, *common.QuotaClamp, bool) {
	if task == nil || task.PrivateData.BillingContext == nil || seconds <= 0 || seconds > relaycommon.MaxTaskDurationSeconds || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return 0, nil, false
	}
	bc := task.PrivateData.BillingContext
	ratio := bc.GroupRatio
	if bc.EffectiveRatio != nil {
		ratio = *bc.EffectiveRatio
	}
	if math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio < 0 || math.IsNaN(bc.ModelPrice) || math.IsInf(bc.ModelPrice, 0) || bc.ModelPrice < 0 {
		return 0, nil, false
	}
	amount := decimal.NewFromFloat(bc.ModelPrice).Mul(decimal.NewFromFloat(seconds)).Div(decimal.NewFromInt(60)).Mul(decimal.NewFromFloat(ratio)).Mul(decimal.NewFromFloat(common.QuotaPerUnit))
	for name, factor := range bc.OtherRatios {
		if name == "minutes" || name == "seconds" {
			continue
		}
		if math.IsNaN(factor) || math.IsInf(factor, 0) || factor <= 0 {
			return 0, nil, false
		}
		amount = amount.Mul(decimal.NewFromFloat(factor))
	}
	quota, clamp := common.QuotaFromDecimalChecked(amount)
	return quota, clamp, true
}

// Main's polling integration uses the checked interface before PerCallBilling
// so minute prices cannot accidentally freeze the initial reservation.
func (a *TaskAdaptor) AdjustBillingOnCompleteChecked(task *model.Task, result *relaycommon.TaskInfo) (int, *common.QuotaClamp, bool) {
	if result == nil || result.Status != model.TaskStatusSuccess {
		return 0, nil, false
	}
	return MinuteQuotaChecked(task, result.OutputVideoDuration)
}
