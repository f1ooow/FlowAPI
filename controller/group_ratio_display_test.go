package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetUserGroupsReturnsFinalRangeOnlyToAuthenticatedUsers(t *testing.T) {
	previousDB := model.DB
	previousDatabaseType := common.MainDatabaseType()
	previousMemoryCache := common.MemoryCacheEnabled
	previousGroups := ratio_setting.GroupRatio2JSONString()
	previousLegacy := ratio_setting.GroupGroupRatio2JSONString()
	previousUsers := ratio_setting.UserGroupRatio2JSONString()
	previousIncludes := ratio_setting.IncludeChannelRatio2JSONString()
	previousMigrationComplete := ratio_setting.IsUserGroupRatioMigrationComplete()
	previousUsable := setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetMainDatabaseType(previousDatabaseType)
		common.MemoryCacheEnabled = previousMemoryCache
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousGroups))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(previousLegacy))
		require.NoError(t, ratio_setting.UpdateUserGroupRatioByJSONString(previousUsers))
		require.NoError(t, ratio_setting.UpdateIncludeChannelRatioByJSONString(previousIncludes))
		ratio_setting.SetUserGroupRatioMigrationState(previousMigrationComplete, nil)
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(previousUsable))
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Channel{}))
	model.DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.MemoryCacheEnabled = false
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "vip-user", Group: "vip"}).Error)
	low := 0.8
	high := 1.5
	require.NoError(t, db.Create(&[]model.Channel{
		{Name: "low", Status: common.ChannelStatusEnabled, Group: "premium", CostRatio: &low},
		{Name: "high", Status: common.ChannelStatusEnabled, Group: "premium", CostRatio: &high},
	}).Error)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"premium":2}`))
	require.NoError(t, ratio_setting.UpdateUserGroupRatioByJSONString(`{"vip":{"premium":0.8}}`))
	require.NoError(t, ratio_setting.UpdateIncludeChannelRatioByJSONString(`{"premium":true}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"premium":"Premium"}`))
	ratio_setting.SetUserGroupRatioMigrationState(true, nil)

	assertResponse := func(userID int) map[string]interface{} {
		t.Helper()
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/groups", nil)
		if userID > 0 {
			ctx.Set("id", userID)
		}
		GetUserGroups(ctx)
		var body map[string]interface{}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
		data, ok := body["data"].(map[string]interface{})
		require.True(t, ok)
		group, ok := data["premium"].(map[string]interface{})
		require.True(t, ok)
		return group
	}

	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"vip":{"premium":1.5}}`))
	ratio_setting.SetUserGroupRatioMigrationState(false, nil)
	legacy := assertResponse(1)
	assert.Equal(t, "single", legacy["ratio_kind"])
	assert.InDelta(t, 1.5, legacy["ratio_min"], 1e-12)
	assert.InDelta(t, 1.5, legacy["ratio_max"], 1e-12)

	ratio_setting.SetUserGroupRatioMigrationState(true, nil)
	authenticated := assertResponse(1)
	assert.Equal(t, "range", authenticated["ratio_kind"])
	assert.InDelta(t, 1.28, authenticated["ratio_min"], 1e-12)
	assert.InDelta(t, 2.4, authenticated["ratio_max"], 1e-12)
	assert.NotContains(t, authenticated, "ratio")
	assert.InDelta(t, 1.28, getUserVisiblePricingRatio("vip", "premium"), 1e-12,
		"pricing catalog must expose only a composed final value, never the pre-channel base")

	public := assertResponse(0)
	assert.NotContains(t, public, "ratio")
	assert.NotContains(t, public, "ratio_kind")
	assert.NotContains(t, public, "ratio_min")
	assert.NotContains(t, public, "ratio_max")
}
