package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

func GetGroups(c *gin.Context) {
	groupNames := make([]string, 0)
	for groupName := range ratio_setting.GetGroupRatioCopy() {
		groupNames = append(groupNames, groupName)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    groupNames,
	})
}

func GetUserGroups(c *gin.Context) {
	usableGroups := make(map[string]map[string]interface{})
	userId := c.GetInt("id")
	userGroup := ""
	authenticated := userId > 0
	if authenticated {
		userGroup, _ = model.GetUserGroup(userId, false)
	}
	userUsableGroups := service.GetUserUsableGroups(userGroup)
	for groupName := range ratio_setting.GetGroupRatioCopy() {
		// UserUsableGroups contains the groups that the user can use
		if desc, ok := userUsableGroups[groupName]; ok {
			groupData := map[string]interface{}{"desc": desc}
			if authenticated {
				groupData["ratio"] = service.GetUserGroupRatio(userGroup, groupName)
				channelRange := model.GetChannelCostRatioRange(groupName)
				if !channelRange.Available {
					delete(groupData, "ratio")
					groupData["ratio_kind"] = "unavailable"
					groupData["available"] = false
				} else {
					baseFinalRatio := service.GetUserGroupRatio(userGroup, groupName)
					minRatio := baseFinalRatio
					maxRatio := baseFinalRatio
					if ratio_setting.IsUserGroupRatioMigrationComplete() && ratio_setting.GetIncludeChannelRatio(groupName) {
						minRatio *= channelRange.Min
						maxRatio *= channelRange.Max
					}
					groupData["ratio_kind"] = "single"
					if minRatio != maxRatio {
						groupData["ratio_kind"] = "range"
					}
					groupData["ratio_min"] = minRatio
					groupData["ratio_max"] = maxRatio
					groupData["available"] = true
					delete(groupData, "ratio")
				}
			}
			usableGroups[groupName] = groupData
		}
	}
	if _, ok := userUsableGroups["auto"]; ok {
		autoGroup := map[string]interface{}{
			"ratio": "自动",
			"desc":  setting.GetUsableGroupDescription("auto"),
		}
		if authenticated {
			delete(autoGroup, "ratio")
			autoGroup["ratio_kind"] = "auto"
			autoGroup["available"] = true
		}
		usableGroups["auto"] = autoGroup
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    usableGroups,
	})
}
