package router

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/relay/channel/task/hailuo"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type h3Transport func(*http.Request) (*http.Response, error)

func (transport h3Transport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

type h3AdmissionHTTPFault struct {
	*sql.DB
	rollback, readOutage bool
	publicID             string
}

func (fault *h3AdmissionHTTPFault) BeginTx(ctx context.Context, opts *sql.TxOptions) (gorm.ConnPool, error) {
	tx, err := fault.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &h3AdmissionHTTPTransaction{Tx: tx, fault: fault}, nil
}

func (fault *h3AdmissionHTTPFault) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if fault.readOutage && fault.publicID != "" {
		return nil, errors.New("fixture database unavailable after commit")
	}
	return fault.DB.QueryContext(ctx, query, args...)
}

type h3AdmissionHTTPTransaction struct {
	*sql.Tx
	fault    *h3AdmissionHTTPFault
	publicID string
}

func (tx *h3AdmissionHTTPTransaction) Commit() error {
	if tx.publicID == "" {
		return tx.Tx.Commit()
	}
	var err error
	if tx.fault.rollback {
		err = tx.Tx.Rollback()
	} else {
		err = tx.Tx.Commit()
	}
	if err != nil {
		return err
	}
	tx.fault.publicID = tx.publicID
	return errors.New("fixture lost admission COMMIT reply")
}

func TestH3FinalQuotaHTTP(t *testing.T) {
	setupRelayRouterTestDB(t)
	db := model.DB
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Task{}, &model.Log{}))
	oldBatch, oldCache, oldLimit, oldFactory := common.BatchUpdateEnabled, common.MemoryCacheEnabled, constant.TaskQueryLimit, service.GetTaskAdaptorFunc
	common.BatchUpdateEnabled, common.MemoryCacheEnabled, constant.TaskQueryLimit = true, false, 10
	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor { return relay.GetTaskAdaptor(platform) }
	t.Cleanup(func() {
		common.BatchUpdateEnabled, common.MemoryCacheEnabled, constant.TaskQueryLimit, service.GetTaskAdaptorFunc = oldBatch, oldCache, oldLimit, oldFactory
	})
	oldPrices := ratio_setting.ModelPrice2JSONString()
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"MiniMax-H3":0.1125}`))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrices)) })
	const balance = 10000000
	user := model.User{Username: "h3-owner", Group: "default", Status: common.UserStatusEnabled, Quota: balance}
	require.NoError(t, db.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: "h3ownerkey", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: balance, UnlimitedQuota: true}
	require.NoError(t, db.Create(&token).Error)
	baseURL := "https://h3.fixture.example"
	channel := model.Channel{Type: constant.ChannelTypeMiniMax, BaseURL: &baseURL, Key: "fixture-key", Status: common.ChannelStatusEnabled, Models: hailuo.ModelMiniMaxH3, Group: "default"}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(db))

	var upstreamID, status, resolution string
	var posts, queries int
	var onDispatch func()
	var postError error
	var rejectionHTTP int
	var missingID string
	var accountTiers map[string]string
	service.InitHttpClient()
	client := service.GetHttpClient()
	oldTransport := client.Transport
	t.Cleanup(func() { client.Transport = oldTransport })
	client.Transport = h3Transport(func(request *http.Request) (*http.Response, error) {
		body := ""
		code := http.StatusOK
		switch request.Method + " " + request.URL.String() {
		case "POST " + baseURL + hailuo.H3CreateEndpoint:
			posts++
			if onDispatch != nil {
				onDispatch()
			}
			if postError != nil {
				return nil, postError
			}
			body = fmt.Sprintf(`{"task_id":%q}`, upstreamID)
			if rejectionHTTP != 0 {
				code, body = rejectionHTTP, `{"error":{"type":"invalid_request_error","http_code":422,"message":"invalid parameter"}}`
			}
		case "GET " + baseURL + "/v2/query/video_generation/" + missingID:
			queries++
			code, body = http.StatusNotFound, `{"error":"unknown job"}`
		case "GET " + baseURL + "/v2/query/video_generation/" + upstreamID:
			queries++
			tier := resolution
			if accountTiers != nil {
				tier = accountTiers[request.Header.Get("Authorization")]
				if tier == "" {
					return nil, fmt.Errorf("unrecognized provider account")
				}
			}
			body = fmt.Sprintf(`{"result":{"task":{"id":%q,"status":%q,"resolution":%q,"content":{"url":"https://cdn.example/video.mp4"},"usage":{"input_seconds":0,"output_seconds":5}}}}`, upstreamID, status, tier)
		default:
			return nil, fmt.Errorf("unexpected fixture request %s %s", request.Method, request.URL)
		}
		return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	engine := gin.New()
	SetVideoRouter(engine)
	call := func(t *testing.T, method, path, key, body string, code int) map[string]any {
		t.Helper()
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+key)
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		require.Equal(t, code, response.Code, response.Body.String())
		var result map[string]any
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		return result
	}
	fetch := func(t *testing.T, id string) map[string]any {
		t.Helper()
		response := call(t, "GET", "/v1/video/generations/"+id, token.Key, "", 200)
		require.Equal(t, "success", response["code"])
		data := response["data"].(map[string]any)
		assert.Equal(t, id, data["task_id"])
		assert.Equal(t, hailuo.ModelMiniMaxH3, data["properties"].(map[string]any)["origin_model_name"])
		return data
	}
	var total, requests, otherTotal int
	var refunds int64
	ledger := func(t *testing.T) {
		t.Helper()
		require.NoError(t, db.First(&user, user.Id).Error)
		require.NoError(t, db.First(&token, token.Id).Error)
		require.NoError(t, db.First(&channel, channel.Id).Error)
		assert.Equal(t, balance-total, user.Quota)
		assert.Equal(t, balance-total, token.RemainQuota)
		assert.Equal(t, total, user.UsedQuota)
		assert.Equal(t, total, token.UsedQuota)
		assert.Equal(t, int64(total+otherTotal), channel.UsedQuota)
		assert.Equal(t, requests, user.RequestCount)
		var count int64
		require.NoError(t, db.Model(&model.Log{}).Where("type = ?", model.LogTypeRefund).Count(&count).Error)
		assert.Equal(t, refunds, count)
	}
	for _, tc := range []struct {
		name, upstreamStatus, tier, terminal string
		final                                int
		interrupt                            bool
	}{
		{"unchanged", "succeeded", "1080P", "SUCCESS", 437500, false},
		{"downgrade", "succeeded", "768P", "SUCCESS", 281250, true},
		{"failed", "failed", "", "FAILURE", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstreamID, status, resolution = "provider-"+tc.name, tc.upstreamStatus, tc.tier
			created := call(t, "POST", "/v1/videos", token.Key, `{"model":"MiniMax-H3","prompt":"fixture","size":"1080P","seconds":"5","metadata":{"ratio":"16:9"}}`, 200)
			id := created["id"].(string)
			require.NotEqual(t, upstreamID, id)
			pending := fetch(t, id)
			assert.NotContains(t, pending, "settled_quota")
			assert.NotContains(t, pending, "settlement_pending")
			assert.Equal(t, float64(0), pending["finish_time"])
			reserved := int(pending["quota"].(float64))
			require.Equal(t, 437500, reserved)
			total += reserved
			requests++
			ledger(t)
			stale, exists, err := model.GetByTaskId(user.Id, id)
			require.NoError(t, err)
			require.True(t, exists)
			if tc.interrupt {
				require.NoError(t, db.Exec(`CREATE TRIGGER reject_usage BEFORE UPDATE OF used_quota ON channels BEGIN SELECT RAISE(ABORT, 'fixture accounting failure'); END`).Error)
			}
			service.RunTaskPollingOnce(t.Context(), nil)
			terminal := fetch(t, id)
			assert.Equal(t, tc.terminal, terminal["status"])
			finish := int64(terminal["finish_time"].(float64))
			assert.GreaterOrEqual(t, finish, stale.SubmitTime)
			assert.LessOrEqual(t, finish, time.Now().Unix())
			if tc.interrupt {
				assert.NotContains(t, terminal, "settled_quota")
				assert.Equal(t, true, terminal["settlement_pending"])
				assert.NotContains(t, terminal, "pending_quota")
				ledger(t)
				require.True(t, model.HasUnfinishedSyncTasks())
				require.NoError(t, db.Exec(`DROP TRIGGER reject_usage`).Error)
			}
			before := queries
			var workers sync.WaitGroup
			for range 2 {
				workers.Go(func() { service.RunTaskPollingOnce(t.Context(), nil) })
			}
			workers.Wait()
			assert.Equal(t, before, queries)
			require.NoError(t, service.UpdateVideoTasks(t.Context(), stale.Platform, map[int][]string{channel.Id: {upstreamID}}, map[string]*model.Task{upstreamID: stale}))
			for range 2 {
				data := fetch(t, id)
				assert.Equal(t, float64(tc.final), data["settled_quota"])
				assert.NotContains(t, data, "settlement_pending")
				assert.Equal(t, float64(finish), data["finish_time"])
			}
			total += tc.final - reserved
			if tc.final < reserved {
				refunds++
			}
			ledger(t)
			require.False(t, model.HasUnfinishedSyncTasks())
		})
		if t.Failed() {
			return
		}
	}
	for _, historical := range []model.Task{
		{TaskID: "old-success", Status: model.TaskStatusSuccess, Quota: 437500},
		{TaskID: "old-failure", Status: model.TaskStatusFailure, Quota: 0},
	} {
		historical.UserId, historical.ChannelId = user.Id, channel.Id
		historical.FinishTime = time.Now().Unix() - 3600
		historical.Properties.OriginModelName = hailuo.ModelMiniMaxH3
		require.NoError(t, db.Create(&historical).Error)
		service.RunTaskPollingOnce(t.Context(), nil)
		legacy := fetch(t, historical.TaskID)
		assert.NotContains(t, legacy, "settled_quota")
		assert.NotContains(t, legacy, "settlement_pending")
	}
	call(t, "GET", "/v1/video/generations/old-success", "invalid", "", 401)
	other := model.User{Username: "other-owner", AffCode: "other", Status: common.UserStatusEnabled, Group: "default", Quota: balance}
	require.NoError(t, db.Create(&other).Error)
	otherToken := model.Token{UserId: other.Id, Key: "otherkey", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true, RemainQuota: balance}
	require.NoError(t, db.Create(&otherToken).Error)
	denied := call(t, "GET", "/v1/video/generations/old-success", otherToken.Key, "", 400)
	assert.Equal(t, "task_not_exist", denied["code"])
	const payload = `{"model":"MiniMax-H3","prompt":"fixture","size":"1080P","seconds":"5","metadata":{"ratio":"16:9"}}`
	t.Run("restart refunds admission before any paid post", func(t *testing.T) {
		beforePosts, beforeQueries := posts, queries
		task := model.Task{
			TaskID: model.GenerateTaskID(), UserId: user.Id, ChannelId: channel.Id, Quota: 437500,
			Status: model.TaskStatusAdmitted, CreatedAt: time.Now().Add(-time.Hour).Unix(),
			Properties:  model.Properties{OriginModelName: hailuo.ModelMiniMaxH3},
			PrivateData: model.TaskPrivateData{H3ReservationConfirmed: common.GetPointer(true), BillingSource: "wallet", TokenId: token.Id, ChannelGeneration: channel.Generation},
		}
		require.NoError(t, task.ReserveH3Dispatch(token.Key, true))
		total += task.Quota
		requests++
		ledger(t)
		require.True(t, model.HasUnfinishedSyncTasks())
		service.RunTaskPollingOnce(t.Context(), nil)
		service.RunTaskPollingOnce(t.Context(), nil)
		data := fetch(t, task.TaskID)
		require.Equal(t, "FAILURE", data["status"])
		require.Equal(t, float64(0), data["settled_quota"])
		require.NoError(t, db.First(&task, task.ID).Error)
		require.Error(t, task.BeginH3Dispatch())
		require.Equal(t, beforePosts, posts)
		require.Equal(t, beforeQueries, queries)
		total -= 437500
		refunds++
		ledger(t)
	})
	for _, httpCode := range []int{http.StatusUnprocessableEntity, http.StatusOK} {
		t.Run(fmt.Sprintf("parameter rejection in HTTP %d", httpCode), func(t *testing.T) {
			rejectionHTTP = httpCode
			t.Cleanup(func() { rejectionHTTP = 0 })
			beforePosts, beforeQueries := posts, queries
			result := call(t, "POST", "/v1/videos", token.Key, payload, http.StatusAccepted)
			id := result["id"].(string)
			require.Equal(t, id, result["task_id"])
			service.RunTaskPollingOnce(t.Context(), nil)
			data := fetch(t, id)
			require.Equal(t, "FAILURE", data["status"])
			require.Equal(t, float64(0), data["settled_quota"])
			require.Equal(t, beforePosts+1, posts)
			require.Equal(t, beforeQueries, queries)
			requests++
			refunds++
			ledger(t)
		})
	}
	for _, scenario := range []struct {
		name                 string
		rollback, readOutage bool
	}{
		{"confirmed rollback", true, false},
		{"unreadable commit", false, true},
		{"unreadable rollback", true, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			beforePosts, beforeQueries := posts, queries
			fault := &h3AdmissionHTTPFault{DB: sqlDB, rollback: scenario.rollback, readOutage: scenario.readOutage}
			model.DB = db.WithContext(t.Context())
			model.DB.Statement.ConnPool = fault
			t.Cleanup(func() { model.DB = db })
			const hook = "fixture:identify_admission_commit"
			require.NoError(t, db.Callback().Create().After("gorm:create").Register(hook, func(write *gorm.DB) {
				if task, ok := write.Statement.Dest.(*model.Task); ok && task.Status == model.TaskStatusAdmitted {
					write.Statement.ConnPool.(*h3AdmissionHTTPTransaction).publicID = task.TaskID
				}
			}))
			t.Cleanup(func() { require.NoError(t, db.Callback().Create().Remove(hook)) })
			result := call(t, "POST", "/v1/videos", token.Key, payload, http.StatusServiceUnavailable)
			require.NotEmpty(t, fault.publicID)
			if scenario.readOutage {
				require.Equal(t, "h3_admission_unconfirmed", result["code"])
				require.Equal(t, fault.publicID, result["id"])
				require.Equal(t, fault.publicID, result["task_id"])
			} else {
				require.Equal(t, "h3_admission_rolled_back", result["code"])
				require.NotContains(t, result, "id")
				require.NotContains(t, result, "task_id")
				require.NotContains(t, fmt.Sprint(result), fault.publicID)
			}
			model.DB = db
			if !scenario.rollback {
				require.Equal(t, "ADMITTED", fetch(t, fault.publicID)["status"])
				require.NoError(t, db.Model(&model.Task{}).Where("task_id = ?", fault.publicID).UpdateColumn("created_at", time.Now().Add(-time.Hour).Unix()).Error)
				requests++
				refunds++
			}
			service.RunTaskPollingOnce(t.Context(), nil)
			if scenario.rollback {
				missing := call(t, "GET", "/v1/video/generations/"+fault.publicID, token.Key, "", http.StatusBadRequest)
				require.Equal(t, "task_not_exist", missing["code"])
			} else {
				data := fetch(t, fault.publicID)
				require.Equal(t, "FAILURE", data["status"])
				require.Equal(t, float64(0), data["settled_quota"])
			}
			require.Equal(t, beforePosts, posts)
			require.Equal(t, beforeQueries, queries)
			ledger(t)
		})
	}
	for _, loseDecision := range []bool{false, true} {
		t.Run(fmt.Sprintf("HTTP 422 with interrupted terminal write and lost rejection marker %t", loseDecision), func(t *testing.T) {
			beforePosts, beforeQueries := posts, queries
			oldTimeout := constant.TaskTimeoutMinutes
			constant.TaskTimeoutMinutes, rejectionHTTP = 0, http.StatusUnprocessableEntity
			t.Cleanup(func() { constant.TaskTimeoutMinutes, rejectionHTTP = oldTimeout, 0 })
			require.NoError(t, db.Exec(`CREATE TRIGGER fail_terminal BEFORE UPDATE OF status ON tasks WHEN NEW.status = 'FAILURE' BEGIN SELECT RAISE(ABORT, 'fixture terminal unavailable'); END`).Error)
			t.Cleanup(func() { require.NoError(t, db.Exec(`DROP TRIGGER IF EXISTS fail_terminal`).Error) })
			if loseDecision {
				require.NoError(t, db.Exec(`CREATE TRIGGER fail_rejection BEFORE UPDATE OF h3_rejection_code ON tasks BEGIN SELECT RAISE(ABORT, 'fixture rejection unavailable'); END`).Error)
				t.Cleanup(func() { require.NoError(t, db.Exec(`DROP TRIGGER fail_rejection`).Error) })
			}
			result := call(t, "POST", "/v1/videos", token.Key, payload, http.StatusAccepted)
			id := result["id"].(string)
			require.Equal(t, id, result["task_id"])
			pending := fetch(t, id)
			require.NotContains(t, pending, "settled_quota")
			total += 437500
			requests++
			ledger(t)
			if loseDecision {
				require.Equal(t, "rejection_reconciliation_required", result["code"])
				constant.TaskTimeoutMinutes = 1
				require.NoError(t, db.Model(&model.Task{}).Where("task_id = ?", id).UpdateColumn("submit_time", time.Now().Add(-time.Hour).Unix()).Error)
			} else {
				require.Equal(t, "rejection_recovery_pending", result["code"])
				require.True(t, model.HasUnfinishedSyncTasks())
			}
			require.NoError(t, db.Exec(`DROP TRIGGER fail_terminal`).Error)
			for range 2 {
				service.RunTaskPollingOnce(t.Context(), nil)
			}
			data := fetch(t, id)
			if loseDecision {
				require.Equal(t, "DISPATCH_UNKNOWN", data["status"])
				require.NotContains(t, data, "settled_quota")
			} else {
				require.Equal(t, "FAILURE", data["status"])
				require.Equal(t, "invalid_request_error", data["fail_reason"])
				require.Equal(t, float64(0), data["settled_quota"])
				total -= 437500
				refunds++
			}
			require.Equal(t, beforePosts+1, posts)
			require.Equal(t, beforeQueries, queries)
			ledger(t)
		})
	}
	t.Run("acknowledged dispatch recovers delivered resolution", func(t *testing.T) {
		upstreamID, status, resolution = "provider-interrupted", "succeeded", "768P"
		beforePosts, beforeQueries := posts, queries
		var intent model.Task
		onDispatch = func() {
			require.NoError(t, db.Where("status = ?", "DISPATCHING").First(&intent).Error)
			require.NoError(t, db.Exec(`CREATE TRIGGER reject_accept BEFORE UPDATE ON tasks WHEN OLD.status = 'DISPATCHING' AND NEW.status = 'NOT_START' BEGIN SELECT RAISE(ABORT, 'interrupted acceptance'); END`).Error)
		}
		defer func() { onDispatch = nil }()
		result := call(t, "POST", "/v1/videos", token.Key, payload, 202)
		require.Equal(t, intent.TaskID, result["id"])
		require.Equal(t, "acceptance_recovery_pending", result["code"])
		require.NoError(t, intent.MarkH3DispatchUnknown())
		assert.Equal(t, "DISPATCHING", fetch(t, intent.TaskID)["status"])
		oldTimeout := constant.TaskTimeoutMinutes
		constant.TaskTimeoutMinutes = 0
		t.Cleanup(func() { constant.TaskTimeoutMinutes = oldTimeout })
		require.NoError(t, db.Model(&intent).Update("submit_time", time.Now().Add(-2*time.Minute).Unix()).Error)
		require.True(t, model.HasUnfinishedSyncTasks())
		service.RunTaskPollingOnce(t.Context(), nil)
		data := fetch(t, intent.TaskID)
		assert.Equal(t, "SUCCESS", data["status"])
		assert.Equal(t, float64(281250), data["settled_quota"])
		denied := call(t, "GET", "/v1/video/generations/"+intent.TaskID, otherToken.Key, "", 400)
		assert.Equal(t, "task_not_exist", denied["code"])
		require.NoError(t, db.Exec(`DROP TRIGGER reject_accept`).Error)
		assert.Equal(t, beforePosts+1, posts)
		assert.Equal(t, beforeQueries+1, queries)
		total += 281250
		requests++
		refunds++
		ledger(t)
	})
	t.Run("provider acknowledges after dispatch timeout", func(t *testing.T) {
		upstreamID, status, resolution = "provider-late", "succeeded", "768P"
		beforePosts, beforeQueries := posts, queries
		oldTimeout := constant.TaskTimeoutMinutes
		constant.TaskTimeoutMinutes = 1
		t.Cleanup(func() { constant.TaskTimeoutMinutes, onDispatch = oldTimeout, nil })
		var intent model.Task
		onDispatch = func() {
			require.NoError(t, db.Where("status = ?", model.TaskStatusDispatching).First(&intent).Error)
			require.NoError(t, db.Model(&intent).Update("submit_time", time.Now().Add(-time.Hour).Unix()).Error)
			service.RunTaskPollingOnce(t.Context(), nil)
			data := fetch(t, intent.TaskID)
			assert.Equal(t, "DISPATCH_UNKNOWN", data["status"])
			assert.NotContains(t, data, "settled_quota")
		}
		result := call(t, "POST", "/v1/videos", token.Key, payload, 200)
		require.Equal(t, intent.TaskID, result["id"])
		service.RunTaskPollingOnce(t.Context(), nil)
		assert.Equal(t, float64(281250), fetch(t, intent.TaskID)["settled_quota"])
		assert.Equal(t, beforePosts+1, posts)
		assert.Equal(t, beforeQueries+1, queries)
		total += 281250
		requests++
		refunds++
		ledger(t)
	})
	t.Run("acknowledged provider outage reaches configured timeout", func(t *testing.T) {
		upstreamID, missingID = "provider-query-unavailable", "provider-query-unavailable"
		oldTimeout := constant.TaskTimeoutMinutes
		previousLimit := constant.TaskQueryLimit
		constant.TaskTimeoutMinutes, constant.TaskQueryLimit = 1, 1
		t.Cleanup(func() {
			constant.TaskTimeoutMinutes, constant.TaskQueryLimit, missingID = oldTimeout, previousLimit, ""
		})
		beforePosts, beforeQueries := posts, queries
		id := call(t, "POST", "/v1/videos", token.Key, payload, 200)["id"].(string)
		upstreamID, status, resolution = "provider-available", "succeeded", "1080P"
		next := call(t, "POST", "/v1/videos", token.Key, payload, 200)["id"].(string)
		service.RunTaskPollingOnce(t.Context(), nil)
		data := fetch(t, id)
		require.NotContains(t, data, "settled_quota")
		require.Equal(t, beforeQueries+1, queries)
		service.RunTaskPollingOnce(t.Context(), nil)
		require.Equal(t, float64(437500), fetch(t, next)["settled_quota"])
		expired := time.Now().Add(-2 * time.Minute).Unix()
		require.NoError(t, db.Model(&model.Task{}).Where("task_id = ?", id).Updates(map[string]any{"submit_time": expired, "h3_acknowledged_at": expired, "data": []byte(`{"request_id":"new-on-every-poll"}`)}).Error)
		service.RunTaskPollingOnce(t.Context(), nil)
		service.RunTaskPollingOnce(t.Context(), nil)
		data = fetch(t, id)
		require.Equal(t, "FAILURE", data["status"])
		require.Positive(t, data["finish_time"])
		var review model.Task
		require.NoError(t, db.Where("task_id = ?", id).First(&review).Error)
		require.Equal(t, model.TaskStatusPollReview, review.Status)
		require.Equal(t, missingID, review.GetUpstreamTaskID())
		require.NotContains(t, data, "settled_quota")
		require.NotContains(t, data, "settlement_pending")
		require.Equal(t, beforePosts+2, posts)
		require.Equal(t, beforeQueries+2, queries)
		require.False(t, model.HasUnfinishedSyncTasks())
		requests += 2
		total += 2 * 437500
		ledger(t)
	})
	for _, scenario := range []struct{ name, code string }{
		{"ack and promotion unavailable", "provider_acknowledgement_unrecorded"},
		{"connection lost after dispatch", "provider_acceptance_unknown"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			upstreamID = "provider-unresolved"
			beforePosts, beforeQueries := posts, queries
			if scenario.code == "provider_acknowledgement_unrecorded" {
				require.NoError(t, db.Exec(`CREATE TRIGGER reject_ack BEFORE UPDATE OF h3_upstream_id ON tasks BEGIN SELECT RAISE(ABORT, 'cannot save provider id'); END`).Error)
				t.Cleanup(func() { require.NoError(t, db.Exec(`DROP TRIGGER reject_ack`).Error) })
			} else {
				oldRetries := common.RetryTimes
				common.RetryTimes = 2
				postError = io.ErrUnexpectedEOF
				t.Cleanup(func() { common.RetryTimes, postError = oldRetries, nil })
			}
			result := call(t, "POST", "/v1/videos", token.Key, payload, 202)
			require.Equal(t, scenario.code, result["code"])
			id := result["id"].(string)
			require.NotEqual(t, upstreamID, id)
			require.Equal(t, id, result["task_id"])
			oldTimeout := constant.TaskTimeoutMinutes
			constant.TaskTimeoutMinutes = 1
			t.Cleanup(func() { constant.TaskTimeoutMinutes = oldTimeout })
			require.NoError(t, db.Model(&model.Task{}).Where("task_id = ?", id).Update("submit_time", time.Now().Add(-time.Hour).Unix()).Error)
			service.RunTaskPollingOnce(t.Context(), nil)
			data := fetch(t, id)
			assert.Equal(t, "DISPATCH_UNKNOWN", data["status"])
			assert.NotContains(t, data, "settled_quota")
			assert.Equal(t, float64(0), data["finish_time"])
			assert.Equal(t, beforePosts+1, posts)
			assert.Equal(t, beforeQueries, queries)
			total += 437500
			requests++
			ledger(t)
		})
	}
	checkOther := func(t *testing.T) {
		t.Helper()
		require.NoError(t, db.First(&other, other.Id).Error)
		require.NoError(t, db.First(&otherToken, otherToken.Id).Error)
		assert.Equal(t, balance-otherTotal, other.Quota)
		assert.Equal(t, balance-otherTotal, otherToken.RemainQuota)
		assert.Equal(t, otherTotal, other.UsedQuota)
		assert.Equal(t, otherTotal, otherToken.UsedQuota)
	}
	t.Run("two keys on one channel share a provider id", func(t *testing.T) {
		upstreamID, status = "account-local-id", "succeeded"
		beforePosts, beforeQueries := posts, queries
		accountTiers = map[string]string{"Bearer fixture-key": "1080P", "Bearer other-account": "768P"}
		t.Cleanup(func() { accountTiers = nil })
		first := call(t, "POST", "/v1/videos", token.Key, payload, 200)["id"].(string)
		require.NoError(t, db.Model(&channel).Update("key", "other-account").Error)
		second := call(t, "POST", "/v1/videos", otherToken.Key, payload, 200)["id"].(string)
		require.NotEqual(t, first, second)
		require.NoError(t, db.Model(&channel).Update("key", "fixture-key").Error)
		service.RunTaskPollingOnce(t.Context(), nil)
		assert.Equal(t, beforePosts+2, posts)
		assert.Equal(t, beforeQueries+2, queries)
		assert.Equal(t, float64(437500), fetch(t, first)["settled_quota"])
		secondData := call(t, "GET", "/v1/video/generations/"+second, otherToken.Key, "", 200)["data"].(map[string]any)
		assert.Equal(t, second, secondData["task_id"])
		assert.Equal(t, float64(281250), secondData["settled_quota"])
		assert.Equal(t, "task_not_exist", call(t, "GET", "/v1/video/generations/"+second, token.Key, "", 400)["code"])
		total += 437500
		otherTotal += 281250
		requests++
		refunds++
		checkOther(t)
		ledger(t)
	})
	t.Run("retired channel and active channel share a provider id", func(t *testing.T) {
		upstreamID, status, resolution = "provider-retired", "succeeded", "768P"
		beforePosts, beforeQueries := posts, queries
		common.MemoryCacheEnabled = true
		model.InitChannelCache()
		common.MemoryCacheEnabled = false
		onDispatch = func() { require.NoError(t, channel.Delete()) }
		defer func() { onDispatch = nil }()
		created := call(t, "POST", "/v1/videos", token.Key, payload, 200)
		id := created["id"].(string)
		saved, exists, err := model.GetByTaskId(user.Id, id)
		require.NoError(t, err)
		require.True(t, exists)
		require.Equal(t, upstreamID, saved.GetUpstreamTaskID())
		require.NotEmpty(t, saved.PrivateData.ChannelGeneration)
		require.Equal(t, channel.Generation, saved.PrivateData.ChannelGeneration)
		onDispatch = nil
		active := model.Channel{Id: channel.Id, Type: constant.ChannelTypeMiniMax, BaseURL: &baseURL, Key: "active-account", Status: common.ChannelStatusEnabled, Models: hailuo.ModelMiniMaxH3, Group: "default"}
		require.NoError(t, db.Create(&active).Error)
		require.NoError(t, active.AddAbilities(db))
		common.MemoryCacheEnabled = true
		cached, err := model.CacheGetChannel(active.Id)
		require.NoError(t, err)
		require.Equal(t, channel.Generation, cached.Generation)
		require.NotEqual(t, active.Generation, cached.Generation)
		oldRetries := common.RetryTimes
		common.RetryTimes = 0
		t.Cleanup(func() { common.RetryTimes = oldRetries })
		denied := call(t, "POST", "/v1/videos", token.Key, payload, http.StatusServiceUnavailable)
		require.Equal(t, "h3_reservation_failed", denied["code"])
		require.Equal(t, beforePosts+1, posts)
		common.MemoryCacheEnabled = false
		activeID := call(t, "POST", "/v1/videos", otherToken.Key, payload, 200)["id"].(string)
		accountTiers = map[string]string{"Bearer active-account": "768P"}
		t.Cleanup(func() { accountTiers = nil })
		require.NoError(t, token.Delete())
		replacement := model.Token{UserId: user.Id, Key: "replacement", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
		require.NoError(t, db.Create(&replacement).Error)
		common.MemoryCacheEnabled = true
		for range 2 {
			service.RunTaskPollingOnce(t.Context(), nil)
		}
		common.MemoryCacheEnabled = false
		assert.Equal(t, beforePosts+2, posts)
		assert.Equal(t, beforeQueries+1, queries)
		activeData := call(t, "GET", "/v1/video/generations/"+activeID, otherToken.Key, "", 200)["data"].(map[string]any)
		assert.Equal(t, "SUCCESS", activeData["status"])
		assert.Equal(t, float64(281250), activeData["settled_quota"])
		otherTotal += 281250
		checkOther(t)
		require.NoError(t, db.First(&active, active.Id).Error)
		assert.Equal(t, int64(281250), active.UsedQuota)
		call(t, "GET", "/v1/video/generations/"+id, token.Key, "", 401)
		data := call(t, "GET", "/v1/video/generations/"+id, replacement.Key, "", 200)["data"].(map[string]any)
		assert.Equal(t, id, data["task_id"])
		assert.Equal(t, "FAILURE", data["status"])
		assert.Equal(t, float64(0), data["settled_quota"])
		assert.NotContains(t, data, "settlement_pending")
		require.NoError(t, db.First(&user, user.Id).Error)
		require.NoError(t, db.Unscoped().First(&token, token.Id).Error)
		assert.True(t, token.DeletedAt.Valid)
		assert.Equal(t, balance-total, token.RemainQuota)
		assert.Equal(t, balance-total, user.Quota)
		assert.Equal(t, total, token.UsedQuota)
		assert.Equal(t, total, user.UsedQuota)
	})
}
