package service

import "time"

const MaxDistinctChannelsPerRequest = 20

type RouteAttemptOutcome string

const (
	RouteAttemptSucceeded RouteAttemptOutcome = "succeeded"
	RouteAttemptRetrying  RouteAttemptOutcome = "retrying_channel"
	RouteAttemptExcluded  RouteAttemptOutcome = "channel_exhausted"
	RouteAttemptStopped   RouteAttemptOutcome = "stopped"
)

type RouteAttempt struct {
	ChannelID   int                 `json:"channel_id"`
	ChannelName string              `json:"channel_name,omitempty"`
	Attempt     int                 `json:"attempt"`
	Outcome     RouteAttemptOutcome `json:"outcome"`
	Reason      string              `json:"reason,omitempty"`
	StatusCode  int                 `json:"status_code,omitempty"`
	Priority    int64               `json:"priority,omitempty"`
	Weight      int                 `json:"weight,omitempty"`
	Timing      *RouteAttemptTiming `json:"timing,omitempty"`
}

type RouteAttemptTiming struct {
	StartedAtMS   int64 `json:"started_at_ms"`
	EndedAtMS     int64 `json:"ended_at_ms"`
	StartOffsetMS int64 `json:"start_offset_ms"`
	EndOffsetMS   int64 `json:"end_offset_ms"`
	DurationMS    int64 `json:"duration_ms"`
}

type RouteState struct {
	ExcludedChannelIDs map[int]struct{}
	AttemptsByChannel  map[int]int
	History            []RouteAttempt
	requestStart       time.Time
	attemptStart       time.Time
	channelID          int
	attemptNumber      int
	timing             *RouteAttemptTiming
}

func NewRouteState() *RouteState {
	return &RouteState{
		ExcludedChannelIDs: make(map[int]struct{}),
		AttemptsByChannel:  make(map[int]int),
		History:            make([]RouteAttempt, 0, 8),
	}
}

func (s *RouteState) BeginAttempt(channelID int) int {
	s.AttemptsByChannel[channelID]++
	return s.AttemptsByChannel[channelID]
}

func (s *RouteState) Exclude(channelID int) {
	s.ExcludedChannelIDs[channelID] = struct{}{}
}

func (s *RouteState) DistinctChannelsAttempted() int {
	return len(s.AttemptsByChannel)
}

func (s *RouteState) CanAttempt(channelID int, maxAttempts int) bool {
	return maxAttempts > 0 && s.AttemptsByChannel[channelID] < maxAttempts
}

func (s *RouteState) CanSelectMoreChannels() bool {
	return s.DistinctChannelsAttempted() < MaxDistinctChannelsPerRequest
}

func (s *RouteState) Record(channelID int, attempt int, outcome RouteAttemptOutcome, reason string) {
	s.RecordDetails(RouteAttempt{
		ChannelID: channelID,
		Attempt:   attempt,
		Outcome:   outcome,
		Reason:    reason,
	})
}

func (s *RouteState) RecordDetails(attempt RouteAttempt) {
	attempt.Timing = s.FinishAttemptTiming(attempt.ChannelID, attempt.Attempt, time.Now())
	s.History = append(s.History, attempt)
}

func (s *RouteState) StartAttemptTiming(channelID, attempt int, requestStart, startedAt time.Time) {
	s.requestStart = requestStart
	s.attemptStart = startedAt
	s.channelID = channelID
	s.attemptNumber = attempt
	s.timing = nil
}

// FinishAttemptTiming freezes the result once, including when billing records
// success before the relay helper returns. Returned snapshots are independent.
func (s *RouteState) FinishAttemptTiming(channelID, attempt int, endedAt time.Time) *RouteAttemptTiming {
	if s.channelID != channelID || s.attemptNumber != attempt || s.attemptStart.IsZero() || s.requestStart.IsZero() {
		return nil
	}
	if s.timing == nil {
		s.timing = &RouteAttemptTiming{
			StartedAtMS: s.attemptStart.UnixMilli(), EndedAtMS: endedAt.UnixMilli(),
			StartOffsetMS: s.attemptStart.Sub(s.requestStart).Milliseconds(),
			EndOffsetMS:   endedAt.Sub(s.requestStart).Milliseconds(),
			DurationMS:    endedAt.Sub(s.attemptStart).Milliseconds(),
		}
	}
	snapshot := *s.timing
	return &snapshot
}
