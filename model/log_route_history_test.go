package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func routeHistoryLogOther(history []interface{}) string {
	return common.MapToJsonStr(map[string]interface{}{
		"admin_info": map[string]interface{}{
			"use_channel":   []interface{}{"11"},
			"route_history": history,
		},
	})
}

func TestRouteHistoryChannelNamesAreBackfilledForAdminLogs(t *testing.T) {
	logs := []*Log{{
		ChannelId: 11,
		Other: routeHistoryLogOther([]interface{}{
			map[string]interface{}{
				"channel_id":  301,
				"attempt":     1,
				"outcome":     "retrying_channel",
				"status_code": 502,
			},
			map[string]interface{}{
				"channel_id":   11,
				"channel_name": "Historical name",
				"attempt":      1,
				"outcome":      "succeeded",
			},
		}),
	}}

	channelIds := types.NewSet[int]()
	collectRouteHistoryChannelIDs(logs, channelIds)
	assert.True(t, channelIds.Contains(301))
	assert.True(t, channelIds.Contains(11))

	enrichRouteHistoryChannelNames(logs, map[int]string{
		301: "Fastapi Codex",
		11:  "Current channel name",
	})

	other, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	history, ok := adminInfo["route_history"].([]interface{})
	require.True(t, ok)
	require.Len(t, history, 2)

	first, ok := history[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "Fastapi Codex", first["channel_name"])

	second, ok := history[1].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "Historical name", second["channel_name"], "existing historical names must not be overwritten")
}

func TestRouteHistoryChannelNamesIgnoreMalformedEntries(t *testing.T) {
	logs := []*Log{{
		Other: routeHistoryLogOther([]interface{}{
			"not-an-attempt",
			map[string]interface{}{"channel_id": "11"},
		}),
	}}

	channelIds := types.NewSet[int]()
	collectRouteHistoryChannelIDs(logs, channelIds)
	assert.Equal(t, 0, channelIds.Len())
	enrichRouteHistoryChannelNames(logs, map[int]string{11: "Unused"})
	assert.Contains(t, logs[0].Other, "not-an-attempt")
}

// TestFormatUserLogsHidesRouteHistory pins the user-facing redaction boundary:
// the routing chain exposes channel ids, names, priorities, weights and
// upstream status codes, so it must disappear from a user's own log response.
func TestFormatUserLogsHidesRouteHistory(t *testing.T) {
	logs := []*Log{{
		ChannelId:   11,
		ChannelName: "Fastapi Codex",
		Other: routeHistoryLogOther([]interface{}{
			map[string]interface{}{
				"channel_id":   301,
				"channel_name": "Fastapi Codex",
				"attempt":      1,
				"outcome":      "retrying_channel",
				"status_code":  502,
			},
		}),
	}}

	formatUserLogs(logs, 0)

	assert.Empty(t, logs[0].ChannelName)
	assert.NotContains(t, logs[0].Other, "route_history")
	assert.NotContains(t, logs[0].Other, "Fastapi Codex")
}
