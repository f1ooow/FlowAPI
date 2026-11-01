package sora

import (
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

func (a *TaskAdaptor) validateKling4(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if c.Request.URL.Path != "/v1/videos" || info.Action == constant.TaskActionRemix {
		return service.TaskErrorWrapperLocal(fmt.Errorf("Kling 4 requires POST /v1/videos; remix is not supported"), "invalid_request", 400)
	}
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("Kling 4 requires application/json"), "invalid_request", 400)
	}
	var fields map[string]json.RawMessage
	if err := common.UnmarshalBodyReusable(c, &fields); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", 400)
	}
	// Rebuild from this allowlist so alternate duration, count or media fields
	// cannot make upstream generation differ from the validated billing inputs.
	for key := range fields {
		switch key {
		case "model", "prompt", "duration", "resolution", "aspect_ratio", "images", "videos", "audios", "n", "metadata", "image", "input_reference", "ratio":
		default:
			return service.TaskErrorWrapperLocal(fmt.Errorf("unsupported Kling 4 field: %s", key), "invalid_request", 400)
		}
	}
	var req dto.Kling4Request
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", 400)
	}
	if err := req.NormalizeAndValidate(); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", 400)
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 90*time.Second)
	defer cancel()
	inputSeconds := decimal.Zero
	for _, media := range []struct {
		kind string
		urls []string
	}{{"image", req.Images}, {"video", req.Videos}, {"audio", req.Audios}} {
		for _, url := range media.urls {
			seconds, err := service.ProbeKling4Media(ctx, url, media.kind)
			if err != nil {
				return service.TaskErrorWrapperLocal(fmt.Errorf("invalid reference %s: %w", media.kind, err), "invalid_media", 400)
			}
			if media.kind == "video" {
				inputSeconds = inputSeconds.Add(decimal.NewFromFloat(seconds))
			}
		}
	}
	// Each entry counts, including repeated URLs. Preserve fractional seconds;
	// rounding belongs to the final quota conversion, not individual media files.
	billedSeconds := inputSeconds.Add(decimal.NewFromInt(int64(*req.Duration)))
	c.Set("kling4_request", &req)
	c.Set("kling4_billed_seconds", billedSeconds.InexactFloat64())
	c.Set(relaycommon.TaskInputVideoSecondsKey, inputSeconds.InexactFloat64())
	c.Set("task_billing_unit", "seconds")
	info.Action = constant.TaskActionGenerate
	return nil
}
