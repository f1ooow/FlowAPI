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
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/abema/go-mp4"
)

// ProbeWanMediaDuration uses the video track, not the movie/audio tail, for output billing.
func ProbeWanMediaDuration(ctx context.Context, rawURL string, video bool) (float64, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
		return 0, fmt.Errorf("media must use HTTP(S)")
	}
	fs := system_setting.GetFetchSetting()
	if err := common.ValidateURLWithFetchSetting(rawURL, fs.EnableSSRFProtection, fs.AllowPrivateIp, fs.DomainFilterMode, fs.IpFilterMode, fs.DomainList, fs.IpList, fs.AllowedPorts, fs.ApplyIPFilterForDomain); err != nil {
		return 0, fmt.Errorf("media URL blocked")
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, err
	}
	client := &http.Client{Timeout: 45 * time.Second, CheckRedirect: checkRedirect}
	if configured := GetSSRFProtectedHTTPClient(); configured != nil {
		client.Transport = configured.Transport
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("media download failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("media download returned HTTP %d", resp.StatusCode)
	}
	maxBytes := int64(constant.MaxFileDownloadMB) * 1024 * 1024
	if maxBytes <= 0 || maxBytes > 100*1024*1024 {
		maxBytes = 100 * 1024 * 1024
	}
	if !video && maxBytes > 15*1024*1024 {
		maxBytes = 15 * 1024 * 1024
	}
	if resp.ContentLength > maxBytes {
		return 0, fmt.Errorf("media exceeds download limit")
	}
	f, err := os.CreateTemp("", "wan-probe-*.media")
	if err != nil {
		return 0, err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	n, err := io.Copy(f, io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return 0, err
	}
	if n > maxBytes {
		return 0, fmt.Errorf("media exceeds download limit")
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	if video {
		return wanVideoTrackDuration(f)
	}
	ext := strings.ToLower(path.Ext(u.Path))
	if ext != ".mp3" && ext != ".wav" {
		return 0, fmt.Errorf("reference audio must be MP3 or WAV")
	}
	return common.GetAudioDuration(ctx, f, ext)
}

func wanVideoTrackDuration(r io.ReadSeeker) (float64, error) {
	var trackVideo bool
	var trackDuration, result float64
	flush := func() {
		if trackVideo && trackDuration > result {
			result = trackDuration
		}
		trackVideo = false
		trackDuration = 0
	}
	_, err := mp4.ReadBoxStructure(r, func(h *mp4.ReadHandle) (interface{}, error) {
		switch h.BoxInfo.Type.String() {
		case "moov", "mdia":
			return h.Expand()
		case "trak":
			flush()
			return h.Expand()
		case "hdlr":
			box, _, e := h.ReadPayload()
			if e != nil {
				return nil, e
			}
			trackVideo = box.(*mp4.Hdlr).HandlerType == [4]byte{'v', 'i', 'd', 'e'}
		case "mdhd":
			box, _, e := h.ReadPayload()
			if e != nil {
				return nil, e
			}
			mdhd := box.(*mp4.Mdhd)
			if mdhd.Timescale > 0 {
				trackDuration = float64(mdhd.GetDuration()) / float64(mdhd.Timescale)
			}
		}
		return nil, nil
	})
	flush()
	if err != nil {
		return 0, err
	}
	if result <= 0 || result > relaycommon.MaxTaskDurationSeconds || math.IsNaN(result) || math.IsInf(result, 0) {
		return 0, fmt.Errorf("no measurable video track")
	}
	return result, nil
}

// wanActualQuotaChecked uses only the immutable submission snapshot. The caller
// must pass the clamp to RecalculateTaskQuota for the admin saturation audit.
func wanActualQuotaChecked(task *model.Task) (int, *common.QuotaClamp) {
	bc := task.PrivateData.BillingContext
	if bc == nil || bc.BillingUnit != "output_seconds" || bc.OutputVideoDuration <= 0 || bc.OutputVideoDuration > 30 {
		return 0, nil
	}
	ratio := bc.GroupRatio
	if bc.EffectiveRatio != nil {
		ratio = *bc.EffectiveRatio
	}
	resolution := 1.0
	if value, ok := bc.OtherRatios["resolution"]; ok {
		resolution = value
	}
	// The task snapshot is immutable, but old rows and corrupted provider data
	// still need to be rejected before arithmetic can produce a credit. Turn
	// every invalid factor into NaN so QuotaRoundChecked emits an auditable clamp.
	for _, factor := range []float64{bc.ModelPrice, ratio, bc.OutputVideoDuration} {
		if factor < 0 || math.IsNaN(factor) || math.IsInf(factor, 0) {
			return common.QuotaRoundChecked(math.NaN())
		}
	}
	if resolution <= 0 || math.IsNaN(resolution) || math.IsInf(resolution, 0) {
		return common.QuotaRoundChecked(math.NaN())
	}
	value := bc.ModelPrice * ratio * resolution * bc.OutputVideoDuration * common.QuotaPerUnit
	return common.QuotaRoundChecked(value)
}
