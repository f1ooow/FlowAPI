package dto

import (
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelReliabilityDefaultsAndValidation(t *testing.T) {
	settings := (ChannelReliabilitySettings{}).WithDefaults()
	assert.Equal(t, 2, settings.MaxAttempts)
	assert.Equal(t, 5, settings.AutoBanThreshold)
	assert.Equal(t, 1800, settings.AutoBanDurationSecs)
	require.NoError(t, settings.Validate())
	require.Error(t, (ChannelReliabilitySettings{MaxAttempts: MaxChannelMaxAttempts + 1}).Validate())
}

func TestChannelTimeoutOverridesPreserveOmissionAndZero(t *testing.T) {
	var absent ChannelReliabilitySettings
	require.NoError(t, kitutil.Unmarshal([]byte(`{}`), &absent))
	assert.Nil(t, absent.WithDefaults().StreamingIdleTimeoutSeconds)
	for _, value := range []int{0, 60, 600} {
		settings := ChannelReliabilitySettings{StreamingIdleTimeoutSeconds: &value}
		require.NoError(t, settings.Validate())
		data, err := kitutil.Marshal(settings.WithDefaults())
		require.NoError(t, err)
		var decoded ChannelReliabilitySettings
		require.NoError(t, kitutil.Unmarshal(data, &decoded))
		require.NotNil(t, decoded.StreamingIdleTimeoutSeconds)
		assert.Equal(t, value, *decoded.StreamingIdleTimeoutSeconds)
	}
	for _, value := range []int{-1, 181} {
		require.Error(t, (ChannelReliabilitySettings{FirstContentTimeoutSeconds: &value}).Validate())
	}
	for _, value := range []int{0, 1, 180} {
		require.NoError(t, (ChannelReliabilitySettings{FirstContentTimeoutSeconds: &value}).Validate())
	}
	for _, value := range []int{-1, 59, 601} {
		require.Error(t, (ChannelReliabilitySettings{StreamingIdleTimeoutSeconds: &value}).Validate())
	}
	for _, value := range []int{-1, 59, 1801} {
		require.Error(t, (ChannelReliabilitySettings{NonStreamingTimeoutSeconds: &value}).Validate())
	}
	for _, value := range []int{0, 60, 1800} {
		require.NoError(t, (ChannelReliabilitySettings{NonStreamingTimeoutSeconds: &value}).Validate())
	}
}
