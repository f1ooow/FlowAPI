package helper

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// newMultipartImageEditContext builds a real /v1/images/edits multipart request
// and runs it through the production validator, so callers get exactly the
// context and DTO the relay path produces.
func newMultipartImageEditContext(t *testing.T, fields [][2]string) (*gin.Context, *dto.ImageRequest) {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, field := range fields {
		require.NoError(t, writer.WriteField(field[0], field[1]))
	}
	part, err := writer.CreateFormFile("image", "input.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("fake image bytes"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())

	request, err := GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
	require.NoError(t, err)
	return c, request
}

func TestResolveIncomingBillingExprRequestInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("Content-Type", "application/json")

	body := []byte(`{"service_tier":"fast"}`)
	ctx.Request.Body = io.NopCloser(bytes.NewReader(body))
	ctx.Set(common.KeyRequestBody, body)

	info := &relaycommon.RelayInfo{
		RequestHeaders: map[string]string{"Content-Type": "application/json"},
	}

	input, err := ResolveIncomingBillingExprRequestInput(ctx, info)
	require.NoError(t, err)
	require.Equal(t, body, input.Body)
	require.Equal(t, "application/json", input.Headers["Content-Type"])
}

func TestBuildBillingExprRequestInputFromRequest(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{
		Model:  "gemini-3.1-pro-preview",
		Stream: lo.ToPtr(true),
		Messages: []dto.Message{
			{
				Role:    "user",
				Content: "hi",
			},
		},
		MaxTokens: lo.ToPtr(uint(3000)),
	}

	input, err := BuildBillingExprRequestInputFromRequest(request, map[string]string{
		"Content-Type": "application/json",
		"X-Test":       "1",
	})
	require.NoError(t, err)
	require.Equal(t, "application/json", input.Headers["Content-Type"])
	require.Equal(t, "1", input.Headers["X-Test"])
	require.True(t, gjson.GetBytes(input.Body, "stream").Bool())
	require.Equal(t, "user", gjson.GetBytes(input.Body, "messages.0.role").String())
	require.Equal(t, float64(3000), gjson.GetBytes(input.Body, "max_tokens").Float())
}

// TestResolveIncomingBillingExprRequestInputProjectsMultipartImage covers the
// /v1/images/edits multipart path, where there is no JSON body for param() to
// read. The projected body must expose the same keys a JSON client sends, so a
// single billing expression tiers both paths identically, and must not carry the
// prompt or any image payload into the frozen billing input.
func TestResolveIncomingBillingExprRequestInputProjectsMultipartImage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// The multipart text "image" field can hold a base64 data URI, which is what
	// makes an unfiltered projection hundreds of KB large.
	largeImageField := "data:image/png;base64," + strings.Repeat("A", 200_000)

	tests := []struct {
		name   string
		nField string
		wantN  float64
	}{
		{name: "declared n is projected", nField: "4", wantN: 4},
		{name: "absent n is normalized to 1", nField: "", wantN: 1},
		{name: "zero n is normalized to 1", nField: "0", wantN: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields := [][2]string{
				{"model", "gpt-image-2.5-flare"},
				{"prompt", "make it brighter"},
				{"size", "2048x1152"},
				{"quality", "high"},
				{"stream", "false"},
				{"watermark", "true"},
				{"image", largeImageField},
			}
			if tt.nField != "" {
				fields = append(fields, [2]string{"n", tt.nField})
			}

			ctx, request := newMultipartImageEditContext(t, fields)
			contentType := ctx.Request.Header.Get("Content-Type")
			info := &relaycommon.RelayInfo{
				Request:        request,
				RequestHeaders: map[string]string{"Content-Type": contentType},
			}

			input, err := ResolveIncomingBillingExprRequestInput(ctx, info)
			require.NoError(t, err)
			require.NotEmpty(t, input.Body)

			assert.Equal(t, "gpt-image-2.5-flare", gjson.GetBytes(input.Body, "model").String())
			assert.Equal(t, "2048x1152", gjson.GetBytes(input.Body, "size").String())
			assert.Equal(t, "high", gjson.GetBytes(input.Body, "quality").String())
			assert.Equal(t, tt.wantN, gjson.GetBytes(input.Body, "n").Float())
			assert.False(t, gjson.GetBytes(input.Body, "stream").Bool())
			assert.True(t, gjson.GetBytes(input.Body, "watermark").Bool())

			assert.Empty(t, gjson.GetBytes(input.Body, "prompt").String())
			assert.False(t, gjson.GetBytes(input.Body, "image").Exists())
			assert.Less(t, len(input.Body), 512, "projected body must not carry image payloads")

			// The DTO the relay forwards upstream must be untouched by the projection.
			assert.Equal(t, "make it brighter", request.Prompt)
			assert.NotEmpty(t, request.Image)

			assert.Contains(t, input.Headers["Content-Type"], "multipart/form-data")
		})
	}
}

// TestResolveIncomingBillingExprRequestInputKeepsJSONBodyVerbatim guards that the
// multipart projection never shadows a real JSON body: expressions must keep
// seeing the client's raw payload, including an explicit n:0 that the DTO
// normalizes to 1 and fields the DTO does not model.
func TestResolveIncomingBillingExprRequestInputKeepsJSONBodyVerbatim(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-image-2","prompt":"a cat","size":"2048x1152","n":0,"unknown_field":"keep me"}`)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	request, err := GetAndValidOpenAIImageRequest(ctx, relayconstant.RelayModeImagesGenerations)
	require.NoError(t, err)
	require.NotNil(t, request.N)
	require.Equal(t, uint(1), *request.N)

	info := &relaycommon.RelayInfo{
		Request:        request,
		RequestHeaders: map[string]string{"Content-Type": "application/json"},
	}

	input, err := ResolveIncomingBillingExprRequestInput(ctx, info)
	require.NoError(t, err)
	assert.Equal(t, body, input.Body)
}

// TestResolveIncomingBillingExprRequestInputSkipsNonImageMultipart keeps audio and
// other multipart relay formats on their previous behaviour: no image DTO, no
// projection, so param() stays empty exactly as before.
func TestResolveIncomingBillingExprRequestInputSkipsNonImageMultipart(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const contentType = "multipart/form-data; boundary=xyz"

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", bytes.NewReader([]byte("ignored")))
	ctx.Request.Header.Set("Content-Type", contentType)

	info := &relaycommon.RelayInfo{
		Request:        &dto.AudioRequest{Model: "whisper-1"},
		RequestHeaders: map[string]string{"Content-Type": contentType},
	}

	input, err := ResolveIncomingBillingExprRequestInput(ctx, info)
	require.NoError(t, err)
	assert.Nil(t, input.Body)
}

// TestResolveIncomingBillingExprRequestInputReusesFrozenInput protects the
// pre-consume/settle invariant: once the billing body is frozen, a later resolve
// must return that same body (as a clone), never a freshly projected DTO.
func TestResolveIncomingBillingExprRequestInputReusesFrozenInput(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const contentType = "multipart/form-data; boundary=xyz"
	frozenBody := `{"model":"gpt-image-2.5-flare","n":1,"size":"1024x1024"}`

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
	ctx.Request.Header.Set("Content-Type", contentType)

	frozen := &billingexpr.RequestInput{Body: []byte(frozenBody)}
	info := &relaycommon.RelayInfo{
		Request:             &dto.ImageRequest{Model: "gpt-image-2.5-flare", Size: "4096x4096", N: lo.ToPtr(uint(8))},
		RequestHeaders:      map[string]string{"Content-Type": contentType},
		BillingRequestInput: frozen,
	}

	input, err := ResolveIncomingBillingExprRequestInput(ctx, info)
	require.NoError(t, err)
	require.Equal(t, frozenBody, string(input.Body))

	input.Body[0] = 'X'
	assert.Equal(t, frozenBody, string(frozen.Body))
}

// TestResolveIncomingBillingExprRequestInputProjectsFormURLEncodedImage keeps the
// urlencoded image path aligned with multipart: any non-JSON body that still
// parses into an image DTO gets the same projection.
func TestResolveIncomingBillingExprRequestInputProjectsFormURLEncodedImage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	form := url.Values{}
	form.Set("model", "gpt-image-2")
	form.Set("prompt", "a cat")
	form.Set("size", "2048x2048")
	form.Set("quality", "high")

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", strings.NewReader(form.Encode()))
	ctx.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	request, err := GetAndValidOpenAIImageRequest(ctx, relayconstant.RelayModeImagesEdits)
	require.NoError(t, err)

	info := &relaycommon.RelayInfo{
		Request:        request,
		RequestHeaders: map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
	}

	input, err := ResolveIncomingBillingExprRequestInput(ctx, info)
	require.NoError(t, err)
	assert.Equal(t, "2048x2048", gjson.GetBytes(input.Body, "size").String())
	assert.Equal(t, "high", gjson.GetBytes(input.Body, "quality").String())
	assert.Equal(t, float64(1), gjson.GetBytes(input.Body, "n").Float())
	assert.Empty(t, gjson.GetBytes(input.Body, "prompt").String())
}
