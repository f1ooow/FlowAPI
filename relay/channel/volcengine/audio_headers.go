package volcengine

import (
	"net/http"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

const contextKeySeedTTSV3SubtitleEnabled = "volcengine_seed_tts_v3_subtitle_enabled"

func setupSeedTTSV3Headers(header *http.Header, info *relaycommon.RelayInfo) error {
	appID, accessToken, err := parseVolcengineAuth(info.ApiKey)
	if err != nil {
		return err
	}
	header.Del("Authorization")
	header.Set("Content-Type", "application/json")
	header.Set("X-Api-App-Id", appID)
	header.Set("X-Api-Access-Key", accessToken)
	header.Set("X-Api-Resource-Id", "seed-tts-2.0")
	header.Set("X-Api-Request-Id", generateRequestID())
	return nil
}
