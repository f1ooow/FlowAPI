package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"strings"
)

// FinalizeHedgeAttempt owns counters, settlement and the correlated log together.
// The coordinator calls it after the attempt's transport/scanner has stopped.
func FinalizeHedgeAttempt(c *gin.Context, info *relaycommon.RelayInfo) {
	a := info.Hedge
	a.Finalize.Do(func() {
		a.Finalizing = true
		a.UsageSource = "upstream"
		a.MeteringStatus = "complete"
		a.SettlementStatus = "settled"
		usage := a.Usage
		if !a.Winner.Load() && a.ReportedToolUsage != nil {
			// Replace adapter counts rather than adding the prefix a second time.
			info.ResponsesUsageInfo = a.ReportedToolUsage
		}
		if a.Winner.Load() && a.ReportedUsage == nil {
			a.UsageSource = "estimated"
		}
		if !a.Winner.Load() || usage == nil {
			usage = a.ReportedUsage
			if usage != nil {
				common.SetContextKey(c, constant.ContextKeyLocalCountTokens, false)
			}
		}
		if usage == nil && !a.Winner.Load() {
			a.UsageSource = "unknown"
			a.MeteringStatus = "unmetered"
			a.SettlementStatus = "unmetered"
			usage = &dto.Usage{}
			// No usage cannot justify tool or constant-expression charges.
			info.ResponsesUsageInfo = nil
		} else if !a.Completed || info.StreamStatus == nil || !info.StreamStatus.IsNormalEnd() || info.StreamStatus.HasErrors() {
			a.MeteringStatus = "partial"
		}
		if a.AudioUsage && a.MeteringStatus != "unmetered" && (a.Winner.Load() || a.BillableLoser) {
			PostAudioConsumeQuota(c, info, effectiveBillingUsage(usage), strings.Join(a.ExtraContent, ", "))
		} else {
			PostTextConsumeQuota(c, info, usage, a.ExtraContent)
		}
	})
}
