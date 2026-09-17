package common

import (
	"sync"
	"sync/atomic"

	"github.com/QuantumNous/new-api/relaykit/dto"
)

// HedgeAttempt is attempt-owned. Callbacks are installed before dispatch; raw
// usage is written by the scanner and read only after the scanner has joined.
type HedgeAttempt struct {
	ID         string
	Number     int
	Commit     func() bool
	Dispatched func()
	Winner     atomic.Bool
	Detached   atomic.Bool
	// Prefix is updated by the scanner before offering content. It is sampled
	// once by the coordinator when selecting the winner.
	ValidPrefix       atomic.Bool
	PrefixMu          *sync.Mutex // shared with winner selection
	BillableLoser     bool        // set before the coordinator releases accounting
	Finalize          sync.Once
	Finalizing        bool
	Usage             *dto.Usage
	AudioUsage        bool
	Completed         bool
	ReportedUsage     *dto.Usage
	ReportedToolUsage *ResponsesUsageInfo // scanner-owned billing evidence, without generated content
	ExtraContent      []string
	SettlementStatus  string
	MeteringStatus    string
	UsageSource       string
}

func (a *HedgeAttempt) ObservePrefix(valid bool) {
	if a.PrefixMu != nil {
		a.PrefixMu.Lock()
		defer a.PrefixMu.Unlock()
	}
	a.ValidPrefix.Store(valid)
}

func (a *HedgeAttempt) LogInfo() map[string]any {
	role := "loser"
	if a.Winner.Load() {
		role = "winner"
	}
	return map[string]any{"attempt_id": a.ID, "attempt": a.Number, "role": role,
		"usage_source": a.UsageSource, "metering_status": a.MeteringStatus, "settlement_status": a.SettlementStatus}
}
