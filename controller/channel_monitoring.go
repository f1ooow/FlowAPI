package controller

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
	"github.com/gin-gonic/gin"
)

const (
	channelMonitoringStateHealthy  = "healthy"
	channelMonitoringStateDegraded = "degraded"
	channelMonitoringStateDown     = "down"
	channelMonitoringStateNoData   = "no-data"

	// Deliberately the same thresholds the group monitoring page uses, so an
	// operator reading both pages compares like with like. They are declared
	// separately because the two pages measure different things (user-visible
	// model availability vs. per-channel attempt availability) and may diverge.
	channelMonitoringHealthyRate  = 99.0
	channelMonitoringDegradedRate = 80.0

	channelMonitoringDefaultRange = "24h"
)

// channelMonitoringWindow pairs the wall-clock span of a range with the display
// bucket width used to draw it. A 7d timeline at the 5 minute storage width
// would be 2016 slots per channel, so wider ranges roll up into wider slots and
// every range stays under ~90 slots.
type channelMonitoringWindow struct {
	windowSeconds int64
	stepSeconds   int64
}

var channelMonitoringWindows = map[string]channelMonitoringWindow{
	"15m": {windowSeconds: 15 * 60, stepSeconds: 5 * 60},
	"1h":  {windowSeconds: 60 * 60, stepSeconds: 5 * 60},
	"6h":  {windowSeconds: 6 * 3600, stepSeconds: 15 * 60},
	"24h": {windowSeconds: 24 * 3600, stepSeconds: 30 * 60},
	"7d":  {windowSeconds: 7 * 24 * 3600, stepSeconds: 2 * 3600},
}

// channelMonitoringRangeKeys is the validation whitelist and the order the UI
// should offer the ranges in.
var channelMonitoringRangeKeys = []string{"15m", "1h", "6h", "24h", "7d"}

type channelMonitoringBucket struct {
	Ts           int64  `json:"ts"`
	AttemptCount int64  `json:"attempt_count"`
	SuccessCount int64  `json:"success_count"`
	State        string `json:"state"`
}

type channelMonitoringChannelSummary struct {
	ChannelId        int                       `json:"channel_id"`
	ChannelName      string                    `json:"channel_name"`
	HasData          bool                      `json:"has_data"`
	State            string                    `json:"state"`
	AvailabilityRate float64                   `json:"availability_rate"`
	ErrorRate        float64                   `json:"error_rate"`
	AvgLatencyMs     int64                     `json:"avg_latency_ms"`
	AttemptCount     int64                     `json:"attempt_count"`
	SuccessCount     int64                     `json:"success_count"`
	Buckets          []channelMonitoringBucket `json:"buckets"`
}

type channelMonitoringOverall struct {
	HasData          bool    `json:"has_data"`
	AvailabilityRate float64 `json:"availability_rate"`
	ErrorRate        float64 `json:"error_rate"`
	AvgLatencyMs     int64   `json:"avg_latency_ms"`
	AttemptCount     int64   `json:"attempt_count"`
}

type channelMonitoringSummaryResponse struct {
	Range         string                            `json:"range"`
	Ranges        []string                          `json:"ranges"`
	StepMinutes   int                               `json:"step_minutes"`
	WindowSeconds int64                             `json:"window_seconds"`
	Overall       channelMonitoringOverall          `json:"overall"`
	Channels      []channelMonitoringChannelSummary `json:"channels"`
}

type channelMonitoringCounters struct {
	attemptCount   int64
	successCount   int64
	totalLatencyMs int64
}

func (counters channelMonitoringCounters) plus(other channelMonitoringCounters) channelMonitoringCounters {
	counters.attemptCount += other.attemptCount
	counters.successCount += other.successCount
	counters.totalLatencyMs += other.totalLatencyMs
	return counters
}

// channelMonitoringAvailability sums the counters first and divides once. Never
// average per-bucket rates: a bucket holding one attempt would then weigh as
// much as a bucket holding ten thousand.
func channelMonitoringAvailability(counters channelMonitoringCounters) float64 {
	if counters.attemptCount <= 0 {
		return 0
	}
	rate := float64(counters.successCount) / float64(counters.attemptCount) * 100
	return math.Round(rate*100) / 100
}

func channelMonitoringHealthState(availabilityRate float64) string {
	switch {
	case availabilityRate >= channelMonitoringHealthyRate:
		return channelMonitoringStateHealthy
	case availabilityRate >= channelMonitoringDegradedRate:
		return channelMonitoringStateDegraded
	default:
		return channelMonitoringStateDown
	}
}

// channelMonitoringStepSeconds clamps the display width to the physical
// channel_metrics bucket width. A slot narrower than the stored bucket cannot
// be produced, so widening the global perf_metrics bucket degrades the timeline
// to the finest width storage can actually serve.
func channelMonitoringStepSeconds(stepSeconds int64) int64 {
	storageSeconds := perf_metrics_setting.GetBucketSeconds()
	if stepSeconds < storageSeconds {
		return storageSeconds
	}
	return stepSeconds
}

// channelMonitoringSeriesEnd returns the exclusive upper bound of the rendered
// timeline. A closed bucket is not readable until every node has flushed it,
// and the flush ticker has no phase relationship with the bucket boundary, so a
// bucket that closed a moment ago can still be sitting in another node's
// memory. Rendering it would show only the slice of attempts that reached the
// nodes which already flushed, turning a couple of failures into a phantom
// outage. The clock is rewound by one flush interval before aligning to the
// display grid, so the bound never depends on how the two widths compare.
func channelMonitoringSeriesEnd(now int64, stepSeconds int64) int64 {
	flushSeconds := int64(perf_metrics_setting.GetFlushIntervalMinutes()) * 60
	settled := now - flushSeconds
	return settled - settled%stepSeconds
}

func buildChannelMonitoringChannels(rows []model.ChannelMetricBucket, identities []model.ChannelIdentity, seriesStart int64, bucketCount int, stepSeconds int64) ([]channelMonitoringChannelSummary, channelMonitoringCounters) {
	seriesEnd := seriesStart + int64(bucketCount)*stepSeconds
	rolled := make(map[int]map[int64]channelMonitoringCounters)
	for _, row := range rows {
		if row.BucketTs < seriesStart || row.BucketTs >= seriesEnd {
			continue
		}
		if _, ok := rolled[row.ChannelId]; !ok {
			rolled[row.ChannelId] = make(map[int64]channelMonitoringCounters)
		}
		rolled[row.ChannelId][row.BucketTs] = rolled[row.ChannelId][row.BucketTs].plus(channelMonitoringCounters{
			attemptCount:   row.AttemptCount,
			successCount:   row.SuccessCount,
			totalLatencyMs: row.TotalLatencyMs,
		})
	}

	// A channel deleted after producing samples keeps its row with an empty
	// name so the history is not silently dropped; the UI falls back to the id.
	names := make(map[int]string, len(identities))
	channelIds := make([]int, 0, len(identities)+len(rolled))
	for _, identity := range identities {
		names[identity.Id] = identity.Name
		channelIds = append(channelIds, identity.Id)
	}
	for channelId := range rolled {
		if _, ok := names[channelId]; !ok {
			channelIds = append(channelIds, channelId)
		}
	}

	overall := channelMonitoringCounters{}
	summaries := make([]channelMonitoringChannelSummary, 0, len(channelIds))
	for _, channelId := range channelIds {
		buckets := rolled[channelId]
		total := channelMonitoringCounters{}
		series := make([]channelMonitoringBucket, bucketCount)
		for index := range series {
			ts := seriesStart + int64(index)*stepSeconds
			value := buckets[ts]
			total = total.plus(value)
			state := channelMonitoringStateNoData
			if value.attemptCount > 0 {
				state = channelMonitoringHealthState(channelMonitoringAvailability(value))
			}
			series[index] = channelMonitoringBucket{
				Ts:           ts,
				AttemptCount: value.attemptCount,
				SuccessCount: value.successCount,
				State:        state,
			}
		}
		overall = overall.plus(total)

		summary := channelMonitoringChannelSummary{
			ChannelId:    channelId,
			ChannelName:  names[channelId],
			HasData:      total.attemptCount > 0,
			State:        channelMonitoringStateNoData,
			AttemptCount: total.attemptCount,
			SuccessCount: total.successCount,
			Buckets:      series,
		}
		if total.attemptCount > 0 {
			summary.AvailabilityRate = channelMonitoringAvailability(total)
			summary.ErrorRate = math.Round((100-summary.AvailabilityRate)*100) / 100
			summary.AvgLatencyMs = total.totalLatencyMs / total.attemptCount
			summary.State = channelMonitoringHealthState(summary.AvailabilityRate)
		}
		summaries = append(summaries, summary)
	}

	// Worst first: an availability page is read to find the broken channel.
	sort.SliceStable(summaries, func(i, j int) bool {
		if summaries[i].HasData != summaries[j].HasData {
			return summaries[i].HasData
		}
		if summaries[i].HasData && summaries[i].AvailabilityRate != summaries[j].AvailabilityRate {
			return summaries[i].AvailabilityRate < summaries[j].AvailabilityRate
		}
		return summaries[i].ChannelId < summaries[j].ChannelId
	})
	return summaries, overall
}

func GetChannelMonitoringSummary(c *gin.Context) {
	rangeKey := c.DefaultQuery("range", channelMonitoringDefaultRange)
	window, ok := channelMonitoringWindows[rangeKey]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": fmt.Sprintf("unsupported range %q", rangeKey)})
		return
	}

	stepSeconds := channelMonitoringStepSeconds(window.stepSeconds)
	bucketCount := int(window.windowSeconds / stepSeconds)
	if bucketCount < 1 {
		bucketCount = 1
	}
	seriesEnd := channelMonitoringSeriesEnd(time.Now().Unix(), stepSeconds)
	seriesStart := seriesEnd - int64(bucketCount)*stepSeconds

	rows, err := model.GetChannelMetricBuckets(seriesStart, seriesEnd-1, stepSeconds)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	identities, err := model.GetChannelIdentities()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	channels, overall := buildChannelMonitoringChannels(rows, identities, seriesStart, bucketCount, stepSeconds)
	response := channelMonitoringSummaryResponse{
		Range:         rangeKey,
		Ranges:        channelMonitoringRangeKeys,
		StepMinutes:   int(stepSeconds / 60),
		WindowSeconds: int64(bucketCount) * stepSeconds,
		Overall: channelMonitoringOverall{
			HasData:      overall.attemptCount > 0,
			AttemptCount: overall.attemptCount,
		},
		Channels: channels,
	}
	if overall.attemptCount > 0 {
		response.Overall.AvailabilityRate = channelMonitoringAvailability(overall)
		response.Overall.ErrorRate = math.Round((100-response.Overall.AvailabilityRate)*100) / 100
		response.Overall.AvgLatencyMs = overall.totalLatencyMs / overall.attemptCount
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": response})
}
