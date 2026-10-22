package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestChannelMonitoringRequiresChannelReadPermission(t *testing.T) {
	previousDB, previousRedis := model.DB, common.RedisEnabled
	previousMaster, previousLog := common.IsMasterNode, common.LogConsumeEnabled
	previousType := common.MainDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Channel{}, &model.ChannelMetric{}, &model.CasbinRule{}, &model.AuthzRole{}))
	model.DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.RedisEnabled, common.LogConsumeEnabled, common.IsMasterNode = false, false, true
	require.NoError(t, authz.Init(db))
	t.Cleanup(func() {
		model.DB, common.RedisEnabled = previousDB, previousRedis
		common.IsMasterNode, common.LogConsumeEnabled = previousMaster, previousLog
		common.SetMainDatabaseType(previousType)
	})

	pat := "monitoring-read-permission-test"
	user := model.User{Username: "monitor-reader", Role: common.RoleAdminUser,
		Status: common.UserStatusEnabled, Group: "default", AccessToken: &pat}
	require.NoError(t, db.Create(&user).Error)
	engine := gin.New()
	SetApiRouter(engine)

	for _, allowed := range []bool{false, true} {
		require.NoError(t, authz.SetUserPermissions(user.Id, authz.PermissionsMap{
			authz.ResourceChannel: {authz.ActionRead: allowed},
		}))
		for _, target := range []string{"/api/channel-monitoring/summary", "/api/channel-monitoring/totals"} {
			request := httptest.NewRequest(http.MethodGet, target, nil)
			request.Header.Set("Authorization", "Bearer "+pat)
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			expected := http.StatusForbidden
			if allowed {
				expected = http.StatusOK
			}
			assert.Equal(t, expected, response.Code, "%s allowed=%t: %s", target, allowed, response.Body.String())
		}
	}
}
