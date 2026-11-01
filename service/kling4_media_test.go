package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type kling4MediaTransport func(*http.Request) (*http.Response, error)

func (f kling4MediaTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func configureKling4MediaServer(t *testing.T, transport kling4MediaTransport) {
	t.Helper()
	configureSSRFTestFetchSetting(t)
	original := ssrfProtectedHTTPClient
	ssrfProtectedHTTPClient = &http.Client{Transport: transport, CheckRedirect: checkProtectedFetchRedirect}
	t.Cleanup(func() { ssrfProtectedHTTPClient = original })
}

func kling4MP4Fixture(videoMilliseconds uint32) []byte {
	box := func(name string, data []byte) []byte {
		b := make([]byte, 8, 8+len(data))
		binary.BigEndian.PutUint32(b, uint32(8+len(data)))
		copy(b[4:], name)
		return append(b, data...)
	}
	track := func(handler string, duration uint32) []byte {
		mdhd := make([]byte, 24)
		binary.BigEndian.PutUint32(mdhd[12:], 1000)
		binary.BigEndian.PutUint32(mdhd[16:], duration)
		hdlr := make([]byte, 25)
		copy(hdlr[8:], handler)
		return box("trak", box("mdia", append(box("mdhd", mdhd), box("hdlr", hdlr)...)))
	}
	return box("moov", append(track("vide", videoMilliseconds), track("soun", 31000)...))
}

func kling4WAVFixture(milliseconds uint32) []byte {
	const sampleRate = 8000
	dataBytes := milliseconds * sampleRate / 1000 * 2
	b := make([]byte, 44+dataBytes)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], 36+dataBytes)
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], 1)
	binary.LittleEndian.PutUint32(b[24:], sampleRate)
	binary.LittleEndian.PutUint32(b[28:], sampleRate*2)
	binary.LittleEndian.PutUint16(b[32:], 2)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], dataBytes)
	return b
}

func TestKling4MediaProbeVideoDurationBounds(t *testing.T) {
	for _, tc := range []struct {
		name         string
		milliseconds uint32
		seconds      float64
		wantError    bool
	}{
		{"fractional video shorter than audio tail", 1250, 1.25, false},
		{"maximum", 30000, 30, false},
		{"above maximum", 30001, 0, true},
		{"zero duration", 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			media := kling4MP4Fixture(tc.milliseconds)
			configureKling4MediaServer(t, func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, http.MethodGet, req.Method)
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(media)), ContentLength: int64(len(media))}, nil
			})
			seconds, err := ProbeKling4Media(context.Background(), "https://8.8.8.8/reference.mp4", "video")
			if tc.wantError {
				require.Error(t, err)
				assert.Zero(t, seconds)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.seconds, seconds)
		})
	}
}

func TestKling4MediaProbeAudioDurationBounds(t *testing.T) {
	for _, tc := range []struct {
		name         string
		milliseconds uint32
		seconds      float64
		wantError    bool
	}{
		{"below minimum", 4999, 0, true},
		{"minimum", 5000, 5, false},
		{"fractional", 7350, 7.35, false},
		{"maximum", 30000, 30, false},
		{"above maximum", 30001, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			media := kling4WAVFixture(tc.milliseconds)
			configureKling4MediaServer(t, func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(media)), ContentLength: int64(len(media))}, nil
			})
			seconds, err := ProbeKling4Media(context.Background(), "https://8.8.8.8/reference-without-extension", "audio")
			if tc.wantError {
				require.ErrorContains(t, err, "duration is outside supported bounds")
				assert.Zero(t, seconds)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.seconds, seconds)
		})
	}
}

func TestKling4MediaProbeRejectsInvalidDownloadedMedia(t *testing.T) {
	for _, tc := range []struct {
		name      string
		kind      string
		path      string
		body      []byte
		wantError string
	}{
		{"HTML masquerading as image", "image", "reference.png", []byte("<!doctype html><html>not an image</html>"), "not a supported image file"},
		{"truncated MP4", "video", "reference.mp4", kling4MP4Fixture(5000)[:20], "cannot measure video duration"},
		{"invalid WAV", "audio", "reference.wav", []byte("not a wave file"), "cannot measure audio duration"},
		{"unsupported audio container", "audio", "reference.ogg", []byte("OggS"), "reference audio must be MP3 or WAV"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configureKling4MediaServer(t, func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"image/png"}}, Body: io.NopCloser(bytes.NewReader(tc.body)), ContentLength: int64(len(tc.body))}, nil
			})
			seconds, err := ProbeKling4Media(context.Background(), "https://8.8.8.8/"+tc.path, tc.kind)
			require.ErrorContains(t, err, tc.wantError)
			assert.Zero(t, seconds)
		})
	}
}

type kling4FailedDownload struct{}

func (kling4FailedDownload) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func TestKling4MediaProbeRejectsFailedDownload(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		body      io.Reader
		wantError string
	}{
		{"upstream HTTP error", http.StatusForbidden, bytes.NewReader(nil), "media download returned HTTP 403"},
		{"truncated download", http.StatusOK, kling4FailedDownload{}, "media download incomplete"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configureKling4MediaServer(t, func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: io.NopCloser(tc.body), ContentLength: -1}, nil
			})
			seconds, err := ProbeKling4Media(context.Background(), "https://8.8.8.8/reference.mp4", "video")
			require.ErrorContains(t, err, tc.wantError)
			assert.Zero(t, seconds)
		})
	}
}

type kling4ZeroReader struct{}

func (kling4ZeroReader) Read(b []byte) (int, error) {
	clear(b)
	return len(b), nil
}

func TestKling4MediaProbeRejectsOversizedMedia(t *testing.T) {
	for _, media := range []struct {
		kind  string
		maxMB int64
	}{
		{"image", 10},
		{"video", 50},
		{"audio", 20},
	} {
		for _, declaredLength := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/declared-length-%t", media.kind, declaredLength), func(t *testing.T) {
				length := media.maxMB*1024*1024 + 1
				configureKling4MediaServer(t, func(req *http.Request) (*http.Response, error) {
					contentLength := int64(-1)
					if declaredLength {
						contentLength = length
					}
					return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(io.LimitReader(kling4ZeroReader{}, length)), ContentLength: contentLength}, nil
				})
				seconds, err := ProbeKling4Media(context.Background(), "https://8.8.8.8/reference", media.kind)
				require.ErrorContains(t, err, fmt.Sprintf("%s exceeds %d MB limit", media.kind, media.maxMB))
				assert.Zero(t, seconds)
			})
		}
	}
}

func TestKling4MediaProbeAcceptsImageAtSizeLimit(t *testing.T) {
	var imageData bytes.Buffer
	require.NoError(t, png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 1, 1))))
	configureKling4MediaServer(t, func(req *http.Request) (*http.Response, error) {
		body := io.MultiReader(bytes.NewReader(imageData.Bytes()), io.LimitReader(kling4ZeroReader{}, 10*1024*1024-int64(imageData.Len())))
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(body), ContentLength: -1}, nil
	})
	seconds, err := ProbeKling4Media(context.Background(), "https://8.8.8.8/reference", "image")
	require.NoError(t, err)
	assert.Zero(t, seconds)
}

func TestKling4MediaProbeBlocksUnsafeURLsBeforeDownload(t *testing.T) {
	for _, rawURL := range []string{
		"http://127.0.0.1/private",
		"http://[::1]/private",
		"http://169.254.169.254/latest/meta-data/",
		"https://8.8.8.8:8443/reference",
		"https://user@8.8.8.8/reference",
		"file:///tmp/reference",
	} {
		t.Run(rawURL, func(t *testing.T) {
			downloaded := false
			configureKling4MediaServer(t, func(req *http.Request) (*http.Response, error) {
				downloaded = true
				return nil, fmt.Errorf("unexpected download")
			})
			_, err := ProbeKling4Media(context.Background(), rawURL, "video")
			require.Error(t, err)
			assert.False(t, downloaded)
		})
	}
}

func TestKling4MediaProbeBlocksPrivateRedirect(t *testing.T) {
	var downloadedURLs []string
	configureKling4MediaServer(t, func(req *http.Request) (*http.Response, error) {
		downloadedURLs = append(downloadedURLs, req.URL.String())
		return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": {"http://127.0.0.1/private"}}, Body: io.NopCloser(bytes.NewReader(nil)), ContentLength: 0}, nil
	})
	_, err := ProbeKling4Media(context.Background(), "https://8.8.8.8/reference.mp4", "video")
	require.ErrorContains(t, err, "media download failed")
	assert.Equal(t, []string{"https://8.8.8.8/reference.mp4"}, downloadedURLs)
}

func TestKling4MediaProbeRespectsPrivateNetworkFetchSetting(t *testing.T) {
	for _, protectionEnabled := range []bool{true, false} {
		t.Run(fmt.Sprintf("ssrf-enabled-%t", protectionEnabled), func(t *testing.T) {
			media := kling4MP4Fixture(4200)
			configureKling4MediaServer(t, func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(media)), ContentLength: int64(len(media))}, nil
			})
			fetchSetting := system_setting.GetFetchSetting()
			fetchSetting.EnableSSRFProtection = protectionEnabled
			fetchSetting.AllowPrivateIp = protectionEnabled
			originalHTTPClient := httpClient
			httpClient = ssrfProtectedHTTPClient
			t.Cleanup(func() { httpClient = originalHTTPClient })

			seconds, err := ProbeKling4Media(context.Background(), "http://127.0.0.1/reference.mp4", "video")
			require.NoError(t, err)
			assert.Equal(t, 4.2, seconds)
		})
	}
}
