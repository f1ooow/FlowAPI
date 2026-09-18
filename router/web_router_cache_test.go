package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebRouterFallbackCachePolicy(t *testing.T) {
	previousRateLimit := common.GlobalWebRateLimitEnable
	common.GlobalWebRateLimitEnable = false
	t.Cleanup(func() { common.GlobalWebRateLimitEnable = previousRateLimit })

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	indexPage := []byte("<!doctype html><html><body>dashboard</body></html>")
	SetWebRouter(engine, WebAssets{IndexPage: indexPage})

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		for _, target := range []string{
			"/api/missing?group=Claude+%E5%AE%98%E8%BD%AC",
			"/v1/missing",
			"/assets/missing.js",
		} {
			t.Run(method+" "+target, func(t *testing.T) {
				recorder := httptest.NewRecorder()
				engine.ServeHTTP(recorder, httptest.NewRequest(method, target, nil))

				require.Equal(t, http.StatusNotFound, recorder.Code)
				assert.Contains(t, recorder.Header().Get("Cache-Control"), "no-store")
				assert.NotContains(t, recorder.Header().Get("Cache-Control"), "max-age=604800")
			})
		}
	}

	for _, target := range []string{"/", "/channels?group=test"} {
		t.Run("SPA "+target, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))

			require.Equal(t, http.StatusOK, recorder.Code)
			assert.Equal(t, "no-cache", recorder.Header().Get("Cache-Control"))
			assert.Equal(t, string(indexPage), recorder.Body.String())
		})
	}
}
