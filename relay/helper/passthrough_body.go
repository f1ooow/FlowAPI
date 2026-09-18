package helper

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// NewPassthroughBody preserves the original storage unless a channel redirect
// changes a JSON body's model. A non-nil closer belongs to the caller; closing
// it never releases the original request storage used by other attempts.
func NewPassthroughBody(storage common.BodyStorage, info *relaycommon.RelayInfo, contentType string) (common.ReplayableBody, io.Closer, error) {
	if !info.IsModelMapped || info.UpstreamModelName == info.OriginModelName ||
		(contentType != "" && contentType != "application/json" && !strings.HasSuffix(contentType, "+json")) {
		return common.NewReplayableBodyReader(storage), nil, nil
	}

	data, err := storage.Bytes()
	if err != nil {
		return nil, nil, err
	}
	var fields map[string]json.RawMessage
	if err := common.Unmarshal(data, &fields); err != nil {
		return nil, nil, fmt.Errorf("invalid JSON passthrough body: %w", err)
	}
	if fields == nil {
		return nil, nil, fmt.Errorf("JSON passthrough body must be an object")
	}
	model, err := common.Marshal(info.UpstreamModelName)
	if err != nil {
		return nil, nil, err
	}
	fields["model"] = model
	data, err = common.Marshal(fields)
	if err != nil {
		return nil, nil, err
	}
	return relaycommon.NewOutboundJSONBody(data)
}
