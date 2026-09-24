// Package volcengine - Doubao Seed ASR ("录音文件识别 2.0") adapter.
//
// Flow:
//
//  1. Client POSTs multipart audio to `/v1/audio/transcriptions` with the
//     volcengine channel selected (model = `doubao-asr-2.0` or similar).
//  2. ConvertAudioRequest reads the `file` part bytes.
//  3. The bytes are uploaded to Volcengine VOD (via vod_uploader.go) using
//     VOD credentials stashed in the channel's OtherInfo map.
//  4. We build a public playback URL `<scheme>://<playback-domain>/<FileName>`
//     and submit it to `https://openspeech.bytedance.com/api/v3/auc/bigmodel/submit`.
//  5. We poll `/query` every 2s until X-Api-Status-Code = 20000000 (success)
//     or a terminal error, with a 5min ceiling.
//  6. utterances → SRT text + segments[]. We return an OpenAI Whisper-compatible
//     JSON `{ text, language, duration, segments }`.
//
// API key format on the channel: "<APP_ID>|<ACCESS_TOKEN>", same as TTS
// (`<APP_ID>|<ACCESS_TOKEN>`). The ACCESS_TOKEN is sent as X-Api-Access-Key
// and the APP_ID as X-Api-App-Key (Doubao 录音文件识别 v3 requires both, no
// X-Api-Sequence which is streaming-only). APP_ID also fills request.user.uid.
//
// Tempo / public URL caveat: the audio URL given to Doubao MUST be reachable
// from Volcengine's data plane. Public CDN-fronted VOD playback URLs satisfy
// this (URL-auth signed when the operator has it enabled). Localhost / LAN
// URLs will not work — that's why we upload to VOD instead of streaming
// directly.
//
// TODO: delete the temporary VOD media after ASR completes. Currently the
// uploaded file lingers in the operator's VOD space. The TS volcvod driver
// has this cleanup; we should replicate it here once we have a clear retention
// policy. The cost per minute is tiny (~¥0.0003) so it isn't a launch blocker.
package volcengine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel/task/volcvod"
	"github.com/QuantumNous/new-api/service"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	asrSubmitEndpoint = "https://openspeech.bytedance.com/api/v3/auc/bigmodel/submit"
	asrQueryEndpoint  = "https://openspeech.bytedance.com/api/v3/auc/bigmodel/query"
	asrResourceID     = "volc.bigasr.auc"

	asrPollIntervalSecs = 2
	asrMaxPollSecs      = 300 // 5 minutes
)

// X-Api-Status-Code (header) values per Doubao ASR error code reference.
const (
	asrStatusSuccess    = "20000000"
	asrStatusProcessing = "20000001"
	asrStatusQueued     = "20000002"
	asrStatusSilent     = "20000003"
)

// Module-level seam for tests to swap in a fake HTTP client / VOD uploader.
//
//	asrHTTPClient and asrVodUploader default to real implementations; tests
//	set them to deterministic fakes via the (un-exported) setters.
var (
	asrHTTPClient = &http.Client{Timeout: 20 * time.Second}

	// uploadFn is the indirection seam for the VOD upload step. Tests can
	// override this to avoid actually hitting Volcengine VOD.
	asrUploadFn = UploadMediaToVOD

	// nowFn is the time source. Tests can stub this to drive deterministic
	// polling without sleeping.
	asrNowFn = time.Now

	// sleepFn is the poll-delay seam. Tests can stub it to return immediately.
	asrSleepFn = time.Sleep
)

// asrRequest is the body shape we POST to Doubao ASR submit.
type asrRequest struct {
	User    asrUserInfo    `json:"user"`
	Audio   asrAudioInfo   `json:"audio"`
	Request asrRequestInfo `json:"request"`
}

type asrUserInfo struct {
	UID string `json:"uid"`
}

type asrAudioInfo struct {
	URL      string `json:"url"`
	Format   string `json:"format"`             // mp3 / wav / ...
	Language string `json:"language,omitempty"` // e.g. "zh-CN"
}

type asrRequestInfo struct {
	ModelName      string `json:"model_name"`            // "bigmodel"
	EnableITN      bool   `json:"enable_itn,omitempty"`  // 文字归一化（数字/日期）
	EnablePunc     bool   `json:"enable_punc,omitempty"` // 标点
	ShowUtterances bool   `json:"show_utterances,omitempty"`
}

// asrSubmitResponse is the JSON body of the submit endpoint.
type asrSubmitResponse struct {
	// Some payloads return `task_id`, some only convey it via the request id
	// echoed in headers; tolerate both.
	TaskID  string `json:"task_id,omitempty"`
	Message string `json:"message,omitempty"`
}

// asrQueryResponse is the JSON body of the query endpoint when a result is
// ready. Schema based on the public Doubao录音识别 2.0 docs (Aug 2025 rev).
type asrQueryResponse struct {
	AudioInfo struct {
		Duration float64 `json:"duration,omitempty"`
	} `json:"audio_info,omitempty"`
	Result struct {
		Text       string            `json:"text,omitempty"`
		Utterances []asrUtterance    `json:"utterances,omitempty"`
		Additions  map[string]string `json:"additions,omitempty"`
	} `json:"result,omitempty"`
	Message string `json:"message,omitempty"`
}

type asrUtterance struct {
	Text      string   `json:"text"`
	StartTime int64    `json:"start_time"` // ms
	EndTime   int64    `json:"end_time"`   // ms
	Definite  bool     `json:"definite,omitempty"`
	Words     []asrWrd `json:"words,omitempty"`
}

type asrWrd struct {
	Text      string `json:"text"`
	StartTime int64  `json:"start_time"`
	EndTime   int64  `json:"end_time"`
}

// readMultipartAudio extracts the audio bytes + filename out of the gin
// request's multipart form. Mirrors the pattern in
// `relay/channel/cloudflare/adaptor.go::ConvertAudioRequest`.
//
// Returns the bytes, the original filename (best-effort, may be empty), and
// any parse error.
func readMultipartAudio(c *gin.Context) ([]byte, string, error) {
	form, err := common.ParseMultipartFormReusable(c)
	if err != nil {
		return nil, "", err
	}
	if len(form.File) != 1 || len(form.File["file"]) != 1 || form.File["file"][0].Size > volcvod.VodSinglePutLimitBytes {
		return nil, "", fmt.Errorf("exactly one audio file up to 20 MiB is required")
	}
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		return nil, "", fmt.Errorf("multipart 'file' part is required: %w", err)
	}
	defer file.Close()

	buf := &bytes.Buffer{}
	if n, err := io.Copy(buf, io.LimitReader(file, volcvod.VodSinglePutLimitBytes+1)); err != nil || n > volcvod.VodSinglePutLimitBytes {
		return nil, "", fmt.Errorf("read 'file' part: %w", err)
	}
	filename := ""
	if header != nil {
		filename = header.Filename
	}
	return buf.Bytes(), filename, nil
}

// asrConvertContextKey stashes intermediate state (file bytes, filename,
// language) between ConvertAudioRequest and DoResponse, since the gin-driven
// API splits these into separate adaptor methods.
const contextKeyASRRequest = "volcengine_asr_request"

type asrContext struct {
	AudioBytes     []byte
	Filename       string
	SourceDuration float64
	ResponseFormat string
	Language       string // "zh-CN" by default; pulled from form param if present
}

// convertAudioRequestForASR is called by Adaptor.ConvertAudioRequest when the
// relay mode is RelayModeAudioTranscription. It does NOT yet upload to VOD —
// that happens in DoRequest, where we have a clean place to do network I/O
// and surface upstream errors.
func convertAudioRequestForASR(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	if info.UpstreamModelName != "doubao-asr-2.0" && info.UpstreamModelName != "" {
		return nil, fmt.Errorf("unsupported Doubao ASR model")
	}
	switch request.ResponseFormat {
	case "", "json", "verbose_json", "text", "srt", "vtt":
	default:
		return nil, fmt.Errorf("unsupported ASR response format")
	}
	c.Set("volcengine_asr_response_format", request.ResponseFormat)
	audioBytes, filename, err := readMultipartAudio(c)
	if err != nil {
		return nil, err
	}
	if len(audioBytes) == 0 {
		return nil, errors.New("uploaded audio file is empty")
	}

	// Language: prefer form param `language`, fallback to default "zh-CN".
	// (OpenAI's whisper accepts ISO-639-1; Doubao wants "zh-CN" / "en-US" etc.)
	language := strings.TrimSpace(c.PostForm("language"))
	if language == "" {
		// dto.AudioRequest.Language is json.RawMessage; try to extract as string.
		if len(request.Language) > 0 {
			var lang string
			if err := common.Unmarshal(request.Language, &lang); err == nil {
				language = strings.TrimSpace(lang)
			}
		}
	}
	if language == "" {
		language = "zh-CN"
	}

	c.Set(contextKeyASRRequest, &asrContext{
		AudioBytes: audioBytes,
		Filename:   filename,
		Language:   language,
	})

	// Return an empty reader: the actual upstream call is orchestrated by
	// the ASR-specific DoRequest path (we don't fit cleanly into newapi's
	// stock "convert→reader→DoRequest pipes the reader" assumption since
	// the ASR submit body is built AFTER the VOD upload completes).
	return bytes.NewReader(nil), nil
}

// doASRRequest runs the full ASR pipeline (upload → submit → poll). It is
// invoked from Adaptor.DoRequest when relay mode is transcription.
//
// Returns a synthetic *http.Response wrapping the final ASR query body, so
// the standard relay pipeline's downstream phases (DoResponse → write to
// client) just work. We synthesize 200 OK with `application/json` body when
// the ASR completes successfully; failures bubble up as plain Go errors so
// the relay layer maps them to a 502.
func doASRRequest(c *gin.Context, info *relaycommon.RelayInfo, getOtherInfo func(channelID int) (map[string]interface{}, error)) (*http.Response, error) {
	ctxVal, ok := c.Get(contextKeyASRRequest)
	if !ok {
		return nil, errors.New("ASR context not initialized; ConvertAudioRequest did not run")
	}
	asrCtx, ok := ctxVal.(*asrContext)
	if !ok || asrCtx == nil {
		return nil, errors.New("ASR context has wrong type")
	}

	// 1. Auth: split <APP_ID>|<ACCESS_TOKEN>.
	appID, accessToken, err := parseVolcengineAuth(info.ApiKey)
	if err != nil {
		return nil, fmt.Errorf("invalid channel api_key (need APP_ID|ACCESS_TOKEN): %w", err)
	}

	// 2. VOD credentials live in OtherInfo (not in the channel Key, which is
	// reserved for the service-level APP_ID|ACCESS_TOKEN).
	creds, credErr := LoadVodCredentialsFromRelayInfo(info, getOtherInfo)
	if credErr != nil {
		return nil, credErr
	}
	if creds.PlaybackDomain == "" {
		return nil, errors.New("vod_playback_domain not configured on channel (required for ASR to fetch the uploaded audio)")
	}

	// 3. Upload audio to VOD.
	ext := inferExtFromFilename(asrCtx.Filename)
	contentType := inferContentTypeFromFilename(asrCtx.Filename)
	uploadOpts := VodUploadOpts{
		SpaceName:     creds.Space,
		Region:        creds.Region,
		AccessKey:     creds.AccessKey,
		SecretKey:     creds.SecretKey,
		FileExtension: ext,
		FileName:      asrCtx.Filename,
		Context:       c.Request.Context(),
	}
	uploadResult, uploadErr := asrUploadFn(asrCtx.AudioBytes, uploadOpts, contentType)
	if uploadErr != nil {
		return nil, fmt.Errorf("VOD upload failed: %w", uploadErr)
	}

	if uploadResult == nil || uploadResult.Duration <= 0 || math.IsNaN(uploadResult.Duration) || math.IsInf(uploadResult.Duration, 0) || uploadResult.Duration > relaycommon.MaxTaskDurationSeconds {
		return nil, fmt.Errorf("VOD upload returned invalid audio duration")
	}
	asrCtx.SourceDuration = uploadResult.Duration
	if info.PriceData.UsePrice {
		return nil, fmt.Errorf("Doubao ASR requires seconds-based ModelRatio pricing")
	}
	quota, clamp := common.QuotaFromFloatChecked(math.Ceil(uploadResult.Duration) * info.PriceData.ModelRatio * info.PriceData.GroupRatioInfo.GroupRatio)
	info.QuotaClamp = clamp
	if clamp != nil {
		return nil, clamp
	}
	if info.Billing != nil {
		if err := info.Billing.Reserve(quota); err != nil {
			return nil, err
		}
	} else if quota > 0 {
		if err := service.PreConsumeBilling(c, quota, info); err != nil {
			return nil, err
		}
	}

	// 4. Build the public audio URL from the operator-configured playback
	// domain. When the channel has a `vod_url_auth_key` configured (the VOD
	// space enforces CDN URL-auth), we sign the URL with a Type-A auth_key so
	// Doubao's data plane can fetch it. When no key is configured we leave the
	// URL unsigned — that's the correct behaviour for spaces with url-auth off.
	scheme := creds.PlaybackScheme
	audioURL := fmt.Sprintf("%s://%s/%s", scheme, creds.PlaybackDomain, strings.TrimPrefix(uploadResult.FileName, "/"))
	if creds.UrlAuthKey != "" {
		signedURL, signErr := signVodPlaybackURL(audioURL, creds.UrlAuthKey, 3600)
		if signErr != nil {
			return nil, fmt.Errorf("sign VOD playback URL: %w", signErr)
		}
		audioURL = signedURL
	}

	// 5. Submit to Doubao ASR.
	taskID, submitErr := asrSubmit(c.Request.Context(), accessToken, appID, audioURL, asrCtx, asrAudioFormatFor(asrCtx.Filename))
	if submitErr != nil {
		return nil, submitErr
	}
	if taskID == "" {
		return nil, errors.New("Doubao ASR submit returned empty task id (X-Api-Request-Id header missing)")
	}

	// 6. Poll until terminal.
	result, pollErr := asrPoll(c.Request.Context(), accessToken, appID, taskID)
	if pollErr != nil {
		return nil, pollErr
	}

	// 7. Translate to OpenAI Whisper-compatible verbose JSON.
	if result.AudioInfo.Duration == 0 {
		result.AudioInfo.Duration = asrCtx.SourceDuration * 1000
	}
	if err := validateASRResult(result); err != nil {
		return nil, err
	}
	whisperJSON := utterancesToWhisperJSON(result, asrCtx.Language)
	jsonBytes, err := common.Marshal(whisperJSON)
	if err != nil {
		return nil, fmt.Errorf("marshal whisper response: %w", err)
	}

	// 8. Wrap in a synthetic http.Response so the downstream relay flow can
	// read body / status uniformly.
	synthetic := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(jsonBytes)),
	}
	synthetic.Header.Set("Content-Type", "application/json")
	return synthetic, nil
}

// asrSubmit POSTs to /api/v3/auc/bigmodel/submit. Returns the task id (we
// use the X-Api-Request-Id header value we sent, which doubles as the query
// key per Doubao's protocol).
func asrSubmit(ctx context.Context, accessToken, appID, audioURL string, asrCtx *asrContext, format string) (string, error) {
	reqID := uuid.NewString()

	body := asrRequest{
		User: asrUserInfo{UID: fallbackUID(appID)},
		Audio: asrAudioInfo{
			URL:      audioURL,
			Format:   format,
			Language: asrCtx.Language,
		},
		Request: asrRequestInfo{
			ModelName:      "bigmodel",
			EnableITN:      true,
			EnablePunc:     true,
			ShowUtterances: true,
		},
	}
	jsonBody, err := common.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal ASR submit body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, asrSubmitEndpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("new ASR submit request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Access-Key", accessToken)
	req.Header.Set("X-Api-Resource-Id", asrResourceID)
	req.Header.Set("X-Api-Request-Id", reqID)
	req.Header.Set("X-Api-App-Key", appID)
	req.Header.Set("X-Api-Sequence", "-1")

	resp, err := asrHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ASR submit network error: %w", err)
	}
	defer resp.Body.Close()
	respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if readErr != nil || len(respBody) > 8<<20 {
		return "", fmt.Errorf("invalid ASR submit response")
	}

	statusCode := resp.Header.Get("X-Api-Status-Code")
	if (resp.StatusCode < 200 || resp.StatusCode >= 300) || (statusCode != asrStatusSuccess && statusCode != asrStatusProcessing && statusCode != asrStatusQueued) {
		return "", asrErrorFromResponse(resp.StatusCode, statusCode, respBody)
	}
	// Per protocol, the request id we sent IS the query handle. Some payloads
	// also echo a `task_id` field; prefer it when present (defensive).
	if len(respBody) > 0 {
		var parsed asrSubmitResponse
		if err := common.Unmarshal(respBody, &parsed); err == nil && parsed.TaskID != "" {
			return parsed.TaskID, nil
		}
	}
	return reqID, nil
}

// asrPoll calls /api/v3/auc/bigmodel/query every 2 seconds until terminal.
// Returns the final query body on success (X-Api-Status-Code = 20000000).
func asrPoll(ctx context.Context, accessToken, appID, taskID string) (*asrQueryResponse, error) {
	deadline := asrNowFn().Add(time.Duration(asrMaxPollSecs) * time.Second)
	for {
		if asrNowFn().After(deadline) {
			return nil, fmt.Errorf("Doubao ASR polling timed out after %ds", asrMaxPollSecs)
		}
		resp, body, statusCode, err := asrQueryOnce(ctx, accessToken, appID, taskID)
		if err != nil {
			return nil, err
		}
		if resp < 200 || resp >= 300 {
			return nil, asrErrorFromResponse(resp, statusCode, body)
		}
		switch statusCode {
		case asrStatusSuccess:
			parsed := &asrQueryResponse{}
			if err := common.Unmarshal(body, parsed); err != nil {
				return nil, fmt.Errorf("ASR query: parse response: %w (body=%s)", err, truncate(string(body), 400))
			}
			return parsed, nil
		case asrStatusProcessing, asrStatusQueued:
			// keep polling
		case asrStatusSilent:
			// Silent audio: success-with-empty-result.
			return &asrQueryResponse{}, nil
		default:
			// Terminal failure; bubble up with the upstream code.
			return nil, asrErrorFromResponse(resp, statusCode, body)
		}
		timer := time.NewTimer(time.Duration(asrPollIntervalSecs) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func asrQueryOnce(ctx context.Context, accessToken, appID, taskID string) (int, []byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, asrQueryEndpoint, bytes.NewReader([]byte("{}")))
	if err != nil {
		return 0, nil, "", fmt.Errorf("new ASR query request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Access-Key", accessToken)
	req.Header.Set("X-Api-Resource-Id", asrResourceID)
	req.Header.Set("X-Api-Request-Id", taskID)
	req.Header.Set("X-Api-App-Key", appID)
	req.Header.Set("X-Api-Sequence", "-1")

	resp, err := asrHTTPClient.Do(req)
	if err != nil {
		return 0, nil, "", fmt.Errorf("ASR query network error: %w", err)
	}
	defer resp.Body.Close()
	respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if readErr != nil || len(respBody) > 8<<20 {
		return 0, nil, "", fmt.Errorf("invalid ASR query response")
	}
	statusCode := resp.Header.Get("X-Api-Status-Code")
	return resp.StatusCode, respBody, statusCode, nil
}

// utterancesToWhisperJSON turns the Doubao ASR result into an OpenAI Whisper-
// compatible verbose JSON. We pin language to the short ISO code (e.g. "zh")
// since the OpenAI shape uses that, not the BCP-47 "zh-CN" we send to Doubao.
func utterancesToWhisperJSON(query *asrQueryResponse, requestedLanguage string) *dto.WhisperVerboseJSONResponse {
	if query == nil {
		return &dto.WhisperVerboseJSONResponse{Text: ""}
	}
	resp := &dto.WhisperVerboseJSONResponse{
		Task:     "transcribe",
		Language: shortLanguage(requestedLanguage),
		Duration: float64(query.AudioInfo.Duration) / 1000.0,
		Text:     query.Result.Text,
	}
	if len(query.Result.Utterances) == 0 {
		return resp
	}
	segments := make([]dto.Segment, 0, len(query.Result.Utterances))
	for i, u := range query.Result.Utterances {
		segments = append(segments, dto.Segment{
			Id:    i,
			Start: float64(u.StartTime) / 1000.0,
			End:   float64(u.EndTime) / 1000.0,
			Text:  u.Text,
		})
	}
	resp.Segments = segments
	if resp.Text == "" {
		// Doubao sometimes omits the joined text; reconstruct from utterances.
		var sb strings.Builder
		for _, u := range query.Result.Utterances {
			sb.WriteString(u.Text)
		}
		resp.Text = sb.String()
	}
	return resp
}

// utterancesToSRT renders utterances as an SRT-formatted string. Exposed so
// callers downstream (CCS frontend) that prefer SRT over the OpenAI-style
// segments array can request it via response_format=srt. Not wired into the
// main response path yet — we return the verbose JSON by default; SRT is
// available as a utility.
func utterancesToSRT(query *asrQueryResponse) string {
	if query == nil || len(query.Result.Utterances) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, u := range query.Result.Utterances {
		fmt.Fprintf(&sb, "%d\n", i+1)
		fmt.Fprintf(&sb, "%s --> %s\n", formatSRTTimestamp(u.StartTime), formatSRTTimestamp(u.EndTime))
		fmt.Fprintf(&sb, "%s\n\n", u.Text)
	}
	return sb.String()
}

func formatSRTTimestamp(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	hours := ms / 3600000
	rem := ms % 3600000
	minutes := rem / 60000
	rem = rem % 60000
	seconds := rem / 1000
	millis := rem % 1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, seconds, millis)
}

func shortLanguage(lang string) string {
	if lang == "" {
		return ""
	}
	if idx := strings.Index(lang, "-"); idx > 0 {
		return strings.ToLower(lang[:idx])
	}
	return strings.ToLower(lang)
}

// asrAudioFormatFor returns the value to put in `audio.format` for the
// Doubao ASR submit request.
func asrAudioFormatFor(filename string) string {
	return inferAudioFormatFromFilename(filename)
}

func fallbackUID(appID string) string {
	if appID != "" {
		return appID
	}
	return "newapi"
}

func asrErrorFromResponse(httpStatus int, statusCode string, body []byte) error {
	snippet := truncate(string(body), 400)
	if statusCode != "" {
		return fmt.Errorf("Doubao ASR upstream error: X-Api-Status-Code=%s HTTP=%d body=%s", statusCode, httpStatus, snippet)
	}
	return fmt.Errorf("Doubao ASR upstream error: HTTP=%d body=%s", httpStatus, snippet)
}

// handleASRResponse is invoked by Adaptor.DoResponse when relay mode is
// transcription. The "response" here is the synthetic one we built inside
// doASRRequest; this function streams it to the client and reports usage.
func handleASRResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	if resp == nil {
		return nil, types.NewErrorWithStatusCode(
			errors.New("ASR: nil response"),
			types.ErrorCodeBadResponse,
			http.StatusInternalServerError,
		)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if err != nil || len(body) > 8<<20 {
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("ASR: invalid response body"),
			types.ErrorCodeReadResponseBodyFailed,
			http.StatusInternalServerError,
		)
	}

	var parsed dto.WhisperVerboseJSONResponse
	if err := common.Unmarshal(body, &parsed); err != nil || math.IsNaN(parsed.Duration) || math.IsInf(parsed.Duration, 0) || parsed.Duration <= 0 || parsed.Duration > relaycommon.MaxTaskDurationSeconds {
		return nil, types.NewErrorWithStatusCode(fmt.Errorf("invalid ASR duration"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	durationSeconds := common.QuotaFromFloat(math.Ceil(parsed.Duration))
	switch c.GetString("volcengine_asr_response_format") {
	case "text":
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(parsed.Text))
	case "srt", "vtt":
		var output strings.Builder
		vtt := c.GetString("volcengine_asr_response_format") == "vtt"
		if vtt {
			output.WriteString("WEBVTT\n\n")
		}
		for i, segment := range parsed.Segments {
			start := formatSRTTimestamp(int64(segment.Start * 1000))
			end := formatSRTTimestamp(int64(segment.End * 1000))
			if vtt {
				start = strings.ReplaceAll(start, ",", ".")
				end = strings.ReplaceAll(end, ",", ".")
			}
			fmt.Fprintf(&output, "%d\n%s --> %s\n%s\n\n", i+1, start, end, segment.Text)
		}
		contentType := "application/x-subrip"
		if vtt {
			contentType = "text/vtt"
		}
		c.Data(http.StatusOK, contentType, []byte(output.String()))
	case "json":
		c.JSON(http.StatusOK, gin.H{"text": parsed.Text})
	default:
		c.Data(http.StatusOK, "application/json", body)
	}

	usage := &dto.Usage{
		PromptTokens:     durationSeconds,
		CompletionTokens: 0,
		TotalTokens:      durationSeconds,
	}
	return usage, nil
}

// asrRequestSupportsMultipart guards against accidentally routing JSON
// transcription requests through this path. Volcengine ASR is multipart-only.
func asrRequestSupportsMultipart(c *gin.Context) bool {
	return strings.Contains(c.Request.Header.Get("Content-Type"), "multipart/form-data")
}

func validateASRResult(result *asrQueryResponse) error {
	if result == nil || math.IsNaN(result.AudioInfo.Duration) || math.IsInf(result.AudioInfo.Duration, 0) || result.AudioInfo.Duration <= 0 || result.AudioInfo.Duration > relaycommon.MaxTaskDurationSeconds*1000 || len(result.Result.Utterances) > 10000 {
		return fmt.Errorf("invalid ASR audio duration or segment count")
	}
	for _, u := range result.Result.Utterances {
		if u.StartTime < 0 || u.EndTime < u.StartTime || float64(u.EndTime) > result.AudioInfo.Duration || len(u.Words) > 10000 {
			return fmt.Errorf("invalid ASR segment times")
		}
	}
	return nil
}
