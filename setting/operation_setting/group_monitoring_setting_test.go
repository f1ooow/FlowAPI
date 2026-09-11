package operation_setting

import (
	"testing"

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

func TestGetGroupMonitoringSettingResolvesVisibilityDefault(t *testing.T) {
	hidden := false
	previous := groupMonitoringSetting
	groupMonitoringSetting = GroupMonitoringSetting{
		BucketMinutes: 5,
		Groups: []GroupMonitoringGroup{
			// Stored before the flag existed: a missing field means visible, so
			// an upgrade cannot blank the page for non-admin users.
			{Group: "legacy", Models: []string{"gpt-4o-mini"}},
			{Group: "internal", VisibleToUsers: &hidden, Models: []string{"gpt-4o-mini"}},
		},
	}
	t.Cleanup(func() { groupMonitoringSetting = previous })

	setting := GetGroupMonitoringSetting()
	require.Len(t, setting.Groups, 2)
	require.NotNil(t, setting.Groups[0].VisibleToUsers)
	assert.True(t, *setting.Groups[0].VisibleToUsers)
	require.NotNil(t, setting.Groups[1].VisibleToUsers)
	assert.False(t, *setting.Groups[1].VisibleToUsers)

	// The resolved pointer must not alias the stored setting.
	*setting.Groups[1].VisibleToUsers = true
	assert.False(t, groupMonitoringSetting.Groups[1].IsVisibleToUsers())
}
