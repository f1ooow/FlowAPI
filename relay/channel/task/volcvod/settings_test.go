package volcvod

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSnapshotOtherInfoKeepsPlaybackSettingsOnly(t *testing.T) {
	snapshot, err := SnapshotOtherInfo(map[string]interface{}{
		"vod_playback_domain": "cdn.example.com",
		"vod_playback_scheme": "https",
		"vod_url_auth_key":    "private-signing-key",
		"vod_space":           "private-space",
		"access_key":          "private-ak",
		"secret_key":          "private-sk",
	})
	require.NoError(t, err)
	require.Contains(t, snapshot, `"vod_playback_domain":"cdn.example.com"`)
	require.Contains(t, snapshot, `"vod_url_auth_key":"private-signing-key"`)
	require.NotContains(t, snapshot, "vod_space")
	require.NotContains(t, snapshot, "access_key")
	require.NotContains(t, snapshot, "secret_key")
	require.False(t, strings.Contains(snapshot, "private-space"))
}
