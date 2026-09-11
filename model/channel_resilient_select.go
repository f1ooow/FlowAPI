package model

import (
	"errors"
	"math/rand"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

type ChannelSelectionOptions struct {
	ExcludedChannelIDs map[int]struct{}
	CandidateAllowed   func(*Channel) bool
}

func GetRandomSatisfiedChannelWithOptions(group string, modelName string, requestPath string, options ChannelSelectionOptions) (*Channel, error) {
	var candidates []*Channel
	if common.MemoryCacheEnabled {
		channelSyncLock.RLock()
		candidates = make([]*Channel, 0, len(channelsIDM))
		for _, channel := range channelsIDM {
			if channel == nil {
				continue
			}
			candidate := *channel
			candidates = append(candidates, &candidate)
		}
		channelSyncLock.RUnlock()
	} else {
		query := ApplyChannelGroupFilter(DB.Where("status IN ?", []int{common.ChannelStatusEnabled, common.ChannelStatusAutoDisabled}), group)
		if err := query.Find(&candidates).Error; err != nil {
			return nil, err
		}
	}

	normalizedModel := ratio_setting.FormatMatchingModelName(modelName)
	eligible := make([]*Channel, 0, len(candidates))
	var highestPriority int64
	hasPriority := false
	for _, channel := range candidates {
		if channel != nil && channel.Status == common.ChannelStatusAutoDisabled {
			if MaybeRestoreAutoBannedChannel(channel) {
				channel.Status = common.ChannelStatusEnabled
			}
		}
		if channel == nil || channel.Status != common.ChannelStatusEnabled || !channelInGroup(channel, group) {
			continue
		}
		if _, excluded := options.ExcludedChannelIDs[channel.Id]; excluded {
			continue
		}
		if !channel.SupportsModel(modelName) && (normalizedModel == modelName || !channel.SupportsModel(normalizedModel)) {
			continue
		}
		if channel.Type == constant.ChannelTypeAdvancedCustom && !channel.SupportsRequestPath(requestPath, modelName) {
			continue
		}
		if options.CandidateAllowed != nil && !options.CandidateAllowed(channel) {
			continue
		}
		priority := channel.GetPriority()
		if !hasPriority || priority > highestPriority {
			highestPriority = priority
			hasPriority = true
			eligible = eligible[:0]
		}
		if priority == highestPriority {
			eligible = append(eligible, channel)
		}
	}
	if len(eligible) == 0 {
		return nil, nil
	}

	weightSum := 0
	for _, channel := range eligible {
		weightSum += channel.GetWeight()
	}
	if weightSum == 0 {
		return eligible[rand.Intn(len(eligible))], nil
	}
	weight := rand.Intn(weightSum)
	for _, channel := range eligible {
		weight -= channel.GetWeight()
		if weight < 0 {
			return channel, nil
		}
	}
	return nil, errors.New("channel not found")
}

func channelInGroup(channel *Channel, group string) bool {
	for _, candidateGroup := range channel.GetGroups() {
		if strings.TrimSpace(candidateGroup) == group {
			return true
		}
	}
	return false
}
