package hailuo

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relaykitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestH3ContextIRWireAndPerRequestBilling(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("task_request", relaycommon.TaskSubmitReq{Model: ModelMiniMaxH3ContextIR, Prompt: "original", Duration: 15, Size: "2K", Metadata: map[string]any{
		"reference_images": []string{"https://cdn.example/1", "https://cdn.example/2", "https://cdn.example/3", "https://cdn.example/4", "https://cdn.example/5", "https://cdn.example/6"},
		"reference_videos": []string{"https://cdn.example/video"}, "ratio": "16:9",
	}})
	info := &relaycommon.RelayInfo{OriginModelName: ModelMiniMaxH3ContextIR, ChannelMeta: &relaycommon.ChannelMeta{
		UpstreamModelName: ModelMiniMaxH3, ChannelBaseUrl: "https://provider.example/minimax",
	}}
	a := &TaskAdaptor{}
	a.Init(info)
	uri, err := a.BuildRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://provider.example/minimax/v2/h3_context_ir", uri)
	body, err := a.BuildRequestBody(c, info)
	require.NoError(t, err)
	raw, err := io.ReadAll(body)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(raw, &payload))
	assert.Equal(t, ModelMiniMaxH3, payload["model"])
	assert.Equal(t, float64(15), payload["duration"])
	assert.Len(t, payload["content"], 8)
	assert.NotContains(t, payload, "resolution")
	assert.NotContains(t, payload, "aigc_watermark")
	assert.Empty(t, a.EstimateBilling(c, info))
	assert.Zero(t, info.PriceData.TaskInputImageCount)
	assert.Equal(t, "request", c.GetString("task_billing_unit"))
	queryURL, err := a.buildFetchURL(info.ChannelBaseUrl, "ir-task", ModelMiniMaxH3ContextIR)
	require.NoError(t, err)
	assert.Equal(t, "https://provider.example/minimax/v2/query/video_generation/ir-task", queryURL)
}

func TestH3ContextIRPromptResult(t *testing.T) {
	raw := []byte(`{"task":{"id":"ir-task","status":"succeeded","task_type":"h3_context_ir","content":{"prompt":"enhanced prompt"},"usage":{"total_tokens":9090}}}`)
	a := &TaskAdaptor{}
	result, err := a.ParseTaskResult(raw)
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, result.Status)
	assert.Equal(t, relaycommon.TaskResultTypeText, result.ResultType)
	assert.Empty(t, result.Url)
	assert.Zero(t, result.TotalTokens, "IR is charged per request, not per token")
	task := &model.Task{TaskID: "public-task", Status: model.TaskStatusSuccess, Data: raw,
		PrivateData: model.TaskPrivateData{ResultURL: "https://flow.example/v1/videos/public-task/content"},
		Properties:  model.Properties{OriginModelName: ModelMiniMaxH3ContextIR, UpstreamModelName: ModelMiniMaxH3ContextIR}}
	response, err := a.ConvertToOpenAIVideo(task)
	require.NoError(t, err)
	var video relaykitdto.OpenAIVideo
	require.NoError(t, common.Unmarshal(response, &video))
	assert.Equal(t, "enhanced prompt", video.Metadata["prompt"])
	assert.Equal(t, "h3_context_ir", video.Metadata["task_type"])
	assert.NotContains(t, video.Metadata, "url", "text results must not expose a stored video proxy URL")
	_, err = a.ParseTaskResult([]byte(`{"task":{"status":"succeeded","task_type":"h3_context_ir","content":{}}}`))
	require.ErrorContains(t, err, "without a prompt")
}

func TestH3ImageBillingUsesForwardedContent(t *testing.T) {
	for _, count := range []int{5, 6, 9} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		images := make([]string, count)
		for i := range images {
			images[i] = "https://cdn.example/image"
		}
		c.Set("task_request", relaycommon.TaskSubmitReq{Prompt: "video", Duration: 5, Size: "2K", Metadata: map[string]any{
			"reference_images": images, "input_image_count": 0, "input_image_price": 0,
		}})
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: ModelMiniMaxH3}, PriceData: types.PriceData{GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1.2}}}
		ratios := (&TaskAdaptor{}).EstimateBilling(c, info)
		assert.Equal(t, 5.0, ratios["hailuo-h3-duration"])
		assert.Equal(t, max(0, count-5), info.PriceData.TaskInputImageCount)
		if count > 5 {
			assert.Equal(t, 0.0625, info.PriceData.TaskInputImagePrice)
		}
	}
}

func TestH3PollPreservesBillingMetersForDownstream(t *testing.T) {
	task := &model.Task{TaskID: "public-task", Status: model.TaskStatusSuccess,
		Data:       []byte(`{"task":{"status":"succeeded","content":{"url":"https://cdn.example/video.mp4"},"usage":{"input_seconds":0,"output_seconds":5,"total_seconds":5,"input_image_count":0}}}`),
		Properties: model.Properties{OriginModelName: ModelMiniMaxH3, UpstreamModelName: ModelMiniMaxH3}}
	response, err := (&TaskAdaptor{}).ConvertToOpenAIVideo(task)
	require.NoError(t, err)
	var video relaykitdto.OpenAIVideo
	require.NoError(t, common.Unmarshal(response, &video))
	assert.Equal(t, map[string]any{"input_seconds": float64(0), "output_seconds": float64(5), "total_seconds": float64(5), "input_image_count": float64(0)}, video.Metadata["usage"])
}

func TestH3BillingSnapshotPreservesAdditivePrice(t *testing.T) {
	info := &relaycommon.RelayInfo{OriginModelName: ModelMiniMaxH3, TaskRelayInfo: &relaycommon.TaskRelayInfo{BillingUnit: H3MediaBillingUnit},
		PriceData: types.PriceData{ModelPrice: 0.1125, UsePrice: true, TaskInputImageCount: 4, TaskInputImagePrice: 0.0625,
			GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1.2}}}
	snapshot := model.NewTaskBillingContext(info)
	encoded, err := common.Marshal(snapshot)
	require.NoError(t, err)
	var stored model.TaskBillingContext
	require.NoError(t, common.Unmarshal(encoded, &stored))
	info.PriceData.TaskInputImagePrice = 99
	info.PriceData.GroupRatioInfo.GroupRatio = 99
	assert.Equal(t, 0.1125, stored.ModelPrice)
	assert.Equal(t, 0.0625, stored.InputImagePrice)
	assert.Equal(t, 4, stored.InputImageCount)
	require.NotNil(t, stored.EffectiveRatio)
	assert.Equal(t, 1.2, *stored.EffectiveRatio)
	assert.True(t, stored.PerCallBilling)
}
