package dto

import (
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
