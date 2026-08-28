package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/require"
)

// TestFormatUserLogsStripsQuotaSaturation verifies the admin-only quota
// saturation marker (nested under other.admin_info) is removed for non-admin
// log views, since formatUserLogs strips the whole admin_info object.
func TestFormatUserLogsStripsQuotaSaturation(t *testing.T) {
	other := common.MapToJsonStr(map[string]interface{}{
		"model_price": 0.004,
		"admin_info": map[string]interface{}{
			"quota_saturation": map[string]interface{}{
				"op":      "QuotaFromDecimal",
				"kind":    "overflow",
				"clamped": common.MaxQuota,
			},
		},
	})
	logs := []*Log{{Other: other}}

	formatUserLogs(logs, 0)

	parsed, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	_, hasAdminInfo := parsed["admin_info"]
	require.False(t, hasAdminInfo, "admin_info (and nested quota_saturation) must be stripped for non-admin views")
	// Non-admin billing fields remain visible.
	require.Contains(t, parsed, "model_price")
}

func TestFormatUserLogsStripsHistoricalBillingFactors(t *testing.T) {
	other := common.MapToJsonStr(map[string]interface{}{
		"group_ratio":           2.4,
		"user_group_ratio":      0.8,
		"base_group_ratio":      2.0,
		"channel_ratio":         1.5,
		"include_channel_ratio": true,
		"effective_ratio":       2.4,
		"billing_ratios": map[string]interface{}{
			"channel_ratio": 1.5,
		},
	})
	logs := []*Log{{Other: other}}

	formatUserLogs(logs, 0)

	parsed, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	require.Equal(t, float64(2.4), parsed["group_ratio"])
	for _, field := range []string{
		"user_group_ratio",
		"base_group_ratio",
		"channel_ratio",
		"include_channel_ratio",
		"effective_ratio",
		"billing_ratios",
	} {
		require.NotContains(t, parsed, field, "historical billing factors must not reach user logs")
	}
}
