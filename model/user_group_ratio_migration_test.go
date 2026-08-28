package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func useUserGroupRatioMigrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := DB
	previousOptionMap := common.OptionMap
	previousGroups := ratio_setting.GroupRatio2JSONString()
	previousLegacy := ratio_setting.GroupGroupRatio2JSONString()
	previousUsers := ratio_setting.UserGroupRatio2JSONString()
	previousComplete := ratio_setting.IsUserGroupRatioMigrationComplete()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	DB = db
	common.OptionMap = make(map[string]string)
	t.Cleanup(func() {
		DB = previousDB
		common.OptionMap = previousOptionMap
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousGroups))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(previousLegacy))
		require.NoError(t, ratio_setting.UpdateUserGroupRatioByJSONString(previousUsers))
		ratio_setting.SetUserGroupRatioMigrationState(previousComplete, nil)
	})
	return db
}

func TestMigrateLegacyUserGroupRatiosPersistsEquivalentConfigOnce(t *testing.T) {
	db := useUserGroupRatioMigrationDB(t)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"premium":2}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"vip":{"premium":1.5}}`))
	require.NoError(t, ratio_setting.UpdateUserGroupRatioByJSONString(`{}`))
	require.NoError(t, db.Create(&Option{Key: "GroupGroupRatio", Value: `{"vip":{"premium":1.5}}`}).Error)

	require.NoError(t, migrateLegacyUserGroupRatios(true))
	assert.True(t, ratio_setting.IsUserGroupRatioMigrationComplete())
	assert.InDelta(t, 0.75, ratio_setting.GetUserGroupRatio("vip", "premium"), 1e-12)
	assert.Equal(t, "1", requireOptionValue(t, db, "UserGroupRatioMigrationVersion"))
	assert.JSONEq(t, `[]`, common.OptionMap["UserGroupRatioMigrationConflicts"])
	firstConfig := requireOptionValue(t, db, "group_ratio_setting.user_group_ratio")

	require.NoError(t, migrateLegacyUserGroupRatios(true))
	assert.JSONEq(t, firstConfig, requireOptionValue(t, db, "group_ratio_setting.user_group_ratio"))
}

func TestMigrateLegacyUserGroupRatiosBlocksZeroBaseConflict(t *testing.T) {
	db := useUserGroupRatioMigrationDB(t)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"free":0}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"vip":{"free":0.5}}`))
	require.NoError(t, ratio_setting.UpdateUserGroupRatioByJSONString(`{}`))
	require.NoError(t, db.Create(&Option{Key: "GroupGroupRatio", Value: `{"vip":{"free":0.5}}`}).Error)

	err := migrateLegacyUserGroupRatios(true)
	require.Error(t, err)
	assert.False(t, ratio_setting.IsUserGroupRatioMigrationComplete())
	require.Len(t, ratio_setting.GetUserGroupRatioMigrationConflicts(), 1)
	var marker Option
	assert.ErrorIs(t, db.Where("key = ?", "UserGroupRatioMigrationVersion").First(&marker).Error, gorm.ErrRecordNotFound)
}

func TestIncludeChannelRatioCannotActivateBeforeUserRatioMigration(t *testing.T) {
	previousComplete := ratio_setting.IsUserGroupRatioMigrationComplete()
	t.Cleanup(func() {
		ratio_setting.SetUserGroupRatioMigrationState(previousComplete, nil)
	})

	ratio_setting.SetUserGroupRatioMigrationState(false, nil)
	assert.NoError(t, validateOptionValue("group_ratio_setting.include_channel_ratio", `{"premium":false}`))
	assert.Error(t, validateOptionValue("group_ratio_setting.include_channel_ratio", `{"premium":true}`))

	ratio_setting.SetUserGroupRatioMigrationState(true, nil)
	assert.NoError(t, validateOptionValue("group_ratio_setting.include_channel_ratio", `{"premium":true}`))
}
