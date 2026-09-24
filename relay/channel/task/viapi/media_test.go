package viapi

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestViapiActionsAndSignerRejectMissingCredentials(t *testing.T) {
	cfg, ok := LookupAction("viapi-erase-subtitles")
	require.True(t, ok)
	assert.Equal(t, "EraseVideoSubtitles", cfg.Action)
	assert.Equal(t, "per-minute", cfg.BillingMode)
	_, err := SignRequest("POST", "videoenhan.cn-shanghai.aliyuncs.com", "/", "", "EraseVideoSubtitles", "2020-03-20", map[string]string{"host": "videoenhan.cn-shanghai.aliyuncs.com"}, nil, "", "")
	require.Error(t, err)
	signed, err := SignRequest("POST", "videoenhan.cn-shanghai.aliyuncs.com", "/", "", "EraseVideoSubtitles", "2020-03-20", map[string]string{"host": "videoenhan.cn-shanghai.aliyuncs.com", "x-acs-date": "2026-09-22T00:00:00Z", "x-acs-signature-nonce": "fixed"}, []byte("a=1"), "ak", "sk")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(signed["authorization"], "ACS3-HMAC-SHA256"))
}
