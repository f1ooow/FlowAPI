package service

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// RecordAutoBanFailure counts qualifying automatic-ban failures and disables
// the channel only after its configured threshold. The ban window is stored in
// channel metadata so the normal channel list and restore flow can observe it.
func RecordAutoBanFailure(channelError types.ChannelError, reason string) {
	result, err := model.RecordChannelAutoBanFailure(channelError.ChannelId, reason)
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to record automatic-ban failure: channel_id=%d, error=%v", channelError.ChannelId, err))
		return
	}
	if result.ConsecutiveFailures == 0 {
		return
	}
	common.SysLog(fmt.Sprintf("channel automatic-ban failure recorded: channel_id=%d, failures=%d, threshold=%d", channelError.ChannelId, result.ConsecutiveFailures, result.Threshold))
	if !result.Banned {
		return
	}
	subject := fmt.Sprintf("通道「%s」（#%d）已被自动封禁", channelError.ChannelName, channelError.ChannelId)
	NotifyRootUser(formatNotifyType(channelError.ChannelId, common.ChannelStatusAutoDisabled), subject, subject+"，原因："+reason)
}

// RecordAutoBanSuccess ends the current consecutive-failure streak. Automatic
// or manual bans are not changed by an in-flight request completing later.
func RecordAutoBanSuccess(channelId int) {
	channel, err := model.CacheGetChannel(channelId)
	if err != nil || channel == nil || channel.GetAutoBanFailureCount() == 0 {
		return
	}
	if _, err := model.ClearChannelAutoBanFailures(channelId); err != nil {
		common.SysLog(fmt.Sprintf("failed to clear automatic-ban failures: channel_id=%d, error=%v", channelId, err))
	}
}

func formatNotifyType(channelId int, status int) string {
	return fmt.Sprintf("%s_%d_%d", dto.NotifyTypeChannelUpdate, channelId, status)
}

// disable & notify
func DisableChannel(channelError types.ChannelError, reason string) {
	common.SysLog(fmt.Sprintf("通道「%s」（#%d）发生错误，准备禁用，原因：%s", channelError.ChannelName, channelError.ChannelId, common.LocalLogPreview(reason)))

	// 检查是否启用自动禁用功能
	if !channelError.AutoBan {
		common.SysLog(fmt.Sprintf("通道「%s」（#%d）未启用自动禁用功能，跳过禁用操作", channelError.ChannelName, channelError.ChannelId))
		return
	}

	success := model.UpdateChannelStatus(channelError.ChannelId, channelError.UsingKey, common.ChannelStatusAutoDisabled, reason)
	if success {
		subject := fmt.Sprintf("通道「%s」（#%d）已被禁用", channelError.ChannelName, channelError.ChannelId)
		content := fmt.Sprintf("通道「%s」（#%d）已被禁用，原因：%s", channelError.ChannelName, channelError.ChannelId, reason)
		NotifyRootUser(formatNotifyType(channelError.ChannelId, common.ChannelStatusAutoDisabled), subject, content)
	}
}

func EnableChannel(channelId int, usingKey string, channelName string) {
	success := model.UpdateChannelStatus(channelId, usingKey, common.ChannelStatusEnabled, "")
	if success {
		subject := fmt.Sprintf("通道「%s」（#%d）已被启用", channelName, channelId)
		content := fmt.Sprintf("通道「%s」（#%d）已被启用", channelName, channelId)
		NotifyRootUser(formatNotifyType(channelId, common.ChannelStatusEnabled), subject, content)
	}
}

func ShouldDisableChannel(err *types.NewAPIError) bool {
	if !common.AutomaticDisableChannelEnabled {
		return false
	}
	if err == nil {
		return false
	}
	if types.IsChannelError(err) {
		return true
	}
	if types.IsSkipRetryError(err) {
		return false
	}
	if operation_setting.ShouldDisableByStatusCode(err.StatusCode) {
		return true
	}

	lowerMessage := strings.ToLower(err.Error())
	search, _ := AcSearch(lowerMessage, operation_setting.AutomaticDisableKeywords, true)
	return search
}

func ShouldEnableChannel(newAPIError *types.NewAPIError, status int) bool {
	if !common.AutomaticEnableChannelEnabled {
		return false
	}
	if newAPIError != nil {
		return false
	}
	if status != common.ChannelStatusAutoDisabled {
		return false
	}
	return true
}
