package volcengine

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSeedreamNativeGenerationAndReferencePayloads(t *testing.T) {
	for _, model := range []string{dto.Seedream40, dto.Seedream45, dto.Seedream50, "seedream-4-0-250828"} {
		for _, image := range []string{"", `,"image":"https://example.test/reference.png"`, `,"image":["https://example.test/one.png","data:image/png;base64,aW1hZ2U="]`} {
			t.Run(model+image, func(t *testing.T) {
				raw := fmt.Sprintf(`{"model":%q,"prompt":"paint a garden","seed":0,"guidance_scale":0,"watermark":false,"stream":false,"sequential_image_generation":"auto","sequential_image_generation_options":{"max_images":2}%s}`, model, image)
				var request dto.ImageRequest
				require.NoError(t, common.Unmarshal([]byte(raw), &request))
				copied, err := common.DeepCopy(&request)
				require.NoError(t, err)
				converted, err := ConvertSeedreamImageRequest(*copied)
				require.NoError(t, err)
				actual, err := common.Marshal(converted)
				require.NoError(t, err)
				assert.JSONEq(t, raw, string(actual), "optional zero/false and references survive relay deepcopy")

			})
		}
	}
	var absent dto.ImageRequest
	require.NoError(t, common.Unmarshal([]byte(`{"model":"doubao-seedream-4-0-250828","prompt":"a garden"}`), &absent))
	converted, err := ConvertSeedreamImageRequest(absent)
	require.NoError(t, err)
	raw, err := common.Marshal(converted)
	require.NoError(t, err)
	for _, field := range []string{"seed", "guidance_scale", "watermark", "stream", "sequential_image_generation", "sequential_image_generation_options", "layer_decomposition"} {
		assert.False(t, gjson.GetBytes(raw, field).Exists(), field)
	}
}

func TestSeedreamNativeRejectsUnsupportedAndUnboundedRequests(t *testing.T) {
	for _, raw := range []string{
		`{"model":"doubao-seedream-5-0-pro-260628","prompt":"a garden"}`,
		`{"model":"doubao-seedream-4-0-250828","prompt":"a garden","layer_decomposition":true}`,
		`{"model":"doubao-seedream-4-5-251128","prompt":"a garden","n":18446744073686646784}`,
		`{"model":"doubao-seedream-5-0-260128","prompt":"a garden","sequential_image_generation_options":{"max_images":129}}`,
	} {
		t.Run(raw, func(t *testing.T) {
			var request dto.ImageRequest
			require.NoError(t, common.Unmarshal([]byte(raw), &request))
			_, err := ConvertSeedreamImageRequest(request)
			require.Error(t, err)
		})
	}
}
