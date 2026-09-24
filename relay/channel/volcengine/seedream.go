package volcengine

import (
	"fmt"

	"github.com/QuantumNous/new-api/relaykit/dto"
)

// ConvertSeedreamImageRequest preserves explicit false/zero and reference images.
// Native Seedream editing uses JSON image input on /api/v3/images/generations.
func ConvertSeedreamImageRequest(request dto.ImageRequest) (any, error) {
	if request.Model == dto.Seedream50Pro {
		return nil, fmt.Errorf("%s requires an OpenAI/NewAPI compatible image channel", request.Model)
	}
	if !dto.IsSeedreamModel(request.Model) {
		return request, nil
	}
	if request.N != nil && *request.N > dto.MaxImageN {
		return nil, fmt.Errorf("n must not exceed %d", dto.MaxImageN)
	}
	if options := request.SequentialImageGenerationOptions; options != nil && options.MaxImages != nil && *options.MaxImages > dto.MaxImageN {
		return nil, fmt.Errorf("sequential_image_generation_options.max_images must not exceed %d", dto.MaxImageN)
	}
	if request.LayerDecomposition != nil && *request.LayerDecomposition {
		return nil, fmt.Errorf("layer_decomposition is only supported by %s", dto.Seedream50Pro)
	}
	return request, nil
}
