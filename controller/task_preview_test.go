package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDashboardTaskPreviewAccess(t *testing.T) {
	oldDB, oldRedis, oldCache := model.DB, common.RedisEnabled, common.MemoryCacheEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	model.DB, common.RedisEnabled, common.MemoryCacheEnabled = db, false, false
	t.Cleanup(func() {
		model.DB, common.RedisEnabled, common.MemoryCacheEnabled = oldDB, oldRedis, oldCache
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.Channel{}, &model.User{}, &model.UserSession{}))
	channel := model.Channel{Type: constant.ChannelTypeKling, Key: "provider-secret"}
	require.NoError(t, db.Create(&channel).Error)
	tokens := make(map[string]string)
	users := make(map[string]int)
	for _, name := range []string{"owner", "other", "admin"} {
		role := common.RoleCommonUser
		if name == "admin" {
			role = common.RoleAdminUser
		}
		user := model.User{Username: "preview-" + name, Password: "unused", Role: role, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: name}
		require.NoError(t, db.Create(&user).Error)
		session, err := service.CreateLoginSession(user.Id, "password", "127.0.0.1", "preview-test")
		require.NoError(t, err)
		tokens[name], users[name] = session.AccessToken, user.Id
	}
	for _, taskID := range []string{"result", "legacy", "pending"} {
		task := model.Task{TaskID: taskID, UserId: users["owner"], ChannelId: channel.Id, Status: model.TaskStatusSuccess}
		if taskID == "legacy" {
			task.FailReason = "data:video/mp4;base64,dmlkZW8="
		} else {
			task.PrivateData.ResultURL = "data:video/mp4;base64,dmlkZW8="
		}
		if taskID == "pending" {
			task.Status = model.TaskStatusInProgress
		}
		require.NoError(t, db.Create(&task).Error)
	}
	for _, userID := range []int{users["owner"], users["other"]} {
		task := model.Task{TaskID: "duplicate-legacy-id", UserId: userID, ChannelId: channel.Id, Status: model.TaskStatusSuccess}
		task.PrivateData.ResultURL = "data:video/mp4;base64,dmlkZW8="
		require.NoError(t, db.Create(&task).Error)
	}
	router := gin.New()
	router.GET("/api/task/:task_id/content", middleware.UserAuth(), DashboardTaskContent)
	router.GET("/v1/videos/:task_id/content", middleware.TokenOrUserAuth(), VideoProxy)
	for _, test := range []struct {
		name, user, path string
		status           int
	}{
		{"owner new result", "owner", "/api/task/result/content", http.StatusOK},
		{"owner legacy result", "owner", "/api/task/legacy/content", http.StatusOK},
		{"admin views another user", "admin", "/api/task/result/content", http.StatusOK},
		{"other user denied", "other", "/api/task/result/content", http.StatusNotFound},
		{"anonymous denied", "", "/api/task/result/content", http.StatusUnauthorized},
		{"unfinished denied", "owner", "/api/task/pending/content", http.StatusBadRequest},
		{"missing denied", "owner", "/api/task/missing/content", http.StatusNotFound},
		{"relay remains owner scoped", "admin", "/v1/videos/result/content", http.StatusNotFound},
		{"ambiguous admin lookup denied", "admin", "/api/task/duplicate-legacy-id/content", http.StatusNotFound},
		{"owner lookup isolates duplicate IDs", "owner", "/api/task/duplicate-legacy-id/content", http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			if test.user != "" {
				request.Header.Set("Authorization", "Bearer "+tokens[test.user])
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			require.Equal(t, test.status, recorder.Code, recorder.Body.String())
			if test.status == http.StatusOK {
				assert.Equal(t, "video", recorder.Body.String())
				assert.Equal(t, "video/mp4", recorder.Header().Get("Content-Type"))
				assert.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
			}
		})
	}
}

func TestTaskLogIncludesNewAndLegacyResults(t *testing.T) {
	tasks := []*model.Task{
		{TaskID: "new", Status: model.TaskStatusSuccess, PrivateData: model.TaskPrivateData{ResultURL: "https://media.example/new.mp4"}},
		{TaskID: "legacy", Status: model.TaskStatusSuccess, FailReason: "https://media.example/old.mp4"},
	}
	items := tasksToDto(tasks, false)
	require.Len(t, items, 2)
	assert.Equal(t, "https://media.example/new.mp4", items[0].ResultURL)
	assert.Empty(t, items[0].FailReason)
	assert.Equal(t, "https://media.example/old.mp4", items[1].ResultURL)
}
