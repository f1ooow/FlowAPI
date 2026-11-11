package relay

import (
	"context"
	"encoding/json"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
)

// videoTaskTimingResponse preserves provider fields and numeric precision while
// adding optional display metadata at the root of every supported poll shape.
func videoTaskTimingResponse(ctx context.Context, task *model.Task, body []byte) []byte {
	var response map[string]json.RawMessage
	if err := common.Unmarshal(body, &response); err != nil || response == nil {
		common.SysError("video task timing: invalid response object")
		return body
	}
	metadata := make(map[string]json.RawMessage)
	if raw, exists := response["metadata"]; exists {
		if err := common.Unmarshal(raw, &metadata); err != nil {
			common.SysError("video task timing: invalid response metadata")
			return body
		}
		if metadata == nil {
			metadata = make(map[string]json.RawMessage)
		}
	}
	timing, err := service.GetVideoTaskGenerationTiming(ctx, task)
	if err != nil {
		// This field is advisory. Do not fail an otherwise valid task poll, or
		// log SQL, provider payloads, credentials, or another user's task data.
		common.SysError("video task timing: statistics unavailable")
	}
	metadata["generation_timing"], err = common.Marshal(timing)
	if err != nil {
		return body
	}
	response["metadata"], err = common.Marshal(metadata)
	if err != nil {
		return body
	}
	annotated, err := common.Marshal(response)
	if err != nil {
		return body
	}
	return annotated
}
