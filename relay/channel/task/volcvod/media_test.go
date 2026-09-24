package volcvod

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestVODResultMediaURLRejectsUnsafeAndSelectsTracks(t *testing.T) {
	erase, _ := ResultMediaURL("vod-result-erase:///?video=https%3A%2F%2Fcdn.example%2Fv.mp4&duration=2", "video")
	assert.Equal(t, "https://cdn.example/v.mp4", erase)
	voice, err := ResultMediaURL("vod-result-audio:///?voice=https%3A%2F%2Fcdn.example%2Fv.aac&bg=https%3A%2F%2Fcdn.example%2Fb.aac", "voice")
	require.NoError(t, err)
	assert.Equal(t, "https://cdn.example/v.aac", voice)
	_, err = ResultMediaURL("vod-result-audio:///?voice=http%3A%2F%2Fbad", "voice")
	require.NoError(t, err)
	_, err = ResultMediaURL("vod-result-audio:///?voice=file%3A%2F%2F%2Fetc%2Fpasswd", "voice")
	require.Error(t, err)
	_, err = ResultMediaURL("vod-result-audio:///?voice=https%3A%2F%2Fcdn.example%2Fv.aac", "unknown")
	require.Error(t, err)
}
func TestVODUploadRejectsBoundsAndMissingCredentials(t *testing.T) {
	_, err := UploadMediaToVOD(nil, VodUploadOpts{}, "audio/mpeg")
	require.Error(t, err)
	_, err = UploadMediaToVOD([]byte("audio"), VodUploadOpts{SpaceName: "s"}, "audio/mpeg")
	require.Error(t, err)
	_, err = UploadMediaToVOD(make([]byte, VodSinglePutLimitBytes+1), VodUploadOpts{SpaceName: "s", AccessKey: "ak", SecretKey: "sk"}, "audio/mpeg")
	require.Error(t, err)
}
