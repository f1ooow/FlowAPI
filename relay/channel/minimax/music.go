package minimax

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

func RelayMiniMaxMusic(c *gin.Context, info *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	var request map[string]json.RawMessage
	if err := common.UnmarshalBodyReusable(c, &request); err != nil {
		return nil, miniMaxError(err, http.StatusBadRequest)
	}
	for field, limit := range map[string]int{"prompt": 2000, "lyrics": 10000} {
		if raw, ok := request[field]; ok {
			var value string
			if err := common.Unmarshal(raw, &value); err != nil || utf8.RuneCountInString(value) > limit {
				return nil, miniMaxError(fmt.Errorf("invalid %s (maximum %d characters)", field, limit), http.StatusBadRequest)
			}
		}
	}
	if raw, ok := request["duration"]; ok {
		var seconds float64
		if common.Unmarshal(raw, &seconds) != nil || seconds <= 0 || seconds > relaycommon.MaxTaskDurationSeconds {
			return nil, miniMaxError(fmt.Errorf("invalid music duration"), http.StatusBadRequest)
		}
	}
	upstreamModel := info.UpstreamModelName
	if upstreamModel == "" {
		upstreamModel = info.OriginModelName
	}
	if upstreamModel == "minimax-music-generation" {
		return nil, miniMaxError(fmt.Errorf("configure music model mapping"), http.StatusBadRequest)
	}
	modelJSON, err := common.Marshal(upstreamModel)
	if err != nil {
		return nil, miniMaxError(err, http.StatusBadRequest)
	}
	request["model"] = modelJSON
	raw, err := common.Marshal(request)
	if err != nil {
		return nil, miniMaxError(err, http.StatusBadRequest)
	}
	response, apiErr := callMiniMaxExtended(c, info, "/v1/music_generation", "application/json", bytes.NewReader(raw))
	if apiErr != nil {
		return nil, apiErr
	}
	c.Data(http.StatusOK, "application/json", response)
	return &dto.Usage{}, nil
}
