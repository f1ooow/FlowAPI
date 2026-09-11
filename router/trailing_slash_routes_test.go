package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newFullRouter builds the same route tree the server serves, minus the frontend
// asset handlers. The tree shape is what matters: gin's trailing-slash redirect
// depends on how every registered path splits the shared radix tree, so a partial
// router would not reproduce production routing behaviour.
func newFullRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)
	SetDashboardRouter(engine)
	SetRelayRouter(engine)
	SetVideoRouter(engine)
	return engine
}

func routeStatus(t *testing.T, engine *gin.Engine, method string, target string) int {
	t.Helper()
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(method, target, nil))
	return recorder.Code
}

// TestApiRouteGroupRootsResolveWithoutTrailingSlash locks in that every registered
// group root ("/api/x/") is also reachable as "/api/x".
//
// gin only recommends a trailing-slash redirect when the radix walk ends on the
// node that owns the handler. Two conditions defeat that: a sibling route sharing
// the group prefix (e.g. /api/channel-monitoring vs /api/channel) splits the node
// at "channel" leaving it handler-less, and the root-level /:mode/mj wildcard makes
// the walk backtrack into the wildcard instead of returning the redirect hint. The
// result is a hard 404 on the bare path. Callers and cached frontend bundles do send
// the bare path, so this must keep working.
func TestApiRouteGroupRootsResolveWithoutTrailingSlash(t *testing.T) {
	engine := newFullRouter(t)
	routes := engine.Routes()
	require.NotEmpty(t, routes)

	type routeKey struct {
		method string
		path   string
	}
	checked := make(map[routeKey]struct{})
	for _, route := range routes {
		if route.Path == "/" || !strings.HasSuffix(route.Path, "/") {
			continue
		}
		bare := strings.TrimSuffix(route.Path, "/")
		if strings.ContainsAny(bare, ":*") {
			continue
		}
		key := routeKey{method: route.Method, path: bare}
		if _, seen := checked[key]; seen {
			continue
		}
		checked[key] = struct{}{}

		assert.NotEqual(t, http.StatusNotFound, routeStatus(t, engine, key.method, key.path),
			"%s %s must resolve (directly or via trailing-slash redirect), got 404", key.method, key.path)
	}
	require.NotEmpty(t, checked)
}

// TestChannelRoutesResolve covers the exact paths that regressed in production plus
// the sibling channel routes that must not be shadowed by the fix.
func TestChannelRoutesResolve(t *testing.T) {
	engine := newFullRouter(t)

	for _, target := range []string{
		"/api/channel",
		"/api/channel/",
		"/api/channel?group=test&tag_mode=false&id_sort=false&p=1&page_size=20",
		"/api/channel/passthrough",
		"/api/channel/search",
		"/api/channel/models",
		"/api/channel/1",
		"/api/channel-monitoring/summary",
		"/api/group",
		"/api/group/",
		"/api/user",
		"/api/user/",
		"/api/user-agreement",
	} {
		assert.NotEqual(t, http.StatusNotFound, routeStatus(t, engine, http.MethodGet, target),
			"GET %s must resolve, got 404", target)
	}
}
