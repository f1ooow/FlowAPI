package hailuo

import (
	"math"
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

func TestH3SettlementRefundsDeliveredResolutionDowngrade(t *testing.T) {
	tests := []struct {
		name            string
		resolutionRatio float64
		reservedQuota   int
		data            string
		wantQuota       int
	}{
		{"2K to 1080P", H3HighResolutionRatio, 500000, `{"task":{"resolution":"1080P","usage":{"input_seconds":0,"output_seconds":4}}}`, 466666},
		{"2K to 768P", H3HighResolutionRatio, 500000, `{"task":{"resolution":"768P","usage":{"input_seconds":0,"output_seconds":4}}}`, 300000},
		{"2K to 480P", H3HighResolutionRatio, 500000, `{"task":{"resolution":"480P","usage":{"input_seconds":0,"output_seconds":4}}}`, 200000},
		{"1080P to 768P", 14.0 / 9.0, 466666, `{"task":{"resolution":"768P","usage":{"input_seconds":0,"output_seconds":4}}}`, 300000},
		{"1080P to 480P", 14.0 / 9.0, 466666, `{"task":{"resolution":"480P","usage":{"input_seconds":0,"output_seconds":4}}}`, 200000},
		{"768P to 480P", 1, 300000, `{"task":{"resolution":"480P","usage":{"input_seconds":0,"output_seconds":4}}}`, 200000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := h3SettlementTask(4, tt.resolutionRatio, 0, tt.reservedQuota, tt.data)
			result := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess}
			quota, clamp, handled := (&TaskAdaptor{}).AdjustBillingOnCompleteChecked(task, result)
			require.True(t, handled)
			assert.Nil(t, clamp)
			assert.Equal(t, tt.wantQuota, quota)
			assert.Equal(t, tt.resolutionRatio, task.PrivateData.BillingContext.OtherRatios["hailuo-h3-resolution"], "the submitted pricing snapshot stays immutable")

			task.Quota = quota
			quota, clamp, handled = (&TaskAdaptor{}).AdjustBillingOnCompleteChecked(task, result)
			assert.False(t, handled, "the settled quota cannot be refunded again")
			assert.Nil(t, clamp)
			assert.Zero(t, quota)
		})
	}
}

func TestH3ResolutionSettlementRequiresKnownDowngrade(t *testing.T) {
	tests := []struct {
		name            string
		resolutionRatio float64
		data            string
	}{
		{"missing delivered resolution", H3HighResolutionRatio, `{"task":{"usage":{"input_seconds":0,"output_seconds":4}}}`},
		{"same delivered resolution", H3HighResolutionRatio, `{"task":{"resolution":"2K","usage":{"input_seconds":0,"output_seconds":4}}}`},
		{"same delivered 1080P", 14.0 / 9.0, `{"task":{"resolution":"1080P","usage":{"input_seconds":0,"output_seconds":4}}}`},
		{"higher delivered resolution", 1, `{"task":{"resolution":"1080P","usage":{"input_seconds":0,"output_seconds":4}}}`},
		{"480P delivered as 2K", 2.0 / 3.0, `{"task":{"resolution":"2K","usage":{"input_seconds":0,"output_seconds":4}}}`},
		{"unknown delivered price", H3HighResolutionRatio, `{"task":{"resolution":"360P","usage":{"input_seconds":0,"output_seconds":4}}}`},
		{"already reserved at 768P", 1, `{"task":{"resolution":"768P","usage":{"input_seconds":0,"output_seconds":4}}}`},
		{"already reserved at 480P", 2.0 / 3.0, `{"task":{"resolution":"480P","usage":{"input_seconds":0,"output_seconds":4}}}`},
		{"whitespace delivered resolution", H3HighResolutionRatio, `{"task":{"resolution":" ","usage":{"input_seconds":0,"output_seconds":4}}}`},
		{"unknown frozen resolution ratio", 1.25, `{"task":{"resolution":"768P","usage":{"input_seconds":0,"output_seconds":4}}}`},
		{"downgrade without usage", H3HighResolutionRatio, `{"task":{"resolution":"768P"}}`},
		{"downgrade with excess seconds", H3HighResolutionRatio, `{"task":{"resolution":"768P","usage":{"input_seconds":0,"output_seconds":5}}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := h3SettlementTask(4, tt.resolutionRatio, 0, 500000, tt.data)
			quota, clamp, handled := (&TaskAdaptor{}).AdjustBillingOnCompleteChecked(task, &relaycommon.TaskInfo{Status: model.TaskStatusSuccess})
			assert.False(t, handled)
			assert.Nil(t, clamp)
			assert.Zero(t, quota)
		})
	}
}

func TestH3ResolutionSettlementNeverIncreasesQuota(t *testing.T) {
	task := h3SettlementTask(4, 14.0/9.0, 0, 250000, `{"task":{"resolution":"768P","usage":{"input_seconds":0,"output_seconds":4}}}`)
	quota, clamp, handled := (&TaskAdaptor{}).AdjustBillingOnCompleteChecked(task, &relaycommon.TaskInfo{Status: model.TaskStatusSuccess})
	assert.False(t, handled, "a lower delivered resolution cannot increase an already reduced quota")
	assert.Nil(t, clamp)
	assert.Zero(t, quota)
}

func TestH3SettlementRejectsInvalidFrozenRatios(t *testing.T) {
	tests := []struct {
		name   string
		key    string
		factor float64
	}{
		{"zero reserved duration", "hailuo-h3-duration", 0},
		{"nonfinite reserved duration", "hailuo-h3-duration", math.Inf(1)},
		{"NaN reserved duration", "hailuo-h3-duration", math.NaN()},
		{"negative resolution ratio", "hailuo-h3-resolution", -1},
		{"nonfinite resolution ratio", "hailuo-h3-resolution", math.Inf(1)},
		{"NaN resolution ratio", "hailuo-h3-resolution", math.NaN()},
		{"invalid additional ratio", "frozen-channel-discount", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := h3SettlementTask(20, H3HighResolutionRatio, 15, 2500000, `{"task":{"resolution":"768P","usage":{"input_seconds":3,"output_seconds":5}}}`)
			task.PrivateData.BillingContext.OtherRatios[tt.key] = tt.factor
			quota, clamp, handled := (&TaskAdaptor{}).AdjustBillingOnCompleteChecked(task, &relaycommon.TaskInfo{Status: model.TaskStatusSuccess})
			assert.False(t, handled)
			assert.Nil(t, clamp)
			assert.Zero(t, quota)
		})
	}
}

func TestH3ResolutionAndDurationRefundKeepsFrozenImageCharge(t *testing.T) {
	tests := []struct {
		name            string
		resolutionRatio float64
		reservedQuota   int
		data            string
		wantQuota       int
	}{
		// (8 seconds × delivered price + 4 images × .0625) × 1.2 × 500000.
		{"2K to 1080P", H3HighResolutionRatio, 2400000, `{"task":{"resolution":"1080P","usage":{"input_seconds":3,"output_seconds":5,"input_image_count":9}}}`, 990000},
		{"2K to 768P", H3HighResolutionRatio, 2400000, `{"task":{"resolution":"768P","usage":{"input_seconds":3,"output_seconds":5,"input_image_count":9}}}`, 690000},
		{"1080P to 768P", 14.0 / 9.0, 2250000, `{"task":{"resolution":"768P","usage":{"input_seconds":3,"output_seconds":5,"input_image_count":9}}}`, 690000},
		{"768P to 480P", 1, 1500000, `{"task":{"resolution":"480P","usage":{"input_seconds":3,"output_seconds":5,"input_image_count":9}}}`, 510000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := h3SettlementTask(20, tt.resolutionRatio, 15, tt.reservedQuota, tt.data)
			ratio := 1.2
			bc := task.PrivateData.BillingContext
			bc.BillingUnit = H3MediaBillingUnit
			bc.ModelPrice = 0.1125
			bc.EffectiveRatio = &ratio
			bc.InputImageCount = 4
			bc.InputImagePrice = 0.0625
			quota, clamp, handled := (&TaskAdaptor{}).AdjustBillingOnCompleteChecked(task, &relaycommon.TaskInfo{Status: model.TaskStatusSuccess})
			require.True(t, handled)
			assert.Nil(t, clamp)
			assert.Equal(t, tt.wantQuota, quota)
			assert.Equal(t, 20.0, bc.OtherRatios["hailuo-h3-duration"])
			assert.Equal(t, tt.resolutionRatio, bc.OtherRatios["hailuo-h3-resolution"])
			assert.Equal(t, 4, bc.InputImageCount)
		})
	}
}

func TestH3ResolutionSettlementKeepsFrozenAdditionalRatios(t *testing.T) {
	task := h3SettlementTask(20, H3HighResolutionRatio, 15, 3525000, `{"task":{"resolution":" 768p ","usage":{"input_seconds":3,"output_seconds":5,"input_image_count":9}}}`)
	ratio := 1.2
	bc := task.PrivateData.BillingContext
	bc.BillingUnit = H3MediaBillingUnit
	bc.ModelPrice = 0.1125
	bc.GroupRatio = 9
	bc.EffectiveRatio = &ratio
	bc.InputImageCount = 4
	bc.InputImagePrice = 0.0625
	bc.OtherRatios["frozen-channel-discount"] = 1.5

	quota, clamp, handled := (&TaskAdaptor{}).AdjustBillingOnCompleteChecked(task, &relaycommon.TaskInfo{Status: model.TaskStatusSuccess})
	require.True(t, handled)
	assert.Nil(t, clamp)
	// (8 seconds × .1125 × 1.5 + 4 images × .0625) × 1.2 × 500000.
	assert.Equal(t, 960000, quota)
	assert.Equal(t, map[string]float64{
		"hailuo-h3-duration":      20,
		"hailuo-h3-resolution":    H3HighResolutionRatio,
		"frozen-channel-discount": 1.5,
	}, bc.OtherRatios, "settlement never changes the submission snapshot")
}

func TestH3DurationSettlementDoesNotGuessDeliveredResolutionPrice(t *testing.T) {
	tests := []struct {
		name            string
		resolutionRatio float64
		data            string
		wantQuota       int
	}{
		{"missing resolution", H3HighResolutionRatio, `{"task":{"usage":{"input_seconds":3,"output_seconds":5}}}`, 1000000},
		{"unknown resolution", H3HighResolutionRatio, `{"task":{"resolution":"4K","usage":{"input_seconds":3,"output_seconds":5}}}`, 1000000},
		{"higher delivered resolution", 1, `{"task":{"resolution":"2K","usage":{"input_seconds":3,"output_seconds":5}}}`, 600000},
		{"unknown frozen resolution ratio", 1.25, `{"task":{"resolution":"768P","usage":{"input_seconds":3,"output_seconds":5}}}`, 750000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := h3SettlementTask(20, tt.resolutionRatio, 15, 2500000, tt.data)
			quota, clamp, handled := (&TaskAdaptor{}).AdjustBillingOnCompleteChecked(task, &relaycommon.TaskInfo{Status: model.TaskStatusSuccess})
			require.True(t, handled)
			assert.Nil(t, clamp)
			assert.Equal(t, tt.wantQuota, quota)
			assert.Equal(t, tt.resolutionRatio, task.PrivateData.BillingContext.OtherRatios["hailuo-h3-resolution"])
		})
	}
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
