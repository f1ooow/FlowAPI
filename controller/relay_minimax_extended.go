package controller

import (
	"fmt"
	"math"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	channelconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/minimax"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// miniMaxExtendedPrice keeps upload free even when a token fallback exists.
// Music and cloning use one explicitly configured per-call price.
func miniMaxExtendedPrice(c *gin.Context, info *relaycommon.RelayInfo) (hosttypes.PriceData, error) {
	ratio := helper.HandleGroupRatio(c, info)
	if info.RelayMode == relayconstant.RelayModeMiniMaxFilesUpload {
		return hosttypes.PriceData{FreeModel: true, UsePrice: true, GroupRatioInfo: ratio}, nil
	}
	price, configured := ratio_setting.GetModelPrice(info.OriginModelName, true)
	if !configured || math.IsNaN(price) || math.IsInf(price, 0) || price < 0 {
		return hosttypes.PriceData{}, fmt.Errorf("MiniMax extended model requires an explicit non-negative per-call price")
	}
	quota, clamp := common.QuotaFromFloatChecked(price * common.QuotaPerUnit * ratio.GroupRatio)
	info.QuotaClamp = clamp
	if clamp != nil {
		return hosttypes.PriceData{}, clamp
	}
	return hosttypes.PriceData{ModelPrice: price, UsePrice: true, FreeModel: quota == 0, Quota: quota,
		PreConsumeQuotaBeforeGroup: price * common.QuotaPerUnit, GroupRatioInfo: ratio}, nil
}

func RelayMiniMaxExtended(c *gin.Context) {
	info := relaycommon.GenRelayInfoOpenAI(c, nil)
	info.RelayMode = relayconstant.Path2RelayMode(c.Request.URL.Path)
	info.InitChannelMeta(c)
	if info.ChannelType != channelconstant.ChannelTypeMiniMax && !(info.ChannelType == channelconstant.ChannelTypeNewAPI && info.RelayMode == relayconstant.RelayModeMiniMaxFilesUpload) {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "MiniMax extended endpoints require a MiniMax channel"}})
		return
	}
	if info.RelayMode == relayconstant.RelayModeMiniMaxVoiceClone {
		info.OriginModelName = "minimax-voice-clone"
	}
	if info.RelayMode == relayconstant.RelayModeMiniMaxFilesUpload {
		info.OriginModelName = "minimax-files-upload"
	}
	if apiErr := helper.ModelMappedHelper(c, info, nil); apiErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": apiErr.Error()}})
		return
	}
	price, err := miniMaxExtendedPrice(c, info)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": err.Error(), "type": "model_price_error"}})
		return
	}
	info.PriceData = price
	var apiErr *types.NewAPIError
	if !price.FreeModel {
		apiErr = service.PreConsumeBilling(c, price.Quota, info)
		if apiErr != nil {
			c.JSON(apiErr.StatusCode, gin.H{"error": apiErr.ToOpenAIError()})
			return
		}
	}
	defer func() {
		if apiErr != nil && info.Billing != nil {
			info.Billing.Refund(c)
		}
	}()
	originalWriter := c.Writer
	buffered := newTaskResponseBuffer(originalWriter)
	c.Writer = buffered
	switch info.RelayMode {
	case relayconstant.RelayModeMiniMaxFilesUpload:
		_, apiErr = minimax.RelayMiniMaxFilesUpload(c, info)
	case relayconstant.RelayModeMiniMaxVoiceClone:
		_, apiErr = minimax.RelayMiniMaxVoiceClone(c, info)
	case relayconstant.RelayModeMiniMaxMusic:
		_, apiErr = minimax.RelayMiniMaxMusic(c, info)
	default:
		apiErr = types.NewErrorWithStatusCode(fmt.Errorf("unsupported MiniMax endpoint"), types.ErrorCodeInvalidRequest, http.StatusBadRequest)
	}
	c.Writer = originalWriter
	if apiErr != nil {
		c.JSON(apiErr.StatusCode, gin.H{"error": apiErr.ToOpenAIError()})
		return
	}
	if info.Billing != nil {
		if err := service.SettleBilling(c, info, price.Quota); err != nil {
			// Upstream succeeded. Do not refund a paid success because settlement failed.
			logger.LogError(c, "MiniMax settlement failed: "+err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "MiniMax settlement failed"}})
			return
		}
	}
	logMiniMaxConsumption(c, info)
	buffered.flushToClient()
}

func logMiniMaxConsumption(c *gin.Context, info *relaycommon.RelayInfo) {
	price := info.PriceData
	other := service.GenerateTextOtherInfo(c, info, 0, price.GroupRatioInfo.GroupRatio, 0, 0, 0, price.ModelPrice, price.GroupRatioInfo.UserGroupRatio)
	other["billing_unit"] = "call"
	if info.RelayMode == relayconstant.RelayModeMiniMaxFilesUpload {
		other["billing_unit"] = "free-upload"
	}
	model.RecordConsumeLog(c, info.UserId, model.RecordConsumeLogParams{
		ChannelId: info.ChannelId, ModelName: info.OriginModelName, TokenName: c.GetString("token_name"), Quota: price.Quota,
		Content: "MiniMax " + c.Request.URL.Path, TokenId: info.TokenId, Group: info.UsingGroup, Other: other,
	})
	model.UpdateUserUsedQuotaAndRequestCount(info.UserId, price.Quota)
	if price.Quota > 0 {
		model.UpdateChannelUsedQuota(info.ChannelId, price.Quota)
	}
}
