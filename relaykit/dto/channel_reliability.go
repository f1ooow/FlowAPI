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
	MaxAttempts         int `json:"max_attempts,omitempty"`
	AutoBanThreshold    int `json:"auto_ban_threshold,omitempty"`
	AutoBanDurationSecs int `json:"auto_ban_duration_seconds,omitempty"`
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
