package service

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"github.com/gin-gonic/gin"
)

func CloseResponseBodyGracefully(httpResponse *http.Response) {
	if httpResponse == nil || httpResponse.Body == nil {
		return
	}
	err := httpResponse.Body.Close()
	if err != nil {
		common.SysError("failed to close response body: " + err.Error())
	}
}

// clientVisibleUpstreamHeaders lists the only upstream response headers a
// client may receive: the ones needed to decode the body, plus the Codex turn
// headers the streaming path already forwards. Everything else (request ids,
// organization ids, rate limits, CDN and CORS headers) identifies the provider
// behind a channel and stays on this side. Content-Length is managed by the
// writer; Content-Encoding only survives when the transport left the body
// encoded, so it must travel with the bytes.
var clientVisibleUpstreamHeaders = map[string]struct{}{
	"content-type":         {},
	"content-encoding":     {},
	"content-disposition":  {},
	"x-codex-turn-state":   {},
	"x-reasoning-included": {},
}

// ShouldCopyUpstreamHeader reports whether an upstream response header may be
// copied to the client response. When the upstream header is
// X-Oneapi-Request-Id, the value is captured into the Gin context for later
// logging instead of being forwarded, preserving the local instance ID.
func ShouldCopyUpstreamHeader(c *gin.Context, k string, v []string) bool {
	if strings.EqualFold(k, common.RequestIdKey) {
		if c != nil && len(v) > 0 {
			c.Set(common.UpstreamRequestIdKey, v[0])
		}
		return false
	}
	_, ok := clientVisibleUpstreamHeaders[strings.ToLower(k)]
	return ok
}

func IOCopyBytesGracefully(c *gin.Context, src *http.Response, data []byte) {
	if c.Writer == nil {
		return
	}

	body := io.NopCloser(bytes.NewBuffer(data))

	// We shouldn't set the header before we parse the response body, because the parse part may fail.
	// And then we will have to send an error response, but in this case, the header has already been set.
	// So the httpClient will be confused by the response.
	// For example, Postman will report error, and we cannot check the response at all.
	if src != nil {
		for k, v := range src.Header {
			if !ShouldCopyUpstreamHeader(c, k, v) {
				continue
			}
			c.Writer.Header().Set(k, v[0])
		}
	}

	// set Content-Length header manually BEFORE calling WriteHeader
	c.Writer.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))

	// Write header with status code (this sends the headers)
	if src != nil {
		c.Writer.WriteHeader(src.StatusCode)
	} else {
		c.Writer.WriteHeader(http.StatusOK)
	}

	_, err := io.Copy(c.Writer, body)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("failed to copy response body: %s", err.Error()))
	}
	c.Writer.Flush()
}
