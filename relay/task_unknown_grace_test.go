package relay

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Only DashScope reports an explicit UNKNOWN for lost tasks. Adaptors that map
// unrecognized provider states to UNKNOWN (H3, FastAI, Wan) must keep polling.
func TestOnlyAliOptsIntoUnknownStatusGrace(t *testing.T) {
	for channelType, want := range map[int]bool{
		constant.ChannelTypeAli:         true,
		constant.ChannelTypeMiniMax:     false,
		constant.ChannelTypeFastAIToken: false,
		constant.ChannelTypeOpenAI:      false,
		constant.ChannelTypeDoubaoVideo: false,
		constant.ChannelTypeVolcVod:     false,
	} {
		adaptor := GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(channelType)))
		require.NotNil(t, adaptor, channelType)
		grace, ok := adaptor.(service.TaskUnknownGraceAdaptor)
		assert.Equal(t, want, ok, channelType)
		if ok {
			assert.Equal(t, int64(120), grace.UnknownStatusGraceSeconds())
		}
	}
}
