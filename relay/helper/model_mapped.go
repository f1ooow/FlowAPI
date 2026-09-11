package helper

import (
	"fmt"

	"github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

func ModelMappedHelper(c *gin.Context, info *common.RelayInfo, request dto.Request) error {
	if info.ChannelMeta == nil {
		info.ChannelMeta = &common.ChannelMeta{}
	}

	// Redirects are always evaluated from the immutable client-requested model.
	// A channel switch therefore cannot inherit the previous channel's target.
	modelMapping := c.GetString("model_mapping")
	if modelMapping != "" {
		rules, err := dto.ParseModelRedirectRules(modelMapping)
		if err != nil {
			return fmt.Errorf("unmarshal_model_mapping_failed: %w", err)
		}
		if target, matched := dto.MatchModelRedirect(rules, info.OriginModelName); matched && target != info.OriginModelName {
			info.UpstreamModelName = target
			info.IsModelMapped = true
		}
	}

	if request != nil {
		request.SetModelName(info.UpstreamModelName)
	}
	return nil
}
