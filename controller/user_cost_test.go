package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupUserCostControllerDB(t *testing.T) (*gorm.DB, *gorm.DB) {
	t.Helper()
	db := setupManageUserTestDB(t)
	t.Setenv("LOG_SQL_DSN", "")
	require.NoError(t, model.InitLogDB())
	previousLogConsumeEnabled := common.LogConsumeEnabled
	common.LogConsumeEnabled = true
	dsn := fmt.Sprintf("file:%s_logs?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	logDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, logDB.AutoMigrate(&model.Log{}))
	model.LOG_DB = logDB
	t.Cleanup(func() {
		common.LogConsumeEnabled = previousLogConsumeEnabled
		sqlDB, err := logDB.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db, logDB
}

type userCostListResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Total int `json:"total"`
		Items []struct {
			model.User
			CostQuota      *int64 `json:"cost_quota"`
			CostIncomplete bool   `json:"cost_incomplete"`
		} `json:"items"`
	} `json:"data"`
}

func requestUserCostList(t *testing.T, handler gin.HandlerFunc, target string) userCostListResponse {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, target, nil)
	handler(c)
	require.Equal(t, http.StatusOK, recorder.Code)
	var response userCostListResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func TestAdminUserListsReturnPageScopedHistoricalCost(t *testing.T) {
	db, logDB := setupUserCostControllerDB(t)
	require.NoError(t, db.Create(&[]model.User{
		{Id: 71, Username: "payer-one", AffCode: "one", Password: "secret", Quota: 1000, UsedQuota: 70},
		{Id: 72, Username: "payer-two", AffCode: "two", Password: "secret", Quota: 2000, UsedQuota: 80},
		{Id: 73, Username: "idle", AffCode: "three", Password: "secret", Quota: 3000, UsedQuota: 90},
	}).Error)
	require.NoError(t, logDB.Create(&[]model.Log{
		{UserId: 71, Username: "previous-username", ChannelId: 999, Type: model.LogTypeConsume, Quota: 70, BaseQuota: 8, ChannelCostRatio: 5_000},
		{UserId: 72, ChannelId: 998, Type: model.LogTypeConsume, Quota: 80, BaseQuota: 100, ChannelCostRatio: 2_500},
		{UserId: 72, ChannelId: 998, Type: model.LogTypeConsume, Quota: 10, BaseQuota: 100, ChannelCostRatio: 10_000, ChannelCostRatioEstimated: true},
	}).Error)
	queries := 0
	require.NoError(t, logDB.Callback().Row().Before("gorm:row").Register("test:count_user_cost_queries", func(tx *gorm.DB) {
		queries++
	}))

	tests := []struct {
		name    string
		handler gin.HandlerFunc
		target  string
		total   int
		ids     []int
		costs   []int64
	}{
		{"list first page", GetAllUsers, "/api/user/?p=1&page_size=2", 3, []int{73, 72}, []int64{0, 25}},
		{"list second page", GetAllUsers, "/api/user/?p=2&page_size=2", 3, []int{71}, []int64{4}},
		{"search", SearchUsers, "/api/user/search?keyword=payer&group=default&role=1&status=1", 2, []int{72, 71}, []int64{25, 4}},
		{"search second page", SearchUsers, "/api/user/search?keyword=payer&p=2&page_size=1", 2, []int{71}, []int64{4}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			queries = 0
			response := requestUserCostList(t, test.handler, test.target)
			require.True(t, response.Success)
			assert.Equal(t, test.total, response.Data.Total)
			require.Len(t, response.Data.Items, len(test.ids))
			for i, item := range response.Data.Items {
				assert.Equal(t, test.ids[i], item.Id)
				require.NotNil(t, item.CostQuota)
				assert.Equal(t, test.costs[i], *item.CostQuota)
				assert.Equal(t, item.Id == 72, item.CostIncomplete)
				assert.Equal(t, (item.Id-70)*1000, item.Quota)
				assert.Equal(t, (item.Id-64)*10, item.UsedQuota)
				assert.Empty(t, item.Password)
			}
			assert.Equal(t, 1, queries, "one aggregate query serves the whole page")
		})
	}
}

func TestAdminUserCostAvailabilityDoesNotBreakUserManagement(t *testing.T) {
	handlers := []struct {
		name    string
		handler gin.HandlerFunc
	}{
		{"list", GetAllUsers},
		{"search", SearchUsers},
	}
	for _, handler := range handlers {
		for _, mode := range []string{"no history", "logging disabled", "log query fails", "empty page", "user query fails"} {
			t.Run(handler.name+"/"+mode, func(t *testing.T) {
				db, logDB := setupUserCostControllerDB(t)
				require.NoError(t, db.Create(&model.User{Id: 81, Username: "cost-user", AffCode: "cost-user", Password: "secret"}).Error)
				target := "/api/user/?keyword=cost-user"
				switch mode {
				case "logging disabled":
					common.LogConsumeEnabled = false
					require.NoError(t, logDB.Migrator().DropTable("logs"))
				case "log query fails":
					require.NoError(t, logDB.Migrator().DropTable("logs"))
				case "empty page":
					target += "&p=2&page_size=1"
				case "user query fails":
					require.NoError(t, db.Migrator().DropTable("users"))
				}
				queries := 0
				require.NoError(t, logDB.Callback().Row().Before("gorm:row").Register("test:count_user_cost_queries", func(tx *gorm.DB) {
					queries++
				}))
				response := requestUserCostList(t, handler.handler, target)
				if mode == "user query fails" {
					assert.False(t, response.Success, "main user query errors retain the existing API error contract")
					assert.Zero(t, queries)
					return
				}
				require.True(t, response.Success)
				if mode == "empty page" {
					assert.Empty(t, response.Data.Items)
					assert.Equal(t, 1, response.Data.Total)
					assert.Zero(t, queries)
					return
				}
				require.Len(t, response.Data.Items, 1)
				item := response.Data.Items[0]
				if mode == "no history" {
					require.NotNil(t, item.CostQuota)
					assert.Zero(t, *item.CostQuota)
					assert.False(t, item.CostIncomplete)
				} else {
					assert.Nil(t, item.CostQuota, "unavailable is not a measured zero cost")
					assert.True(t, item.CostIncomplete)
				}
				if mode == "logging disabled" {
					assert.Zero(t, queries)
				} else {
					assert.Equal(t, 1, queries)
				}
			})
		}
	}
}

func TestUserCostFieldsRemainExclusiveToAdminLists(t *testing.T) {
	db, logDB := setupUserCostControllerDB(t)
	user := model.User{Id: 91, Username: "ordinary-user", AffCode: "ordinary-user", Password: "secret", Quota: 100, UsedQuota: 40}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, logDB.Create(&model.Log{UserId: user.Id, Type: model.LogTypeConsume, Quota: 40, BaseQuota: 80, ChannelCostRatio: 5_000}).Error)
	adminList := requestUserCostList(t, GetAllUsers, "/api/user/")
	require.Len(t, adminList.Data.Items, 1)
	require.NotNil(t, adminList.Data.Items[0].CostQuota)
	assert.EqualValues(t, 40, *adminList.Data.Items[0].CostQuota)

	for _, handler := range []struct {
		name    string
		handler gin.HandlerFunc
		role    int
	}{
		{"self", GetSelf, common.RoleCommonUser},
		{"detail", GetUser, common.RoleRootUser},
	} {
		t.Run(handler.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/user/91", nil)
			c.Set("id", user.Id)
			c.Set("role", handler.role)
			c.Params = gin.Params{{Key: "id", Value: "91"}}
			handler.handler(c)
			require.Equal(t, http.StatusOK, recorder.Code)
			var response struct {
				Success bool                   `json:"success"`
				Data    map[string]interface{} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.True(t, response.Success)
			assert.NotContains(t, response.Data, "cost_quota")
			assert.NotContains(t, response.Data, "cost_incomplete")
			assert.EqualValues(t, 100, response.Data["quota"])
			assert.EqualValues(t, 40, response.Data["used_quota"])
		})
	}
}
