package service

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// ProbeKling4Media validates the downloaded bytes and returns unrounded seconds.
// Call it for every reference entry: repeated URLs still represent billed inputs.
func ProbeKling4Media(ctx context.Context, rawURL, kind string) (float64, error) {
	var maxBytes int64
	switch kind {
	case "image":
		maxBytes = 10 * 1024 * 1024
	case "video":
		maxBytes = 50 * 1024 * 1024
	case "audio":
		maxBytes = 20 * 1024 * 1024
	default:
		return 0, fmt.Errorf("unsupported reference media kind")
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return 0, fmt.Errorf("media must use HTTP(S) without credentials")
	}
	if err := ValidateSSRFProtectedFetchURL(rawURL); err != nil {
		return 0, fmt.Errorf("media URL blocked")
	}
	client := GetSSRFProtectedHTTPClient()
	if client == nil {
		return 0, fmt.Errorf("media fetch client is unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("media download failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("media download returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxBytes {
		return 0, fmt.Errorf("%s exceeds %d MB limit", kind, maxBytes/(1024*1024))
	}
	f, err := os.CreateTemp("", "kling4-probe-*.media")
	if err != nil {
		return 0, err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	n, err := io.Copy(f, io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return 0, fmt.Errorf("media download incomplete")
	}
	if n > maxBytes {
		return 0, fmt.Errorf("%s exceeds %d MB limit", kind, maxBytes/(1024*1024))
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	var header [512]byte
	read, err := f.Read(header[:])
	if err != nil && err != io.EOF {
		return 0, err
	}
	contentType := http.DetectContentType(header[:read])
	if kind == "image" {
		if !strings.HasPrefix(contentType, "image/") {
			return 0, fmt.Errorf("reference image is not a supported image file")
		}
		return 0, nil
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	var seconds float64
	if kind == "video" {
		seconds, err = wanVideoTrackDuration(f)
	} else {
		ext := strings.ToLower(path.Ext(u.Path))
		switch contentType {
		case "audio/wave", "audio/wav", "audio/x-wav":
			ext = ".wav"
		case "audio/mpeg":
			ext = ".mp3"
		}
		if ext != ".mp3" && ext != ".wav" {
			return 0, fmt.Errorf("reference audio must be MP3 or WAV")
		}
		seconds, err = common.GetAudioDuration(ctx, f, ext)
	}
	if err != nil {
		return 0, fmt.Errorf("cannot measure %s duration: %w", kind, err)
	}
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 || seconds > 30 || (kind == "audio" && seconds < 5) {
		return 0, fmt.Errorf("reference %s duration is outside supported bounds", kind)
	}
	return seconds, nil
}
