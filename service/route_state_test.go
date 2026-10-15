package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouteStateTracksAttemptsExclusionAndSafetyCap(t *testing.T) {
	state := NewRouteState()
	assert.Equal(t, 1, state.BeginAttempt(10))
	assert.Equal(t, 2, state.BeginAttempt(10))
	assert.False(t, state.CanAttempt(10, 2))
	for channelID := 11; channelID < 30; channelID++ {
		state.BeginAttempt(channelID)
	}
	assert.Equal(t, MaxDistinctChannelsPerRequest, state.DistinctChannelsAttempted())
	assert.False(t, state.CanSelectMoreChannels())
	state.Exclude(10)
	_, excluded := state.ExcludedChannelIDs[10]
	assert.True(t, excluded)
}

func TestGenerateTextOtherInfoIncludesCurrentSuccessfulRouteAttempt(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	state := NewRouteState()
	state.Record(10, 1, RouteAttemptRetrying, "retryable_upstream_status")
	c.Set("route_state", state)
	c.Set("route_attempt_channel_id", 11)
	c.Set("route_attempt_number", 1)

	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	other := GenerateTextOtherInfo(c, info, 1, 1, 1, 0, 0, 0, 1)
	data, err := common.Marshal(routeHistoryFromOther(t, other))
	require.NoError(t, err)
	assert.JSONEq(t, `[
		{"channel_id":10,"attempt":1,"outcome":"retrying_channel","reason":"retryable_upstream_status"},
		{"channel_id":11,"attempt":1,"outcome":"succeeded","status_code":200}
	]`, string(data))
}

// routeHistoryFromOther asserts the routing chain stays inside admin_info.
// model.formatUserLogs drops admin_info wholesale for non-admin viewers, which
// is the only thing keeping channel ids, names, priorities and upstream status
// codes out of a regular user's own log responses.
func routeHistoryFromOther(t *testing.T, other map[string]interface{}) interface{} {
	t.Helper()
	_, leaked := other["route_history"]
	require.False(t, leaked, "route_history must not sit at the top level of other")
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	history, ok := adminInfo["route_history"]
	require.True(t, ok)
	return history
}

func TestGenerateTextOtherInfoDoesNotDuplicateRecordedSuccess(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	state := NewRouteState()
	state.RecordDetails(RouteAttempt{
		ChannelID:   11,
		ChannelName: "CCTQ CC",
		Attempt:     1,
		Outcome:     RouteAttemptSucceeded,
		StatusCode:  200,
	})
	c.Set("route_state", state)
	c.Set("route_attempt_channel_id", 11)
	c.Set("route_attempt_channel_name", "CCTQ CC")
	c.Set("route_attempt_number", 1)

	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	other := GenerateTextOtherInfo(c, info, 1, 1, 1, 0, 0, 0, 1)
	data, err := common.Marshal(routeHistoryFromOther(t, other))
	require.NoError(t, err)
	assert.JSONEq(t, `[
		{"channel_id":11,"channel_name":"CCTQ CC","attempt":1,"outcome":"succeeded","status_code":200}
	]`, string(data))
}

func TestRouteStateRecordsReadableChannelIdentity(t *testing.T) {
	state := NewRouteState()
	state.RecordDetails(RouteAttempt{
		ChannelID:   11,
		ChannelName: "CCTQ CC",
		Attempt:     2,
		Outcome:     RouteAttemptRetrying,
		Reason:      "invalid_precommit_upstream_response",
		StatusCode:  502,
		Priority:    11,
		Weight:      1,
	})

	data, err := common.Marshal(state.History)
	require.NoError(t, err)
	assert.JSONEq(t, `[
		{"channel_id":11,"channel_name":"CCTQ CC","attempt":2,"outcome":"retrying_channel","reason":"invalid_precommit_upstream_response","status_code":502,"priority":11,"weight":1}
	]`, string(data))
}

func TestRouteAttemptTimingFreezesAndResetsAcrossRetries(t *testing.T) {
	requestStart := time.Unix(1700000000, 0)
	state := NewRouteState()
	require.Nil(t, state.FinishAttemptTiming(10, 1, requestStart))
	state.StartAttemptTiming(10, 1, requestStart, requestStart.Add(150*time.Millisecond))
	first := state.FinishAttemptTiming(10, 1, requestStart.Add(12300*time.Millisecond))
	require.NotNil(t, first)
	assert.Equal(t, &RouteAttemptTiming{
		StartedAtMS: requestStart.UnixMilli() + 150, EndedAtMS: requestStart.UnixMilli() + 12300,
		StartOffsetMS: 150, EndOffsetMS: 12300, DurationMS: 12150,
	}, first)
	assert.Equal(t, first, state.FinishAttemptTiming(10, 1, requestStart.Add(time.Hour)))
	state.Record(10, 1, RouteAttemptRetrying, "retryable_upstream_status")
	state.StartAttemptTiming(10, 2, requestStart, requestStart.Add(13000*time.Millisecond))
	require.Nil(t, state.FinishAttemptTiming(10, 1, requestStart.Add(time.Hour)), "preparation failures must not reuse the previous attempt")
	second := state.FinishAttemptTiming(10, 2, requestStart.Add(14500*time.Millisecond))
	require.NotNil(t, second)
	assert.Equal(t, int64(1500), second.DurationMS)
	assert.Equal(t, int64(14500), second.EndOffsetMS)
	state.Record(10, 2, RouteAttemptExcluded, "attempts_exhausted")
	assert.Equal(t, first, state.History[0].Timing)
	assert.Equal(t, second, state.History[1].Timing)
	first.DurationMS = -1
	assert.Equal(t, int64(12150), state.History[0].Timing.DurationMS, "snapshots must not share mutable timing")
}

func TestRouteAttemptTimingPreservesZeroMilliseconds(t *testing.T) {
	now := time.Unix(1700000000, 0)
	state := NewRouteState()
	state.StartAttemptTiming(11, 1, now, now)
	timing := state.FinishAttemptTiming(11, 1, now)
	require.NotNil(t, timing)
	data, err := common.Marshal(timing)
	require.NoError(t, err)
	assert.JSONEq(t, `{"started_at_ms":1700000000000,"ended_at_ms":1700000000000,"start_offset_ms":0,"end_offset_ms":0,"duration_ms":0}`, string(data))
}

func TestEarlySuccessLogFreezesCurrentAttemptBeforeHelperReturns(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	state := NewRouteState()
	c.Set("route_state", state)
	c.Set("route_attempt_channel_id", 11)
	c.Set("route_attempt_number", 1)
	start := time.Now().Add(-time.Second)
	state.StartAttemptTiming(11, 1, start, start)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	other := GenerateTextOtherInfo(c, info, 1, 1, 1, 0, 0, 0, 1)
	history, ok := routeHistoryFromOther(t, other).([]RouteAttempt)
	require.True(t, ok)
	require.Len(t, history, 1)
	require.NotNil(t, history[0].Timing)
	frozen := *history[0].Timing
	assert.Equal(t, &frozen, state.FinishAttemptTiming(11, 1, start.Add(time.Hour)))
	state.Record(11, 1, RouteAttemptSucceeded, "")
	later := GenerateTextOtherInfo(c, info, 1, 1, 1, 0, 0, 0, 1)
	assert.Len(t, routeHistoryFromOther(t, later), 1)
	state.StartAttemptTiming(12, 1, start, start.Add(time.Hour))
	state.FinishAttemptTiming(12, 1, start.Add(2*time.Hour))
	assert.Equal(t, frozen, *history[0].Timing)
}
