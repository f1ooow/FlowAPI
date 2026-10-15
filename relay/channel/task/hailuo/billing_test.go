package hailuo

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func h3SettlementTask(reservedSeconds float64, resolutionRatio float64, inputReserved float64, quota int, data string) *model.Task {
	ratio := 1.0
	return &model.Task{
		Quota: quota,
		Data:  []byte(data),
		PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
			BillingUnit: H3BillingUnit, ModelPrice: 0.15, GroupRatio: ratio, EffectiveRatio: &ratio,
			InputVideoDuration: inputReserved, PerCallBilling: true,
			OtherRatios: map[string]float64{"hailuo-h3-duration": reservedSeconds, "hailuo-h3-resolution": resolutionRatio},
		}},
	}
}

func TestH3SettlementFromTaskUsage(t *testing.T) {
	success := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess}
	tests := []struct {
		name        string
		task        *model.Task
		result      *relaycommon.TaskInfo
		wantQuota   int
		wantHandled bool
	}{
		{
			name:      "2K reference video refunds unused reservation",
			task:      h3SettlementTask(20, H3HighResolutionRatio, 15, 2500000, `{"task":{"status":"succeeded","usage":{"input_seconds":6,"output_seconds":5,"total_seconds":11}}}`),
			result:    success,
			wantQuota: 11 * 125000, wantHandled: true,
		},
		{
			name:      "768P reference video refunds unused reservation",
			task:      h3SettlementTask(19, 1, 15, 19*75000, `{"task":{"status":"succeeded","usage":{"input_seconds":3,"output_seconds":4}}}`),
			result:    success,
			wantQuota: 7 * 75000, wantHandled: true,
		},
		{
			name:   "zero input seconds with reference video keeps reservation",
			task:   h3SettlementTask(20, H3HighResolutionRatio, 15, 2500000, `{"task":{"status":"succeeded","usage":{"input_seconds":0,"output_seconds":5}}}`),
			result: success,
		},
		{
			name:   "no reference video and matching usage keeps reservation",
			task:   h3SettlementTask(5, 1, 0, 375000, `{"task":{"status":"succeeded","usage":{"input_seconds":0,"output_seconds":5}}}`),
			result: success,
		},
		{
			name:   "usage above reservation never charges more",
			task:   h3SettlementTask(5, 1, 0, 375000, `{"task":{"status":"succeeded","usage":{"input_seconds":0,"output_seconds":15}}}`),
			result: success,
		},
		{
			name:   "missing usage keeps reservation",
			task:   h3SettlementTask(20, 1, 15, 1500000, `{"task":{"status":"succeeded"}}`),
			result: success,
		},
		{
			name:   "missing input seconds keeps reservation",
			task:   h3SettlementTask(20, 1, 15, 1500000, `{"task":{"status":"succeeded","usage":{"output_seconds":5}}}`),
			result: success,
		},
		{
			name:   "output out of range keeps reservation",
			task:   h3SettlementTask(20, 1, 15, 1500000, `{"task":{"status":"succeeded","usage":{"input_seconds":5,"output_seconds":3}}}`),
			result: success,
		},
		{
			name:   "input out of range keeps reservation",
			task:   h3SettlementTask(20, 1, 15, 1500000, `{"task":{"status":"succeeded","usage":{"input_seconds":16,"output_seconds":4}}}`),
			result: success,
		},
		{
			name:   "failed task is not settled",
			task:   h3SettlementTask(20, 1, 15, 1500000, `{"task":{"status":"failed","usage":{"input_seconds":5,"output_seconds":5}}}`),
			result: &relaycommon.TaskInfo{Status: model.TaskStatusFailure},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quota, clamp, handled := (&TaskAdaptor{}).AdjustBillingOnCompleteChecked(tt.task, tt.result)
			assert.Nil(t, clamp)
			assert.Equal(t, tt.wantHandled, handled)
			assert.Equal(t, tt.wantQuota, quota)
		})
	}
}

func TestH3LegacySnapshotIsNotAdjusted(t *testing.T) {
	task := h3SettlementTask(20, 1.25, 15, 1500000, `{"task":{"status":"succeeded","usage":{"input_seconds":5,"output_seconds":5}}}`)
	task.PrivateData.BillingContext.BillingUnit = ""
	quota, clamp, handled := (&TaskAdaptor{}).AdjustBillingOnCompleteChecked(task, &relaycommon.TaskInfo{Status: model.TaskStatusSuccess})
	require.False(t, handled)
	assert.Nil(t, clamp)
	assert.Zero(t, quota)
}

func TestH3SettlementTruncatesLikeReservation(t *testing.T) {
	// 0.15 × 0.41 × 7s × 500000 is exactly 215250; float products land one below.
	ratio := 0.41
	task := h3SettlementTask(19, 1, 15, 19*30750, `{"task":{"status":"succeeded","usage":{"input_seconds":3,"output_seconds":4}}}`)
	task.PrivateData.BillingContext.EffectiveRatio = &ratio
	quota, _, handled := (&TaskAdaptor{}).AdjustBillingOnCompleteChecked(task, &relaycommon.TaskInfo{Status: model.TaskStatusSuccess})
	require.True(t, handled)
	assert.Equal(t, 215250, quota)
}

func TestH3MediaSettlementKeepsImageChargeAndFrozenPrices(t *testing.T) {
	ratio := 1.2
	task := h3SettlementTask(20, H3HighResolutionRatio, 15, 2400000, `{"task":{"status":"succeeded","usage":{"input_seconds":3,"output_seconds":5,"input_image_count":9}}}`)
	bc := task.PrivateData.BillingContext
	bc.BillingUnit = H3MediaBillingUnit
	bc.ModelPrice = 0.1125
	bc.EffectiveRatio = &ratio
	bc.InputImageCount = 4
	bc.InputImagePrice = 0.0625
	quota, clamp, handled := (&TaskAdaptor{}).AdjustBillingOnCompleteChecked(task, &relaycommon.TaskInfo{Status: model.TaskStatusSuccess})
	require.True(t, handled)
	assert.Nil(t, clamp)
	// (8 seconds × .1875 + 4 images × .0625) × 1.2 × 500000.
	assert.Equal(t, 1050000, quota)
	bc.BillingUnit = H3BillingUnit
	quota, _, handled = (&TaskAdaptor{}).AdjustBillingOnCompleteChecked(task, &relaycommon.TaskInfo{Status: model.TaskStatusSuccess})
	require.True(t, handled)
	assert.Equal(t, 900000, quota, "older snapshots never acquire the new image charge")
}
