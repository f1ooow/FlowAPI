package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
)

func TestTaskQuotaWithRatiosIsExact(t *testing.T) {
	tests := []struct {
		name       string
		price      float64
		groupRatio float64
		ratios     map[string]float64
		want       int
	}{
		{name: "H3 2K one second", price: 0.15, groupRatio: 1, ratios: map[string]float64{"hailuo-h3-duration": 1, "hailuo-h3-resolution": 5.0 / 3.0}, want: 125000},
		{name: "H3 768P one second", price: 0.15, groupRatio: 1, ratios: map[string]float64{"hailuo-h3-duration": 1, "hailuo-h3-resolution": 1}, want: 75000},
		{name: "H3 2K 25 seconds", price: 0.15, groupRatio: 1, ratios: map[string]float64{"hailuo-h3-duration": 25, "hailuo-h3-resolution": 5.0 / 3.0}, want: 25 * 125000},
		{name: "H3 2K with reference reservation and group ratio", price: 0.15, groupRatio: 1.5, ratios: map[string]float64{"hailuo-h3-duration": 20, "hailuo-h3-resolution": 5.0 / 3.0}, want: 3750000},
		{name: "fractional group ratio", price: 0.15, groupRatio: 0.41, ratios: map[string]float64{"hailuo-h3-duration": 4, "hailuo-h3-resolution": 1}, want: 123000},
		{name: "Wan output plus reference seconds", price: 0.6, groupRatio: 1.2, ratios: map[string]float64{"seconds": 7, "resolution": 0.5}, want: 1260000},
		{name: "per-minute 10 seconds", price: 0.6, groupRatio: 1, ratios: map[string]float64{"minutes": 10.0 / 60}, want: 50000},
		{name: "per-minute 10 seconds with group ratio", price: 1, groupRatio: 1.2, ratios: map[string]float64{"minutes": 10.0 / 60}, want: 100000},
		{name: "genuine fraction still truncates", price: 0.000003, groupRatio: 1, ratios: map[string]float64{"seconds": 1}, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			priceData := types.PriceData{
				ModelPrice: tt.price, UsePrice: true,
				PreConsumeQuotaBeforeGroup: tt.price * common.QuotaPerUnit,
				GroupRatioInfo:             types.GroupRatioInfo{GroupRatio: tt.groupRatio},
			}
			for key, ratio := range tt.ratios {
				priceData.AddOtherRatio(key, ratio)
			}
			quota, clamp := taskQuotaWithRatios(priceData)
			assert.Nil(t, clamp)
			assert.Equal(t, tt.want, quota)
		})
	}
}
