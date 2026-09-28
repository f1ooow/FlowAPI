package minimax

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
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
	header := form.File["file"][0]
	limit, err := miniMaxUploadLimit(purpose, header)
	if err != nil {
		return nil, miniMaxError(err, http.StatusBadRequest)
	}
	if header.Size <= 0 || header.Size > limit {
		return nil, miniMaxError(fmt.Errorf("file must be between 1 and %d bytes", limit), http.StatusBadRequest)
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
	count, err := io.Copy(part, io.LimitReader(file, limit+1))
	if err != nil || count > limit {
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

// UploadRoutingModel names the capability that later consumes an upload of this
// purpose. Files are bound to the uploading account, so the upload must land on
// a channel that can serve that capability. Pricing stays minimax-files-upload.
func UploadRoutingModel(purpose string) string {
	switch purpose {
	case "voice_clone", "prompt_audio":
		return "minimax-voice-clone"
	case "video_generation_input", "video_generation":
		return "MiniMax-H3"
	}
	return "minimax-files-upload"
}

// miniMaxUploadLimit applies MiniMax's per-purpose upload limits. Video
// generation inputs are limited by media kind; the global request body limit
// always takes precedence.
func miniMaxUploadLimit(purpose string, header *multipart.FileHeader) (int64, error) {
	var limit int64
	switch purpose {
	case "voice_clone", "prompt_audio", "t2a_async_input", "video_understanding":
		limit = MaxUploadBytes
	case "video_generation_input", "video_generation":
		mediaType, _, _ := mime.ParseMediaType(header.Header.Get("Content-Type"))
		if mediaType == "" || mediaType == "application/octet-stream" {
			mediaType, _, _ = mime.ParseMediaType(mime.TypeByExtension(strings.ToLower(filepath.Ext(header.Filename))))
		}
		switch {
		case strings.HasPrefix(mediaType, "video/"):
			limit = 50 << 20
		case strings.HasPrefix(mediaType, "image/"):
			limit = 30 << 20
		case strings.HasPrefix(mediaType, "audio/"):
			limit = 15 << 20
		default:
			return 0, fmt.Errorf("video generation input must be an image, video or audio file")
		}
	default:
		return 0, fmt.Errorf("unsupported upload purpose")
	}
	if constant.MaxRequestBodyMB > 0 {
		limit = min(limit, int64(constant.MaxRequestBodyMB)<<20)
	}
	return limit, nil
}
