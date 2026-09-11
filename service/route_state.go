package service

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
}

type RouteState struct {
	ExcludedChannelIDs map[int]struct{}
	AttemptsByChannel  map[int]int
	History            []RouteAttempt
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
	s.History = append(s.History, attempt)
}
