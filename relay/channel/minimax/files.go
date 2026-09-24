package minimax

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
)

// MiniMaxFileObject 上传后单个文件的元信息（file_id 跨边界使用 string 避免 JS Number 精度问题）。
// 上游历史响应存在 JSON number/string 两种形态，因此自定义反序列化后统一输出 string。
type MiniMaxFileObject struct {
	FileID    string `json:"file_id"`
	Bytes     int64  `json:"bytes,omitempty"`
	CreatedAt int64  `json:"created_at,omitempty"`
	Filename  string `json:"filename,omitempty"`
	Purpose   string `json:"purpose,omitempty"`
}

func (f *MiniMaxFileObject) UnmarshalJSON(data []byte) error {
	type fileObjectAlias struct {
		FileID    json.RawMessage `json:"file_id"`
		Bytes     int64           `json:"bytes,omitempty"`
		CreatedAt int64           `json:"created_at,omitempty"`
		Filename  string          `json:"filename,omitempty"`
		Purpose   string          `json:"purpose,omitempty"`
	}
	var value fileObjectAlias
	if err := common.Unmarshal(data, &value); err != nil {
		return err
	}
	f.Bytes = value.Bytes
	f.CreatedAt = value.CreatedAt
	f.Filename = value.Filename
	f.Purpose = value.Purpose
	if len(value.FileID) == 0 || string(value.FileID) == "null" {
		f.FileID = ""
		return nil
	}
	if err := common.Unmarshal(value.FileID, &f.FileID); err == nil {
		f.FileID = strings.TrimSpace(f.FileID)
		return nil
	}
	f.FileID = strings.TrimSpace(string(value.FileID))
	_, err := strconv.ParseInt(f.FileID, 10, 64)
	return err
}

// MiniMaxFileResponse 上传响应
type MiniMaxFileResponse struct {
	File     MiniMaxFileObject `json:"file"`
	BaseResp MiniMaxBaseResp   `json:"base_resp"`
}

func RelayMiniMaxFilesUpload(c *gin.Context, info *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	form, err := common.ParseMultipartFormReusable(c)
	if err != nil {
		return nil, miniMaxError(err, http.StatusBadRequest)
	}
	defer form.RemoveAll()
	if len(form.File) != 1 || len(form.File["file"]) != 1 || len(form.Value) > 3 {
		return nil, miniMaxError(fmt.Errorf("exactly one file is required"), http.StatusBadRequest)
	}
	for key, values := range form.Value {
		if (key != "purpose" && key != "model") || len(values) != 1 || len(values[0]) > 128 {
			return nil, miniMaxError(fmt.Errorf("invalid upload field %s", key), http.StatusBadRequest)
		}
	}
	purpose := ""
	if len(form.Value["purpose"]) == 1 {
		purpose = form.Value["purpose"][0]
	}
	switch purpose {
	case "voice_clone", "prompt_audio", "video_generation", "t2a_async_input":
	default:
		return nil, miniMaxError(fmt.Errorf("unsupported upload purpose"), http.StatusBadRequest)
	}
	header := form.File["file"][0]
	if header.Size <= 0 || header.Size > MaxUploadBytes {
		return nil, miniMaxError(fmt.Errorf("file must be between 1 and %d bytes", MaxUploadBytes), http.StatusBadRequest)
	}
	file, err := header.Open()
	if err != nil {
		return nil, miniMaxError(err, http.StatusBadRequest)
	}
	defer file.Close()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("purpose", purpose); err != nil {
		return nil, miniMaxError(err, http.StatusBadRequest)
	}
	part, err := writer.CreateFormFile("file", header.Filename)
	if err != nil {
		return nil, miniMaxError(err, http.StatusBadRequest)
	}
	count, err := io.Copy(part, io.LimitReader(file, MaxUploadBytes+1))
	if err != nil || count > MaxUploadBytes {
		return nil, miniMaxError(fmt.Errorf("invalid or oversized upload"), http.StatusBadRequest)
	}
	if err := writer.Close(); err != nil {
		return nil, miniMaxError(err, http.StatusBadRequest)
	}
	raw, apiErr := callMiniMaxExtended(c, info, "/v1/files/upload", writer.FormDataContentType(), &body)
	if apiErr != nil {
		return nil, apiErr
	}
	var response MiniMaxFileResponse
	if err := common.Unmarshal(raw, &response); err != nil {
		return nil, miniMaxError(fmt.Errorf("invalid MiniMax upload response"), http.StatusBadGateway)
	}
	id, err := strconv.ParseInt(response.File.FileID, 10, 64)
	if err != nil || id <= 0 {
		return nil, miniMaxError(fmt.Errorf("invalid MiniMax file_id"), http.StatusBadGateway)
	}
	if err := BindResource(info, "file", response.File.FileID); err != nil {
		return nil, miniMaxError(err, http.StatusConflict)
	}
	c.JSON(http.StatusOK, response)
	return &dto.Usage{}, nil
}

func buildMiniMaxFilesUploadURL(baseURL string) (string, error) {
	return miniMaxEndpoint(baseURL, "/v1/files/upload")
}
