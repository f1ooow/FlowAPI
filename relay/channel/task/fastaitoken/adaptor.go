package fastaitoken

import (
	"bytes"
	"fmt"
	relaykitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/doubao"

	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

// TaskAdaptor 对接 fastaitoken 的异步视频任务接口。
//
// 与 sora / doubao 适配器的区别（也是必须单独实现的原因）：
//   - sora 适配器把请求体原样透传，fastaitoken 只认顶层 resolution/ratio/duration，
//     画布把它们放在 metadata 和 seconds/size 里，必须摊平；
//   - sora 适配器成功时故意不回填 result_url（让客户端走 newapi 的 /content 代理），
//     画布读的是 result_url，这里必须回填上游的 video_url；
//   - doubao 适配器写死火山 Ark 的 /api/v3/contents/generations/tasks 路径，
//     fastaitoken 上不存在该路由。
type TaskAdaptor struct {
	ChannelType   int
	apiKey        string
	baseURL       string
	upstreamModel string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
	a.upstreamModel = info.UpstreamModelName
}

// ValidateRequestAndSetAction 复用通用任务校验。FastAI 的 time/token Seedance
// 模型都支持文档定义的图片、视频和音频素材，具体能力交给上游校验。
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	if taskErr = relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate); taskErr != nil {
		return taskErr
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err == nil {
		_, err = buildPayload(&req)
	}
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	return nil
}

// EstimateBilling chooses the unit from the mapped upstream model. Customer price lookup remains on OriginModelName.
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}
	upstream, unit, ok := ResolveModel(info.UpstreamModelName)
	if !ok {
		return nil
	}
	info.UpstreamModelName = upstream
	a.upstreamModel = upstream
	c.Set("task_billing_unit", unit)
	payload, err := buildPayload(&req)
	if err != nil {
		return nil
	}
	if unit == "seconds" {
		return map[string]float64{"seconds": float64(payload.Duration)}
	}
	ratio, ok := doubao.GetVideoInputRatio(info.OriginModelName, payload.Resolution, len(payload.Videos) > 0)
	if ok && ratio != 1 {
		return map[string]float64{"video_input": ratio}
	}
	return nil
}

func (a *TaskAdaptor) AdjustBillingOnSubmit(_ *relaycommon.RelayInfo, _ []byte) map[string]float64 {
	return nil
}

func (a *TaskAdaptor) AdjustBillingOnComplete(_ *model.Task, _ *relaycommon.TaskInfo) int {
	return 0
}

func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return a.baseURL + SubmitEndpoint, nil
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

// BuildRequestBody 把画布的嵌套请求摊平成 fastaitoken 的顶层参数。
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}
	payload, err := buildPayload(&req)
	if err != nil {
		return nil, errors.Wrap(err, "convert request payload failed")
	}
	modelName := req.Model
	if info.IsModelMapped {
		modelName = info.UpstreamModelName
	}
	upstream, unit, ok := ResolveModel(modelName)
	if !ok {
		return nil, fmt.Errorf("unsupported FastAI video model: %s", modelName)
	}
	if unit == "seconds" && !info.PriceData.UsePrice {
		return nil, fmt.Errorf("FastAI time models require an explicit per-second ModelPrice for the public model")
	}
	payload.Model = upstream
	info.UpstreamModelName = upstream
	a.upstreamModel = upstream
	data, err := common.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse 解析提交响应（HTTP 202，{"id":...,"status":"processing"}），
// 并按 doubao 渠道相同的形态把公开 task id 回给客户端。
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	var dResp fetchResponse
	if err := common.Unmarshal(responseBody, &dResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	if dResp.Error != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("FastAI submit failed: %s", dResp.failReason()), "upstream_error", http.StatusBadGateway)
	}
	upstreamID := dResp.ID
	if upstreamID == "" {
		upstreamID = dResp.TaskID
	}
	if upstreamID == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	ov := relaykitdto.NewOpenAIVideo()
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName

	c.JSON(http.StatusOK, ov)
	return upstreamID, responseBody, nil
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	if taskID == "" {
		return nil, fmt.Errorf("invalid task_id")
	}
	if modelName, ok := body["model"].(string); ok {
		a.upstreamModel = modelName
	}
	uri := strings.TrimRight(baseUrl, "/") + fmt.Sprintf(FetchEndpoint, url.PathEscape(taskID))
	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

// ParseTaskResult 映射状态词并回填视频直链。
// 上游成功态源仓库样例是 completed，视频地址在顶层 video_url —— 不回填 Url 的话
// 画布读的 result_url 永远为空。
func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	resTask := fetchResponse{}
	if err := common.Unmarshal(respBody, &resTask); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	if resTask.Status == "" && resTask.Error != nil {
		return nil, fmt.Errorf("FastAI query error: %s", resTask.failReason())
	}
	taskResult := relaycommon.TaskInfo{Code: 0, TaskID: resTask.ID}
	if taskResult.TaskID == "" {
		taskResult.TaskID = resTask.TaskID
	}

	switch strings.ToLower(strings.TrimSpace(resTask.Status)) {
	case StatusQueued, StatusPending:
		taskResult.Status = model.TaskStatusQueued
		taskResult.Progress = taskcommon.ProgressQueued
	case StatusProcessing, StatusRunning, StatusInProgress:
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = taskcommon.ProgressInProgress
	case StatusCompleted, StatusSucceeded:
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = taskcommon.ProgressComplete
		taskResult.Url = resTask.videoURL()
		taskResult.CompletionTokens = resTask.Usage.CompletionTokens
		taskResult.TotalTokens = resTask.Usage.TotalTokens
		if _, unit, ok := ResolveModel(a.upstreamModel); ok && unit == "seconds" {
			taskResult.CompletionTokens = 0
			taskResult.TotalTokens = 0
		}
		taskResult.OutputVideoDuration = resTask.Usage.OutputVideoDuration
		if taskResult.OutputVideoDuration < 0 || taskResult.OutputVideoDuration > relaycommon.MaxTaskDurationSeconds {
			taskResult.OutputVideoDuration = 0
		}
		if taskResult.Url == "" {
			return nil, fmt.Errorf("FastAI completed without a video URL")
		}
	case StatusFailed, StatusExpired, StatusCancelled:
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = taskcommon.ProgressComplete
		taskResult.Reason = resTask.failReason()
	default:
		taskResult.Status = model.TaskStatusUnknown
		taskResult.Progress = taskcommon.ProgressInProgress
	}
	if resTask.Progress > 0 && resTask.Progress < 100 {
		taskResult.Progress = fmt.Sprintf("%d%%", resTask.Progress)
	}

	return &taskResult, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var dResp fetchResponse
	if err := common.Unmarshal(originTask.Data, &dResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal fastaitoken task data failed")
	}

	openAIVideo := relaykitdto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.TaskID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)
	openAIVideo.SetMetadata("url", dResp.videoURL())
	openAIVideo.CreatedAt = originTask.CreatedAt
	openAIVideo.CompletedAt = originTask.UpdatedAt
	openAIVideo.Model = originTask.Properties.OriginModelName

	if originTask.Status == model.TaskStatusFailure {
		openAIVideo.Error = &relaykitdto.OpenAIVideoError{
			Message: originTask.FailReason,
		}
	}

	return common.Marshal(openAIVideo)
}

// ---------------------------------------------------------------------------
// 请求映射
// ---------------------------------------------------------------------------

func parseMetadata(req *relaycommon.TaskSubmitReq) (*taskMetadata, error) {
	meta := &taskMetadata{}
	if err := taskcommon.UnmarshalMetadata(req.Metadata, meta); err != nil {
		return nil, err
	}
	return meta, nil
}

// resolveDuration 取时长：h3 driver 发顶层 duration（int），seedance driver 发顶层 seconds（string）。
func resolveDuration(req *relaycommon.TaskSubmitReq) int {
	if req.Duration > 0 {
		return req.Duration
	}
	if sec, err := strconv.Atoi(strings.TrimSpace(req.Seconds)); err == nil && sec > 0 {
		return sec
	}
	return 0
}

// resolveResolution 取分辨率：seedance driver 放 metadata.resolution，h3 driver 放顶层 size。
// fastaitoken 只认小写（源仓库样例 768P 不生效、768p 生效）。
func resolveResolution(req *relaycommon.TaskSubmitReq, meta *taskMetadata) string {
	value := strings.TrimSpace(meta.Resolution)
	if value == "" {
		value = strings.TrimSpace(req.Size)
	}
	return strings.ToLower(value)
}

func buildPayload(req *relaycommon.TaskSubmitReq) (*requestPayload, error) {
	meta, err := parseMetadata(req)
	if err != nil {
		return nil, err
	}
	images, videos, audio := collectMedia(req, meta)
	if req.Duration < 0 || req.Duration > relaycommon.MaxTaskDurationSeconds {
		return nil, fmt.Errorf("invalid duration")
	}
	if req.Seconds != "" {
		seconds, err := strconv.Atoi(req.Seconds)
		if err != nil || seconds < 1 || seconds > relaycommon.MaxTaskDurationSeconds {
			return nil, fmt.Errorf("invalid seconds")
		}
	}
	if len(images) > 9 || len(videos) > 3 || len(audio) > 3 {
		return nil, fmt.Errorf("too many reference media inputs")
	}
	payload := &requestPayload{
		Model:      req.Model,
		Prompt:     strings.TrimSpace(req.Prompt),
		Mode:       resolveMode(req, meta, images, videos, audio),
		Resolution: resolveResolution(req, meta),
		Ratio:      strings.TrimSpace(meta.Ratio),
		Duration:   resolveDuration(req),
		Images:     images,
		Videos:     videos,
		Audio:      audio,
		Seed:       meta.Seed,
	}
	if payload.Duration == 0 {
		payload.Duration = 5
	}
	if meta.GenerateAudio != nil {
		payload.GenerateAudio = meta.GenerateAudio
	}
	if meta.Watermark != nil {
		payload.Watermark = meta.Watermark
	} else if meta.AigcWatermark != nil {
		payload.Watermark = meta.AigcWatermark
	}
	return payload, nil
}

func collectMedia(req *relaycommon.TaskSubmitReq, meta *taskMetadata) (images, videos, audio []string) {
	appendUnique := func(target *[]string, values ...string) {
		for _, value := range values {
			value = strings.TrimSpace(value)
			alreadyAdded := false
			for _, existing := range *target {
				if existing == value {
					alreadyAdded = true
					break
				}
			}
			if value == "" || alreadyAdded {
				continue
			}
			*target = append(*target, value)
		}
	}

	appendUnique(&images, req.Images...)
	appendUnique(&images, req.Image, req.InputReference, meta.FirstFrameImage, meta.LastFrameImage)
	appendUnique(&images, meta.ReferenceImages...)
	appendUnique(&videos, meta.ReferenceVideos...)
	appendUnique(&audio, meta.ReferenceAudios...)
	for _, item := range meta.Content {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		itemType, _ := itemMap["type"].(string)
		url := contentMediaURL(itemMap, itemType)
		switch strings.ToLower(itemType) {
		case "image_url":
			appendUnique(&images, url)
		case "video_url":
			appendUnique(&videos, url)
		case "audio_url":
			appendUnique(&audio, url)
		}
	}
	return
}

func contentMediaURL(item map[string]any, itemType string) string {
	value, ok := item[strings.ToLower(itemType)]
	if !ok {
		return ""
	}
	if url, ok := value.(string); ok {
		return url
	}
	if object, ok := value.(map[string]any); ok {
		url, _ := object["url"].(string)
		return url
	}
	return ""
}

func resolveMode(req *relaycommon.TaskSubmitReq, meta *taskMetadata, images, videos, audio []string) string {
	if mode := strings.TrimSpace(req.Mode); mode != "" {
		switch strings.ToLower(mode) {
		case "text", "t2v", "text_to_video":
			return "text_to_video"
		case "frames", "first_last_frame":
			return "first_last_frame"
		case "images", "i2v", "image_to_video":
			return "image_to_video"
		case "all_ref", "omni":
			return "omni"
		default:
			return mode
		}
	}
	if len(videos) > 0 || len(audio) > 0 {
		return "omni"
	}
	if hasFrameContent(meta) {
		return "first_last_frame"
	}
	if len(images) == 1 {
		return "image_to_video"
	}
	if len(images) > 1 {
		return "image_to_video"
	}
	return "text_to_video"
}

func hasFrameContent(meta *taskMetadata) bool {
	if strings.TrimSpace(meta.FirstFrameImage) != "" || strings.TrimSpace(meta.LastFrameImage) != "" {
		return true
	}
	for _, item := range meta.Content {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role, _ := itemMap["role"].(string)
		if role == "first_frame" || role == "last_frame" {
			return true
		}
	}
	return false
}
