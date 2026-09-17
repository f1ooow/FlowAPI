package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHedgeRacingBillsIndependentAttempts(t *testing.T) {
	for _, include := range []bool{false, true} {
		for _, winning := range []int{0, 1} {
			t.Run(fmt.Sprintf("include_channel_%t_winner_%d", include, winning), func(t *testing.T) {
				db := setupModelListControllerTestDB(t)
				require.NoError(t, db.AutoMigrate(&model.Token{}, &model.Log{}))
				oldBatch, oldCache, oldLogs := common.BatchUpdateEnabled, common.MemoryCacheEnabled, common.LogConsumeEnabled
				common.BatchUpdateEnabled, common.MemoryCacheEnabled, common.LogConsumeEnabled = false, false, true
				oldModel, oldCompletion := ratio_setting.ModelRatio2JSONString(), ratio_setting.CompletionRatio2JSONString()
				oldGroup, oldInclude := ratio_setting.GroupRatio2JSONString(), ratio_setting.IncludeChannelRatio2JSONString()
				t.Cleanup(func() {
					common.BatchUpdateEnabled, common.MemoryCacheEnabled, common.LogConsumeEnabled = oldBatch, oldCache, oldLogs
					require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldModel))
					require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(oldCompletion))
					require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroup))
					require.NoError(t, ratio_setting.UpdateIncludeChannelRatioByJSONString(oldInclude))
				})
				require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"hedge-test":1}`))
				require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"hedge-test":2}`))
				require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":2}`))
				require.NoError(t, ratio_setting.UpdateIncludeChannelRatioByJSONString(fmt.Sprintf(`{"default":%t}`, include)))
				service.InitHttpClient()
				user := model.User{Id: 7001, Username: "hedge-user", Quota: 100000, Group: "default"}
				token := model.Token{Id: 7001, UserId: 7001, Key: "hedge-token", RemainQuota: 100000, Status: common.TokenStatusEnabled}
				require.NoError(t, db.Create(&user).Error)
				require.NoError(t, db.Create(&token).Error)
				started := []chan struct{}{make(chan struct{}), make(chan struct{})}
				release := []chan struct{}{make(chan struct{}), make(chan struct{})}
				channels := make([]*model.Channel, 2)
				for i := range channels {
					i := i
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "text/event-stream")
						fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n")
						w.(http.Flusher).Flush()
						close(started[i])
						select {
						case <-release[i]:
						case <-r.Context().Done():
							return
						}
						fmt.Fprintf(w, "data: {\"id\":\"up-%d\",\"model\":\"hedge-test\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"answer-%d\"}}]}\n\n", i, i)
						fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":2,\"total_tokens\":12}}\n\ndata: [DONE]\n\n")
					}))
					t.Cleanup(upstream.Close)
					weight, priority, ratio := uint(100), int64(10-i), []float64{0.5, 1.5}[i]
					setting := `{"reliability":{"first_content_timeout_seconds":1,"max_attempts":1}}`
					channels[i] = &model.Channel{Id: 7010 + i, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: fmt.Sprint("hedge-", i), Key: "up-key", BaseURL: &upstream.URL, Group: "default", Models: "hedge-test", Weight: &weight, Priority: &priority, CostRatio: &ratio, Setting: &setting}
					require.NoError(t, db.Create(channels[i]).Error)
					require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "hedge-test", ChannelId: channels[i].Id, Enabled: true, Priority: &priority, Weight: weight}).Error)
				}
				writer := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(writer)
				ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"hedge-test","stream":true,"messages":[{"role":"user","content":"hello"}]}`))
				ctx.Request.Header.Set("Content-Type", "application/json")
				ctx.Set(common.RequestIdKey, "shared-request")
				ctx.Set("id", user.Id)
				ctx.Set("token_id", token.Id)
				ctx.Set("token_key", token.Key)
				ctx.Set("token_name", "hedge-token")
				ctx.Set("token_group", "default")
				ctx.Set("group", "default")
				common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
				common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")
				require.Nil(t, middleware.SetupContextForSelectedChannel(ctx, channels[0], "hedge-test"))
				request, err := helper.GetAndValidateRequest(ctx, types.RelayFormatOpenAI)
				require.NoError(t, err)
				info, err := relaycommon.GenRelayInfo(ctx, types.RelayFormatOpenAI, request, nil)
				require.NoError(t, err)
				require.True(t, hedgeEligible(ctx, info, channels[0]))
				result := make(chan *types.NewAPIError, 1)
				go func() { result <- relayWithHedge(ctx, info, channels[0], 10, &types.TokenCountMeta{}) }()
				for _, signal := range started {
					select {
					case <-signal:
					case <-time.After(5 * time.Second):
						t.Fatal("upstream was not dispatched")
					}
				}
				var reserved model.User
				require.NoError(t, db.First(&reserved, user.Id).Error)
				assert.Equal(t, 98000, reserved.Quota, "A and B must both reserve, including trusted accounts")
				close(release[winning])
				select {
				case apiErr := <-result:
					require.Nil(t, apiErr)
				case <-time.After(5 * time.Second):
					t.Fatal("winner blocked on loser")
				}
				assert.Contains(t, writer.Body.String(), fmt.Sprint("answer-", winning))
				assert.NotContains(t, writer.Body.String(), fmt.Sprint("answer-", 1-winning))
				assert.Equal(t, channels[winning].Id, ctx.GetInt("channel_id"))
				common.CleanupBodyStorage(ctx)
				close(release[1-winning])
				// The losing row is inserted asynchronously after the client returns.
				require.Eventually(t, func() bool {
					var count int64
					db.Model(&model.Log{}).Where("request_id = ?", "shared-request").Count(&count)
					return count == 2
				}, 5*time.Second, 5*time.Millisecond)
				var logs []model.Log
				require.NoError(t, db.Order("channel_id").Find(&logs).Error)
				require.Len(t, logs, 2)
				expected := []int{28, 28}
				if include {
					expected = []int{14, 42}
				}
				for i, entry := range logs {
					assert.Equal(t, expected[i], entry.Quota)
					assert.Equal(t, "shared-request", entry.RequestId)
				}
				var finalUser model.User
				require.NoError(t, db.First(&finalUser, user.Id).Error)
				assert.Equal(t, 99944, finalUser.Quota)
				assert.Equal(t, 56, finalUser.UsedQuota)
				assert.Equal(t, 1, finalUser.RequestCount)
			})
		}
	}
}

func TestHedgeReplaySafety(t *testing.T) {
	for _, test := range []struct {
		body string
		safe bool
	}{
		{`{"tools":[{"type":"function","function":{"name":"local"}}]}`, true},
		{`{"previous_response_id":"resp_1"}`, false},
		{`{"tools":[{"type":"web_search"}]}`, false},
		{`{"tools":[{"codeExecution":{}}]}`, false},
		{`{"tools":[{"functionDeclarations":[]}]}`, true},
		{`{"tools":[{"name":"local","input_schema":{}}]}`, true},
		{`{"background":true}`, false},
		{`{"modalities":["audio"]}`, false},
		{`{"generationConfig":{"response_modalities":["IMAGE"]}}`, false},
		{`{"generationConfig":{"response_modalities":["TEXT","AUDIO"]}}`, false},
		{`{"generationConfig":{"responseModalities":["IMAGE"]}}`, false},
		{`{"generation_config":{"responseModalities":["AUDIO"]}}`, false},
		{`{"generation_config":{"response_modalities":["IMAGE"]}}`, false},
		{`{"generationConfig":{"response_modalities":["TEXT"]}}`, true},
		{`{"GenerationConfig":{"Response_Modalities":["IMAGE"]}}`, false},
		{`{"cachedContent":"cachedContents/shared"}`, false},
		{`{"cached_content":"cachedContents/shared"}`, false},
	} {
		assert.Equal(t, test.safe, hedgeReplaySafe([]byte(test.body)), test.body)
	}
}

func TestHedgePrivateWriterDiscardsUncommittedOutput(t *testing.T) {
	w := &hedgeResponseWriter{header: make(http.Header), status: http.StatusOK, size: -1}
	_, err := w.WriteString("loser")
	require.NoError(t, err)
	assert.Equal(t, 5, w.Size())
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	w.target = ctx.Writer
	_, err = w.WriteString("winner")
	require.NoError(t, err)
	assert.Equal(t, "winner", recorder.Body.String())
}

func TestHedgeDrainDeadlineCancelsAttemptStillWaitingForHeaders(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &hedgeWorker{info: &relaycommon.RelayInfo{Hedge: &relaycommon.HedgeAttempt{}}, cancel: cancel, done: make(chan struct{})}
	go func() { <-ctx.Done(); close(w.done) }()
	detachHedgeLoser(w, time.Millisecond)
	select {
	case <-w.done:
	case <-time.After(time.Second):
		t.Fatal("unanswered losing attempt was not cancelled")
	}
	assert.True(t, w.info.Hedge.Detached.Load())
}
