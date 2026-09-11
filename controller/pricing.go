package controller

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

// filterPricingByUsableGroups keeps only the models the caller can reach and
// narrows every model's EnableGroup to the caller's own usable groups. Without
// the second step the catalog discloses group names that were granted to other
// users through group_ratio_setting.group_special_usable_group, because a model
// enabled in both a shared group and a restricted one would carry the restricted
// name in its response.
//
// The returned items must not share the EnableGroup backing array with the
// process-wide pricing cache, so each kept model gets a freshly built slice.
func filterPricingByUsableGroups(pricing []model.Pricing, usableGroup map[string]string) []model.Pricing {
	if len(pricing) == 0 {
		return pricing
	}
	if len(usableGroup) == 0 {
		return []model.Pricing{}
	}

	filtered := make([]model.Pricing, 0, len(pricing))
	for _, item := range pricing {
		visibleGroups := make([]string, 0, len(item.EnableGroup))
		for _, group := range item.EnableGroup {
			// "all" is a wildcard marker meaning every group, not a grant to a
			// specific group, so it is never a disclosure.
			if group == "all" {
				visibleGroups = append(visibleGroups, group)
				continue
			}
			if _, ok := usableGroup[group]; ok {
				visibleGroups = append(visibleGroups, group)
			}
		}
		if len(visibleGroups) == 0 {
			continue
		}
		item.EnableGroup = visibleGroups
		filtered = append(filtered, item)
	}
	return filtered
}

func GetPricing(c *gin.Context) {
	pricing := model.GetPricing()
	userId, exists := c.Get("id")
	usableGroup := map[string]string{}
	groupRatio := map[string]float64{}
	for groupName := range ratio_setting.GetGroupRatioCopy() {
		groupRatio[groupName] = getUserVisiblePricingRatio("", groupName)
	}
	var group string
	if exists {
		user, err := model.GetUserCache(userId.(int))
		if err == nil {
			group = user.Group
			for g := range groupRatio {
				groupRatio[g] = getUserVisiblePricingRatio(group, g)
			}
		}
	}

	usableGroup = service.GetUserUsableGroups(group)
	pricing = filterPricingByUsableGroups(pricing, usableGroup)
	// check groupRatio contains usableGroup
	for group := range ratio_setting.GetGroupRatioCopy() {
		if _, ok := usableGroup[group]; !ok {
			delete(groupRatio, group)
		}
	}

	c.JSON(200, gin.H{
		"success":            true,
		"data":               pricing,
		"vendors":            model.GetVendors(),
		"group_ratio":        groupRatio,
		"usable_group":       usableGroup,
		"supported_endpoint": model.GetSupportedEndpointMap(),
		"auto_groups":        service.GetUserAutoGroup(group),
		"pricing_version":    "a42d372ccf0b5dd13ecf71203521f9d2",
	})
}

// getUserVisiblePricingRatio returns an already-composed final ratio for the
// pricing catalog. It intentionally never returns a base group ratio: an
// authenticated user may also receive a final range from /user/self/groups,
// so exposing the pre-channel value here would allow the channel cost factor
// to be derived by division.
func getUserVisiblePricingRatio(userGroup, billingGroup string) float64 {
	ratio := service.GetUserGroupRatio(userGroup, billingGroup)
	if !ratio_setting.IsUserGroupRatioMigrationComplete() || !ratio_setting.GetIncludeChannelRatio(billingGroup) {
		return ratio
	}
	channelRange := model.GetChannelCostRatioRange(billingGroup)
	if !channelRange.Available {
		return ratio
	}
	return ratio * channelRange.Min
}

func ResetModelRatio(c *gin.Context) {
	defaultStr := ratio_setting.DefaultModelRatio2JSONString()
	err := model.UpdateOption("ModelRatio", defaultStr)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	err = ratio_setting.UpdateModelRatioByJSONString(defaultStr)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"success": true,
		"message": "重置模型倍率成功",
	})
}
