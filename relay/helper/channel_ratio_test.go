package helper

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleGroupRatioMultipliesGroupUserAndChannelFactors(t *testing.T) {
	previousGroups := ratio_setting.GroupRatio2JSONString()
	previousUsers := ratio_setting.UserGroupRatio2JSONString()
	previousIncludes := ratio_setting.IncludeChannelRatio2JSONString()
	previousComplete := ratio_setting.IsUserGroupRatioMigrationComplete()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousGroups))
		require.NoError(t, ratio_setting.UpdateUserGroupRatioByJSONString(previousUsers))
		require.NoError(t, ratio_setting.UpdateIncludeChannelRatioByJSONString(previousIncludes))
		ratio_setting.SetUserGroupRatioMigrationState(previousComplete, nil)
	})

	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"premium":2}`))
	require.NoError(t, ratio_setting.UpdateUserGroupRatioByJSONString(`{"vip":{"premium":0.8}}`))
	require.NoError(t, ratio_setting.UpdateIncludeChannelRatioByJSONString(`{"premium":true}`))
	ratio_setting.SetUserGroupRatioMigrationState(true, nil)

	ctx, _ := gin.CreateTestContext(nil)
	common.SetContextKey(ctx, constant.ContextKeyChannelCostRatio, 1.5)
	info := &relaycommon.RelayInfo{UserGroup: "vip", UsingGroup: "premium"}

	ratioInfo := HandleGroupRatio(ctx, info)

	assert.Equal(t, 2.0, ratioInfo.BaseGroupRatio)
	assert.Equal(t, 0.8, ratioInfo.UserGroupRatio)
	assert.Equal(t, 1.5, ratioInfo.ChannelRatio)
	assert.True(t, ratioInfo.IncludeChannelRatio)
	assert.InDelta(t, 2.4, ratioInfo.GroupRatio, 1e-12)
}
