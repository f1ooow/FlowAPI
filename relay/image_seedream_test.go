package relay

import (
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type seedreamReservation struct {
	reserved int
	targets  []int
	err      error
}

func (r *seedreamReservation) Reserve(target int) error {
	r.targets = append(r.targets, target)
	if r.err != nil {
		return r.err
	}
	if target > r.reserved {
		r.reserved = target
	}
	return nil
}
func (r *seedreamReservation) GetPreConsumedQuota() int { return r.reserved }
func (*seedreamReservation) Settle(int) error           { return nil }
func (*seedreamReservation) Refund(*gin.Context)        {}
func (*seedreamReservation) NeedsRefund() bool          { return false }

func seedreamRequestContext(t *testing.T, raw string) (*gin.Context, *relaycommon.RelayInfo, *seedreamReservation) {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(raw))
	c.Request.Header.Set("Content-Type", "application/json")
	t.Cleanup(func() { common.CleanupBodyStorage(c) })
	request, err := helper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesGenerations)
	require.NoError(t, err)
	billing := &seedreamReservation{reserved: 120000}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: request.Model}, Request: request, RelayMode: relayconstant.RelayModeImagesGenerations, Billing: billing}
	info.PriceData.UsePrice = true
	info.PriceData.ModelPrice = .1
	info.PriceData.GroupRatioInfo.GroupRatio = 2.4
	return c, info, billing
}

func TestSeedreamProPayloadOmissionAndExplicitValues(t *testing.T) {
	for _, model := range []string{dto.Seedream40, dto.Seedream45, dto.Seedream50, dto.Seedream50Pro} {
		t.Run(model, func(t *testing.T) {
			raw := fmt.Sprintf(`{"model":%q,"prompt":"a garden","seed":0,"guidance_scale":0,"watermark":false,"stream":false,"layer_decomposition":false,"image":["https://example.test/one.png","https://example.test/two.png"],"sequential_image_generation":"auto","sequential_image_generation_options":{"max_images":0},"tools":[{"type":"web_search"}],"unknown":9007199254740993}`, model)
			actual, request, err := normalizeSeedreamImagePayload([]byte(raw))
			require.NoError(t, err)
			assert.False(t, request.IsSeedreamLayerDecomposition())
			for _, field := range []string{"seed", "guidance_scale", "watermark", "stream", "layer_decomposition", "image", "unknown"} {
				assert.Equal(t, gjson.Get(raw, field).Raw, gjson.GetBytes(actual, field).Raw, field)
			}
			for _, field := range []string{"sequential_image_generation", "sequential_image_generation_options", "tools"} {
				assert.Equal(t, model != dto.Seedream50Pro, gjson.GetBytes(actual, field).Exists(), field)
			}
		})
	}
	raw, _, err := normalizeSeedreamImagePayload([]byte(`{"model":"doubao-seedream-5-0-pro-260628","prompt":"a garden"}`))
	require.NoError(t, err)
	for _, field := range []string{"seed", "guidance_scale", "watermark", "stream", "layer_decomposition"} {
		assert.False(t, gjson.GetBytes(raw, field).Exists(), field)
	}
}

func TestSeedreamLayerReservationAndModePriority(t *testing.T) {
	raw := `{"model":"doubao-seedream-5-0-pro-260628","prompt":"separate the layers","layer_decomposition":true,"n":1,"sequential_image_generation":"auto","sequential_image_generation_options":{"max_images":10}}`
	c, info, billing := seedreamRequestContext(t, raw)
	assert.Equal(t, 8.5, info.Request.GetTokenCountMeta().BillingRatios["n"])
	info.PriceData.AddOtherRatio("quality", 2)
	body, apiErr := prepareSeedreamImageRequest(c, info, strings.NewReader(raw))
	require.Nil(t, apiErr)
	actual, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.False(t, gjson.GetBytes(actual, "sequential_image_generation").Exists())
	assert.False(t, gjson.GetBytes(actual, "sequential_image_generation_options").Exists())
	assert.Equal(t, []int{2040000}, billing.targets, ".1 * 500000 * 2.4 * 8.5 * 2")
	assert.Equal(t, 2040000, info.FinalPreConsumedQuota)
	assert.Equal(t, 8.5, info.PriceData.OtherRatios()["n"])
	assert.True(t, c.GetBool("seedream_layer_decomposition"))
	// Route changes reuse this output bound and retain the session's maximum reservation.
	info.PriceData.GroupRatioInfo.GroupRatio = 1.6
	_, apiErr = prepareSeedreamImageRequest(c, info, strings.NewReader(raw))
	require.Nil(t, apiErr)
	assert.Equal(t, 2040000, info.FinalPreConsumedQuota)
	assert.Equal(t, []int{2040000, 1360000}, billing.targets)

	t.Run("expression owns quantity", func(t *testing.T) {
		c, info, billing := seedreamRequestContext(t, raw)
		info.PriceData.UsePrice = false
		info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ExprString: "300000", GroupRatio: 2.4}
		_, apiErr := prepareSeedreamImageRequest(c, info, strings.NewReader(raw))
		require.Nil(t, apiErr)
		assert.Empty(t, billing.targets)
		assert.Empty(t, info.PriceData.OtherRatios())
	})
	t.Run("mapped alias reserves before send", func(t *testing.T) {
		c, info, billing := seedreamRequestContext(t, raw)
		info.Request.(*dto.ImageRequest).Model = "client-image-alias"
		_, apiErr := prepareSeedreamImageRequest(c, info, strings.NewReader(raw))
		require.Nil(t, apiErr)
		assert.Equal(t, []int{1020000}, billing.targets)
	})
	t.Run("override enables layers", func(t *testing.T) {
		c, info, billing := seedreamRequestContext(t, strings.Replace(raw, `"layer_decomposition":true`, `"layer_decomposition":false`, 1))
		_, apiErr := prepareSeedreamImageRequest(c, info, strings.NewReader(raw))
		require.Nil(t, apiErr)
		assert.Equal(t, []int{1020000}, billing.targets)
	})
}

func TestSeedreamInvalidRequestsCannotReachUpstream(t *testing.T) {
	for _, extra := range []string{`,"layer_decomposition":"true"`, `,"layer_decomposition":1`, `,"n":18446744073686646784`, `,"layer_decomposition":true,"n":2`, `,"layer_decomposition":true,"stream":true`} {
		t.Run(extra, func(t *testing.T) {
			raw := `{"model":"doubao-seedream-5-0-pro-260628","prompt":"layers"` + extra + `}`
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(raw))
			c.Request.Header.Set("Content-Type", "application/json")
			info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesGenerations}
			_, apiErr := prepareSeedreamImageRequest(c, info, strings.NewReader(raw))
			require.NotNil(t, apiErr)
			assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
			assert.True(t, types.IsSkipRetryError(apiErr))
		})
	}
	for _, model := range []string{dto.Seedream40, dto.Seedream45, dto.Seedream50} {
		_, _, err := normalizeSeedreamImagePayload([]byte(fmt.Sprintf(`{"model":%q,"prompt":"a garden","sequential_image_generation_options":{"max_images":129}}`, model)))
		require.Error(t, err)
		_, _, err = normalizeSeedreamImagePayload([]byte(fmt.Sprintf(`{"model":%q,"prompt":"a garden","layer_decomposition":true}`, model)))
		require.Error(t, err)
	}
}

func TestSeedreamReservationErrorsAndUnsupportedEndpoint(t *testing.T) {
	raw := `{"model":"doubao-seedream-5-0-pro-260628","prompt":"layers","layer_decomposition":true}`
	t.Run("insufficient funds", func(t *testing.T) {
		c, info, billing := seedreamRequestContext(t, raw)
		billing.err = errors.New("insufficient quota")
		body, apiErr := prepareSeedreamImageRequest(c, info, strings.NewReader(raw))
		require.NotNil(t, apiErr)
		assert.Nil(t, body)
		assert.True(t, types.IsSkipRetryError(apiErr))
		assert.Equal(t, 120000, billing.reserved)
	})
	t.Run("quota overflow", func(t *testing.T) {
		c, info, billing := seedreamRequestContext(t, raw)
		info.PriceData.ModelPrice = math.MaxFloat64
		body, apiErr := prepareSeedreamImageRequest(c, info, strings.NewReader(raw))
		require.NotNil(t, apiErr)
		assert.Nil(t, body)
		assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
		assert.Empty(t, billing.targets)
	})
	for _, contentType := range []string{"application/json", "multipart/form-data; boundary=example"} {
		t.Run("edits "+contentType, func(t *testing.T) {
			c, info, billing := seedreamRequestContext(t, raw)
			c.Request.Header.Set("Content-Type", contentType)
			info.RelayMode = relayconstant.RelayModeImagesEdits
			body, apiErr := prepareSeedreamImageRequest(c, info, strings.NewReader(raw))
			require.NotNil(t, apiErr)
			assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
			assert.Nil(t, body)
			assert.Empty(t, billing.targets)
		})
	}
}

func TestSeedreamNormalSequentialReservation(t *testing.T) {
	for _, tc := range []struct {
		mode string
		want int
	}{{"auto", 480000}, {"disabled", 120000}} {
		t.Run(tc.mode, func(t *testing.T) {
			raw := fmt.Sprintf(`{"model":"doubao-seedream-4-5-251128","prompt":"a garden","n":1,"sequential_image_generation":%q,"sequential_image_generation_options":{"max_images":4}}`, tc.mode)
			c, info, billing := seedreamRequestContext(t, raw)
			_, apiErr := prepareSeedreamImageRequest(c, info, strings.NewReader(raw))
			require.Nil(t, apiErr)
			assert.Equal(t, []int{tc.want}, billing.targets)
		})
	}
}

func TestImageHelperForwardsToolsOnlyToSeedream(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	previous := settings.PassThroughRequestEnabled
	settings.PassThroughRequestEnabled = false
	t.Cleanup(func() { settings.PassThroughRequestEnabled = previous })

	for _, tc := range []struct {
		model     string
		wantTools bool
	}{
		{model: "gpt-image-1", wantTools: false},
		{model: dto.Seedream50, wantTools: true},
	} {
		t.Run(tc.model, func(t *testing.T) {
			raw := fmt.Sprintf(`{"model":%q,"prompt":"a garden","tools":[{"type":"web_search"}]}`, tc.model)
			received := make(chan []byte, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				received <- body
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, `{"error":{"message":"captured upstream request","type":"invalid_request_error"}}`)
			}))
			defer upstream.Close()
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(raw))
			ctx.Request.Header.Set("Content-Type", "application/json")
			defer common.CleanupBodyStorage(ctx)
			common.SetContextKey(ctx, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
			common.SetContextKey(ctx, constant.ContextKeyChannelBaseUrl, upstream.URL)
			common.SetContextKey(ctx, constant.ContextKeyChannelKey, "test-key")
			common.SetContextKey(ctx, constant.ContextKeyOriginalModel, tc.model)
			common.SetContextKey(ctx, constant.ContextKeyChannelSetting, dto.ChannelSettings{})
			var request dto.ImageRequest
			require.NoError(t, common.Unmarshal([]byte(raw), &request))
			info := &relaycommon.RelayInfo{OriginModelName: tc.model, Request: &request,
				RequestURLPath: "/v1/images/generations", RelayMode: relayconstant.RelayModeImagesGenerations}

			apiErr := ImageHelper(ctx, info)
			require.NotNil(t, apiErr)
			require.Len(t, received, 1, "handler must have reached the upstream")
			actual := <-received
			assert.Equal(t, tc.wantTools, gjson.GetBytes(actual, "tools").Exists())
			if tc.wantTools {
				assert.JSONEq(t, `[{"type":"web_search"}]`, gjson.GetBytes(actual, "tools").Raw)
			}
			assert.JSONEq(t, `[{"type":"web_search"}]`, string(request.Tools), "inbound request stays intact for retries")
		})
	}
}
