package dto

import "fmt"

const (
	DefaultChannelMaxAttempts     = 2
	DefaultAutoBanThreshold       = 5
	DefaultAutoBanDurationSeconds = 30 * 60
	MaxChannelMaxAttempts         = 10
	MaxAutoBanThreshold           = 100
	MaxAutoBanDurationSeconds     = 24 * 60 * 60
)

type ChannelReliabilitySettings struct {
	FirstContentTimeoutSeconds  *int `json:"first_content_timeout_seconds,omitempty"`
	StreamingIdleTimeoutSeconds *int `json:"streaming_idle_timeout_seconds,omitempty"`
	NonStreamingTimeoutSeconds  *int `json:"non_streaming_timeout_seconds,omitempty"`
	MaxAttempts                 int  `json:"max_attempts,omitempty"`
	AutoBanThreshold            int  `json:"auto_ban_threshold,omitempty"`
	AutoBanDurationSecs         int  `json:"auto_ban_duration_seconds,omitempty"`
}

func DefaultChannelReliabilitySettings() ChannelReliabilitySettings {
	return ChannelReliabilitySettings{
		MaxAttempts:         DefaultChannelMaxAttempts,
		AutoBanThreshold:    DefaultAutoBanThreshold,
		AutoBanDurationSecs: DefaultAutoBanDurationSeconds,
	}
}

func (s ChannelReliabilitySettings) WithDefaults() ChannelReliabilitySettings {
	defaults := DefaultChannelReliabilitySettings()
	if s.MaxAttempts == 0 {
		s.MaxAttempts = defaults.MaxAttempts
	}
	if s.AutoBanThreshold == 0 {
		s.AutoBanThreshold = defaults.AutoBanThreshold
	}
	if s.AutoBanDurationSecs == 0 {
		s.AutoBanDurationSecs = defaults.AutoBanDurationSecs
	}
	return s
}

func (s ChannelReliabilitySettings) Validate() error {
	for _, bound := range []struct {
		name     string
		value    *int
		min, max int
	}{
		{"first_content_timeout_seconds", s.FirstContentTimeoutSeconds, 1, 180},
		{"streaming_idle_timeout_seconds", s.StreamingIdleTimeoutSeconds, 60, 600},
		{"non_streaming_timeout_seconds", s.NonStreamingTimeoutSeconds, 60, 1800},
	} {
		if bound.value != nil && *bound.value != 0 && (*bound.value < bound.min || *bound.value > bound.max) {
			return fmt.Errorf("reliability.%s must be 0 or between %d and %d", bound.name, bound.min, bound.max)
		}
	}
	s = s.WithDefaults()
	if s.MaxAttempts < 1 || s.MaxAttempts > MaxChannelMaxAttempts {
		return fmt.Errorf("reliability.max_attempts must be between 1 and %d", MaxChannelMaxAttempts)
	}
	if s.AutoBanThreshold < 1 || s.AutoBanThreshold > MaxAutoBanThreshold {
		return fmt.Errorf("reliability.auto_ban_threshold must be between 1 and %d", MaxAutoBanThreshold)
	}
	if s.AutoBanDurationSecs < 1 || s.AutoBanDurationSecs > MaxAutoBanDurationSeconds {
		return fmt.Errorf("reliability.auto_ban_duration_seconds must be between 1 and %d", MaxAutoBanDurationSeconds)
	}
	return nil
}
