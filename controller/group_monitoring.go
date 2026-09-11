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
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

const (
	groupMonitoringWindowHours   = 24
	groupMonitoringWindowSeconds = int64(groupMonitoringWindowHours) * 3600

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
	ModelName        string                  `json:"model_name"`
	HasData          bool                    `json:"has_data"`
	State            string                  `json:"state"`
	AvailabilityRate float64                 `json:"availability_rate"`
	AvgLatencyMs     int64                   `json:"avg_latency_ms"`
	AvgTtftMs        *int64                  `json:"avg_ttft_ms"`
	TtftSampleCount  int64                   `json:"ttft_sample_count"`
	RequestCount     int64                   `json:"request_count"`
	Buckets          []groupMonitoringBucket `json:"buckets"`
}

type groupMonitoringGroupSummary struct {
	GroupName   string                        `json:"group_name"`
	Description string                        `json:"description"`
	Models      []groupMonitoringModelSummary `json:"models"`
}

type groupMonitoringSummaryResponse struct {
	Enabled       bool                          `json:"enabled"`
	BucketMinutes int                           `json:"bucket_minutes"`
	WindowHours   int                           `json:"window_hours"`
	Groups        []groupMonitoringGroupSummary `json:"groups"`
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

func buildGroupMonitoringGroups(rows []model.PerfMetricGroupBucket, groups []operation_setting.GroupMonitoringGroup, seriesStart int64, bucketCount int, displaySeconds int64) []groupMonitoringGroupSummary {
	seriesEnd := seriesStart + int64(bucketCount)*displaySeconds
	rolled := make(map[string]map[int64]groupMonitoringCounters, len(rows))
	for _, row := range rows {
		displayTs := row.BucketTs - row.BucketTs%displaySeconds
		if displayTs < seriesStart || displayTs >= seriesEnd {
			continue
		}
		key := row.Group + "\x00" + row.ModelName
		if _, ok := rolled[key]; !ok {
			rolled[key] = make(map[int64]groupMonitoringCounters)
		}
		rolled[key][displayTs] = rolled[key][displayTs].plus(groupMonitoringCounters{
			requestCount:   row.RequestCount,
			successCount:   row.SuccessCount,
			totalLatencyMs: row.TotalLatencyMs,
			ttftSumMs:      row.TtftSumMs,
			ttftCount:      row.TtftCount,
		})
	}

	summaries := make([]groupMonitoringGroupSummary, 0, len(groups))
	for _, group := range groups {
		summary := groupMonitoringGroupSummary{
			GroupName:   group.Group,
			Description: group.Description,
			Models:      make([]groupMonitoringModelSummary, 0, len(group.Models)),
		}
		for _, modelName := range group.Models {
			buckets := rolled[group.Group+"\x00"+modelName]
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

			modelSummary := groupMonitoringModelSummary{
				ModelName:       modelName,
				HasData:         total.requestCount > 0,
				State:           groupMonitoringStateNoData,
				TtftSampleCount: total.ttftCount,
				RequestCount:    total.requestCount,
				Buckets:         series,
			}
			if total.requestCount > 0 {
				modelSummary.AvailabilityRate = groupMonitoringAvailability(total)
				modelSummary.AvgLatencyMs = total.totalLatencyMs / total.requestCount
				modelSummary.State = groupMonitoringHealthState(modelSummary.AvailabilityRate)
			}
			// Non-streaming relays (image generation, embeddings, rerank) never
			// accumulate TTFT, so a zero average would be indistinguishable from
			// a genuine 0 ms. Report null and let the UI fall back to latency.
			if total.ttftCount > 0 {
				avgTtft := total.ttftSumMs / total.ttftCount
				modelSummary.AvgTtftMs = &avgTtft
			}
			summary.Models = append(summary.Models, modelSummary)
		}
		summaries = append(summaries, summary)
	}
	return summaries
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
		Groups:        []groupMonitoringGroupSummary{},
	}
	if !setting.Enabled || len(setting.Groups) == 0 {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": response})
		return
	}

	groupNames, modelNames := groupMonitoringQueryScope(setting.Groups)
	rows, err := model.GetPerfMetricGroupBuckets(groupNames, modelNames, seriesStart, seriesEnd-1)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	response.Groups = buildGroupMonitoringGroups(rows, setting.Groups, seriesStart, bucketCount, displaySeconds)
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
