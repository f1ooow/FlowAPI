package dto

import (
	"fmt"
	"net/url"
	"strings"
)

const Kling4ProtocolRolldek = "rolldek-kling4"

type Kling4Request struct {
	Model          string         `json:"model"`
	Prompt         string         `json:"prompt"`
	Duration       *int           `json:"duration"`
	Resolution     string         `json:"resolution"`
	AspectRatio    *string        `json:"aspect_ratio,omitempty"`
	Images         []string       `json:"images,omitempty"`
	Videos         []string       `json:"videos,omitempty"`
	Audios         []string       `json:"audios,omitempty"`
	N              *int           `json:"n,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	Image          *string        `json:"image,omitempty"`
	InputReference *string        `json:"input_reference,omitempty"`
	Ratio          *string        `json:"ratio,omitempty"`
}

func (r *Kling4Request) NormalizeAndValidate() error {
	if r.Model != "kling-4.0" {
		return fmt.Errorf("unsupported Kling 4 model")
	}
	if strings.TrimSpace(r.Prompt) == "" {
		return fmt.Errorf("prompt is required")
	}
	if r.Duration == nil || *r.Duration < 3 || *r.Duration > 30 {
		return fmt.Errorf("duration must be an integer from 3 to 30")
	}
	if r.Resolution != "720p" && r.Resolution != "1080p" {
		return fmt.Errorf("resolution must be 720p or 1080p")
	}
	if r.N != nil && *r.N != 1 {
		return fmt.Errorf("n must be 1")
	}
	if len(r.Metadata) != 0 {
		return fmt.Errorf("Kling 4 metadata extensions are not supported")
	}
	if r.Images == nil {
		if r.Image != nil && *r.Image != "" {
			r.Images = []string{*r.Image}
		} else if r.InputReference != nil && *r.InputReference != "" {
			r.Images = []string{*r.InputReference}
		}
	}
	if r.AspectRatio == nil {
		r.AspectRatio = r.Ratio
	}
	for _, media := range []struct {
		name   string
		urls   []string
		maxLen int
	}{
		{"images", r.Images, 10},
		{"videos", r.Videos, 5},
		{"audios", r.Audios, 3},
	} {
		if len(media.urls) > media.maxLen {
			return fmt.Errorf("%s supports at most %d inputs", media.name, media.maxLen)
		}
		for _, rawURL := range media.urls {
			u, err := url.Parse(rawURL)
			if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
				return fmt.Errorf("%s must use HTTP(S) URLs without credentials", media.name)
			}
		}
	}
	r.Image = nil
	r.InputReference = nil
	r.Ratio = nil
	r.Metadata = nil
	return nil
}

func (r Kling4Request) ResolutionRatio() float64 {
	if r.Resolution == "1080p" {
		return 1.4
	}
	return 1
}
