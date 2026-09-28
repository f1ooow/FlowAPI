package openai

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedreamLayerResponseUsage(t *testing.T) {
	for _, count := range []int{1, 17} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("count=%d/json-as-stream=%t", count, stream), func(t *testing.T) {
				images := strings.TrimSuffix(strings.Repeat(`{"url":"https://example.test/image.png","b64_json":"aW1hZ2U="},`, count), ",")
				body := fmt.Sprintf(`{"created":1710000000,"data":[%s],"usage":{"generated_images":%d,"output_tokens":100,"total_tokens":100}}`, images, count)
				c, recorder, resp, info := newImageTestContext(t, body, "application/json", stream)
				info.UpstreamModelName = dto.Seedream50Pro
				info.Request = &dto.ImageRequest{Model: dto.Seedream50Pro, LayerDecomposition: common.GetPointer(true), N: common.GetPointer(uint(1))}
				info.PriceData.UsePrice = true
				info.PriceData.AddOtherRatio("n", 8.5)
				info.PriceData.AddOtherRatio("quality", 2)
				handler := OpenaiImageHandler
				if stream {
					handler = OpenaiImageStreamHandler
				}
				usage, apiErr := handler(c, info, resp)
				require.Nil(t, apiErr)
				require.NotNil(t, usage)
				assert.Equal(t, 100, usage.CompletionTokens)
				assert.Equal(t, float64(count)*0.5, info.PriceData.OtherRatios()["n"])
				assert.Equal(t, 2.0, info.PriceData.OtherRatios()["quality"])
				assert.Equal(t, count, c.GetInt("seedream_layer_image_count"))
				assert.Contains(t, recorder.Body.String(), `"url":"https://example.test/image.png"`)
				assert.Contains(t, recorder.Body.String(), `"b64_json":"aW1hZ2U="`)
				if !stream {
					assert.Equal(t, body, recorder.Body.String())
				}
			})
		}
	}
}

func TestSeedreamLayerBillsDeliveredImages(t *testing.T) {
	const image = `{"url":"https://example.test/image.png"}`
	const failed = `{"error":{"code":"OutputImageSensitiveContentDetected","message":"rejected"}}`
	eighteen := strings.TrimSuffix(strings.Repeat(image+",", 18), ",")
	for _, tc := range []struct {
		name, data, usage string
		wantImages        int
	}{
		{"error item billed by delivered images", image + "," + failed + "," + image, `,"usage":{"generated_images":3}`, 2},
		{"generated below delivered", image + "," + image + "," + image, `,"usage":{"generated_images":2}`, 2},
		{"generated above delivered", image, `,"usage":{"generated_images":2}`, 1},
		{"b64 delivered", `{"b64_json":"aW1hZ2U="}`, `,"usage":{"generated_images":1}`, 1},
		{"usage missing", image + "," + image, ``, 2},
		{"generated missing", image + "," + image, `,"usage":{}`, 2},
		{"generated null", image, `,"usage":{"generated_images":null}`, 1},
		{"generated zero", image, `,"usage":{"generated_images":0}`, 1},
		{"generated negative", image, `,"usage":{"generated_images":-1}`, 1},
		{"generated above bound", image, `,"usage":{"generated_images":18}`, 1},
		{"generated wrapped", image, `,"usage":{"generated_images":18446744073709551615}`, 1},
		{"generated string", image, `,"usage":{"generated_images":"1"}`, 1},
		{"generated fraction", image, `,"usage":{"generated_images":1.5}`, 1},
		{"delivered capped at bound", eighteen, `,"usage":{"generated_images":18}`, 17},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/json-as-stream=%t", tc.name, stream), func(t *testing.T) {
				body := `{"data":[` + tc.data + `]` + tc.usage + `}`
				c, recorder, resp, info := newImageTestContext(t, body, "application/json", stream)
				info.UpstreamModelName = dto.Seedream50Pro
				info.Request = &dto.ImageRequest{Model: dto.Seedream50Pro, LayerDecomposition: common.GetPointer(true)}
				info.PriceData.UsePrice = true
				info.PriceData.AddOtherRatio("n", 8.5)
				handler := OpenaiImageHandler
				if stream {
					handler = OpenaiImageStreamHandler
				}
				parsed, apiErr := handler(c, info, resp)
				require.Nil(t, apiErr)
				require.NotNil(t, parsed)
				if !stream {
					assert.Equal(t, body, recorder.Body.String(), "delivered response is written unchanged, including error items")
				}
				assert.Equal(t, float64(tc.wantImages)*dto.SeedreamLayerImagePriceFactor, info.PriceData.OtherRatios()["n"])
				assert.Equal(t, tc.wantImages, c.GetInt("seedream_layer_image_count"))
			})
		}
	}
}

func TestSeedreamLayerRejectsResponseWithoutDeliveredImage(t *testing.T) {
	for _, data := range []string{
		``,
		`{"error":{"code":"InternalServiceError","message":"failed"}}`,
		`{"url":"https://example.test/image.png","error":{"code":"InternalServiceError"}}`,
		`{"url":""}`,
		`{"revised_prompt":"no image"}`,
	} {
		t.Run(data, func(t *testing.T) {
			body := `{"data":[` + data + `],"usage":{"generated_images":1}}`
			c, recorder, resp, info := newImageTestContext(t, body, "application/json", false)
			info.UpstreamModelName = dto.Seedream50Pro
			info.Request = &dto.ImageRequest{Model: dto.Seedream50Pro, LayerDecomposition: common.GetPointer(true)}
			info.PriceData.UsePrice = true
			info.PriceData.AddOtherRatio("n", 8.5)
			parsed, apiErr := OpenaiImageHandler(c, info, resp)
			require.NotNil(t, apiErr)
			assert.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
			assert.True(t, types.IsSkipRetryError(apiErr), "an accepted paid generation must not be retried")
			assert.Nil(t, parsed)
			assert.Empty(t, recorder.Body.String())
			assert.Equal(t, 8.5, info.PriceData.OtherRatios()["n"], "failed response cannot become a zero or negative charge")
		})
	}
}

func TestSeedreamImageModeAndModelIsolation(t *testing.T) {
	for _, tc := range []struct {
		name, model string
		layer       *bool
		expression  bool
		want        float64
	}{
		{"native 4", dto.Seedream40, nil, false, 2},
		{"native 4.5", dto.Seedream45, nil, false, 2},
		{"native 5", dto.Seedream50, nil, false, 2},
		{"Pro absent", dto.Seedream50Pro, nil, false, 2},
		{"Pro false", dto.Seedream50Pro, common.GetPointer(false), false, 2},
		{"unrelated true", "gpt-image-1", common.GetPointer(true), false, 2},
		{"Gemini normal", "gemini-3-pro-image-preview", nil, false, 2},
		{"expression priority", dto.Seedream50Pro, common.GetPointer(true), true, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _, resp, info := newImageTestContext(t, `{"data":[{"url":"one"},{"b64_json":"two"}],"usage":{"generated_images":2,"input_tokens":5,"output_tokens":10,"total_tokens":15}}`, "application/json", false)
			info.UpstreamModelName = tc.model
			info.Request = &dto.ImageRequest{Model: tc.model, LayerDecomposition: tc.layer}
			info.PriceData.UsePrice = true
			info.PriceData.AddOtherRatio("n", 3)
			if tc.expression {
				info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ExprString: "200000"}
			}
			usage, apiErr := OpenaiImageHandler(c, info, resp)
			require.Nil(t, apiErr)
			assert.Equal(t, tc.want, info.PriceData.OtherRatios()["n"])
			assert.Equal(t, 5, usage.PromptTokens)
			assert.Equal(t, 10, usage.CompletionTokens)
		})
	}
}

func TestSeedreamFinalOutboundModeWins(t *testing.T) {
	c, _, resp, info := newImageTestContext(t, `{"data":[{"url":"one"}],"usage":{"generated_images":1}}`, "application/json", false)
	info.UpstreamModelName = dto.Seedream50Pro
	info.Request = &dto.ImageRequest{Model: dto.Seedream50Pro, LayerDecomposition: common.GetPointer(true)}
	c.Set("seedream_layer_decomposition", false)
	info.PriceData.UsePrice = true
	info.PriceData.AddOtherRatio("n", 8.5)
	_, apiErr := OpenaiImageHandler(c, info, resp)
	require.Nil(t, apiErr)
	assert.Equal(t, 1.0, info.PriceData.OtherRatios()["n"])
}

func TestSeedreamRejectsOversizedOutputAndUnexpectedLayerStream(t *testing.T) {
	t.Run("native output bound", func(t *testing.T) {
		images := strings.TrimSuffix(strings.Repeat(`{"url":"image"},`, dto.MaxImageN+1), ",")
		c, recorder, resp, info := newImageTestContext(t, `{"data":[`+images+`]}`, "application/json", false)
		info.UpstreamModelName = dto.Seedream40
		_, apiErr := OpenaiImageHandler(c, info, resp)
		require.NotNil(t, apiErr)
		assert.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
		assert.Empty(t, recorder.Body.String())
	})
	t.Run("unexpected layer stream", func(t *testing.T) {
		c, recorder, resp, info := newImageTestContext(t, "data: [DONE]\n\n", "text/event-stream", true)
		info.UpstreamModelName = dto.Seedream50Pro
		c.Set("seedream_layer_decomposition", true)
		_, apiErr := OpenaiImageStreamHandler(c, info, resp)
		require.NotNil(t, apiErr)
		assert.True(t, types.IsSkipRetryError(apiErr))
		assert.Empty(t, recorder.Body.String())
	})
}

func TestSeedreamLayerMalformedResponseCannotBeRetried(t *testing.T) {
	for _, body := range []string{`{"data":[`, `{"data":[{"url":"one"}],"usage":{"generated_images":1,"output_tokens":18446744073709551615}}`} {
		t.Run(body, func(t *testing.T) {
			c, recorder, resp, info := newImageTestContext(t, body, "application/json", false)
			c.Set("seedream_layer_decomposition", true)
			_, apiErr := OpenaiImageHandler(c, info, resp)
			require.NotNil(t, apiErr)
			assert.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
			assert.True(t, types.IsSkipRetryError(apiErr))
			assert.Empty(t, recorder.Body.String())
		})
	}
}

func TestSeedreamProCompatibleEndpoint(t *testing.T) {
	_, _, _, info := newImageTestContext(t, `{}`, "application/json", false)
	info.ChannelType = constant.ChannelTypeNewAPI
	info.ChannelBaseUrl = "https://newapi.example.test"
	info.UpstreamModelName = dto.Seedream50Pro
	info.RequestURLPath = "/v1/images/generations"
	endpoint, err := (&Adaptor{}).GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://newapi.example.test/v1/images/generations", endpoint)
}
