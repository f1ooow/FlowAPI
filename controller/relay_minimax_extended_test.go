package controller

import (
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMiniMaxUploadAlwaysFreeAndCloneRequiresPerCallPrice(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeMiniMaxFilesUpload, OriginModelName: "unconfigured-upload", UserGroup: "default", UsingGroup: "default", ChannelMeta: &relaycommon.ChannelMeta{}}
	price, err := miniMaxExtendedPrice(ctx, info)
	require.NoError(t, err)
	assert.True(t, price.FreeModel)
	assert.Zero(t, price.Quota)
	assert.True(t, price.UsePrice)
	info.RelayMode = relayconstant.RelayModeMiniMaxVoiceClone
	info.OriginModelName = "media-test-clone-unconfigured"
	_, err = miniMaxExtendedPrice(ctx, info)
	require.Error(t, err)
	// Existing explicit zero still counts as a configured per-call price.
	original := ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(original)) })
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"media-test-clone-unconfigured":0}`))
	price, err = miniMaxExtendedPrice(ctx, info)
	require.NoError(t, err)
	assert.True(t, price.FreeModel)
	assert.Zero(t, price.Quota)
}
