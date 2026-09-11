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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pricingResponse struct {
	Success     bool               `json:"success"`
	Data        []model.Pricing    `json:"data"`
	UsableGroup map[string]string  `json:"usable_group"`
	GroupRatio  map[string]float64 `json:"group_ratio"`
}

// TestGetPricingNarrowsEnableGroupsToCallerUsableGroups locks the disclosure
// boundary of /api/pricing: a model that is enabled in both a shared group and
// a group only granted through group_special_usable_group must not leak the
// restricted group name to users who cannot use it.
func TestGetPricingNarrowsEnableGroupsToCallerUsableGroups(t *testing.T) {
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	originalGroupRatio := ratio_setting.GroupRatio2JSONString()
	originalSpecialGroups := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.ReadAll()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatio))
		specialGroups := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup
		specialGroups.Clear()
		specialGroups.AddAll(originalSpecialGroups)
		model.InvalidatePricingCache()
	})

	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":2,"zz-secret":3}`))
	specialGroups := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup
	specialGroups.Clear()
	specialGroups.Set("vip", map[string]string{"zz-secret": "内部分组"})

	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.Channel{
		Id:     1,
		Name:   "pricing-channel",
		Type:   1,
		Key:    "sk-pricing",
		Status: common.ChannelStatusEnabled,
	}).Error)
	require.NoError(t, db.Create(&[]model.User{
		{Id: 2001, Username: "pricing-default-user", Password: "password", AffCode: "aff-2001", Group: "default", Role: common.RoleCommonUser, Status: common.UserStatusEnabled},
		{Id: 2002, Username: "pricing-vip-user", Password: "password", AffCode: "aff-2002", Group: "vip", Role: common.RoleCommonUser, Status: common.UserStatusEnabled},
		{Id: 2003, Username: "pricing-admin-user", Password: "password", AffCode: "aff-2003", Group: "default", Role: common.RoleAdminUser, Status: common.UserStatusEnabled},
	}).Error)
	require.NoError(t, db.Create(&[]model.Ability{
		{Group: "default", Model: "zz-pricing-shared-model", ChannelId: 1, Enabled: true},
		{Group: "zz-secret", Model: "zz-pricing-shared-model", ChannelId: 1, Enabled: true},
		{Group: "zz-secret", Model: "zz-pricing-secret-model", ChannelId: 1, Enabled: true},
		{Group: "all", Model: "zz-pricing-wildcard-model", ChannelId: 1, Enabled: true},
	}).Error)
	model.InvalidatePricingCache()

	requestPricing := func(userID int) pricingResponse {
		t.Helper()
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/pricing", nil)
		if userID > 0 {
			ctx.Set("id", userID)
		}
		GetPricing(ctx)

		require.Equal(t, http.StatusOK, recorder.Code)
		var payload pricingResponse
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
		require.True(t, payload.Success)
		require.NotNil(t, payload.Data)
		for _, item := range payload.Data {
			assert.NotEmpty(t, item.EnableGroup,
				"a model kept in the response must always carry at least one visible group")
		}
		return payload
	}

	byName := func(payload pricingResponse) map[string]model.Pricing {
		byModel := make(map[string]model.Pricing, len(payload.Data))
		for _, item := range payload.Data {
			byModel[item.ModelName] = item
		}
		return byModel
	}

	// A default-group user must never see the group name that only vip users
	// receive via group_special_usable_group.
	defaultPayload := requestPricing(2001)
	defaultModels := byName(defaultPayload)
	require.Contains(t, defaultModels, "zz-pricing-shared-model")
	assert.Equal(t, []string{"default"}, defaultModels["zz-pricing-shared-model"].EnableGroup)
	assert.NotContains(t, defaultModels, "zz-pricing-secret-model")
	assert.Equal(t, []string{"all"}, defaultModels["zz-pricing-wildcard-model"].EnableGroup,
		"the wildcard marker is not a per-group grant and stays visible")
	assert.NotContains(t, defaultPayload.UsableGroup, "zz-secret")
	assert.NotContains(t, defaultPayload.GroupRatio, "zz-secret")

	// /api/pricing has no role branch: it scopes every field to the caller's own
	// group, so an admin sees exactly what a default-group user sees.
	adminModels := byName(requestPricing(2003))
	assert.Equal(t, []string{"default"}, adminModels["zz-pricing-shared-model"].EnableGroup)
	assert.NotContains(t, adminModels, "zz-pricing-secret-model")

	// Anonymous callers (TryUserAuth) fall back to the global usable groups,
	// which never include special grants.
	anonymousPayload := requestPricing(0)
	anonymousModels := byName(anonymousPayload)
	assert.Equal(t, []string{"default"}, anonymousModels["zz-pricing-shared-model"].EnableGroup)
	assert.NotContains(t, anonymousModels, "zz-pricing-secret-model")
	assert.Equal(t, map[string]string{"default": "默认分组"}, anonymousPayload.UsableGroup)

	// A user who really holds the special grant still sees both groups.
	vipModels := byName(requestPricing(2002))
	assert.ElementsMatch(t, []string{"default", "zz-secret"}, vipModels["zz-pricing-shared-model"].EnableGroup)
	require.Contains(t, vipModels, "zz-pricing-secret-model")
	assert.Equal(t, []string{"zz-secret"}, vipModels["zz-pricing-secret-model"].EnableGroup)

	// Per-request narrowing must not write back into the shared pricing cache.
	for _, item := range model.GetPricing() {
		if item.ModelName == "zz-pricing-shared-model" {
			assert.ElementsMatch(t, []string{"default", "zz-secret"}, item.EnableGroup,
				"the pricing cache must keep the full group list for other callers")
		}
	}
}
