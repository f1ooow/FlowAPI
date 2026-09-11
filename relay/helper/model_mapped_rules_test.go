package helper

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelMappedHelperUsesFirstOrderedRule(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	c.Set("model_mapping", `[{"match_type":"contains","source":"opus","target":"claude-opus-4-6"},{"match_type":"regex","source":"^claude-.*","target":"fallback"}]`)
	request := &dto.GeneralOpenAIRequest{Model: "claude-opus-5"}
	info := &relaycommon.RelayInfo{
		OriginModelName: "claude-opus-5",
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "claude-opus-5"},
	}

	require.NoError(t, ModelMappedHelper(c, info, request))
	assert.True(t, info.IsModelMapped)
	assert.Equal(t, "claude-opus-4-6", info.UpstreamModelName)
	assert.Equal(t, "claude-opus-4-6", request.Model)
}
