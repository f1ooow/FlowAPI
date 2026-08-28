package ratio_setting

import (
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/types"
)

var defaultGroupRatio = map[string]float64{
	"default": 1,
	"vip":     1,
	"svip":    1,
}

var groupRatioMap = types.NewRWMap[string, float64]()

var defaultGroupGroupRatio = map[string]map[string]float64{
	"vip": {
		"edit_this": 0.9,
	},
}

var groupGroupRatioMap = types.NewRWMap[string, map[string]float64]()

var userGroupRatioMap = types.NewRWMap[string, map[string]float64]()
var includeChannelRatioMap = types.NewRWMap[string, bool]()

var defaultGroupSpecialUsableGroup = map[string]map[string]string{}

type GroupRatioSetting struct {
	GroupRatio              *types.RWMap[string, float64]            `json:"group_ratio"`
	GroupGroupRatio         *types.RWMap[string, map[string]float64] `json:"group_group_ratio"`
	UserGroupRatio          *types.RWMap[string, map[string]float64] `json:"user_group_ratio"`
	IncludeChannelRatio     *types.RWMap[string, bool]               `json:"include_channel_ratio"`
	GroupSpecialUsableGroup *types.RWMap[string, map[string]string]  `json:"group_special_usable_group"`
}

var groupRatioSetting GroupRatioSetting
var userGroupRatioMigrationComplete atomic.Bool
var migrationConflictMu sync.RWMutex
var migrationConflicts []UserGroupRatioMigrationConflict

type UserGroupRatioMigrationConflict struct {
	UserGroup    string  `json:"user_group"`
	BillingGroup string  `json:"billing_group"`
	BaseRatio    float64 `json:"base_ratio"`
	LegacyRatio  float64 `json:"legacy_ratio"`
}

func init() {
	groupSpecialUsableGroup := types.NewRWMap[string, map[string]string]()
	groupSpecialUsableGroup.AddAll(defaultGroupSpecialUsableGroup)

	groupRatioMap.AddAll(defaultGroupRatio)
	groupGroupRatioMap.AddAll(defaultGroupGroupRatio)
	userGroupRatioMigrationComplete.Store(true)

	groupRatioSetting = GroupRatioSetting{
		GroupSpecialUsableGroup: groupSpecialUsableGroup,
		GroupRatio:              groupRatioMap,
		GroupGroupRatio:         groupGroupRatioMap,
		UserGroupRatio:          userGroupRatioMap,
		IncludeChannelRatio:     includeChannelRatioMap,
	}

	config.GlobalConfig.Register("group_ratio_setting", &groupRatioSetting)
}

func GetGroupRatioSetting() *GroupRatioSetting {
	if groupRatioSetting.GroupSpecialUsableGroup == nil {
		groupRatioSetting.GroupSpecialUsableGroup = types.NewRWMap[string, map[string]string]()
		groupRatioSetting.GroupSpecialUsableGroup.AddAll(defaultGroupSpecialUsableGroup)
	}
	if groupRatioSetting.UserGroupRatio == nil {
		groupRatioSetting.UserGroupRatio = types.NewRWMap[string, map[string]float64]()
		userGroupRatioMap = groupRatioSetting.UserGroupRatio
	}
	if groupRatioSetting.IncludeChannelRatio == nil {
		groupRatioSetting.IncludeChannelRatio = types.NewRWMap[string, bool]()
		includeChannelRatioMap = groupRatioSetting.IncludeChannelRatio
	}
	return &groupRatioSetting
}

func GetGroupRatioCopy() map[string]float64 {
	return groupRatioMap.ReadAll()
}

func ContainsGroupRatio(name string) bool {
	_, ok := groupRatioMap.Get(name)
	return ok
}

func GroupRatio2JSONString() string {
	return groupRatioMap.MarshalJSONString()
}

func UpdateGroupRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonString(groupRatioMap, jsonStr)
}

func GetGroupRatio(name string) float64 {
	ratio, ok := groupRatioMap.Get(name)
	if !ok {
		common.SysLog("group ratio not found: " + name)
		return 1
	}
	return ratio
}

func GetGroupGroupRatio(userGroup, usingGroup string) (float64, bool) {
	gp, ok := groupGroupRatioMap.Get(userGroup)
	if !ok {
		return -1, false
	}
	ratio, ok := gp[usingGroup]
	if !ok || math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio < 0 {
		if ok {
			common.SysLog(fmt.Sprintf("invalid legacy group ratio ignored: %s -> %s", userGroup, usingGroup))
		}
		return -1, false
	}
	return ratio, true
}

func GroupGroupRatio2JSONString() string {
	return groupGroupRatioMap.MarshalJSONString()
}

func UpdateGroupGroupRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonString(groupGroupRatioMap, jsonStr)
}

func GetUserGroupRatio(userGroup, billingGroup string) float64 {
	ratio, ok := LookupUserGroupRatio(userGroup, billingGroup)
	if !ok {
		return 1
	}
	return ratio
}

func LookupUserGroupRatio(userGroup, billingGroup string) (float64, bool) {
	groupRatios, ok := userGroupRatioMap.Get(userGroup)
	if !ok {
		return 1, false
	}
	ratio, ok := groupRatios[billingGroup]
	if !ok || math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio < 0 || ratio > constant.MaxChannelCostRatio {
		return 1, false
	}
	return ratio, true
}

func GetIncludeChannelRatio(group string) bool {
	include, ok := includeChannelRatioMap.Get(group)
	return ok && include
}

func UserGroupRatio2JSONString() string {
	return userGroupRatioMap.MarshalJSONString()
}

func IncludeChannelRatio2JSONString() string {
	return includeChannelRatioMap.MarshalJSONString()
}

func UpdateUserGroupRatioByJSONString(jsonStr string) error {
	if err := CheckUserGroupRatio(jsonStr); err != nil {
		return err
	}
	return types.LoadFromJsonString(userGroupRatioMap, jsonStr)
}

func UpdateIncludeChannelRatioByJSONString(jsonStr string) error {
	var values map[string]bool
	if err := common.Unmarshal([]byte(jsonStr), &values); err != nil {
		return err
	}
	return types.LoadFromJsonString(includeChannelRatioMap, jsonStr)
}

func CheckUserGroupRatio(jsonStr string) error {
	var values map[string]map[string]float64
	if err := common.Unmarshal([]byte(jsonStr), &values); err != nil {
		return err
	}
	for userGroup, ratios := range values {
		for billingGroup, ratio := range ratios {
			if math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio < 0 || ratio > constant.MaxChannelCostRatio {
				return fmt.Errorf("user group ratio %s -> %s must be between 0 and %g", userGroup, billingGroup, constant.MaxChannelCostRatio)
			}
		}
	}
	return nil
}

func ConvertLegacyUserGroupRatios(legacy map[string]map[string]float64, baseRatios map[string]float64) (map[string]map[string]float64, []UserGroupRatioMigrationConflict) {
	converted := make(map[string]map[string]float64, len(legacy))
	conflicts := make([]UserGroupRatioMigrationConflict, 0)
	for userGroup, ratios := range legacy {
		for billingGroup, legacyRatio := range ratios {
			baseRatio, ok := baseRatios[billingGroup]
			if !ok {
				baseRatio = 1
			}
			if baseRatio == 0 && legacyRatio > 0 {
				conflicts = append(conflicts, UserGroupRatioMigrationConflict{
					UserGroup: userGroup, BillingGroup: billingGroup, BaseRatio: baseRatio, LegacyRatio: legacyRatio,
				})
				continue
			}
			newRatio := 1.0
			if baseRatio != 0 {
				newRatio = legacyRatio / baseRatio
			}
			if converted[userGroup] == nil {
				converted[userGroup] = make(map[string]float64)
			}
			converted[userGroup][billingGroup] = newRatio
		}
	}
	return converted, conflicts
}

func SetUserGroupRatioMigrationState(complete bool, conflicts []UserGroupRatioMigrationConflict) {
	userGroupRatioMigrationComplete.Store(complete)
	migrationConflictMu.Lock()
	migrationConflicts = append([]UserGroupRatioMigrationConflict(nil), conflicts...)
	migrationConflictMu.Unlock()
}

func IsUserGroupRatioMigrationComplete() bool {
	return userGroupRatioMigrationComplete.Load()
}

func GetUserGroupRatioMigrationConflicts() []UserGroupRatioMigrationConflict {
	migrationConflictMu.RLock()
	defer migrationConflictMu.RUnlock()
	return append([]UserGroupRatioMigrationConflict(nil), migrationConflicts...)
}

func CheckGroupRatio(jsonStr string) error {
	checkGroupRatio := make(map[string]float64)
	err := common.Unmarshal([]byte(jsonStr), &checkGroupRatio)
	if err != nil {
		return err
	}
	for name, ratio := range checkGroupRatio {
		if ratio < 0 {
			return errors.New("group ratio must be not less than 0: " + name)
		}
	}
	return nil
}
