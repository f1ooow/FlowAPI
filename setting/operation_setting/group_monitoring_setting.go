package operation_setting

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
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
	// VisibleToGroups is the allow list of *user* groups that may see this
	// monitored group on the user-facing page. Both sides are called "group"
	// but mean different things: Group above names the channel pool being
	// measured, while these name the user populations allowed to look at it.
	// Administrators always see every monitored group.
	//
	// Three states, and they must stay distinguishable end to end:
	//   - nil (key absent or null): visible to every logged-in user. The page
	//     shipped before any visibility control existed, so an absent key must
	//     not blank the page for regular users right after an upgrade.
	//   - empty slice: visible to no regular user, administrators only.
	//   - non-empty slice: visible only to users whose group is listed.
	//
	// The tag carries no omitempty on purpose: omitempty would serialise an
	// empty slice away, and the next read would widen "administrators only"
	// back into "everyone".
	VisibleToGroups []string `json:"visible_to_groups"`
	Models          []string `json:"models"`
}

// UnmarshalJSON folds the removed visible_to_users boolean into the allow list
// so a configuration written before the allow list existed keeps its meaning:
// true becomes nil (everyone still sees it) and false becomes an empty list
// (administrators only). The legacy key is read here and nowhere else; the next
// settings save persists visible_to_groups alone.
func (group *GroupMonitoringGroup) UnmarshalJSON(data []byte) error {
	// The alias drops the method set, otherwise decoding recurses into here.
	type plainGroupMonitoringGroup GroupMonitoringGroup
	*group = GroupMonitoringGroup{}
	payload := struct {
		*plainGroupMonitoringGroup
		LegacyVisibleToUsers *bool `json:"visible_to_users"`
	}{plainGroupMonitoringGroup: (*plainGroupMonitoringGroup)(group)}
	if err := common.Unmarshal(data, &payload); err != nil {
		return err
	}
	if payload.LegacyVisibleToUsers != nil && group.VisibleToGroups == nil && !*payload.LegacyVisibleToUsers {
		group.VisibleToGroups = []string{}
	}
	return nil
}

// IsVisibleToUserGroup reports whether a regular user in userGroup may see this
// monitored group.
func (group GroupMonitoringGroup) IsVisibleToUserGroup(userGroup string) bool {
	if group.VisibleToGroups == nil {
		return true
	}
	for _, allowed := range group.VisibleToGroups {
		if allowed == userGroup {
			return true
		}
	}
	return false
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
		copied := GroupMonitoringGroup{
			Group:       group.Group,
			Description: group.Description,
			Models:      append([]string(nil), group.Models...),
		}
		// append onto a nil slice yields nil for an empty source, which would
		// turn "administrators only" into "everyone"; allocate explicitly.
		if group.VisibleToGroups != nil {
			copied.VisibleToGroups = make([]string, len(group.VisibleToGroups))
			copy(copied.VisibleToGroups, group.VisibleToGroups)
		}
		setting.Groups = append(setting.Groups, copied)
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
		if len(group.VisibleToGroups) > MaxGroupMonitoringGroups {
			return fmt.Errorf("group %q supports at most %d user groups in its visibility list", name, MaxGroupMonitoringGroups)
		}
		seenVisible := make(map[string]struct{}, len(group.VisibleToGroups))
		for _, visibleGroup := range group.VisibleToGroups {
			visibleGroup = strings.TrimSpace(visibleGroup)
			if visibleGroup == "" {
				return fmt.Errorf("group %q contains an empty user group in its visibility list", name)
			}
			if len(visibleGroup) > groupMonitoringMaxGroupNameLength {
				return fmt.Errorf("group monitoring group must not exceed %d characters", groupMonitoringMaxGroupNameLength)
			}
			if _, ok := seenVisible[visibleGroup]; ok {
				return fmt.Errorf("user group %q is duplicated in the visibility list of group %q", visibleGroup, name)
			}
			seenVisible[visibleGroup] = struct{}{}
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
