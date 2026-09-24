package volcvod

import (
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"net/url"
)

// SnapshotMediaSettings is private task metadata, never a client response.
func (a *TaskAdaptor) SnapshotMediaSettings() map[string]string {
	return map[string]string{"vod_playback_domain": a.playbackDomain, "vod_playback_scheme": a.playbackScheme, "vod_url_auth_key": a.urlAuthKey}
}

// SnapshotOtherInfo extracts only public playback configuration. VOD access
// keys and service tokens must never be copied into task private data, logs,
// or client-visible metadata.
func SnapshotOtherInfo(other map[string]interface{}) (string, error) {
	settings := map[string]string{
		"vod_playback_domain": pickOtherInfoString(other, "vod_playback_domain", "vodPlaybackDomain"),
		"vod_playback_scheme": pickOtherInfoString(other, "vod_playback_scheme", "vodPlaybackScheme"),
		"vod_url_auth_key":    pickOtherInfoString(other, "vod_url_auth_key", "vodUrlAuthKey", "vod_auth_key", "urlAuthKey"),
	}
	if settings["vod_playback_scheme"] != "http" && settings["vod_playback_scheme"] != "https" {
		settings["vod_playback_scheme"] = "https"
	}
	returnString, err := common.Marshal(settings)
	return string(returnString), err
}

// RestoreMediaSettings must run after Init when polling an existing task.
func (a *TaskAdaptor) RestoreMediaSettings(settings map[string]string) {
	a.playbackDomain = settings["vod_playback_domain"]
	a.playbackScheme = settings["vod_playback_scheme"]
	if a.playbackScheme == "" {
		a.playbackScheme = "https"
	}
	a.urlAuthKey = settings["vod_url_auth_key"]
}

// ResultMediaURL extracts a downloadable output from the established result
// envelope. AudioExtract supports voice (default) and bg; erase supports video.
func ResultMediaURL(encoded, track string) (string, error) {
	parsed, err := url.Parse(encoded)
	if err != nil {
		return "", err
	}
	switch parsed.Scheme {
	case "vod-result-erase":
		if track == "" {
			track = "video"
		}
		if track != "video" {
			return "", fmt.Errorf("unknown VOD output track")
		}
	case "vod-result-audio":
		if track == "" {
			track = "voice"
		}
		if track != "voice" && track != "bg" {
			return "", fmt.Errorf("unknown VOD output track")
		}
	default:
		return "", fmt.Errorf("VOD output has no playable URL")
	}
	raw := parsed.Query().Get(track)
	target, err := url.Parse(raw)
	if err != nil || target.Host == "" || (target.Scheme != "http" && target.Scheme != "https") || target.User != nil {
		return "", fmt.Errorf("invalid VOD media URL")
	}
	return raw, nil
}
