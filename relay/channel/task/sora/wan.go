package sora

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func (a *TaskAdaptor) validateWan(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if !a.wanProtocol {
		return service.TaskErrorWrapperLocal(fmt.Errorf("selected channel has no Wan protocol profile"), "unsupported_channel_protocol", 400)
	}
	if c.GetHeader("X-DashScope-Async") != "enable" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("X-DashScope-Async must be enable"), "invalid_request", 400)
	}
	var req dto.WanRequest
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", 400)
	}
	if err := req.NormalizeAndValidate(); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", 400)
	}
	p := req.Parameters
	// Rolldek does not document these controls; never silently ignore a non-default instruction.
	if p.Audio != nil && !*p.Audio || p.Seed != nil && *p.Seed != -1 || p.PromptExtend != nil && !*p.PromptExtend || p.Watermark != nil && *p.Watermark {
		return service.TaskErrorWrapperLocal(fmt.Errorf("this channel has not verified audio=false, fixed seed, prompt_extend=false or watermark=true"), "unsupported_channel_capability", 400)
	}
	// Rolldek bills each distinct reference video URL once; probe once per URL
	// and bill whole seconds so its per-second rounding cannot exceed our charge.
	var videoSeconds, billedVideoSeconds, audioSeconds float64
	probedVideos := map[string]bool{}
	for _, m := range req.Input.Media {
		if m.Type == "file" || m.Type == "link" || strings.HasPrefix(m.URL, "oss:") {
			return service.TaskErrorWrapperLocal(fmt.Errorf("this channel has not verified file, link or private OSS media"), "unsupported_channel_capability", 400)
		}
		if m.Type != "reference_video" && m.Type != "reference_audio" {
			continue
		}
		if m.Type == "reference_video" {
			if probedVideos[m.URL] {
				continue
			}
			probedVideos[m.URL] = true
		}
		seconds, err := service.ProbeWanMediaDuration(c.Request.Context(), m.URL, m.Type == "reference_video")
		if err != nil {
			return service.TaskErrorWrapperLocal(fmt.Errorf("cannot validate %s duration: %w", m.Type, err), "invalid_media", 400)
		}
		if seconds < 1 || seconds > 15 {
			return service.TaskErrorWrapperLocal(fmt.Errorf("reference media duration must be 1–15 seconds"), "invalid_media", 400)
		}
		if m.Type == "reference_video" {
			videoSeconds += seconds
			billedVideoSeconds += math.Ceil(seconds)
		} else {
			audioSeconds += seconds
		}
	}
	if videoSeconds > 15 || audioSeconds > 15 || videoSeconds+float64(*p.Duration) > 30 {
		return service.TaskErrorWrapperLocal(fmt.Errorf("reference total must be <=15 seconds and input video + output <=30 seconds"), "invalid_media", 400)
	}
	c.Set("wan_request", &req)
	c.Set(relaycommon.TaskInputVideoSecondsKey, billedVideoSeconds)
	info.Action = constant.TaskActionGenerate
	return nil
}

func (a *TaskAdaptor) buildWan(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	req := c.MustGet("wan_request").(*dto.WanRequest)
	// Model aliases come exclusively from the public model + validated resolution, never caller metadata.
	info.UpstreamModelName = req.Model + "-" + strings.ToLower(req.Parameters.Resolution)
	body := map[string]any{"model": info.UpstreamModelName, "prompt": req.Input.Prompt, "seconds": strconv.Itoa(*req.Parameters.Duration), "size": req.Parameters.Resolution, "aspect_ratio": req.Parameters.Ratio, "watermark": false}
	if len(req.Input.Media) > 0 {
		body["input"] = map[string]any{"media": req.Input.Media}
	}
	data, err := common.Marshal(body)
	return bytes.NewReader(data), err
}

type wanResult struct {
	ID          string `json:"id"`
	TaskID      string `json:"task_id"`
	Status      string `json:"status"`
	VideoURL    string `json:"video_url"`
	DownloadURL string `json:"download_url"`
	ResultURL   string `json:"result_url"`
	Metadata    struct {
		URL string `json:"url"`
	} `json:"metadata"`
	Error struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error"`
	ErrorCode    string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
	Usage        struct {
		OutputVideoDuration float64 `json:"output_video_duration"`
	} `json:"usage"`
}

func parseWanResult(data []byte) (*relaycommon.TaskInfo, error) {
	var r wanResult
	if err := common.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	if r.Status == "" && (r.ErrorCode != "" || r.Error.Code != "" || r.Error.Message != "") {
		return nil, fmt.Errorf("Wan query error: %s %s", r.Error.Message, r.ErrorMessage)
	}
	result := &relaycommon.TaskInfo{TaskID: r.ID, OutputVideoDuration: r.Usage.OutputVideoDuration}
	if result.OutputVideoDuration < 0 || result.OutputVideoDuration > 30 {
		result.OutputVideoDuration = 0
	}
	if result.TaskID == "" {
		result.TaskID = r.TaskID
	}
	switch strings.ToLower(r.Status) {
	case "queued", "pending":
		result.Status = model.TaskStatusQueued
	case "processing", "in_progress", "running":
		result.Status = model.TaskStatusInProgress
	case "completed", "succeeded":
		result.Status = model.TaskStatusSuccess
	case "failed", "cancelled", "canceled":
		result.Status = model.TaskStatusFailure
	default:
		result.Status = model.TaskStatusUnknown
	}
	result.Reason = r.Error.Message
	if result.Reason == "" {
		result.Reason = r.ErrorMessage
	}
	for _, u := range []string{r.Metadata.URL, r.VideoURL, r.DownloadURL, r.ResultURL} {
		if u != "" {
			result.Url = u
			break
		}
	}
	if r.ErrorCode != "" || r.Error.Code != "" {
		result.Status = model.TaskStatusFailure
	}
	if result.Status == model.TaskStatusSuccess && result.Url == "" {
		return nil, fmt.Errorf("Wan completed without a video URL")
	}
	return result, nil
}

func (a *TaskAdaptor) wanSubmitResponse(c *gin.Context, data []byte, info *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	result, err := parseWanResult(data)
	if err != nil {
		return "", nil, service.TaskErrorWrapperLocal(err, "invalid_response", 502)
	}
	if result.TaskID == "" {
		return "", nil, service.TaskErrorWrapperLocal(fmt.Errorf("missing upstream task ID"), "invalid_response", 502)
	}
	// Even immediate failures with an ID are stored and refunded by the polling lifecycle.
	c.JSON(http.StatusOK, dto.WanResponse{Output: dto.WanOutput{TaskID: info.PublicTaskID, TaskStatus: "PENDING"}})
	return result.TaskID, data, nil
}
