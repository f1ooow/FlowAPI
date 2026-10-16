package helper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// NewPassthroughBody preserves the original storage unless a channel redirect
// changes the outbound model. A non-nil closer belongs to the caller; closing
// it never releases the original request storage used by other attempts.
func NewPassthroughBody(storage common.BodyStorage, info *relaycommon.RelayInfo, contentType string) (common.ReplayableBody, io.Closer, error) {
	if !info.IsModelMapped || info.UpstreamModelName == info.OriginModelName {
		return common.NewReplayableBodyReader(storage), nil, nil
	}

	if isMultipartContentType(contentType) {
		data, err := storage.Bytes()
		if err != nil {
			return nil, nil, err
		}
		return rewriteMultipartModel(data, contentType, info.UpstreamModelName)
	}
	if contentType != "" && contentType != "application/json" && !strings.HasSuffix(contentType, "+json") {
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

func isMultipartContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	return err == nil && strings.EqualFold(mediaType, "multipart/form-data")
}

func rewriteMultipartModel(data []byte, contentType, upstreamModel string) (common.ReplayableBody, io.Closer, error) {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, nil, err
	}
	boundary := params["boundary"]
	if boundary == "" {
		return nil, nil, fmt.Errorf("multipart boundary not found")
	}

	form, err := multipart.NewReader(bytes.NewReader(data), boundary).ReadForm(32 << 20)
	if err != nil {
		return nil, nil, err
	}
	defer form.RemoveAll()

	var rewritten bytes.Buffer
	writer := multipart.NewWriter(&rewritten)
	if err := writer.SetBoundary(boundary); err != nil {
		return nil, nil, err
	}
	if err := writer.WriteField("model", upstreamModel); err != nil {
		return nil, nil, err
	}
	for key, values := range form.Value {
		if key == "model" {
			continue
		}
		for _, value := range values {
			if err := writer.WriteField(key, value); err != nil {
				return nil, nil, err
			}
		}
	}
	for _, files := range form.File {
		for _, fileHeader := range files {
			file, err := fileHeader.Open()
			if err != nil {
				return nil, nil, err
			}
			part, err := writer.CreatePart(fileHeader.Header)
			if err == nil {
				_, err = io.Copy(part, file)
			}
			closeErr := file.Close()
			if err != nil {
				return nil, nil, err
			}
			if closeErr != nil {
				return nil, nil, closeErr
			}
		}
	}
	if err := writer.Close(); err != nil {
		return nil, nil, err
	}
	return relaycommon.NewOutboundJSONBody(rewritten.Bytes())
}
