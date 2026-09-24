package service

import (
	"math"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedreamLayerPriceReservationAndSettlement(t *testing.T) {
	for _, tc := range []struct {
		name           string
		count          int
		actual, refund int
	}{
		{"one image", 1, 60000, 960000},
		{"base plus sixteen layers", 17, 1020000, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			info := &relaycommon.RelayInfo{OriginModelName: dto.Seedream50Pro, StartTime: time.Now(), PriceData: hosttypes.PriceData{UsePrice: true, ModelPrice: .1, GroupRatioInfo: hosttypes.GroupRatioInfo{BaseGroupRatio: 2, UserGroupRatio: .8, ChannelRatio: 1.5, GroupRatio: 2.4, IncludeChannelRatio: true}}}
			info.PriceData.AddOtherRatio("n", dto.MaxSeedreamLayerImages*dto.SeedreamLayerImagePriceFactor)
			usage := &dto.Usage{PromptTokens: 1, TotalTokens: 1}
			reservation := calculateTextQuotaSummary(ctx, info, usage).Quota
			assert.Equal(t, 1020000, reservation)
			// The response adapter replaces n, it never appends a separate layer multiplier.
			info.PriceData.AddOtherRatio("n", float64(tc.count)*dto.SeedreamLayerImagePriceFactor)
			summary := calculateTextQuotaSummary(ctx, info, usage)
			assert.Equal(t, tc.actual, summary.Quota)
			assert.Equal(t, tc.refund, reservation-summary.Quota)
			assert.Nil(t, info.QuotaClamp)
		})
	}
}

func TestSeedreamLayerSettlementSaturationIsAuditable(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{OriginModelName: dto.Seedream50Pro, StartTime: time.Now(), PriceData: hosttypes.PriceData{UsePrice: true, ModelPrice: math.MaxFloat64, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 2.4}}}
	info.PriceData.AddOtherRatio("n", 8.5)
	summary := calculateTextQuotaSummary(ctx, info, &dto.Usage{PromptTokens: 1, TotalTokens: 1})
	assert.Positive(t, summary.Quota)
	require.NotNil(t, info.QuotaClamp)
}

func TestSeedreamLayerExpressionIgnoresModelPriceAndCount(t *testing.T) {
	info := &relaycommon.RelayInfo{OriginModelName: dto.Seedream50Pro, TieredBillingSnapshot: &billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ExprString: `param("layer_decomposition") == true ? 200000 : 100000`, QuotaPerUnit: common.QuotaPerUnit, GroupRatio: 2.4}, BillingRequestInput: &billingexpr.RequestInput{Body: []byte(`{"layer_decomposition":true,"n":1}`)}}
	info.PriceData.UsePrice = true
	info.PriceData.ModelPrice = 99
	info.PriceData.AddOtherRatio("n", 8.5)
	applied, quota, result := TryTieredSettle(info, billingexpr.TokenParams{P: 1})
	require.True(t, applied)
	require.NotNil(t, result)
	assert.Equal(t, 240000, quota, "expression is the complete quantity/pricing source")
}
