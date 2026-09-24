package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitTaskPreservesProviderAccountAndProtocol(t *testing.T) {
	override := &dto.TaskEndpointOverride{FetchPath: "/original/{task_id}"}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: 42, ChannelType: constant.ChannelTypeMiniMax, ApiKey: "selected-test-key",
			ChannelBaseUrl: "https://original.example", UpstreamModelName: "provider-model",
			ChannelOtherSettings: dto.ChannelOtherSettings{VideoProtocol: "wan-rolldek-v1", TaskEndpointOverride: override},
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
		UserId:        7, OriginModelName: "client-model",
	}
	task := InitTask(constant.TaskPlatform("35"), info)
	override.FetchPath = "/changed/{task_id}"
	info.ApiKey = "rotated-test-key"
	require.NotNil(t, task.PrivateData.ChannelSettings)
	require.NotNil(t, task.PrivateData.ChannelSettings.TaskEndpointOverride)
	assert.Equal(t, "selected-test-key", task.PrivateData.Key)
	assert.Equal(t, "https://original.example", task.PrivateData.ChannelBaseURL)
	assert.Equal(t, "wan-rolldek-v1", task.PrivateData.ChannelSettings.VideoProtocol)
	assert.Equal(t, "/original/{task_id}", task.PrivateData.ChannelSettings.TaskEndpointOverride.FetchPath)
	assert.Equal(t, "provider-model", task.Properties.UpstreamModelName)
	assert.Equal(t, "client-model", task.Properties.OriginModelName)
	assert.Equal(t, "task_public", task.TaskID)
}
