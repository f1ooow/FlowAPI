package hailuo

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestH3PollingRefundsDeliveredResolutionOnce(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.User{}, &model.Token{}, &model.Channel{}, &model.Log{}))
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	oldRedis, oldMemory, oldBatch := common.RedisEnabled, common.MemoryCacheEnabled, common.BatchUpdateEnabled
	oldFactory := service.GetTaskAdaptorFunc
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMainType, oldLogType)
		common.RedisEnabled, common.MemoryCacheEnabled, common.BatchUpdateEnabled = oldRedis, oldMemory, oldBatch
		service.GetTaskAdaptorFunc = oldFactory
	})
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled, common.MemoryCacheEnabled, common.BatchUpdateEnabled = false, false, false
	service.GetTaskAdaptorFunc = func(constant.TaskPlatform) service.TaskPollingAdaptor { return &TaskAdaptor{} }

	service.InitHttpClient()
	client := service.GetHttpClient()
	oldTransport := client.Transport
	t.Cleanup(func() { client.Transport = oldTransport })
	client.Transport = hailuoFixtureTransport(func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, "https://fixture.example/v2/query/video_generation/upstream-h3", r.URL.String())
		assert.Equal(t, "Bearer fixture-key", r.Header.Get("Authorization"))
		body := `{"result":{"task":{"id":"upstream-h3","status":"succeeded","resolution":"768P","content":{"url":"https://cdn.example/video.mp4"},"usage":{"input_seconds":3,"output_seconds":5}}}}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})

	// Reserved: (20s × 0.175 + 4 images × 0.0625) × 1.2 = 4.5.
	// Delivered: (8s × 0.1125 + 4 images × 0.0625) × 1.2 = 1.38.
	const remaining, reserved, actual = 10000000, 2250000, 690000
	user := model.User{Id: 1, Username: "h3-fixture", Quota: remaining, UsedQuota: reserved, RequestCount: 1, Status: common.UserStatusEnabled}
	token := model.Token{Id: 2, UserId: 1, Key: "fixture-token", RemainQuota: remaining, UsedQuota: reserved, Status: common.TokenStatusEnabled}
	baseURL := "https://fixture.example"
	channel := model.Channel{Id: 3, Type: constant.ChannelTypeMiniMax, Key: "fixture-key", BaseURL: &baseURL, UsedQuota: reserved, Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&token).Error)
	require.NoError(t, db.Create(&channel).Error)
	ratio := 1.2
	task := &model.Task{
		TaskID: "public-h3", UserId: 1, ChannelId: 3, Quota: reserved, Status: model.TaskStatusInProgress, SubmitTime: time.Now().Unix(),
		Properties: model.Properties{OriginModelName: ModelMiniMaxH3, UpstreamModelName: ModelMiniMaxH3},
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "upstream-h3", TokenId: 2, BillingSource: service.BillingSourceWallet,
			BillingContext: &model.TaskBillingContext{
				BillingUnit: H3MediaBillingUnit, PerCallBilling: true, ModelPrice: 0.1125, GroupRatio: ratio, EffectiveRatio: &ratio,
				InputVideoDuration: 15, InputImageCount: 4, InputImagePrice: 0.0625,
				OtherRatios: map[string]float64{"hailuo-h3-duration": 20, "hailuo-h3-resolution": 14.0 / 9.0},
			},
		},
	}
	require.NoError(t, task.Insert())
	var staleTask model.Task
	require.NoError(t, db.First(&staleTask, task.ID).Error)
	for _, polled := range []*model.Task{task, &staleTask, task} {
		require.NoError(t, service.UpdateVideoTasks(context.Background(), constant.TaskPlatform("hailuo"),
			map[int][]string{3: {"upstream-h3"}}, map[string]*model.Task{"upstream-h3": polled}))
	}
	var stored model.Task
	require.NoError(t, db.First(&stored, task.ID).Error)
	require.NoError(t, db.First(&user, user.Id).Error)
	require.NoError(t, db.First(&token, token.Id).Error)
	require.NoError(t, db.First(&channel, channel.Id).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), stored.Status)
	assert.Equal(t, "https://cdn.example/video.mp4", stored.GetResultURL())
	assert.Equal(t, actual, stored.Quota)
	assert.Equal(t, remaining+reserved-actual, user.Quota)
	assert.Equal(t, remaining+reserved-actual, token.RemainQuota)
	assert.Equal(t, actual, user.UsedQuota)
	assert.Equal(t, actual, token.UsedQuota)
	assert.Equal(t, int64(actual), channel.UsedQuota)
	assert.Equal(t, 1, user.RequestCount)
	assert.Equal(t, 14.0/9.0, stored.PrivateData.BillingContext.OtherRatios["hailuo-h3-resolution"])
	var logs []model.Log
	require.NoError(t, db.Find(&logs).Error)
	require.Len(t, logs, 1, "repeated and stale polls must not refund twice")
	assert.Equal(t, model.LogTypeRefund, logs[0].Type)
	assert.Equal(t, reserved-actual, logs[0].Quota)
}
