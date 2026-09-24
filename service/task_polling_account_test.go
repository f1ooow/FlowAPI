package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type accountPollingFixture struct {
	info             *relaycommon.RelayInfo
	base, key, proxy string
	request          map[string]any
}

func (a *accountPollingFixture) Init(info *relaycommon.RelayInfo) { a.info = info }
func (a *accountPollingFixture) FetchTask(base, key string, request map[string]any, proxy string) (*http.Response, error) {
	a.base = base
	a.key = key
	a.proxy = proxy
	a.request = request
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"status":"running"}`))}, nil
}
func (a *accountPollingFixture) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) {
	return &relaycommon.TaskInfo{Status: model.TaskStatusInProgress, Progress: "30%"}, nil
}
func (a *accountPollingFixture) AdjustBillingOnComplete(*model.Task, *relaycommon.TaskInfo) int {
	return 0
}

func TestVideoPollingUsesPerTaskFrozenAccountAndProtocol(t *testing.T) {
	truncate(t)
	channelBase := "https://changed.example"
	channel := &model.Channel{Id: 803, Type: 1, Key: "changed-key", BaseURL: &channelBase, OtherInfo: `{"space":"changed"}`}
	for _, tc := range []struct {
		name, base, key, other string
		settings               *dto.ChannelOtherSettings
		legacy                 bool
	}{
		{name: "original Wan profile", base: "https://original.example", key: "original-account", other: `{"space":"original"}`, settings: &dto.ChannelOtherSettings{VideoProtocol: taskdto.WanProtocolRolldek, TaskEndpointOverride: &dto.TaskEndpointOverride{SubmitPath: "/create", FetchPath: "/tasks/{task_id}"}}},
		{name: "explicitly empty old settings", base: "https://empty.example", key: "other-original-account", other: "", settings: &dto.ChannelOtherSettings{}},
		{name: "legacy task fallback", legacy: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			task := makeTask(801, 803, 0, 802, "wallet", 0)
			task.TaskID = "task-" + tc.name
			task.Properties = model.Properties{OriginModelName: "public-alias", UpstreamModelName: "MiniMax-H3"}
			task.PrivateData.UpstreamTaskID = "provider-id"
			task.StartTime = 1
			task.Progress = "30%"
			if !tc.legacy {
				task.PrivateData.Key = tc.key
				task.PrivateData.ChannelBaseURL = tc.base
				task.PrivateData.ChannelSettings = tc.settings
				other := tc.other
				task.PrivateData.ChannelOtherInfo = &other
			}
			require.NoError(t, task.Insert())
			a := &accountPollingFixture{}
			require.NoError(t, updateVideoSingleTask(context.Background(), a, channel, "provider-id", map[string]*model.Task{"provider-id": task}))
			require.NotNil(t, a.info)
			assert.Equal(t, 803, a.info.ChannelId)
			assert.Equal(t, "MiniMax-H3", a.info.UpstreamModelName)
			assert.Equal(t, "public-alias", a.info.OriginModelName)
			assert.Equal(t, "provider-id", a.request["task_id"])
			assert.Equal(t, "MiniMax-H3", a.request["model"])
			if tc.legacy {
				assert.Equal(t, channelBase, a.base)
				assert.Equal(t, "changed-key", a.key)
				assert.Equal(t, channel.OtherInfo, a.info.ChannelOtherInfo)
			} else {
				assert.Equal(t, tc.base, a.base)
				assert.Equal(t, tc.key, a.key)
				assert.Equal(t, tc.other, a.info.ChannelOtherInfo)
				assert.Equal(t, tc.settings.VideoProtocol, a.info.ChannelOtherSettings.VideoProtocol)
				assert.Equal(t, tc.settings.TaskEndpointOverride, a.info.ChannelOtherSettings.TaskEndpointOverride)
			}
		})
	}
}
