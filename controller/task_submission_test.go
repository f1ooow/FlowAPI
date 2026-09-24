package controller

import (
	"context"
	"errors"
	relaykitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type submissionFixtureTransport func(*http.Request) (*http.Response, error)

func (f submissionFixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

const submissionBody = `{"model":"wan3.0-video","input":{"prompt":"private prompt"},"parameters":{"duration":5,"resolution":"720P"}}`

func submissionTestRouter(t *testing.T, upstream http.HandlerFunc) (*gin.Engine, *gorm.DB) {
	t.Helper()
	baseURL := "https://submission-provider.example"
	oldDB, oldLog := model.DB, model.LOG_DB
	oldPrice := ratio_setting.ModelPrice2JSONString()
	oldFree := operation_setting.GetQuotaSetting().EnableFreeModelPreConsume
	operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = false
	oldRedis, oldBatch, oldLogEnabled := common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	model.DB, model.LOG_DB = db, db
	common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled = false, false, false
	require.NoError(t, db.AutoMigrate(&model.TaskSubmission{}, &model.Task{}, &model.User{}, &model.Channel{}))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"wan3.0-video":0}`))
	service.InitHttpClient()
	client := service.GetHttpClient()
	oldTransport := client.Transport
	client.Transport = submissionFixtureTransport(func(r *http.Request) (*http.Response, error) {
		recorder := httptest.NewRecorder()
		upstream.ServeHTTP(recorder, r)
		if recorder.Header().Get("Test-Transport-Error") != "" {
			return nil, io.ErrUnexpectedEOF
		}
		return recorder.Result(), nil
	})
	t.Cleanup(func() { client.Transport = oldTransport })
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLog
		operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = oldFree
		common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled = oldRedis, oldBatch, oldLogEnabled
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrice))
		_ = sqlDB.Close()
	})
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", 1)
		c.Set("token_id", 2)
		if c.GetHeader("Test-User") == "other" {
			c.Set("id", 3)
		}
		if c.GetHeader("Test-Token") == "other" {
			c.Set("token_id", 4)
		}
		c.Next()
	})
	router.POST(dto.WanCreatePath, middleware.TaskSubmission(), func(c *gin.Context) {
		c.Set("channel_type", constant.ChannelTypeSora)
		c.Set("channel_id", 1)
		c.Set("base_url", baseURL)
		common.SetContextKey(c, constant.ContextKeyChannelKey, "test-only")
		c.Set("original_model", "wan3.0-video")
		c.Set("group", "default")
		common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, relaykitdto.ChannelOtherSettings{VideoProtocol: dto.WanProtocolRolldek})
		c.Next()
	}, RelayTask)
	router.GET("/api/v1/task-submissions/:client_key", GetTaskSubmission)
	return router, db
}

func submissionRequest(ctx context.Context, key, body string) *http.Request {
	req := httptest.NewRequest("POST", dto.WanCreatePath, strings.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-DashScope-Async", "enable")
	req.Header.Set("X-Keli-Submission-Key", key)
	return req
}

func TestTaskSubmissionLostResponseConcurrentDuplicateAndLookup(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	router, db := submissionTestRouter(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		close(started)
		<-release
		_, _ = w.Write([]byte(`{"id":"upstream-real-id","status":"queued"}`))
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	completed := make(chan struct{})
	first := httptest.NewRecorder()
	go func() {
		defer close(completed)
		router.ServeHTTP(first, submissionRequest(ctx, "exact-key", submissionBody))
	}()
	select {
	case <-started:
	case <-completed:
		t.Fatalf("submit returned before dispatch: %d %s", first.Code, first.Body.String())
	}
	cancel() // caller leaves before upstream returns the ID
	duplicate := httptest.NewRecorder()
	router.ServeHTTP(duplicate, submissionRequest(context.Background(), "exact-key", submissionBody))
	require.Equal(t, 409, duplicate.Code)
	assert.Contains(t, duplicate.Body.String(), "submission_pending")
	pending, _ := model.GetTaskSubmission(1, 2, "exact-key")
	require.NotNil(t, pending)
	assert.Equal(t, "submitting", pending.Status)
	close(release)
	<-completed
	require.Equal(t, 200, first.Code, first.Body.String())
	accepted, err := model.GetTaskSubmission(1, 2, "exact-key")
	require.NoError(t, err)
	require.Equal(t, "accepted", accepted.Status)
	var task model.Task
	require.NoError(t, db.Where("task_id = ?", accepted.TaskID).First(&task).Error)
	assert.Equal(t, "upstream-real-id", task.PrivateData.UpstreamTaskID)
	assert.Contains(t, first.Body.String(), accepted.TaskID)
	replay := httptest.NewRecorder()
	router.ServeHTTP(replay, submissionRequest(context.Background(), "exact-key", submissionBody))
	require.Equal(t, 200, replay.Code)
	assert.Contains(t, replay.Body.String(), accepted.TaskID)
	assert.EqualValues(t, 1, calls.Load())
	conflict := httptest.NewRecorder()
	router.ServeHTTP(conflict, submissionRequest(context.Background(), "exact-key", strings.ReplaceAll(submissionBody, "private prompt", "different")))
	assert.Equal(t, 409, conflict.Code)
	assert.Contains(t, conflict.Body.String(), "idempotency_conflict")
	assert.EqualValues(t, 1, calls.Load())
	for _, identity := range []string{"", "Test-User", "Test-Token"} {
		req := httptest.NewRequest("GET", "/api/v1/task-submissions/exact-key", nil)
		if identity != "" {
			req.Header.Set(identity, "other")
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if identity == "" {
			assert.Equal(t, 200, rec.Code)
			assert.JSONEq(t, `{"submission_key":"exact-key","task_id":"`+accepted.TaskID+`","status":"accepted"}`, rec.Body.String())
		} else {
			assert.Equal(t, 404, rec.Code)
			assert.NotContains(t, rec.Body.String(), accepted.TaskID)
		}
	}
}

func TestTaskSubmissionAmbiguousTransportNeverRetries(t *testing.T) {
	var calls atomic.Int32
	router, _ := submissionTestRouter(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Test-Transport-Error", "ambiguous accepted response")
	})
	first := httptest.NewRecorder()
	router.ServeHTTP(first, submissionRequest(context.Background(), "lost-upstream", submissionBody))
	require.Equal(t, 500, first.Code)
	state, err := model.GetTaskSubmission(1, 2, "lost-upstream")
	require.NoError(t, err)
	assert.Equal(t, "unknown", state.Status)
	retry := httptest.NewRecorder()
	router.ServeHTTP(retry, submissionRequest(context.Background(), "lost-upstream", submissionBody))
	assert.Equal(t, 409, retry.Code)
	assert.EqualValues(t, 1, calls.Load())
}

func TestTaskSubmissionPersistenceFailureDoesNotAdvertiseSuccess(t *testing.T) {
	router, db := submissionTestRouter(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"accepted-upstream","status":"queued"}`))
	})
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("fail_task_insert", func(tx *gorm.DB) {
		if tx.Statement.Table == "tasks" {
			tx.AddError(errors.New("simulated disk failure"))
		}
	}))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, submissionRequest(context.Background(), "insert-failed", submissionBody))
	assert.Equal(t, 500, rec.Code)
	assert.Contains(t, rec.Body.String(), "task_persistence_failed")
	assert.NotContains(t, rec.Body.String(), "task_id")
	state, err := model.GetTaskSubmission(1, 2, "insert-failed")
	require.NoError(t, err)
	assert.Equal(t, "unknown", state.Status)
	var count int64
	require.NoError(t, db.Model(&model.Task{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestTaskSubmissionDefiniteRejectionDoesNotReplay(t *testing.T) {
	var calls atomic.Int32
	router, _ := submissionTestRouter(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid media"}}`))
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, submissionRequest(context.Background(), "definite-rejection", submissionBody))
	require.Equal(t, 400, rec.Code)
	saved, err := model.GetTaskSubmission(1, 2, "definite-rejection")
	require.NoError(t, err)
	assert.Equal(t, "failed", saved.Status)
	retry := httptest.NewRecorder()
	router.ServeHTTP(retry, submissionRequest(context.Background(), "definite-rejection", submissionBody))
	assert.Equal(t, 409, retry.Code)
	assert.EqualValues(t, 1, calls.Load())
}
