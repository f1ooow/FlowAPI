package controller

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

const (
	groupMonitoringWindowHours   = 24
	groupMonitoringWindowSeconds = int64(groupMonitoringWindowHours) * 3600

	// Latency deliberately uses a much shorter window than availability.
	// Availability is a stability metric: a 24h uptime figure is what tells a
	// user whether a pool is dependable. Latency is a "how does it feel right
	// now" metric, and a 24h mean buries a slowdown that started an hour ago
	// under a day of healthy samples. Do not "tidy this up" by unifying them.
	groupMonitoringLatencyWindowSeconds = int64(3600)

	// A display bucket built from one or two requests carries no signal: with
	// two requests a single failure already reads as 50% and paints the whole
	// slot red. Below this many requests the slot is rendered as no-data.
	groupMonitoringMinBucketRequests = 3

	groupMonitoringStateHealthy  = "healthy"
	groupMonitoringStateDegraded = "degraded"
	groupMonitoringStateDown     = "down"
	groupMonitoringStateNoData   = "no-data"

	groupMonitoringHealthyRate  = 99.0
	groupMonitoringDegradedRate = 80.0
)

type groupMonitoringBucket struct {
	Ts           int64  `json:"ts"`
	RequestCount int64  `json:"request_count"`
	SuccessCount int64  `json:"success_count"`
	State        string `json:"state"`
}

type groupMonitoringModelSummary struct {
	ModelName string `json:"model_name"`
	// Icon and vendor come from the pricing catalog, the same source the model
	// square renders, so a model looks identical in both places.
	Icon       string `json:"icon,omitempty"`
	VendorName string `json:"vendor_name,omitempty"`
	VendorIcon string `json:"vendor_icon,omitempty"`
	HasData    bool   `json:"has_data"`
	State      string `json:"state"`
	// AvailabilityRate and RequestCount cover the full 24h window; the two
	// latency averages cover the last hour only.
	AvailabilityRate float64 `json:"availability_rate"`
	// AvgLatencyMs is null when the model served no request in the latency
	// window, even if it served plenty earlier in the day. Falling back to the
	// 24h mean would leave the user unable to tell which window they read.
	AvgLatencyMs *int64 `json:"avg_latency_ms"`
	// AvgTtftMs is null when the latency window holds no streamed sample.
	AvgTtftMs       *int64 `json:"avg_ttft_ms"`
	TtftSampleCount int64  `json:"ttft_sample_count"`
	// AvgTtftMs24h and TtftSampleCount24h cover the full 24h availability
	// window. The card falls back to the 24h mean when the last hour happens to
	// hold no streamed sample, so that a low-traffic streaming model keeps
	// showing a first-token figure instead of switching to total latency. Only
	// a model with no streamed sample in the whole day (image generation,
	// embeddings, rerank) reaches the total-latency fallback, and the sample
	// count is what lets the client tell that case apart from "quiet hour".
	AvgTtftMs24h       *int64                  `json:"avg_ttft_ms_24h"`
	TtftSampleCount24h int64                   `json:"ttft_sample_count_24h"`
	RequestCount       int64                   `json:"request_count"`
	Buckets            []groupMonitoringBucket `json:"buckets"`
}

type groupMonitoringGroupSummary struct {
	GroupName   string `json:"group_name"`
	Description string `json:"description"`
	// VisibleToGroups mirrors the configured user-group allow list: null means
	// every logged-in user, an empty array means administrators only. A regular
	// viewer only ever receives groups they are allowed to see; an admin
	// receives all of them and uses this to spot the restricted ones.
	VisibleToGroups []string                      `json:"visible_to_groups"`
	Models          []groupMonitoringModelSummary `json:"models"`
}

// groupMonitoringThresholds mirrors the constants this file applies to a
// display bucket. The timeline collapses adjacent buckets to fit the card
// width, and a collapsed bar must be coloured from the summed counters rather
// than from an average of the child rates, so the client needs the very same
// thresholds. Shipping them keeps one source of truth instead of a second copy
// hard-coded in the frontend.
type groupMonitoringThresholds struct {
	HealthyRate       float64 `json:"healthy_rate"`
	DegradedRate      float64 `json:"degraded_rate"`
	MinBucketRequests int64   `json:"min_bucket_requests"`
}

type groupMonitoringSummaryResponse struct {
	Enabled       bool                          `json:"enabled"`
	BucketMinutes int                           `json:"bucket_minutes"`
	WindowHours   int                           `json:"window_hours"`
	Thresholds    groupMonitoringThresholds     `json:"thresholds"`
	Groups        []groupMonitoringGroupSummary `json:"groups"`
}

// groupMonitoringModelMeta is the presentation metadata of a monitored model.
type groupMonitoringModelMeta struct {
	Icon       string
	VendorName string
	VendorIcon string
}

type groupMonitoringCounters struct {
	requestCount   int64
	successCount   int64
	totalLatencyMs int64
	ttftSumMs      int64
	ttftCount      int64
}

func (counters groupMonitoringCounters) plus(other groupMonitoringCounters) groupMonitoringCounters {
	counters.requestCount += other.requestCount
	counters.successCount += other.successCount
	counters.totalLatencyMs += other.totalLatencyMs
	counters.ttftSumMs += other.ttftSumMs
	counters.ttftCount += other.ttftCount
	return counters
}

func canonicalGroupMonitoringModel(configured string, available []string) (string, bool) {
	for _, candidate := range available {
		if candidate == configured {
			return candidate, true
		}
	}
	matched := ""
	for _, candidate := range available {
		if !strings.EqualFold(candidate, configured) {
			continue
		}
		if matched != "" {
			return "", false
		}
		matched = candidate
	}
	return matched, matched != ""
}

// groupMonitoringDisplaySeconds resolves the configured timeline width against
// the physical perf_metrics bucket width. A display bucket narrower than the
// storage bucket cannot be produced, so when an administrator widens the global
// bucket the timeline degrades to the finest width storage can actually serve.
func groupMonitoringDisplaySeconds(bucketMinutes int) int64 {
	displaySeconds := int64(bucketMinutes) * 60
	storageSeconds := perf_metrics_setting.GetBucketSeconds()
	if displaySeconds < storageSeconds {
		return storageSeconds
	}
	return displaySeconds
}

// groupMonitoringSeriesEnd returns the exclusive upper bound of the rendered
// timeline. A closed bucket is not readable until every node has flushed it,
// and the flush ticker has no phase relationship with the bucket boundary, so a
// bucket that closed a moment ago can still be sitting in some node's memory for
// up to one full flush interval. Rendering it shows only the slice of traffic
// that happened to reach the nodes that already flushed, which turns a couple of
// failures into a phantom outage.
//
// The clock is therefore rewound by one flush interval *before* aligning to the
// display grid, so the bound never depends on how display and flush widths
// compare.
func groupMonitoringSeriesEnd(now int64, displaySeconds int64) int64 {
	flushSeconds := int64(perf_metrics_setting.GetFlushIntervalMinutes()) * 60
	settled := now - flushSeconds
	return settled - settled%displaySeconds
}

func groupMonitoringHealthState(availabilityRate float64) string {
	switch {
	case availabilityRate >= groupMonitoringHealthyRate:
		return groupMonitoringStateHealthy
	case availabilityRate >= groupMonitoringDegradedRate:
		return groupMonitoringStateDegraded
	default:
		return groupMonitoringStateDown
	}
}

// groupMonitoringAvailability sums the counters first and divides once. Never
// average per-bucket rates: a bucket with 1 request would then weigh as much as
// a bucket with 10 000.
func groupMonitoringAvailability(counters groupMonitoringCounters) float64 {
	if counters.requestCount <= 0 {
		return 0
	}
	rate := float64(counters.successCount) / float64(counters.requestCount) * 100
	return math.Round(rate*100) / 100
}

func groupMonitoringQueryScope(groups []operation_setting.GroupMonitoringGroup) ([]string, []string) {
	groupNames := make([]string, 0, len(groups))
	modelSet := make(map[string]struct{})
	for _, group := range groups {
		groupNames = append(groupNames, group.Group)
		for _, modelName := range group.Models {
			modelSet[modelName] = struct{}{}
		}
	}
	modelNames := make([]string, 0, len(modelSet))
	for modelName := range modelSet {
		modelNames = append(modelNames, modelName)
	}
	sort.Strings(modelNames)
	return groupNames, modelNames
}

// groupMonitoringModelMetaMap resolves icons and vendors from the pricing
// cache the model square already renders from, so the monitoring page does not
// grow a second model-to-vendor mapping that can drift.
func groupMonitoringModelMetaMap(groups []operation_setting.GroupMonitoringGroup) map[string]groupMonitoringModelMeta {
	wanted := make(map[string]struct{})
	for _, group := range groups {
		for _, modelName := range group.Models {
			wanted[modelName] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return nil
	}
	vendors := make(map[int]model.PricingVendor)
	for _, vendor := range model.GetVendors() {
		vendors[vendor.ID] = vendor
	}
	metas := make(map[string]groupMonitoringModelMeta, len(wanted))
	for _, pricing := range model.GetPricing() {
		if _, ok := wanted[pricing.ModelName]; !ok {
			continue
		}
		meta := groupMonitoringModelMeta{Icon: pricing.Icon}
		if vendor, ok := vendors[pricing.VendorID]; ok {
			meta.VendorName = vendor.Name
			meta.VendorIcon = vendor.Icon
		}
		metas[pricing.ModelName] = meta
	}
	return metas
}

func buildGroupMonitoringGroups(rows []model.PerfMetricGroupBucket, groups []operation_setting.GroupMonitoringGroup, metas map[string]groupMonitoringModelMeta, seriesStart int64, bucketCount int, displaySeconds int64, latencyStart int64) []groupMonitoringGroupSummary {
	seriesEnd := seriesStart + int64(bucketCount)*displaySeconds
	rolled := make(map[string]map[int64]groupMonitoringCounters, len(rows))
	// Latency is summed from the storage buckets rather than from the display
	// series, so the one hour window stays exact even when the timeline renders
	// hour-wide bars.
	recent := make(map[string]groupMonitoringCounters, len(rows))
	for _, row := range rows {
		displayTs := row.BucketTs - row.BucketTs%displaySeconds
		if displayTs < seriesStart || displayTs >= seriesEnd {
			continue
		}
		key := row.Group + "\x00" + row.ModelName
		counters := groupMonitoringCounters{
			requestCount:   row.RequestCount,
			successCount:   row.SuccessCount,
			totalLatencyMs: row.TotalLatencyMs,
			ttftSumMs:      row.TtftSumMs,
			ttftCount:      row.TtftCount,
		}
		if _, ok := rolled[key]; !ok {
			rolled[key] = make(map[int64]groupMonitoringCounters)
		}
		rolled[key][displayTs] = rolled[key][displayTs].plus(counters)
		if row.BucketTs >= latencyStart && row.BucketTs < seriesEnd {
			recent[key] = recent[key].plus(counters)
		}
	}

	summaries := make([]groupMonitoringGroupSummary, 0, len(groups))
	for _, group := range groups {
		summary := groupMonitoringGroupSummary{
			GroupName:       group.Group,
			Description:     group.Description,
			VisibleToGroups: group.VisibleToGroups,
			Models:          make([]groupMonitoringModelSummary, 0, len(group.Models)),
		}
		for _, modelName := range group.Models {
			key := group.Group + "\x00" + modelName
			buckets := rolled[key]
			total := groupMonitoringCounters{}
			series := make([]groupMonitoringBucket, bucketCount)
			for index := range series {
				ts := seriesStart + int64(index)*displaySeconds
				value := buckets[ts]
				total = total.plus(value)
				state := groupMonitoringStateNoData
				if value.requestCount >= groupMonitoringMinBucketRequests {
					state = groupMonitoringHealthState(groupMonitoringAvailability(value))
				}
				series[index] = groupMonitoringBucket{
					Ts:           ts,
					RequestCount: value.requestCount,
					SuccessCount: value.successCount,
					State:        state,
				}
			}

			meta := metas[modelName]
			recentCounters := recent[key]
			modelSummary := groupMonitoringModelSummary{
				ModelName:          modelName,
				Icon:               meta.Icon,
				VendorName:         meta.VendorName,
				VendorIcon:         meta.VendorIcon,
				HasData:            total.requestCount > 0,
				State:              groupMonitoringStateNoData,
				TtftSampleCount:    recentCounters.ttftCount,
				TtftSampleCount24h: total.ttftCount,
				RequestCount:       total.requestCount,
				Buckets:            series,
			}
			// The badge state follows the 24h availability and must stay
			// independent of the shorter latency window.
			if total.requestCount > 0 {
				modelSummary.AvailabilityRate = groupMonitoringAvailability(total)
				modelSummary.State = groupMonitoringHealthState(modelSummary.AvailabilityRate)
			}
			if recentCounters.requestCount > 0 {
				avgLatency := recentCounters.totalLatencyMs / recentCounters.requestCount
				modelSummary.AvgLatencyMs = &avgLatency
			}
			// Non-streaming relays (image generation, embeddings, rerank) never
			// accumulate TTFT, so a zero average would be indistinguishable from
			// a genuine 0 ms. Report null and let the UI fall back to latency.
			if recentCounters.ttftCount > 0 {
				avgTtft := recentCounters.ttftSumMs / recentCounters.ttftCount
				modelSummary.AvgTtftMs = &avgTtft
			}
			// Same rule for the 24h fallback: a streaming model that happened to
			// be idle for an hour still gets a first-token figure, and a model
			// with no streamed sample all day gets none.
			if total.ttftCount > 0 {
				avgTtft24h := total.ttftSumMs / total.ttftCount
				modelSummary.AvgTtftMs24h = &avgTtft24h
			}
			summary.Models = append(summary.Models, modelSummary)
		}
		summaries = append(summaries, summary)
	}
	return summaries
}

// groupMonitoringVisibleGroups drops the groups a regular user must not see.
// Administrators are exempt from the allow list; everyone else is matched by
// the group their account belongs to. The route stays behind UserAuth, the
// split is by role inside the handler, so no anonymous entry point exists.
func groupMonitoringVisibleGroups(groups []operation_setting.GroupMonitoringGroup, isAdmin bool, userGroup string) []operation_setting.GroupMonitoringGroup {
	if isAdmin {
		return groups
	}
	visible := make([]operation_setting.GroupMonitoringGroup, 0, len(groups))
	for _, group := range groups {
		if group.IsVisibleToUserGroup(userGroup) {
			visible = append(visible, group)
		}
	}
	return visible
}

// groupMonitoringLatencyStart returns the inclusive lower bound of the latency
// window, aligned to whole storage buckets. The bucket count is derived from
// the configured storage width instead of being hard-coded, so an administrator
// widening the global bucket to an hour still gets exactly one hour of latency.
func groupMonitoringLatencyStart(seriesEnd int64) int64 {
	storageSeconds := perf_metrics_setting.GetBucketSeconds()
	bucketCount := groupMonitoringLatencyWindowSeconds / storageSeconds
	if bucketCount < 1 {
		bucketCount = 1
	}
	return seriesEnd - bucketCount*storageSeconds
}

func GetGroupMonitoringSummary(c *gin.Context) {
	setting := operation_setting.GetGroupMonitoringSetting()
	displaySeconds := groupMonitoringDisplaySeconds(setting.BucketMinutes)
	bucketCount := int(groupMonitoringWindowSeconds / displaySeconds)
	seriesEnd := groupMonitoringSeriesEnd(time.Now().Unix(), displaySeconds)
	seriesStart := seriesEnd - int64(bucketCount)*displaySeconds

	response := groupMonitoringSummaryResponse{
		Enabled:       setting.Enabled,
		BucketMinutes: int(displaySeconds / 60),
		WindowHours:   groupMonitoringWindowHours,
		Thresholds: groupMonitoringThresholds{
			HealthyRate:       groupMonitoringHealthyRate,
			DegradedRate:      groupMonitoringDegradedRate,
			MinBucketRequests: groupMonitoringMinBucketRequests,
		},
		Groups: []groupMonitoringGroupSummary{},
	}
	isAdmin := c.GetInt("role") >= common.RoleAdminUser
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	groups := groupMonitoringVisibleGroups(setting.Groups, isAdmin, userGroup)
	if !setting.Enabled || len(groups) == 0 {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": response})
		return
	}

	groupNames, modelNames := groupMonitoringQueryScope(groups)
	rows, err := model.GetPerfMetricGroupBuckets(groupNames, modelNames, seriesStart, seriesEnd-1)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	response.Groups = buildGroupMonitoringGroups(rows, groups, groupMonitoringModelMetaMap(groups), seriesStart, bucketCount, displaySeconds, groupMonitoringLatencyStart(seriesEnd))
	if !isAdmin {
		// The allow list names other user groups, which a regular viewer has no
		// business learning about. They already passed the filter above, so the
		// restriction badge is only meaningful to an administrator anyway.
		for index := range response.Groups {
			response.Groups[index].VisibleToGroups = nil
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": response})
}

func GetGroupMonitoringAdmin(c *gin.Context) {
	setting := operation_setting.GetGroupMonitoringSetting()
	groups := make([]string, 0)
	for group := range ratio_setting.GetGroupRatioCopy() {
		groups = append(groups, group)
	}
	sort.Strings(groups)
	availableModels := make(map[string][]string, len(groups))
	for _, group := range groups {
		models := model.GetGroupEnabledModels(group)
		sort.Strings(models)
		availableModels[group] = models
	}
	for index := range setting.Groups {
		available := availableModels[setting.Groups[index].Group]
		for modelIndex, modelName := range setting.Groups[index].Models {
			if canonical, ok := canonicalGroupMonitoringModel(modelName, available); ok {
				setting.Groups[index].Models[modelIndex] = canonical
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"setting":                   setting,
		"available_groups":          groups,
		"available_models_by_group": availableModels,
		// The timeline cannot render a bucket finer than the global
		// perf_metrics bucket, so the settings UI needs the physical width to
		// disable the options it cannot honour.
		"storage_bucket_minutes": perf_metrics_setting.GetBucketSeconds() / 60,
	}})
}

func UpdateGroupMonitoringAdmin(c *gin.Context) {
	var setting operation_setting.GroupMonitoringSetting
	if err := c.ShouldBindJSON(&setting); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	for index := range setting.Groups {
		setting.Groups[index].Group = strings.TrimSpace(setting.Groups[index].Group)
		setting.Groups[index].Description = strings.TrimSpace(setting.Groups[index].Description)
		// Trim in place: allocating a new slice here would turn the "no regular
		// user" empty list into a nil one, which means the opposite.
		for visibleIndex := range setting.Groups[index].VisibleToGroups {
			setting.Groups[index].VisibleToGroups[visibleIndex] = strings.TrimSpace(setting.Groups[index].VisibleToGroups[visibleIndex])
		}
		for modelIndex := range setting.Groups[index].Models {
			setting.Groups[index].Models[modelIndex] = strings.TrimSpace(setting.Groups[index].Models[modelIndex])
		}
	}
	if err := operation_setting.ValidateGroupMonitoringSetting(setting); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	knownGroups := ratio_setting.GetGroupRatioCopy()
	for index, group := range setting.Groups {
		if _, ok := knownGroups[group.Group]; !ok {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": fmt.Sprintf("unknown group %q", group.Group)})
			return
		}
		// The allow list is checked against the same group universe as the
		// monitored group itself, so a typo cannot silently hide a group from
		// everyone. Rejecting matches how an unknown monitored group is handled.
		for _, visibleGroup := range group.VisibleToGroups {
			if _, ok := knownGroups[visibleGroup]; !ok {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": fmt.Sprintf("unknown user group %q in the visibility list of group %q", visibleGroup, group.Group)})
				return
			}
		}
		available := model.GetGroupEnabledModels(group.Group)
		for modelIndex, modelName := range group.Models {
			canonical, ok := canonicalGroupMonitoringModel(modelName, available)
			if !ok {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": fmt.Sprintf("model %q is not available in group %q", modelName, group.Group)})
				return
			}
			setting.Groups[index].Models[modelIndex] = canonical
		}
	}
	groupsJSON, err := common.Marshal(setting.Groups)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	err = model.UpdateOptionsBulk(map[string]string{
		"group_monitoring_setting.enabled":        strconv.FormatBool(setting.Enabled),
		"group_monitoring_setting.bucket_minutes": strconv.Itoa(setting.BucketMinutes),
		"group_monitoring_setting.groups":         string(groupsJSON),
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
