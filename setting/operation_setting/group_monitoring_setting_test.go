package operation_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateGroupMonitoringSetting(t *testing.T) {
	valid := GroupMonitoringSetting{
		Enabled:       true,
		BucketMinutes: 15,
		Groups: []GroupMonitoringGroup{
			{Group: "default", Description: "shared pool", Models: []string{"gpt-4o-mini", "claude-3-haiku"}},
		},
	}

	require.NoError(t, ValidateGroupMonitoringSetting(valid))

	tests := []struct {
		name    string
		mutate  func(*GroupMonitoringSetting)
		message string
	}{
		{
			name:    "rejects unsupported bucket width",
			mutate:  func(setting *GroupMonitoringSetting) { setting.BucketMinutes = 10 },
			message: "bucket must be one of",
		},
		{
			name:    "rejects zero bucket width",
			mutate:  func(setting *GroupMonitoringSetting) { setting.BucketMinutes = 0 },
			message: "bucket must be one of",
		},
		{
			name: "rejects blank group name",
			mutate: func(setting *GroupMonitoringSetting) {
				setting.Groups[0].Group = ""
			},
			message: "requires a group name",
		},
		{
			name: "rejects duplicate groups",
			mutate: func(setting *GroupMonitoringSetting) {
				setting.Groups = append(setting.Groups, GroupMonitoringGroup{Group: "default", Models: []string{"gpt-4o-mini"}})
			},
			message: "duplicated",
		},
		{
			name: "rejects group without models",
			mutate: func(setting *GroupMonitoringSetting) {
				setting.Groups[0].Models = nil
			},
			message: "at least one monitored model",
		},
		{
			name: "rejects duplicate models inside a group",
			mutate: func(setting *GroupMonitoringSetting) {
				setting.Groups[0].Models = []string{"gpt-4o-mini", "gpt-4o-mini"}
			},
			message: "duplicated in group",
		},
		{
			name: "rejects empty model name",
			mutate: func(setting *GroupMonitoringSetting) {
				setting.Groups[0].Models = []string{"gpt-4o-mini", " "}
			},
			message: "empty model name",
		},
		{
			name: "rejects oversized group names",
			mutate: func(setting *GroupMonitoringSetting) {
				setting.Groups[0].Group = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789abc"
			},
			message: "64 characters",
		},
		{
			name: "rejects an empty user group in the visibility list",
			mutate: func(setting *GroupMonitoringSetting) {
				setting.Groups[0].VisibleToGroups = []string{"vip", " "}
			},
			message: "empty user group",
		},
		{
			name: "rejects a duplicated user group in the visibility list",
			mutate: func(setting *GroupMonitoringSetting) {
				setting.Groups[0].VisibleToGroups = []string{"vip", "vip"}
			},
			message: "duplicated in the visibility list",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setting := valid
			setting.Groups = append([]GroupMonitoringGroup(nil), valid.Groups...)
			test.mutate(&setting)

			err := ValidateGroupMonitoringSetting(setting)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.message)
		})
	}
}

func TestGetGroupMonitoringSettingIsAnIsolatedCopy(t *testing.T) {
	previous := groupMonitoringSetting
	groupMonitoringSetting = GroupMonitoringSetting{
		BucketMinutes: 0,
		Groups: []GroupMonitoringGroup{
			{Group: "default", Models: []string{"gpt-4o-mini"}},
		},
	}
	t.Cleanup(func() { groupMonitoringSetting = previous })

	setting := GetGroupMonitoringSetting()
	assert.Equal(t, DefaultGroupMonitoringBucketMinutes, setting.BucketMinutes)
	require.Len(t, setting.Groups, 1)

	setting.Groups[0].Models[0] = "mutated"
	assert.Equal(t, "gpt-4o-mini", groupMonitoringSetting.Groups[0].Models[0])
	assert.Equal(t, 0, groupMonitoringSetting.BucketMinutes)
}

func TestGroupMonitoringGroupUnmarshalJSONVisibility(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    []string
	}{
		{
			// Written before any visibility control existed.
			name:    "absent keys stay visible to everyone",
			payload: `{"group":"default","models":["gpt-4o-mini"]}`,
			want:    nil,
		},
		{
			name:    "explicit null stays visible to everyone",
			payload: `{"group":"default","visible_to_groups":null}`,
			want:    nil,
		},
		{
			name:    "empty list means administrators only",
			payload: `{"group":"default","visible_to_groups":[]}`,
			want:    []string{},
		},
		{
			name:    "allow list is kept as written",
			payload: `{"group":"default","visible_to_groups":["vip","internal"]}`,
			want:    []string{"vip", "internal"},
		},
		{
			name:    "legacy true migrates to visible to everyone",
			payload: `{"group":"default","visible_to_users":true}`,
			want:    nil,
		},
		{
			name:    "legacy false migrates to administrators only",
			payload: `{"group":"default","visible_to_users":false}`,
			want:    []string{},
		},
		{
			name:    "an allow list wins over a stale legacy flag",
			payload: `{"group":"default","visible_to_users":false,"visible_to_groups":["vip"]}`,
			want:    []string{"vip"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var group GroupMonitoringGroup
			require.NoError(t, common.Unmarshal([]byte(test.payload), &group))
			assert.Equal(t, "default", group.Group)
			if test.want == nil {
				assert.Nil(t, group.VisibleToGroups)
			} else {
				require.NotNil(t, group.VisibleToGroups, "an empty allow list must stay distinct from an absent one")
				assert.Equal(t, test.want, group.VisibleToGroups)
			}
		})
	}
}

func TestGroupMonitoringGroupVisibilitySurvivesRoundTrip(t *testing.T) {
	groups := []GroupMonitoringGroup{
		{Group: "everyone", Models: []string{"gpt-4o-mini"}},
		{Group: "admin-only", VisibleToGroups: []string{}, Models: []string{"gpt-4o-mini"}},
		{Group: "vip-only", VisibleToGroups: []string{"vip"}, Models: []string{"gpt-4o-mini"}},
	}

	encoded, err := common.Marshal(groups)
	require.NoError(t, err)
	// omitempty on the allow list would drop this and widen the group back to
	// "visible to everyone" on the next read.
	assert.Contains(t, string(encoded), `"visible_to_groups":[]`)
	assert.NotContains(t, string(encoded), "visible_to_users")

	// Decoding reuses the existing backing array, so a stale allow list must not
	// bleed into a group that no longer has one.
	decoded := []GroupMonitoringGroup{{Group: "stale", VisibleToGroups: []string{"leftover"}}}
	require.NoError(t, common.Unmarshal(encoded, &decoded))
	require.Len(t, decoded, 3)
	assert.Nil(t, decoded[0].VisibleToGroups)
	require.NotNil(t, decoded[1].VisibleToGroups)
	assert.Empty(t, decoded[1].VisibleToGroups)
	assert.Equal(t, []string{"vip"}, decoded[2].VisibleToGroups)
}

func TestIsVisibleToUserGroup(t *testing.T) {
	assert.True(t, GroupMonitoringGroup{}.IsVisibleToUserGroup("default"), "an unconfigured allow list is visible to everyone")
	assert.False(t, GroupMonitoringGroup{VisibleToGroups: []string{}}.IsVisibleToUserGroup("default"))
	allowed := GroupMonitoringGroup{VisibleToGroups: []string{"vip", "internal"}}
	assert.True(t, allowed.IsVisibleToUserGroup("internal"))
	assert.False(t, allowed.IsVisibleToUserGroup("default"))
	assert.False(t, allowed.IsVisibleToUserGroup(""))
}

func TestGetGroupMonitoringSettingKeepsVisibilityStates(t *testing.T) {
	previous := groupMonitoringSetting
	groupMonitoringSetting = GroupMonitoringSetting{
		BucketMinutes: 5,
		Groups: []GroupMonitoringGroup{
			{Group: "everyone", Models: []string{"gpt-4o-mini"}},
			{Group: "admin-only", VisibleToGroups: []string{}, Models: []string{"gpt-4o-mini"}},
			{Group: "vip-only", VisibleToGroups: []string{"vip"}, Models: []string{"gpt-4o-mini"}},
		},
	}
	t.Cleanup(func() { groupMonitoringSetting = previous })

	setting := GetGroupMonitoringSetting()
	require.Len(t, setting.Groups, 3)
	assert.Nil(t, setting.Groups[0].VisibleToGroups)
	require.NotNil(t, setting.Groups[1].VisibleToGroups, "copying must not collapse an empty allow list into a nil one")
	assert.Empty(t, setting.Groups[1].VisibleToGroups)
	assert.Equal(t, []string{"vip"}, setting.Groups[2].VisibleToGroups)

	setting.Groups[2].VisibleToGroups[0] = "mutated"
	assert.Equal(t, []string{"vip"}, groupMonitoringSetting.Groups[2].VisibleToGroups)
}
