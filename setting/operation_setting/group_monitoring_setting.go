package operation_setting

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

const (
	DefaultGroupMonitoringBucketMinutes = 5
	MaxGroupMonitoringGroups            = 50
	MaxGroupMonitoringModelsPerGroup    = 50
	MaxGroupMonitoringDescriptionLength = 500
	groupMonitoringMaxGroupNameLength   = 64
	groupMonitoringMaxModelNameLength   = 255
)

// GroupMonitoringBucketMinuteOptions are the display bucket widths the timeline
// offers. They are all multiples of the finest perf_metrics storage bucket, so
// a display bucket is always an exact roll-up of whole storage buckets.
var GroupMonitoringBucketMinuteOptions = []int{5, 15, 30, 60}

type GroupMonitoringGroup struct {
	Group       string `json:"group"`
	Description string `json:"description"`
	// VisibleToUsers gates the group on the user-facing monitoring page.
	// Administrators always see every configured group.
	//
	// A nil pointer means "visible". The monitoring page shipped before this
	// flag existed and showed every configured group to every logged-in user,
	// so decoding a missing field into the Go zero value would blank the page
	// for non-admins right after an upgrade. The pointer keeps an absent field
	// apart from an explicit false, which does hide the group.
	VisibleToUsers *bool    `json:"visible_to_users"`
	Models         []string `json:"models"`
}

// IsVisibleToUsers reports whether non-admin users may see this group.
func (group GroupMonitoringGroup) IsVisibleToUsers() bool {
	return group.VisibleToUsers == nil || *group.VisibleToUsers
}

type GroupMonitoringSetting struct {
	Enabled       bool                   `json:"enabled"`
	BucketMinutes int                    `json:"bucket_minutes"`
	Groups        []GroupMonitoringGroup `json:"groups"`
}

var groupMonitoringSetting = GroupMonitoringSetting{
	BucketMinutes: DefaultGroupMonitoringBucketMinutes,
	Groups:        []GroupMonitoringGroup{},
}

func init() {
	config.GlobalConfig.Register("group_monitoring_setting", &groupMonitoringSetting)
}

func GetGroupMonitoringSetting() GroupMonitoringSetting {
	setting := groupMonitoringSetting
	if !IsGroupMonitoringBucketMinutesSupported(setting.BucketMinutes) {
		setting.BucketMinutes = DefaultGroupMonitoringBucketMinutes
	}
	setting.Groups = make([]GroupMonitoringGroup, 0, len(groupMonitoringSetting.Groups))
	for _, group := range groupMonitoringSetting.Groups {
		// Resolve the visibility default here so every consumer, including the
		// settings UI, sees a concrete value instead of re-deriving it.
		visibleToUsers := group.IsVisibleToUsers()
		setting.Groups = append(setting.Groups, GroupMonitoringGroup{
			Group:          group.Group,
			Description:    group.Description,
			VisibleToUsers: &visibleToUsers,
			Models:         append([]string(nil), group.Models...),
		})
	}
	return setting
}

func IsGroupMonitoringBucketMinutesSupported(minutes int) bool {
	for _, option := range GroupMonitoringBucketMinuteOptions {
		if option == minutes {
			return true
		}
	}
	return false
}

func ValidateGroupMonitoringSetting(setting GroupMonitoringSetting) error {
	if !IsGroupMonitoringBucketMinutesSupported(setting.BucketMinutes) {
		return fmt.Errorf("group monitoring bucket must be one of %v minutes", GroupMonitoringBucketMinuteOptions)
	}
	if len(setting.Groups) > MaxGroupMonitoringGroups {
		return fmt.Errorf("group monitoring supports at most %d groups", MaxGroupMonitoringGroups)
	}
	seenGroups := make(map[string]struct{}, len(setting.Groups))
	for _, group := range setting.Groups {
		name := strings.TrimSpace(group.Group)
		if name == "" {
			return fmt.Errorf("each monitored group requires a group name")
		}
		if len(name) > groupMonitoringMaxGroupNameLength {
			return fmt.Errorf("group monitoring group must not exceed %d characters", groupMonitoringMaxGroupNameLength)
		}
		if _, ok := seenGroups[name]; ok {
			return fmt.Errorf("group monitoring group %q is duplicated", name)
		}
		seenGroups[name] = struct{}{}
		if len(group.Description) > MaxGroupMonitoringDescriptionLength {
			return fmt.Errorf("group monitoring description must not exceed %d characters", MaxGroupMonitoringDescriptionLength)
		}
		if len(group.Models) == 0 {
			return fmt.Errorf("group %q requires at least one monitored model", name)
		}
		if len(group.Models) > MaxGroupMonitoringModelsPerGroup {
			return fmt.Errorf("group %q supports at most %d monitored models", name, MaxGroupMonitoringModelsPerGroup)
		}
		seenModels := make(map[string]struct{}, len(group.Models))
		for _, modelName := range group.Models {
			modelName = strings.TrimSpace(modelName)
			if modelName == "" {
				return fmt.Errorf("group %q contains an empty model name", name)
			}
			if len(modelName) > groupMonitoringMaxModelNameLength {
				return fmt.Errorf("group monitoring model must not exceed %d characters", groupMonitoringMaxModelNameLength)
			}
			if _, ok := seenModels[modelName]; ok {
				return fmt.Errorf("model %q is duplicated in group %q", modelName, name)
			}
			seenModels[modelName] = struct{}{}
		}
	}
	return nil
}
