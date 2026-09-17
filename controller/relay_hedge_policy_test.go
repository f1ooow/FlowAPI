package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Real HTTP client dispatch with an in-memory upstream makes cancellation,
// scanner readiness and billing ordering deterministic without binding a port.
type hedgePolicyTransport struct{ attempts [3]*hedgePolicyUpstream }
type hedgePolicyUpstream struct {
	started    chan struct{}
	prefixRead chan struct{}
	release    chan struct{}
	cancelled  chan struct{}
	prefix     string
	tail       string
	headers    bool
	status     int
}

func (tr *hedgePolicyTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	i := 0
	if r.URL.Host == "hedge-b.test" {
		i = 1
	} else if r.URL.Host == "hedge-c.test" {
		i = 2
	}
	u := tr.attempts[i]
	close(u.started)
	if u.status != 0 {
		return &http.Response{StatusCode: u.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":{"message":"provider failed","type":"server_error"}}`)), Request: r}, nil
	}
	if !u.headers {
		<-r.Context().Done()
		close(u.cancelled)
		return nil, r.Context().Err()
	}
	reader, writer := io.Pipe()
	stop := context.AfterFunc(r.Context(), func() { _ = reader.CloseWithError(r.Context().Err()); close(u.cancelled) })
	body := &hedgePolicyBody{PipeReader: reader, stop: stop, observed: u.prefixRead}
	go func() {
		defer writer.Close()
		if u.prefix != "" {
			if _, err := io.WriteString(writer, u.prefix); err != nil {
				return
			}
		}
		select {
		case <-u.release:
		case <-r.Context().Done():
			return
		}
		if u.tail != "" {
			_, _ = io.WriteString(writer, u.tail)
		} else {
			_, _ = fmt.Fprintf(writer, "data: {\"choices\":[{\"delta\":{\"content\":\"answer-%d\"}}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":2,\"total_tokens\":12}}\n\ndata: [DONE]\n\n", i)
		}
	}()
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: body, Request: r}, nil
}

type hedgePolicyBody struct {
	*io.PipeReader
	stop     func() bool
	observed chan struct{}
	reads    int
	once     sync.Once
}

func (b *hedgePolicyBody) Read(p []byte) (int, error) {
	if b.reads > 0 {
		b.once.Do(func() { close(b.observed) })
	}
	b.reads++
	return b.PipeReader.Read(p)
}
func (b *hedgePolicyBody) Close() error { b.stop(); return b.PipeReader.Close() }

type hedgePolicyFixture struct {
	db        *gorm.DB
	ctx       *gin.Context
	info      *relaycommon.RelayInfo
	channels  [2]*model.Channel
	upstreams [2]*hedgePolicyUpstream
	recorder  *httptest.ResponseRecorder
	cancel    context.CancelFunc
	result    chan *types.NewAPIError
	done      chan struct{}
}

func newHedgePolicyFixture(t *testing.T, bill bool, quota int) *hedgePolicyFixture {
	t.Helper()
	f := &hedgePolicyFixture{db: setupModelListControllerTestDB(t), recorder: httptest.NewRecorder(), result: make(chan *types.NewAPIError, 1)}
	require.NoError(t, f.db.AutoMigrate(&model.Token{}, &model.Log{}))
	oldBatch, oldCache, oldLogs := common.BatchUpdateEnabled, common.MemoryCacheEnabled, common.LogConsumeEnabled
	common.BatchUpdateEnabled, common.MemoryCacheEnabled, common.LogConsumeEnabled = false, false, true
	oldModel, oldCompletion := ratio_setting.ModelRatio2JSONString(), ratio_setting.CompletionRatio2JSONString()
	oldGroup, oldInclude := ratio_setting.GroupRatio2JSONString(), ratio_setting.IncludeChannelRatio2JSONString()
	oldBill := operation_setting.SnapshotGeneralSetting().BillHedgeLosers
	t.Cleanup(func() {
		common.BatchUpdateEnabled, common.MemoryCacheEnabled, common.LogConsumeEnabled = oldBatch, oldCache, oldLogs
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldModel))
		require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(oldCompletion))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroup))
		require.NoError(t, ratio_setting.UpdateIncludeChannelRatioByJSONString(oldInclude))
		require.NoError(t, config.UpdateConfigFromMap(operation_setting.GetGeneralSetting(), map[string]string{"bill_hedge_losers": fmt.Sprint(oldBill)}))
	})
	require.NoError(t, config.UpdateConfigFromMap(operation_setting.GetGeneralSetting(), map[string]string{"bill_hedge_losers": fmt.Sprint(bill)}))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"hedge-test":1}`))
	require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"hedge-test":2}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":2}`))
	require.NoError(t, ratio_setting.UpdateIncludeChannelRatioByJSONString(`{"default":true}`))
	service.InitHttpClient()
	client := service.GetHttpClient()
	oldTransport := client.Transport
	tr := &hedgePolicyTransport{}
	client.Transport = tr
	t.Cleanup(func() { client.Transport = oldTransport })
	require.NoError(t, f.db.Create(&model.User{Id: 7001, Username: "hedge-policy", Quota: quota, Group: "default"}).Error)
	require.NoError(t, f.db.Create(&model.Token{Id: 7001, UserId: 7001, Key: "policy-token", RemainQuota: quota, Status: common.TokenStatusEnabled}).Error)
	for i := range f.channels {
		u := &hedgePolicyUpstream{started: make(chan struct{}), prefixRead: make(chan struct{}), release: make(chan struct{}), cancelled: make(chan struct{}), headers: true, prefix: "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}],\"usage\":{\"prompt_tokens\":10,\"total_tokens\":10}}\n\n"}
		tr.attempts[i], f.upstreams[i] = u, u
		weight, priority, ratio := uint(100), int64(10-i), []float64{0.5, 1.5}[i]
		base := []string{"http://hedge-a.test", "http://hedge-b.test"}[i]
		settings := `{"reliability":{"first_content_timeout_seconds":1,"max_attempts":1}}`
		f.channels[i] = &model.Channel{Id: 7010 + i, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: fmt.Sprint("policy-", i), Key: "up-key", BaseURL: &base, Group: "default", Models: "hedge-test", Weight: &weight, Priority: &priority, CostRatio: &ratio, Setting: &settings}
		require.NoError(t, f.db.Create(f.channels[i]).Error)
		require.NoError(t, f.db.Create(&model.Ability{Group: "default", Model: "hedge-test", ChannelId: f.channels[i].Id, Enabled: true, Priority: &priority, Weight: weight}).Error)
	}
	f.ctx, _ = gin.CreateTestContext(f.recorder)
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	t.Cleanup(cancel)
	f.ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"hedge-test","stream":true,"messages":[{"role":"user","content":"hello"}]}`)).WithContext(ctx)
	f.ctx.Request.Header.Set("Content-Type", "application/json")
	for key, value := range map[string]any{common.RequestIdKey: "policy-request", "id": 7001, "token_id": 7001, "token_key": "policy-token", "token_name": "policy-token", "token_group": "default", "group": "default"} {
		f.ctx.Set(key, value)
	}
	common.SetContextKey(f.ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(f.ctx, constant.ContextKeyUsingGroup, "default")
	require.Nil(t, middleware.SetupContextForSelectedChannel(f.ctx, f.channels[0], "hedge-test"))
	req, err := helper.GetAndValidateRequest(f.ctx, types.RelayFormatOpenAI)
	require.NoError(t, err)
	f.info, err = relaycommon.GenRelayInfo(f.ctx, types.RelayFormatOpenAI, req, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		f.cancel()
		if f.done != nil {
			awaitHedgeSignal(t, f.done)
		}
		common.CleanupBodyStorage(f.ctx)
	})
	return f
}
func (f *hedgePolicyFixture) start() {
	f.done = make(chan struct{})
	go func() {
		defer close(f.done)
		f.result <- relayWithHedge(f.ctx, f.info, f.channels[0], 10, &types.TokenCountMeta{})
	}()
}
func awaitHedgeSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("hedge lifecycle signal missing")
	}
}
func (f *hedgePolicyFixture) finish(t *testing.T) *types.NewAPIError {
	t.Helper()
	select {
	case err := <-f.result:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("hedge request did not finish")
		return nil
	}
}
func (f *hedgePolicyFixture) logs(t *testing.T, count int) []model.Log {
	t.Helper()
	require.Eventually(t, func() bool {
		var n int64
		f.db.Model(&model.Log{}).Where("request_id = ?", "policy-request").Count(&n)
		return int(n) == count
	}, 5*time.Second, time.Millisecond)
	var rows []model.Log
	require.NoError(t, f.db.Order("channel_id").Find(&rows).Error)
	return rows
}

func TestHedgePolicySnapshotAndIndependentSettlement(t *testing.T) {
	for _, bill := range []bool{true, false} {
		for _, winner := range []int{0, 1} {
			t.Run(fmt.Sprintf("bill_%t_winner_%d", bill, winner), func(t *testing.T) {
				f := newHedgePolicyFixture(t, bill, 100000)
				f.start()
				for _, u := range f.upstreams {
					awaitHedgeSignal(t, u.prefixRead)
				}
				var reserved model.User
				require.NoError(t, f.db.First(&reserved, 7001).Error)
				assert.Equal(t, 98000, reserved.Quota)
				// A live option change must not change this request's policy or pricing.
				require.NoError(t, config.UpdateConfigFromMap(operation_setting.GetGeneralSetting(), map[string]string{"bill_hedge_losers": fmt.Sprint(!bill)}))
				require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":100}`))
				close(f.upstreams[winner].release)
				require.Nil(t, f.finish(t))
				assert.Contains(t, f.recorder.Body.String(), fmt.Sprint("answer-", winner))
				assert.NotContains(t, f.recorder.Body.String(), fmt.Sprint("answer-", 1-winner))
				assert.Equal(t, f.channels[winner].Id, f.ctx.GetInt("channel_id"))
				if bill {
					close(f.upstreams[1-winner].release)
				} else {
					awaitHedgeSignal(t, f.upstreams[1-winner].cancelled)
				}
				rows := f.logs(t, 2)
				expected := []int{14, 42}
				if !bill {
					expected[1-winner] = 0
				}
				for i, row := range rows {
					assert.Equal(t, expected[i], row.Quota)
					assert.Equal(t, "policy-request", row.RequestId)
					var other struct {
						Hedge map[string]any `json:"hedge"`
					}
					require.NoError(t, common.UnmarshalJsonStr(row.Other, &other))
					if !bill && i != winner {
						assert.Equal(t, "not_billed", other.Hedge["settlement_status"])
						assert.Equal(t, "upstream", other.Hedge["usage_source"])
						assert.Equal(t, "partial", other.Hedge["metering_status"])
						assert.Equal(t, 10, row.PromptTokens)
					} else {
						assert.Equal(t, "settled", other.Hedge["settlement_status"])
					}
				}
				var user model.User
				require.NoError(t, f.db.First(&user, 7001).Error)
				assert.Equal(t, 100000-expected[0]-expected[1], user.Quota)
				assert.Equal(t, 1, user.RequestCount)
			})
		}
	}
}

func TestHedgePolicyCancelsLoserWithoutValidPrefix(t *testing.T) {
	for _, mode := range []string{"headers_pending", "headers_only", "heartbeat"} {
		t.Run(mode, func(t *testing.T) {
			f := newHedgePolicyFixture(t, true, 100000)
			f.upstreams[0].prefix = ""
			if mode == "headers_pending" {
				f.upstreams[0].headers = false
			}
			if mode == "heartbeat" {
				f.upstreams[0].prefix = "data: ping\n\n"
			}
			f.start()
			awaitHedgeSignal(t, f.upstreams[1].prefixRead)
			close(f.upstreams[1].release)
			require.Nil(t, f.finish(t))
			awaitHedgeSignal(t, f.upstreams[0].cancelled)
			rows := f.logs(t, 2)
			assert.Zero(t, rows[0].Quota)
			assert.Equal(t, 42, rows[1].Quota)
		})
	}
}

func TestHedgePolicyClientCancellationRefundsBoth(t *testing.T) {
	f := newHedgePolicyFixture(t, true, 100000)
	f.start()
	for _, u := range f.upstreams {
		awaitHedgeSignal(t, u.prefixRead)
	}
	f.cancel()
	require.NotNil(t, f.finish(t))
	rows := f.logs(t, 2)
	for _, row := range rows {
		assert.Zero(t, row.Quota)
	}
	var user model.User
	require.NoError(t, f.db.First(&user, 7001).Error)
	assert.Equal(t, 100000, user.Quota)
	assert.Zero(t, user.RequestCount)
	assert.Empty(t, f.recorder.Body.String())
}

func TestHedgePolicyUnavailableBackupPreservesFundedOriginal(t *testing.T) {
	for _, reason := range []string{"no_eligible_backup", "backup_admission_failed"} {
		t.Run(reason, func(t *testing.T) {
			quota := 100000
			if reason == "backup_admission_failed" {
				quota = 1000
			}
			f := newHedgePolicyFixture(t, true, quota)
			if reason == "no_eligible_backup" {
				require.NoError(t, f.db.Model(&model.Channel{}).Where("id = ?", f.channels[1].Id).Update("status", common.ChannelStatusManuallyDisabled).Error)
			}
			f.start()
			awaitHedgeSignal(t, f.upstreams[0].prefixRead)
			require.Eventually(t, func() bool {
				history, ok := f.ctx.Get("route_history")
				if !ok {
					return false
				}
				for _, entry := range history.([]service.RouteAttempt) {
					if entry.Reason == reason {
						return true
					}
				}
				return false
			}, 5*time.Second, time.Millisecond)
			select {
			case <-f.upstreams[1].started:
				t.Fatal("unfunded or ineligible backup dispatched")
			default:
			}
			close(f.upstreams[0].release)
			require.Nil(t, f.finish(t))
			rows := f.logs(t, 1)
			assert.Equal(t, 14, rows[0].Quota)
			var other struct {
				Admin struct {
					History []service.RouteAttempt `json:"route_history"`
				} `json:"admin_info"`
			}
			require.NoError(t, common.UnmarshalJsonStr(rows[0].Other, &other))
			var reasons []string
			for _, entry := range other.Admin.History {
				reasons = append(reasons, entry.Reason)
			}
			assert.Contains(t, reasons, reason, "the consume row must persist the coordinator's skipped-backup decision")
			var user model.User
			require.NoError(t, f.db.First(&user, 7001).Error)
			assert.Equal(t, quota-14, user.Quota)
		})
	}
}

func TestHedgePolicyIncludeChannelDisabledAndZeroUserFactor(t *testing.T) {
	for _, zero := range []bool{false, true} {
		t.Run(fmt.Sprint("zero_user_", zero), func(t *testing.T) {
			f := newHedgePolicyFixture(t, true, 100000)
			require.NoError(t, ratio_setting.UpdateIncludeChannelRatioByJSONString(`{"default":false}`))
			previous := ratio_setting.UserGroupRatio2JSONString()
			t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateUserGroupRatioByJSONString(previous)) })
			if zero {
				require.NoError(t, ratio_setting.UpdateUserGroupRatioByJSONString(`{"default":{"default":0}}`))
			}
			f.start()
			for _, u := range f.upstreams {
				awaitHedgeSignal(t, u.prefixRead)
			}
			close(f.upstreams[1].release)
			require.Nil(t, f.finish(t))
			close(f.upstreams[0].release)
			rows := f.logs(t, 2)
			expected := 28
			if zero {
				expected = 0
			}
			for _, row := range rows {
				assert.Equal(t, expected, row.Quota)
			}
		})
	}
}

func TestHedgePolicyDetachedLoserSurvivesClientCancellation(t *testing.T) {
	f := newHedgePolicyFixture(t, true, 100000)
	f.start()
	for _, u := range f.upstreams {
		awaitHedgeSignal(t, u.prefixRead)
	}
	close(f.upstreams[1].release)
	require.Nil(t, f.finish(t))
	f.cancel()
	select {
	case <-f.upstreams[0].cancelled:
		t.Fatal("detached loser incorrectly follows client context")
	default:
	}
	close(f.upstreams[0].release)
	rows := f.logs(t, 2)
	assert.Equal(t, 14, rows[0].Quota)
	assert.Equal(t, 42, rows[1].Quota)
}

func TestHedgePolicySimultaneousContentHasOneResponseOwner(t *testing.T) {
	f := newHedgePolicyFixture(t, true, 100000)
	f.start()
	for _, u := range f.upstreams {
		awaitHedgeSignal(t, u.prefixRead)
	}
	close(f.upstreams[0].release)
	close(f.upstreams[1].release)
	require.Nil(t, f.finish(t))
	rows := f.logs(t, 2)
	assert.Equal(t, 56, rows[0].Quota+rows[1].Quota)
	body := f.recorder.Body.String()
	assert.NotEqual(t, strings.Contains(body, "answer-0"), strings.Contains(body, "answer-1"))
	var user model.User
	require.NoError(t, f.db.First(&user, 7001).Error)
	assert.Equal(t, 1, user.RequestCount)
}

func TestHedgePolicyFailedCandidateReleasesSlot(t *testing.T) {
	f := newHedgePolicyFixture(t, true, 100000)
	f.upstreams[1].status = http.StatusServiceUnavailable
	tr := service.GetHttpClient().Transport.(*hedgePolicyTransport)
	third := &hedgePolicyUpstream{started: make(chan struct{}), prefixRead: make(chan struct{}), release: make(chan struct{}), cancelled: make(chan struct{}), headers: true, prefix: f.upstreams[0].prefix}
	tr.attempts[2] = third
	channel := *f.channels[1]
	channel.Id = 7012
	channel.Name = "policy-2"
	base := "http://hedge-c.test"
	channel.BaseURL = &base
	priority := int64(8)
	channel.Priority = &priority
	require.NoError(t, f.db.Create(&channel).Error)
	require.NoError(t, f.db.Create(&model.Ability{Group: "default", Model: "hedge-test", ChannelId: channel.Id, Enabled: true, Priority: &priority, Weight: 100}).Error)
	f.start()
	awaitHedgeSignal(t, third.prefixRead)
	select {
	case <-f.upstreams[0].cancelled:
		t.Fatal("live original was cancelled on peer failure")
	default:
	}
	close(third.release)
	require.Nil(t, f.finish(t))
	close(f.upstreams[0].release)
	rows := f.logs(t, 3)
	assert.Equal(t, 14, rows[0].Quota)
	assert.Zero(t, rows[1].Quota)
	assert.Equal(t, 42, rows[2].Quota)
	assert.Contains(t, f.recorder.Body.String(), "answer-2")
	assert.NotContains(t, f.recorder.Body.String(), "answer-0")
}

func TestHedgePolicyAutoGroupsRetainOwnPricingSnapshots(t *testing.T) {
	f := newHedgePolicyFixture(t, true, 100000)
	oldGroups := setting.UserUsableGroups2JSONString()
	oldMax := setting.GetMaxTokenAutoGroups()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(oldGroups))
		require.NoError(t, setting.UpdateMaxTokenAutoGroups(fmt.Sprint(oldMax)))
	})
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP"}`))
	require.NoError(t, setting.UpdateMaxTokenAutoGroups("2"))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":2,"vip":4}`))
	require.NoError(t, ratio_setting.UpdateIncludeChannelRatioByJSONString(`{"default":false,"vip":true}`))
	require.NoError(t, f.db.Model(&model.Channel{}).Where("id = ?", f.channels[1].Id).Update("group", "vip").Error)
	require.NoError(t, f.db.Model(&model.Ability{}).Where("channel_id = ?", f.channels[1].Id).Update("group", "vip").Error)
	f.info.TokenGroup = "auto"
	f.ctx.Set("token_group", "auto")
	common.SetContextKey(f.ctx, constant.ContextKeyAutoGroup, "default")
	common.SetContextKey(f.ctx, constant.ContextKeyTokenAutoGroups, []string{"default", "vip"})
	f.start()
	for _, u := range f.upstreams {
		awaitHedgeSignal(t, u.prefixRead)
	}
	close(f.upstreams[1].release)
	require.Nil(t, f.finish(t))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":99,"vip":99}`))
	require.NoError(t, ratio_setting.UpdateIncludeChannelRatioByJSONString(`{"default":true,"vip":false}`))
	close(f.upstreams[0].release)
	rows := f.logs(t, 2)
	assert.Equal(t, "default", rows[0].Group)
	assert.Equal(t, 28, rows[0].Quota)
	assert.Equal(t, "vip", rows[1].Group)
	assert.Equal(t, 84, rows[1].Quota)
}

func TestHedgePolicyResponsesFunctionSurcharge(t *testing.T) {
	operation_setting.SetToolPriceForTest("hedge_priced_function", 5)
	t.Cleanup(func() { operation_setting.DeleteToolPriceForTest("hedge_priced_function") })
	for _, winner := range []int{0, 1} {
		t.Run(fmt.Sprint("winner_", winner), func(t *testing.T) {
			f := newHedgePolicyFixture(t, true, 100000)
			common.CleanupBodyStorage(f.ctx)
			f.ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"hedge-test","stream":true,"input":"hello","tools":[{"type":"function","name":"hedge_priced_function","parameters":{"type":"object"}}]}`)).WithContext(f.ctx.Request.Context())
			f.ctx.Request.Header.Set("Content-Type", "application/json")
			req, err := helper.GetAndValidateRequest(f.ctx, types.RelayFormatOpenAIResponses)
			require.NoError(t, err)
			f.info, err = relaycommon.GenRelayInfo(f.ctx, types.RelayFormatOpenAIResponses, req, nil)
			require.NoError(t, err)
			for i, u := range f.upstreams {
				u.prefix = "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_test\",\"status\":\"in_progress\"}}\n\n"
				u.tail = fmt.Sprintf("data: {\"type\":\"response.output_text.delta\",\"delta\":\"answer-%d\"}\n\n", i) + "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":10,\"output_tokens\":2,\"total_tokens\":12}}}\n\ndata: [DONE]\n\n"
			}
			// The prefix has a completed call with empty arguments (neutral to the
			// content gate). Another completed call arrives after winner selection.
			f.upstreams[0].prefix += "data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"function_call\",\"id\":\"call_prefix\",\"name\":\"hedge_priced_function\",\"arguments\":\"\"}}\n\n"
			f.upstreams[0].tail = "data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"function_call\",\"id\":\"call_tail\",\"name\":\"hedge_priced_function\",\"arguments\":\"private-call-arguments\"}}\n\n" + f.upstreams[0].tail
			f.start()
			for _, u := range f.upstreams {
				awaitHedgeSignal(t, u.prefixRead)
			}
			close(f.upstreams[winner].release)
			require.Nil(t, f.finish(t))
			close(f.upstreams[1-winner].release)
			rows := f.logs(t, 2)
			assert.Equal(t, 5014, rows[0].Quota, "two $5/1K calls plus 14 token quota, counted exactly once")
			assert.Equal(t, 42, rows[1].Quota)
			assert.Contains(t, rows[0].Other, `"count":2`)
			assert.NotContains(t, rows[0].Other, "private-call-arguments")
			assert.Contains(t, f.recorder.Body.String(), fmt.Sprint("answer-", winner))
			assert.NotContains(t, f.recorder.Body.String(), fmt.Sprint("answer-", 1-winner))
			if winner == 1 {
				assert.NotContains(t, f.recorder.Body.String(), "private-call-arguments")
			}
			var user model.User
			require.NoError(t, f.db.First(&user, 7001).Error)
			assert.Equal(t, 94944, user.Quota)
			assert.Equal(t, 1, user.RequestCount)
		})
	}
}

func TestHedgePolicyCancellationDuringBackupAdmissionPreventsDispatch(t *testing.T) {
	f := newHedgePolicyFixture(t, true, 100000)
	var updates atomic.Int32
	require.NoError(t, f.db.Callback().Update().After("gorm:update").Register("hedge_cancel_during_admission", func(tx *gorm.DB) {
		if tx.Statement.Table == "users" {
			if updates.Add(1) == 2 {
				// Cancel after B's independent reservation is persisted, while
				// the coordinator is still inside synchronous admission.
				f.cancel()
			}
		}
	}))
	f.start()
	awaitHedgeSignal(t, f.upstreams[0].prefixRead)
	require.NotNil(t, f.finish(t))
	select {
	case <-f.upstreams[1].started:
		t.Fatal("backup dispatched after client cancellation during admission")
	default:
	}
	rows := f.logs(t, 2)
	for _, row := range rows {
		assert.Zero(t, row.Quota)
	}
	var user model.User
	require.NoError(t, f.db.First(&user, 7001).Error)
	assert.Equal(t, 100000, user.Quota)
	assert.Zero(t, user.RequestCount)
	assert.Empty(t, f.recorder.Body.String())
}
