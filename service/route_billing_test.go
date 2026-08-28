package service

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type routeBillingRecorder struct {
	preConsumed int
	targets     []int
}

func (*routeBillingRecorder) Settle(int) error           { return nil }
func (*routeBillingRecorder) Refund(*gin.Context)        {}
func (*routeBillingRecorder) NeedsRefund() bool          { return false }
func (r *routeBillingRecorder) GetPreConsumedQuota() int { return r.preConsumed }
func (r *routeBillingRecorder) Reserve(target int) error {
	r.targets = append(r.targets, target)
	if target > r.preConsumed {
		r.preConsumed = target
	}
	return nil
}

func TestPrepareBillingForSelectedRouteReservesFinalRouteEstimate(t *testing.T) {
	billing := &routeBillingRecorder{preConsumed: 160}
	info := &relaycommon.RelayInfo{
		Billing: billing,
		PriceData: types.PriceData{
			PreConsumeQuotaBeforeGroup: 100,
			GroupRatioInfo:             types.GroupRatioInfo{GroupRatio: 2.4},
		},
	}

	require.Nil(t, PrepareBillingForSelectedRoute(nil, info))
	assert.Equal(t, []int{240}, billing.targets)
	assert.Equal(t, 240, info.PriceData.QuotaToPreConsume)
	assert.Equal(t, 240, info.FinalPreConsumedQuota)
}
