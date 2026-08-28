package service

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAttachBillingRatioInfoStoresAuditableComponentsUnderAdminInfo(t *testing.T) {
	relayInfo := &relaycommon.RelayInfo{
		PriceData: types.PriceData{
			GroupRatioInfo: types.GroupRatioInfo{
				GroupRatio:          2.4,
				BaseGroupRatio:      2,
				UserGroupRatio:      0.8,
				ChannelRatio:        1.5,
				IncludeChannelRatio: true,
			},
		},
	}
	other := map[string]interface{}{}

	attachBillingRatioInfo(relayInfo, other)

	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	ratios, ok := adminInfo["billing_ratios"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, 2.0, ratios["base_group_ratio"])
	assert.Equal(t, 0.8, ratios["user_group_ratio"])
	assert.Equal(t, 1.5, ratios["channel_ratio"])
	assert.Equal(t, true, ratios["include_channel_ratio"])
	assert.Equal(t, 2.4, ratios["effective_ratio"])
	assert.NotContains(t, other, "channel_ratio")
}
