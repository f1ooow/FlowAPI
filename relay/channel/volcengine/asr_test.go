package volcengine

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type asrRecordingTransport struct {
	requests []*http.Request
}

func (t *asrRecordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.requests = append(t.requests, req)
	header := http.Header{}
	header.Set("X-Api-Status-Code", asrStatusSuccess)
	return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
}

// The file ASR protocol is only verified end to end without X-Api-Sequence;
// that header is the streaming-only sequence marker.
func TestASRSubmitAndQueryHeaders(t *testing.T) {
	transport := &asrRecordingTransport{}
	original := asrHTTPClient
	asrHTTPClient = &http.Client{Transport: transport}
	t.Cleanup(func() { asrHTTPClient = original })

	taskID, err := asrSubmit(context.Background(), "access-token", "app-id", "https://example.com/audio.mp3", &asrContext{Language: "zh-CN"}, "mp3")
	require.NoError(t, err)
	status, _, statusCode, err := asrQueryOnce(context.Background(), "access-token", "app-id", taskID)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, asrStatusSuccess, statusCode)

	require.Len(t, transport.requests, 2)
	tests := []struct {
		name     string
		endpoint string
	}{
		{name: "submit", endpoint: asrSubmitEndpoint},
		{name: "query", endpoint: asrQueryEndpoint},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := transport.requests[i]
			assert.Equal(t, tt.endpoint, req.URL.String())
			_, hasSequence := req.Header["X-Api-Sequence"]
			assert.False(t, hasSequence)
			assert.Equal(t, "access-token", req.Header.Get("X-Api-Access-Key"))
			assert.Equal(t, "app-id", req.Header.Get("X-Api-App-Key"))
			assert.Equal(t, "volc.bigasr.auc", req.Header.Get("X-Api-Resource-Id"))
			assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
			assert.NotEmpty(t, taskID)
			assert.Equal(t, taskID, req.Header.Get("X-Api-Request-Id"))
		})
	}
}
