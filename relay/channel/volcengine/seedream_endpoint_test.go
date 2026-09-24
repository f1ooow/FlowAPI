package volcengine

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedreamNativeEndpoint(t *testing.T) {
	for _, model := range []string{dto.Seedream40, dto.Seedream45, dto.Seedream50} {
		t.Run(model, func(t *testing.T) {
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://ark.example.test", UpstreamModelName: model}, RelayMode: relayconstant.RelayModeImagesGenerations}
			endpoint, err := (&Adaptor{}).GetRequestURL(info)
			require.NoError(t, err)
			assert.Equal(t, "https://ark.example.test/api/v3/images/generations", endpoint)
		})
	}
}
