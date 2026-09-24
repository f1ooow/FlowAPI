package dto

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"
)

const WanCreatePath = "/api/v1/services/aigc/video-generation/video-synthesis"
const WanProtocolRolldek = "wan-rolldek-v1"

func IsWanModel(name string) bool { return name == "wan3.0-video" || name == "wan3.0-video-prime" }

type WanMedia struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}
type WanInput struct {
	Prompt string     `json:"prompt,omitempty"`
	Media  []WanMedia `json:"media,omitempty"`
}
type WanParameters struct {
	Resolution   string `json:"resolution,omitempty"`
	Ratio        string `json:"ratio,omitempty"`
	Duration     *int   `json:"duration,omitempty"`
	Audio        *bool  `json:"audio,omitempty"`
	Seed         *int64 `json:"seed,omitempty"`
	PromptExtend *bool  `json:"prompt_extend,omitempty"`
	Watermark    *bool  `json:"watermark,omitempty"`
}
type WanRequest struct {
	Model      string        `json:"model"`
	Input      WanInput      `json:"input"`
	Parameters WanParameters `json:"parameters"`
}

func (r *WanRequest) NormalizeAndValidate() error {
	if !IsWanModel(r.Model) {
		return fmt.Errorf("unsupported Wan model")
	}
	if strings.TrimSpace(r.Input.Prompt) == "" && len(r.Input.Media) == 0 {
		return fmt.Errorf("input.prompt or input.media is required")
	}
	if utf8.RuneCountInString(r.Input.Prompt) > 20000 {
		r.Input.Prompt = string([]rune(r.Input.Prompt)[:20000])
	}
	p := &r.Parameters
	if p.Resolution == "" {
		p.Resolution = "1080P"
	}
	if !slices.Contains([]string{"480P", "720P", "1080P"}, p.Resolution) {
		return fmt.Errorf("invalid resolution")
	}
	if p.Ratio == "" {
		p.Ratio = "adaptive"
	}
	if !slices.Contains([]string{"adaptive", "16:9", "9:16", "1:1", "4:3", "3:4"}, p.Ratio) {
		return fmt.Errorf("invalid ratio")
	}
	if p.Duration == nil {
		d := 5
		p.Duration = &d
	}
	if *p.Duration < 2 || *p.Duration > 30 {
		return fmt.Errorf("duration must be an integer from 2 to 30; smart duration is not enabled")
	}
	if p.Seed != nil && (*p.Seed < -1 || *p.Seed > 2147483647) {
		return fmt.Errorf("invalid seed")
	}
	limits := map[string]int{"first_frame": 1, "last_frame": 1, "reference_image": 10, "reference_video": 5, "reference_audio": 5, "file": 1, "link": 1}
	counts := map[string]int{}
	for _, m := range r.Input.Media {
		limit, ok := limits[m.Type]
		if !ok {
			return fmt.Errorf("invalid media type: %s", m.Type)
		}
		counts[m.Type]++
		if counts[m.Type] > limit {
			return fmt.Errorf("too many %s inputs", m.Type)
		}
		u, err := url.Parse(m.URL)
		image := m.Type == "reference_image" || m.Type == "first_frame" || m.Type == "last_frame"
		if err != nil || m.URL == "" || !((u.Scheme == "http" || u.Scheme == "https" || u.Scheme == "oss") && u.Host != "" || image && strings.HasPrefix(m.URL, "data:image/")) {
			return fmt.Errorf("invalid media URL")
		}
		if m.Type == "link" && u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("link must be a public HTTP(S) URL")
		}
	}
	if counts["last_frame"] > 0 && counts["first_frame"] == 0 {
		return fmt.Errorf("last_frame requires first_frame")
	}
	frames := counts["first_frame"] + counts["last_frame"]
	if frames > 0 && frames != len(r.Input.Media) {
		return fmt.Errorf("frame and reference modes are mutually exclusive")
	}
	if counts["file"] > 0 && counts["link"] > 0 {
		return fmt.Errorf("file and link are mutually exclusive")
	}
	if (counts["file"]+counts["link"] > 0) && p.PromptExtend != nil && !*p.PromptExtend {
		return fmt.Errorf("file/link requires prompt_extend=true")
	}
	return nil
}

func (p WanParameters) ResolutionRatio() float64 {
	switch p.Resolution {
	case "480P":
		return .5
	case "720P":
		return 1
	default:
		return 2
	}
}

type WanOutput struct {
	TaskID     string `json:"task_id"`
	TaskStatus string `json:"task_status"`
	VideoURL   string `json:"video_url,omitempty"`
	Code       string `json:"code,omitempty"`
	Message    string `json:"message,omitempty"`
}
type WanUsage struct {
	VideoCount          int     `json:"video_count"`
	InputVideoDuration  float64 `json:"input_video_duration"`
	Duration            float64 `json:"duration"`
	OutputVideoDuration float64 `json:"output_video_duration"`
}
type WanResponse struct {
	Output WanOutput `json:"output"`
	Usage  *WanUsage `json:"usage,omitempty"`
}
