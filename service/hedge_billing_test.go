package service

import (
	"errors"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
	"time"
)

type hedgeBillingRecorder struct {
	charges []int
	err     error
}

func (b *hedgeBillingRecorder) Settle(q int) error     { b.charges = append(b.charges, q); return b.err }
func (*hedgeBillingRecorder) Refund(*gin.Context)      {}
func (*hedgeBillingRecorder) NeedsRefund() bool        { return false }
func (*hedgeBillingRecorder) GetPreConsumedQuota() int { return 100 }
func (*hedgeBillingRecorder) Reserve(int) error        { return nil }

func TestHedgeUnmeteredFinalizationDoesNotChargeEstimatesOrConstantExpression(t *testing.T) {
	oldLogs := common.LogConsumeEnabled
	common.LogConsumeEnabled = false
	t.Cleanup(func() { common.LogConsumeEnabled = oldLogs })
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	billing := &hedgeBillingRecorder{}
	info := &relaycommon.RelayInfo{Billing: billing, StartTime: time.Now(), ChannelMeta: &relaycommon.ChannelMeta{}, Hedge: &relaycommon.HedgeAttempt{ID: "attempt-a", Number: 1}, TieredBillingSnapshot: &billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ExprString: "1000000", GroupRatio: 1}}
	info.SetEstimatePromptTokens(1000)
	PostTextConsumeQuota(ctx, info, &dto.Usage{PromptTokens: 1000, CompletionTokens: 50}, nil)
	assert.Empty(t, billing.charges, "adapter settlement is deferred to attempt finalization")
	FinalizeHedgeAttempt(ctx, info)
	FinalizeHedgeAttempt(ctx, info)
	assert.Equal(t, []int{0}, billing.charges)
	assert.Equal(t, "unmetered", info.Hedge.MeteringStatus)
	assert.Equal(t, "unknown", info.Hedge.UsageSource)
}

func TestHedgeSettlementFailureRemainsObservableAndIsNotRetried(t *testing.T) {
	oldLogs := common.LogConsumeEnabled
	common.LogConsumeEnabled = false
	t.Cleanup(func() { common.LogConsumeEnabled = oldLogs })
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	billing := &hedgeBillingRecorder{err: errors.New("uncertain wallet write")}
	info := &relaycommon.RelayInfo{Billing: billing, StartTime: time.Now(), ChannelMeta: &relaycommon.ChannelMeta{}, Hedge: &relaycommon.HedgeAttempt{}}
	FinalizeHedgeAttempt(ctx, info)
	FinalizeHedgeAttempt(ctx, info)
	require.Len(t, billing.charges, 1)
	assert.Equal(t, "failed", info.Hedge.LogInfo()["settlement_status"])
}
