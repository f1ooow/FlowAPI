package relay

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relaykitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKling4SubmitRejectsProfileDowngradeAndWrongChannel(t *testing.T) {
	for _, tc := range []struct {
		name        string
		channelType int
		profile     string
		retry       bool
		platform    string
	}{
		{"retry cannot drop input-video and resolution billing", constant.ChannelTypeSora, "", true, ""},
		{"legacy database profile cannot select native Kling adaptor", constant.ChannelTypeKling, dto.Kling4ProtocolRolldek, false, ""},
		{"endpoint cannot select another task adaptor", constant.ChannelTypeSora, dto.Kling4ProtocolRolldek, false, "suno"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			common.SetContextKey(c, constant.ContextKeyChannelType, tc.channelType)
			common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, relaykitdto.ChannelOtherSettings{VideoProtocol: tc.profile})
			c.Set("platform", tc.platform)
			if tc.retry {
				c.Set("kling4_request", &dto.Kling4Request{})
			}
			result, err := RelayTaskSubmit(c, &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}})
			require.NotNil(t, err)
			assert.Nil(t, result)
			assert.Equal(t, "unsupported_channel_protocol", err.Code)
			assert.Equal(t, 400, err.StatusCode)
			assert.False(t, c.GetBool("task_paid_dispatched"))
		})
	}
}
