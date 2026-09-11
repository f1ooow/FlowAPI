package router

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelPriceSyncRoutesAreRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)

	routes := engine.Routes()
	require.NotEmpty(t, routes)

	registered := make(map[string]struct{}, len(routes))
	for _, route := range routes {
		registered[route.Method+" "+route.Path] = struct{}{}
	}

	assert.Contains(t, registered, http.MethodGet+" /api/ratio_sync/channels")
	assert.Contains(t, registered, http.MethodPost+" /api/ratio_sync/fetch")
}
