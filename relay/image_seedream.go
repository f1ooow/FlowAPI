package relay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// Validate the final payload, including passthrough and channel overrides, before
// reserving its output bound. The route's effective ratio is already snapshotted.
func prepareSeedreamImageRequest(c *gin.Context, info *relaycommon.RelayInfo, body io.Reader) (io.Reader, *types.NewAPIError) {
	if info.RelayMode != relayconstant.RelayModeImagesGenerations || c.ContentType() != "application/json" {
		return nil, types.NewErrorWithStatusCode(fmt.Errorf("Seedream reference editing requires JSON image input on /v1/images/generations"), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	raw, err := io.ReadAll(body)
	if err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	raw, request, err := normalizeSeedreamImagePayload(raw)
	if err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	layer := request.IsSeedreamLayerDecomposition()
	c.Set("seedream_layer_decomposition", layer)
	info.IsStream = request.Stream != nil && *request.Stream
	if info.PriceData.UsePrice && info.TieredBillingSnapshot == nil {
		n := 1.0
		if request.N != nil && *request.N > 0 {
			n = float64(*request.N)
		}
		if options := request.SequentialImageGenerationOptions; request.SequentialImageGeneration != nil && *request.SequentialImageGeneration == "auto" && options != nil && options.MaxImages != nil && float64(*options.MaxImages) > n {
			n = float64(*options.MaxImages)
		}
		if layer {
			n = dto.MaxSeedreamLayerImages * dto.SeedreamLayerImagePriceFactor
		}
		info.PriceData.AddOtherRatio("n", n)
		info.PriceData.PreConsumeQuotaBeforeGroup = info.PriceData.ApplyOtherRatiosToFloat(info.PriceData.ModelPrice * common.QuotaPerUnit)
		if apiErr := service.PrepareBillingForSelectedRoute(c, info); apiErr != nil {
			return nil, apiErr
		}
	}
	return bytes.NewReader(raw), nil
}

func normalizeSeedreamImagePayload(raw []byte) ([]byte, *dto.ImageRequest, error) {
	// Preserve arbitrary passthrough fields while removing only Pro's explicitly
	// unsupported sequential and web search controls. Never roundtrip the full
	// body through DTO.
	var fields map[string]json.RawMessage
	if err := common.Unmarshal(raw, &fields); err != nil {
		return nil, nil, err
	}
	var model string
	if err := common.Unmarshal(fields["model"], &model); err != nil {
		return nil, nil, err
	}
	if model == dto.Seedream50Pro {
		delete(fields, "sequential_image_generation")
		delete(fields, "sequential_image_generation_options")
		delete(fields, "tools")
		var err error
		raw, err = common.Marshal(fields)
		if err != nil {
			return nil, nil, err
		}
	}
	var request dto.ImageRequest
	if err := common.Unmarshal(raw, &request); err != nil {
		return nil, nil, err
	}
	if request.N != nil && *request.N > dto.MaxImageN {
		return nil, nil, fmt.Errorf("n must not exceed %d", dto.MaxImageN)
	}
	if options := request.SequentialImageGenerationOptions; options != nil && options.MaxImages != nil && *options.MaxImages > dto.MaxImageN {
		return nil, nil, fmt.Errorf("sequential_image_generation_options.max_images must not exceed %d", dto.MaxImageN)
	}
	if request.LayerDecomposition != nil && *request.LayerDecomposition {
		if model != dto.Seedream50Pro {
			return nil, nil, fmt.Errorf("layer_decomposition is only supported by %s", dto.Seedream50Pro)
		}
		if request.N != nil && *request.N > 1 {
			return nil, nil, fmt.Errorf("layer_decomposition requires n=1")
		}
		if request.Stream != nil && *request.Stream {
			return nil, nil, fmt.Errorf("streaming layer_decomposition is not supported")
		}
	}
	return raw, &request, nil
}
