package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelValidateSettingsRejectsInvalidHTTPTransport(t *testing.T) {
	tests := []struct {
		name    string
		setting dto.ChannelSettings
		wantErr string
	}{
		{
			name:    "auto with shards is valid",
			setting: dto.ChannelSettings{HTTPProtocol: "auto", HTTP2ConnectionShards: 4},
		},
		{
			name:    "http1 with shards greater than one rejected",
			setting: dto.ChannelSettings{HTTPProtocol: "http1", HTTP2ConnectionShards: 2},
			wantErr: "http2_connection_shards",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel := &Channel{}
			channel.SetSetting(tt.setting)
			err := channel.ValidateSettings()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestAdvancedCustomChannelRequiresModelListRouteOnlyWhenUpdateChecksEnabled(t *testing.T) {
	inferenceRoute := dto.AdvancedCustomRoute{
		IncomingPath: "/v1/chat/completions",
		UpstreamPath: "/v1/chat/completions",
		Converter:    "none",
	}

	tests := []struct {
		name          string
		checksEnabled bool
		routes        []dto.AdvancedCustomRoute
		wantErr       string
	}{
		{
			name:   "legacy channel without discovery route remains valid",
			routes: []dto.AdvancedCustomRoute{inferenceRoute},
		},
		{
			name:          "enabled checks require discovery route",
			checksEnabled: true,
			routes:        []dto.AdvancedCustomRoute{inferenceRoute},
			wantErr:       dto.AdvancedCustomModelListPath,
		},
		{
			name:          "enabled checks accept discovery route",
			checksEnabled: true,
			routes: []dto.AdvancedCustomRoute{
				inferenceRoute,
				{
					IncomingPath: dto.AdvancedCustomModelListPath,
					UpstreamPath: dto.AdvancedCustomModelListPath,
					Converter:    "none",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel := &Channel{Type: constant.ChannelTypeAdvancedCustom}
			channel.SetOtherSettings(dto.ChannelOtherSettings{
				UpstreamModelUpdateCheckEnabled: tt.checksEnabled,
				AdvancedCustom: &dto.AdvancedCustomConfig{
					Routes: tt.routes,
				},
			})

			err := channel.ValidateSettings()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// The channel form stores reliability in the settings column; the legacy
// setting column must not feed any reader.
func TestChannelReliabilitySettingsReadFromSettingsColumn(t *testing.T) {
	legacySetting := `{"reliability":{"max_attempts":5,"auto_ban_threshold":9}}`
	tests := []struct {
		name          string
		setting       *string
		otherSettings string
		want          dto.ChannelReliabilitySettings
	}{
		{
			name:          "saved values",
			otherSettings: `{"reliability":{"max_attempts":3,"auto_ban_threshold":7,"auto_ban_duration_seconds":600,"streaming_idle_timeout_seconds":0,"non_streaming_timeout_seconds":120}}`,
			want: dto.ChannelReliabilitySettings{
				MaxAttempts:                 3,
				AutoBanThreshold:            7,
				AutoBanDurationSecs:         600,
				StreamingIdleTimeoutSeconds: common.GetPointer(0),
				NonStreamingTimeoutSeconds:  common.GetPointer(120),
			},
		},
		{
			name:          "missing fields use defaults",
			otherSettings: `{"reliability":{"non_streaming_timeout_seconds":120}}`,
			want: dto.ChannelReliabilitySettings{
				MaxAttempts:                dto.DefaultChannelMaxAttempts,
				AutoBanThreshold:           dto.DefaultAutoBanThreshold,
				AutoBanDurationSecs:        dto.DefaultAutoBanDurationSeconds,
				NonStreamingTimeoutSeconds: common.GetPointer(120),
			},
		},
		{
			name:          "missing reliability uses defaults",
			otherSettings: `{"upstream_model_update_check_enabled":true}`,
			want:          dto.DefaultChannelReliabilitySettings(),
		},
		{
			name:    "legacy setting column is ignored",
			setting: &legacySetting,
			want:    dto.DefaultChannelReliabilitySettings(),
		},
		{
			// Channels saved while provider racing existed still carry the
			// retired first-content threshold; it must not affect any reader.
			name:          "retired first-content threshold is ignored",
			otherSettings: `{"reliability":{"max_attempts":3,"first_content_timeout_seconds":60}}`,
			want: dto.ChannelReliabilitySettings{
				MaxAttempts:         3,
				AutoBanThreshold:    dto.DefaultAutoBanThreshold,
				AutoBanDurationSecs: dto.DefaultAutoBanDurationSeconds,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel := &Channel{Setting: tt.setting, OtherSettings: tt.otherSettings}
			assert.Equal(t, tt.want, channel.GetReliabilitySettings())
		})
	}
}

func TestChannelValidateSettingsBoundsReliability(t *testing.T) {
	tests := []struct {
		name          string
		otherSettings string
		wantErr       string
	}{
		{
			name:          "saved form values",
			otherSettings: `{"reliability":{"max_attempts":3,"auto_ban_threshold":5,"auto_ban_duration_seconds":1800,"streaming_idle_timeout_seconds":0,"non_streaming_timeout_seconds":0}}`,
		},
		{
			name:          "max attempts above range",
			otherSettings: `{"reliability":{"max_attempts":11}}`,
			wantErr:       "max_attempts",
		},
		{
			name:          "auto-ban threshold above range",
			otherSettings: `{"reliability":{"auto_ban_threshold":101}}`,
			wantErr:       "auto_ban_threshold",
		},
		{
			name:          "auto-ban duration above range",
			otherSettings: `{"reliability":{"auto_ban_duration_seconds":86401}}`,
			wantErr:       "auto_ban_duration_seconds",
		},
		{
			name:          "streaming idle timeout below range",
			otherSettings: `{"reliability":{"streaming_idle_timeout_seconds":30}}`,
			wantErr:       "streaming_idle_timeout_seconds",
		},
		{
			name:          "fractional timeout",
			otherSettings: `{"reliability":{"non_streaming_timeout_seconds":90.5}}`,
			wantErr:       "non_streaming_timeout_seconds",
		},
		{
			name:          "retired first-content threshold does not block saving",
			otherSettings: `{"reliability":{"max_attempts":3,"first_content_timeout_seconds":181}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel := &Channel{OtherSettings: tt.otherSettings}
			err := channel.ValidateSettings()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
