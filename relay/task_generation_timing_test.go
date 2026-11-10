package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var videoTimingTestChannelSequence atomic.Int64

func TestVideoPollGenerationTimingContract(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previousDB, previousType, previousRedis := model.DB, common.MainDatabaseType(), common.RedisEnabled
	model.DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetMainDatabaseType(previousType)
		common.RedisEnabled = previousRedis
		_ = sqlDB.Close()
	})
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.Channel{}))
	for _, tc := range []struct {
		name, path, origin string
		samples, dbFailure bool
	}{
		{"generic", "/task/query/", "MiniMax-H3", true, false},
		{"openai", "/v1/videos/", "MiniMax-H3", true, false},
		{"wan", "/api/v1/tasks/", "wan3.0-video", true, false},
		{"empty", "/task/query/", "MiniMax-H3", false, false},
		{"statistics_failure", "/task/query/", "MiniMax-H3", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			task := &model.Task{TaskID: "poll-" + tc.name, UserId: 10, ChannelId: 1000 + int(videoTimingTestChannelSequence.Add(1)),
				Platform: constant.TaskPlatform(fmt.Sprint(constant.ChannelTypeMiniMax)), Status: model.TaskStatusInProgress, Progress: "50%",
				Properties:  model.Properties{UpstreamModelName: "MiniMax-H3", OriginModelName: tc.origin},
				PrivateData: model.TaskPrivateData{Key: "private-upstream-secret"},
			}
			require.NoError(t, db.Create(task).Error)
			if tc.samples {
				history := *task
				history.ID = 0
				history.TaskID += "-history"
				history.UserId = 20
				history.Status = model.TaskStatusSuccess
				history.FinishTime = time.Now().Unix() - 10
				history.SubmitTime = history.FinishTime - 45
				require.NoError(t, db.Create(&history).Error)
			}
			statisticsQueries := 0
			require.NoError(t, db.Callback().Row().Before("gorm:row").Register("count_timing", func(tx *gorm.DB) {
				if tx.Statement.TableExpr != nil && strings.Contains(tx.Statement.TableExpr.SQL, "timing_samples") {
					statisticsQueries++
					if tc.dbFailure {
						tx.AddError(errors.New("statistics unavailable"))
					}
				}
			}))
			defer db.Callback().Row().Remove("count_timing")
			poll := func(userID int) ([]byte, string) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest("GET", tc.path+task.TaskID, nil)
				c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
				c.Set("id", userID)
				body, taskErr := videoFetchByIDRespBodyBuilder(c)
				if taskErr != nil {
					return body, fmt.Sprint(taskErr)
				}
				return body, ""
			}
			body, taskErr := poll(10)
			require.Empty(t, taskErr)
			var out map[string]any
			require.NoError(t, common.Unmarshal(body, &out))
			metadata, ok := out["metadata"].(map[string]any)
			require.True(t, ok, "metadata must be on the outermost response")
			if tc.samples && !tc.dbFailure {
				assert.Equal(t, map[string]any{"average_seconds": 45.0, "sample_count": 1.0}, metadata["generation_timing"])
			} else {
				assert.Contains(t, metadata, "generation_timing")
				assert.Nil(t, metadata["generation_timing"])
			}
			assert.NotContains(t, string(body), "private-upstream-secret")
			assert.NotContains(t, string(body), "-history")
			if tc.name == "wan" {
				assert.Equal(t, "RUNNING", out["output"].(map[string]any)["task_status"])
			}
			if tc.name == "openai" {
				assert.Equal(t, "in_progress", out["status"])
			}
			if strings.HasPrefix(tc.path, "/task/") {
				assert.Equal(t, "success", out["code"])
				assert.Equal(t, "IN_PROGRESS", out["data"].(map[string]any)["status"])
			}
			secondBody, taskErr := poll(10)
			require.Empty(t, taskErr)
			assert.JSONEq(t, string(body), string(secondBody))
			assert.Equal(t, 1, statisticsQueries, "repeated polls must cache success, empty history and statistics failures")
			body, taskErr = poll(11)
			assert.Empty(t, body)
			assert.Contains(t, taskErr, "task_not_exist")
			assert.Equal(t, 1, statisticsQueries, "unauthorized lookup must not query statistics")
		})
	}
}

func TestVideoTimingMetadataPreservesProviderPayload(t *testing.T) {
	body := []byte(`{"id":"video-1","status":"queued","metadata":{"url":"https://example.test/result.mp4","usage":{"tokens":9007199254740993},"generation_timing":{"average_seconds":1}},"data":{"value":9007199254740995}}`)
	annotated := videoTaskTimingResponse(context.Background(), nil, body)
	var result map[string]json.RawMessage
	require.NoError(t, common.Unmarshal(annotated, &result))
	assert.Equal(t, `{"value":9007199254740995}`, string(result["data"]))
	assert.JSONEq(t, `{"url":"https://example.test/result.mp4","usage":{"tokens":9007199254740993},"generation_timing":null}`, string(result["metadata"]))
	var metadata map[string]json.RawMessage
	require.NoError(t, common.Unmarshal(result["metadata"], &metadata))
	assert.Equal(t, `{"tokens":9007199254740993}`, string(metadata["usage"]))
	assert.Equal(t, `"queued"`, string(result["status"]))
}
