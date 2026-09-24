package minimax

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

const MaxUploadBytes = 20 << 20
const MaxSpeechCharacters = 10000
const maxExtendedResponseBytes = 64 << 20

func miniMaxError(err error, status int) *types.NewAPIError {
	return types.NewErrorWithStatusCode(err, types.ErrorCodeBadResponse, status, types.ErrOptionWithSkipRetry())
}

func miniMaxEndpoint(baseURL, path string) (string, error) {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.minimax.chat"
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("invalid MiniMax channel base URL")
	}
	return strings.TrimRight(baseURL, "/") + path, nil
}

// callMiniMaxExtended validates both HTTP and protocol status before callers
// write success or settle a per-call charge. It never retries paid operations.
func callMiniMaxExtended(c *gin.Context, info *relaycommon.RelayInfo, path, contentType string, body io.Reader) ([]byte, *types.NewAPIError) {
	endpoint, err := miniMaxEndpoint(info.ChannelBaseUrl, path)
	if err != nil {
		return nil, miniMaxError(err, http.StatusBadRequest)
	}
	request, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, endpoint, body)
	if err != nil {
		return nil, miniMaxError(err, http.StatusBadRequest)
	}
	request.Header.Set("Authorization", "Bearer "+info.ApiKey)
	request.Header.Set("Content-Type", contentType)
	response, err := service.GetHttpClient().Do(request)
	if err != nil {
		return nil, miniMaxError(fmt.Errorf("MiniMax upstream request failed: %w", err), http.StatusBadGateway)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxExtendedResponseBytes+1))
	if err != nil || len(raw) > maxExtendedResponseBytes {
		return nil, miniMaxError(fmt.Errorf("invalid or oversized MiniMax response"), http.StatusBadGateway)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, miniMaxError(fmt.Errorf("MiniMax upstream HTTP %d", response.StatusCode), response.StatusCode)
	}
	var envelope struct {
		BaseResp *MiniMaxBaseResp `json:"base_resp"`
	}
	if err := common.Unmarshal(raw, &envelope); err != nil || envelope.BaseResp == nil {
		return nil, miniMaxError(fmt.Errorf("MiniMax response is missing base_resp"), http.StatusBadGateway)
	}
	if envelope.BaseResp.StatusCode != 0 {
		return nil, miniMaxError(fmt.Errorf("MiniMax error %d: %s", envelope.BaseResp.StatusCode, envelope.BaseResp.StatusMsg), http.StatusBadRequest)
	}
	return raw, nil
}
