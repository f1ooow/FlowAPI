package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relaykitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type kling4PollingAdaptor struct {
	unknownStatusPollingAdaptor
	status  string
	profile string
}

func (a *kling4PollingAdaptor) Init(info *relaycommon.RelayInfo) {
	a.profile = info.ChannelOtherSettings.VideoProtocol
}

func (a *kling4PollingAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) {
	return &relaycommon.TaskInfo{Status: a.status, Reason: "fixture failure", TotalTokens: 9999999, OutputVideoDuration: 1}, nil
}

func TestKling4FrozenChargeAndFailureRefundAcrossProfileRollout(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		for _, failed := range []bool{false, true} {
			name := "precise"
			if legacy {
				name = "legacy"
			}
			if failed {
				name += "/failed"
			} else {
				name += "/completed"
			}
			t.Run(name, func(t *testing.T) {
				truncate(t)
				const initialQuota = 100000000
				reserved, inputSeconds, billedSeconds, resolution, profile := 44100000, 150.0, 180.0, 1.4, "rolldek-kling4"
				if legacy {
					reserved, inputSeconds, billedSeconds, resolution, profile = 1750000, 0, 10, 1, ""
				}
				seedUser(t, 931, initialQuota-reserved)
				seedToken(t, 932, 931, "kling4-polling", initialQuota-reserved)
				seedChannel(t, 933)
				seedChargedAccounting(t, 931, 933, 932, reserved, 1)
				info := &relaycommon.RelayInfo{
					TaskRelayInfo:   &relaycommon.TaskRelayInfo{BillingUnit: "seconds"},
					OriginModelName: "kling-4.0",
					PriceData:       types.PriceData{ModelPrice: .35, UsePrice: true, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}},
				}
				info.PriceData.AddOtherRatio("seconds", billedSeconds)
				info.PriceData.AddOtherRatio("resolution", resolution)
				task := makeTask(931, 933, reserved, 932, BillingSourceWallet, 0)
				task.TaskID = "kling4-frozen"
				task.SubmitTime = time.Now().Unix()
				task.Properties.OriginModelName = "kling-4.0"
				task.PrivateData.ChannelSettings = &relaykitdto.ChannelOtherSettings{VideoProtocol: profile}
				task.PrivateData.BillingContext = model.NewTaskBillingContext(info)
				task.PrivateData.BillingContext.InputVideoDuration = inputSeconds
				require.True(t, task.PrivateData.BillingContext.PerCallBilling)
				require.NoError(t, task.Insert())
				var stale model.Task
				require.NoError(t, model.DB.First(&stale, task.ID).Error)
				channel := &model.Channel{Id: 933, Type: constant.ChannelTypeSora, Key: "fixture"}
				channel.SetOtherSettings(relaykitdto.ChannelOtherSettings{VideoProtocol: "rolldek-kling4"})
				a := &kling4PollingAdaptor{status: model.TaskStatusSuccess}
				if failed {
					a.status = model.TaskStatusFailure
				}
				for _, polled := range []*model.Task{task, &stale} {
					require.NoError(t, updateVideoSingleTask(context.Background(), a, channel, polled.TaskID, map[string]*model.Task{polled.TaskID: polled}))
					assert.Equal(t, profile, a.profile, "old in-flight tasks retain the submission profile")
				}
				var stored model.Task
				require.NoError(t, model.DB.First(&stored, task.ID).Error)
				assert.Equal(t, inputSeconds, stored.PrivateData.BillingContext.InputVideoDuration)
				if failed {
					assert.Zero(t, stored.Quota)
					assert.Equal(t, initialQuota, getUserQuota(t, 931))
					assert.Equal(t, initialQuota, getTokenRemainQuota(t, 932))
					assert.EqualValues(t, 1, countLogs(t), "stale polling must not refund twice")
				} else {
					assert.Equal(t, reserved, stored.Quota, "do not use Wan's 30-second settlement cap or provider token counters")
					assert.Equal(t, initialQuota-reserved, getUserQuota(t, 931))
					assert.Equal(t, initialQuota-reserved, getTokenRemainQuota(t, 932))
					assert.Zero(t, countLogs(t), "completion must not charge a second time")
				}
			})
		}
	}
}
