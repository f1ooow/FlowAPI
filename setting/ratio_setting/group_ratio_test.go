package ratio_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertLegacyUserGroupRatiosPreservesEffectiveRatio(t *testing.T) {
	converted, conflicts := ConvertLegacyUserGroupRatios(
		map[string]map[string]float64{
			"vip": {"premium": 1.5, "missing": 0.8, "free": 0},
		},
		map[string]float64{"premium": 2, "free": 0},
	)

	require.Empty(t, conflicts)
	assert.InDelta(t, 0.75, converted["vip"]["premium"], 1e-12)
	assert.InDelta(t, 0.8, converted["vip"]["missing"], 1e-12)
	assert.InDelta(t, 1, converted["vip"]["free"], 1e-12)
}

func TestConvertLegacyUserGroupRatiosReportsZeroBaseConflict(t *testing.T) {
	converted, conflicts := ConvertLegacyUserGroupRatios(
		map[string]map[string]float64{"vip": {"free": 0.5}},
		map[string]float64{"free": 0},
	)

	assert.Empty(t, converted)
	require.Len(t, conflicts, 1)
	assert.Equal(t, "vip", conflicts[0].UserGroup)
	assert.Equal(t, "free", conflicts[0].BillingGroup)
	assert.Equal(t, 0.5, conflicts[0].LegacyRatio)
}

func TestUserGroupAndChannelRatioSettingsDefaultSafely(t *testing.T) {
	previousUserRatios := UserGroupRatio2JSONString()
	previousIncludes := IncludeChannelRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateUserGroupRatioByJSONString(previousUserRatios))
		require.NoError(t, UpdateIncludeChannelRatioByJSONString(previousIncludes))
	})

	require.NoError(t, UpdateUserGroupRatioByJSONString(`{"vip":{"premium":0.8}}`))
	require.NoError(t, UpdateIncludeChannelRatioByJSONString(`{"premium":true}`))

	assert.Equal(t, 0.8, GetUserGroupRatio("vip", "premium"))
	assert.Equal(t, 1.0, GetUserGroupRatio("default", "premium"))
	assert.True(t, GetIncludeChannelRatio("premium"))
	assert.False(t, GetIncludeChannelRatio("default"))
}

func TestLegacyGroupRatioRejectsNegativeCredits(t *testing.T) {
	previousLegacy := GroupGroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateGroupGroupRatioByJSONString(previousLegacy))
	})

	require.NoError(t, UpdateGroupGroupRatioByJSONString(`{"vip":{"premium":-0.5}}`))
	_, ok := GetGroupGroupRatio("vip", "premium")
	assert.False(t, ok)
}
