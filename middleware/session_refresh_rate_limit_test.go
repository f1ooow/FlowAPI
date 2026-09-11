package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	firstRefreshSessionID  = "11111111-1111-4111-8111-111111111111"
	secondRefreshSessionID = "22222222-2222-4222-8222-222222222222"
)

// useAuthRateLimitSettings pins the refresh and critical budgets so the test
// asserts a known configuration instead of whatever a previous test left in
// the shared package-level variables.
func useAuthRateLimitSettings(t *testing.T, refreshNum, refreshIPNum, criticalNum int) {
	t.Helper()

	previous := struct {
		refreshEnable    bool
		refreshNum       int
		refreshIPNum     int
		refreshDuration  int64
		criticalEnable   bool
		criticalNum      int
		criticalDuration int64
	}{
		common.SessionRefreshRateLimitEnable,
		common.SessionRefreshRateLimitNum,
		common.SessionRefreshIPRateLimitNum,
		common.SessionRefreshRateLimitDuration,
		common.CriticalRateLimitEnable,
		common.CriticalRateLimitNum,
		common.CriticalRateLimitDuration,
	}
	t.Cleanup(func() {
		common.SessionRefreshRateLimitEnable = previous.refreshEnable
		common.SessionRefreshRateLimitNum = previous.refreshNum
		common.SessionRefreshIPRateLimitNum = previous.refreshIPNum
		common.SessionRefreshRateLimitDuration = previous.refreshDuration
		common.CriticalRateLimitEnable = previous.criticalEnable
		common.CriticalRateLimitNum = previous.criticalNum
		common.CriticalRateLimitDuration = previous.criticalDuration
	})

	common.SessionRefreshRateLimitEnable = true
	common.SessionRefreshRateLimitNum = refreshNum
	common.SessionRefreshIPRateLimitNum = refreshIPNum
	common.SessionRefreshRateLimitDuration = 20 * 60
	common.CriticalRateLimitEnable = true
	common.CriticalRateLimitNum = criticalNum
	common.CriticalRateLimitDuration = 20 * 60
}

// newAuthRateLimitRouter mirrors the production wiring of the two endpoints
// that used to share one counter.
func newAuthRateLimitRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.POST("/api/user/auth/refresh", SessionRefreshRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	router.POST("/api/user/login", CriticalRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	return router
}

func performRefreshRequest(router http.Handler, remoteAddr string, sessionID string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/user/auth/refresh", nil)
	request.RemoteAddr = remoteAddr
	if sessionID != "" {
		request.AddCookie(&http.Cookie{Name: service.RefreshCookieName, Value: sessionID + ".secret"})
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func performLoginRequest(router http.Handler, remoteAddr string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/user/login", nil)
	request.RemoteAddr = remoteAddr
	router.ServeHTTP(recorder, request)
	return recorder
}

// A dashboard page load refreshes the access token, so the refresh budget must
// be well above the critical (login) budget that used to be shared with it.
func TestSessionRefreshSurvivesManyPageLoadsOnShippedSettings(t *testing.T) {
	useRateLimitMiniRedis(t)
	// The shipped defaults from common/constants.go.
	useAuthRateLimitSettings(t, 120, 1200, 20)
	router := newAuthRateLimitRouter(t)

	for range common.CriticalRateLimitNum + 10 {
		assert.Equal(t, http.StatusNoContent, performRefreshRequest(router, "192.0.2.70:12345", firstRefreshSessionID).Code)
	}
}

// This is the regression that locked users out of the dashboard: refresh and
// login shared one Redis counter, so reloading the page enough times made
// login answer 429 as well.
func TestExhaustedSessionRefreshBudgetLeavesLoginUsable(t *testing.T) {
	redisServer, _ := useRateLimitMiniRedis(t)
	useAuthRateLimitSettings(t, 3, 10, 5)
	router := newAuthRateLimitRouter(t)

	const remoteAddr = "192.0.2.71:12345"
	for range common.SessionRefreshRateLimitNum {
		assert.Equal(t, http.StatusNoContent, performRefreshRequest(router, remoteAddr, firstRefreshSessionID).Code)
	}
	assert.Equal(t, http.StatusTooManyRequests, performRefreshRequest(router, remoteAddr, firstRefreshSessionID).Code)

	criticalKey := redisIPRateLimitKey("CT", "192.0.2.71")
	assert.False(t, redisServer.Exists(criticalKey), "refresh must not consume the login counter")

	// Login keeps its own untouched budget, with the unchanged threshold.
	for range common.CriticalRateLimitNum {
		assert.Equal(t, http.StatusNoContent, performLoginRequest(router, remoteAddr).Code)
	}
	assert.Equal(t, http.StatusTooManyRequests, performLoginRequest(router, remoteAddr).Code)

	criticalCount, err := redisServer.Get(criticalKey)
	require.NoError(t, err)
	assert.Equal(t, "6", criticalCount)
}

func TestSessionRefreshBudgetIsPerSessionAndCappedPerIP(t *testing.T) {
	redisServer, _ := useRateLimitMiniRedis(t)
	useAuthRateLimitSettings(t, 2, 5, 20)
	router := newAuthRateLimitRouter(t)

	const remoteAddr = "192.0.2.72:12345"
	for range 2 {
		assert.Equal(t, http.StatusNoContent, performRefreshRequest(router, remoteAddr, firstRefreshSessionID).Code)
	}
	assert.Equal(t, http.StatusTooManyRequests, performRefreshRequest(router, remoteAddr, firstRefreshSessionID).Code)

	// Same egress IP (office NAT), different login session: independent budget.
	for range 2 {
		assert.Equal(t, http.StatusNoContent, performRefreshRequest(router, remoteAddr, secondRefreshSessionID).Code)
	}

	// The session id comes from a client-controlled cookie, so the IP counter
	// still bounds a client that rotates session ids.
	assert.Equal(t, http.StatusNoContent, performRefreshRequest(router, remoteAddr, "33333333-3333-4333-8333-333333333333").Code)
	assert.Equal(t, http.StatusTooManyRequests, performRefreshRequest(router, remoteAddr, "44444444-4444-4444-8444-444444444444").Code)

	// A different IP is unaffected by the exhausted IP counter.
	assert.Equal(t, http.StatusNoContent, performRefreshRequest(router, "198.51.100.72:12345", "55555555-5555-4555-8555-555555555555").Code)

	assert.True(t, redisServer.Exists(redisSessionRateLimitKey(sessionRefreshSessionRateLimitMark, firstRefreshSessionID)))
	assert.True(t, redisServer.Exists(redisIPRateLimitKey(sessionRefreshIPRateLimitMark, "192.0.2.72")))
}

// Without a parseable refresh cookie there is no session to key on, so the
// request falls back to the IP counter only.
func TestSessionRefreshWithoutCookieConsumesOnlyIPBudget(t *testing.T) {
	redisServer, _ := useRateLimitMiniRedis(t)
	useAuthRateLimitSettings(t, 2, 3, 20)
	router := newAuthRateLimitRouter(t)

	const remoteAddr = "192.0.2.73:12345"
	for range 3 {
		assert.Equal(t, http.StatusNoContent, performRefreshRequest(router, remoteAddr, "").Code)
	}
	limited := performRefreshRequest(router, remoteAddr, "")
	assert.Equal(t, http.StatusTooManyRequests, limited.Code)
	assert.Equal(t, "1200", limited.Header().Get("Retry-After"))

	ipCount, err := redisServer.Get(redisIPRateLimitKey(sessionRefreshIPRateLimitMark, "192.0.2.73"))
	require.NoError(t, err)
	assert.Equal(t, "4", ipCount)
}

func TestSessionRefreshRateLimitCanBeDisabled(t *testing.T) {
	redisServer, _ := useRateLimitMiniRedis(t)
	useAuthRateLimitSettings(t, 1, 1, 20)
	common.SessionRefreshRateLimitEnable = false
	router := newAuthRateLimitRouter(t)

	for range 3 {
		assert.Equal(t, http.StatusNoContent, performRefreshRequest(router, "192.0.2.74:12345", firstRefreshSessionID).Code)
	}
	assert.False(t, redisServer.Exists(redisIPRateLimitKey(sessionRefreshIPRateLimitMark, "192.0.2.74")))
}
