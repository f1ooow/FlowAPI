package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Upstream response headers identify the provider behind a channel (for
// example anthropic-organization-id, request-id formats, Cloudflare markers).
// Clients must only receive the headers needed to decode the body, matching
// what the streaming path already sends.
func TestIOCopyBytesGracefullyForwardsOnlyBodyHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	upstream := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":                           {"application/json"},
			"Content-Encoding":                       {"br"},
			"Content-Disposition":                    {`attachment; filename="speech.mp3"`},
			"X-Codex-Turn-State":                     {"turn-state"},
			"X-Reasoning-Included":                   {"true"},
			"Content-Length":                         {"999"},
			"X-Oneapi-Request-Id":                    {"upstream-oneapi-id"},
			"Anthropic-Organization-Id":              {"org_zivv"},
			"Request-Id":                             {"req_902581bb"},
			"X-Request-Id":                           {"req_902581bb"},
			"X-Client-Request-Id":                    {"417e9c1f-70dd"},
			"Cf-Ray":                                 {"a4103245cc48ed89-HKG"},
			"Cf-Cache-Status":                        {"DYNAMIC"},
			"Nel":                                    {`{"report_to":"cf-nel"}`},
			"Report-To":                              {`{"group":"cf-nel"}`},
			"Alt-Svc":                                {`h3=":443"; ma=86400`},
			"Server":                                 {"cloudflare"},
			"Set-Cookie":                             {"__cf_bm=abc"},
			"Access-Control-Allow-Origin":            {"*"},
			"Access-Control-Expose-Headers":          {"request-id, anthropic-organization-id"},
			"Anthropic-Ratelimit-Requests-Remaining": {"42"},
			"X-Frame-Options":                        {"DENY"},
		},
	}

	IOCopyBytesGracefully(c, upstream, []byte(`{"ok":true}`))

	got := recorder.Header()
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "application/json", got.Get("Content-Type"))
	assert.Equal(t, "br", got.Get("Content-Encoding"))
	assert.Equal(t, `attachment; filename="speech.mp3"`, got.Get("Content-Disposition"))
	assert.Equal(t, "turn-state", got.Get("X-Codex-Turn-State"))
	assert.Equal(t, "true", got.Get("X-Reasoning-Included"))
	assert.Equal(t, "11", got.Get("Content-Length"))
	assert.Empty(t, got.Values("X-Oneapi-Request-Id"))

	for _, name := range []string{
		"Anthropic-Organization-Id",
		"Request-Id",
		"X-Request-Id",
		"X-Client-Request-Id",
		"Cf-Ray",
		"Cf-Cache-Status",
		"Nel",
		"Report-To",
		"Alt-Svc",
		"Server",
		"Set-Cookie",
		"Access-Control-Allow-Origin",
		"Access-Control-Expose-Headers",
		"Anthropic-Ratelimit-Requests-Remaining",
		"X-Frame-Options",
	} {
		assert.Empty(t, got.Values(name), "upstream header %s must not reach the client", name)
	}

	upstreamRequestID, ok := c.Get(common.UpstreamRequestIdKey)
	require.True(t, ok)
	assert.Equal(t, "upstream-oneapi-id", upstreamRequestID)
	assert.Equal(t, `{"ok":true}`, recorder.Body.String())
}

func TestShouldCopyUpstreamHeaderIsCaseInsensitive(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	assert.True(t, ShouldCopyUpstreamHeader(c, "content-type", []string{"audio/mpeg"}))
	assert.True(t, ShouldCopyUpstreamHeader(c, "x-codex-turn-state", []string{"state"}))
	assert.False(t, ShouldCopyUpstreamHeader(c, "anthropic-organization-id", []string{"org_zivv"}))
	assert.False(t, ShouldCopyUpstreamHeader(c, "content-length", []string{"10"}))
}
