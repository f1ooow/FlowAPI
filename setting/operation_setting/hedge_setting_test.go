package operation_setting

import (
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
)

func TestBillHedgeLosersDefaultsAndSnapshot(t *testing.T) {
	original := SnapshotGeneralSetting()
	require.True(t, original.BillHedgeLosers, "missing persisted option enables loser billing")
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(GetGeneralSetting(), map[string]string{"bill_hedge_losers": "true"}))
	})
	require.NoError(t, config.UpdateConfigFromMap(GetGeneralSetting(), map[string]string{"bill_hedge_losers": "false"}))
	snapshot := SnapshotGeneralSetting()
	assert.False(t, snapshot.BillHedgeLosers)
	assert.Equal(t, "false", config.GlobalConfig.ExportAllConfigs()["general_setting.bill_hedge_losers"])
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = config.UpdateConfigFromMap(GetGeneralSetting(), map[string]string{"bill_hedge_losers": "true"})
	}()
	go func() { defer wg.Done(); _ = SnapshotGeneralSetting() }()
	wg.Wait()
	assert.False(t, snapshot.BillHedgeLosers, "request snapshot is immutable")
	assert.True(t, SnapshotGeneralSetting().BillHedgeLosers)
}
