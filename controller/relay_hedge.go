package controller

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const hedgeDrainTimeout = 120 * time.Second

var hedgeDrainSlots = make(chan struct{}, 64)

func hedgeChannelAllowed(ch *model.Channel) bool {
	if ch == nil || ch.Status != common.ChannelStatusEnabled {
		return false
	}
	apiType, _ := common.ChannelType2APIType(ch.Type)
	switch apiType {
	case constant.APITypeOpenAI, constant.APITypeOpenRouter, constant.APITypeXinference, constant.APITypeAnthropic, constant.APITypeGemini:
		// Parameter overrides can inject upstream tools or stateful dependencies.
		return len(ch.GetParamOverride()) == 0
	default:
		return false
	}
}

func hedgeEligible(c *gin.Context, info *relaycommon.RelayInfo, initial *model.Channel) bool {
	modelName := strings.ToLower(info.OriginModelName)
	if strings.Contains(modelName, "audio") || strings.Contains(modelName, "-search") {
		return false
	}
	if !info.IsStream || !hedgeChannelAllowed(initial) || service.ShouldSkipRetryAfterChannelAffinityFailure(c) {
		return false
	}
	if _, specific := c.Get("specific_channel_id"); specific {
		return false
	}
	if len(common.GetContextKeyStringMap(c, constant.ContextKeyChannelParamOverride)) != 0 {
		return false
	}
	s := initial.GetReliabilitySettings()
	if s.FirstContentTimeoutSeconds == nil || *s.FirstContentTimeoutSeconds <= 0 {
		return false
	}
	switch info.RelayMode {
	case relayconstant.RelayModeChatCompletions, relayconstant.RelayModeResponses:
	default:
		if info.RelayFormat != types.RelayFormatClaude && info.RelayFormat != types.RelayFormatGemini {
			return false
		}
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return false
	}
	body, err := storage.Bytes()
	return err == nil && hedgeReplaySafe(body)
}

func hedgeReplaySafe(body []byte) bool {
	root := gjson.ParseBytes(body)
	for _, path := range []string{"previous_response_id", "conversation", "audio", "web_search_options", "cachedContent", "cached_content"} {
		if root.Get(path).Exists() {
			return false
		}
	}
	if root.Get("background").Bool() {
		return false
	}
	for _, modality := range root.Get("modalities").Array() {
		if modality.String() != "text" {
			return false
		}
	}
	var gemini struct {
		GenerationConfig      dto.GeminiChatGenerationConfig `json:"generationConfig"`
		GenerationConfigSnake dto.GeminiChatGenerationConfig `json:"generation_config"`
	}
	if common.Unmarshal(body, &gemini) != nil {
		return false
	}
	for _, config := range []dto.GeminiChatGenerationConfig{gemini.GenerationConfig, gemini.GenerationConfigSnake} {
		for _, modality := range config.ResponseModalities {
			if !strings.EqualFold(modality, "text") {
				return false
			}
		}
	}
	for _, tool := range root.Get("tools").Array() {
		typeName := tool.Get("type").String()
		if typeName == "function" {
			continue
		}
		if typeName == "" && tool.Get("name").Exists() && tool.Get("input_schema").Exists() {
			continue
		}
		if typeName == "" && (tool.Get("functionDeclarations").Exists() || tool.Get("function_declarations").Exists()) && len(tool.Map()) == 1 {
			continue
		}
		return false
	}
	return true
}

type hedgeWorker struct {
	c              *gin.Context
	info           *relaycommon.RelayInfo
	channel        *model.Channel
	writer         *hedgeResponseWriter
	cancel         context.CancelFunc
	done           chan struct{}
	err            *types.NewAPIError
	started        time.Time
	attempt        int
	accounting     chan struct{}
	accountingOnce sync.Once
	history        []service.RouteAttempt
}

// No worker may settle until the coordinator has frozen its billing role.
func (w *hedgeWorker) decideBilling(billableLoser bool, history []service.RouteAttempt) {
	w.accountingOnce.Do(func() {
		w.info.Hedge.BillableLoser = billableLoser
		w.history = append([]service.RouteAttempt(nil), history...)
		close(w.accounting)
	})
}

type hedgeGateOffer struct {
	worker   *hedgeWorker
	accepted chan bool
}

// Selection, admission and winner promotion run on this goroutine. Workers
// own fresh Gin contexts, DTOs, billing sessions and response sinks.
func relayWithHedge(c *gin.Context, original *relaycommon.RelayInfo, initial *model.Channel, tokens int, meta *types.TokenCountMeta) *types.NewAPIError {
	billLosers := operation_setting.SnapshotGeneralSetting().BillHedgeLosers
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return types.NewError(err, types.ErrorCodeReadRequestBodyFailed)
	}
	baseline := cloneHedgeContext(c)
	selection := cloneHedgeContext(c)
	state := service.NewRouteState()
	c.Set("resilient_route_started", true)
	c.Set("resilient_route", true)
	offers := make(chan hedgeGateOffer)
	results := make(chan *hedgeWorker, 2)
	thresholds := make(chan *hedgeWorker, 2)
	finished := make(chan struct{})
	defer close(finished)
	active := make(map[int]*hedgeWorker)
	var winner *hedgeWorker
	var prefixMu sync.Mutex
	var lastErr *types.NewAPIError
	ordinal := 0
	clientContext := c.Request.Context()
	clientDone := clientContext.Done()

	launch := func(ch *model.Channel, source *gin.Context, first bool) *types.NewAPIError {
		if err := clientContext.Err(); err != nil {
			return types.NewError(err, types.ErrorCodeDoRequestFailed, types.ErrOptionWithSkipRetry())
		}
		ctx := cloneHedgeContext(source)
		attemptCtx, cancel := context.WithCancel(context.Background())
		ctx.Request = source.Request.Clone(attemptCtx)
		fork, forkErr := common.ForkBodyStorage(storage)
		if forkErr != nil {
			cancel()
			return types.NewError(forkErr, types.ErrorCodeReadRequestBodyFailed)
		}
		ctx.Set(common.KeyBodyStorage, fork)
		ctx.Request.Body = io.NopCloser(fork)
		if !first {
			if setupErr := middleware.SetupContextForSelectedChannel(ctx, ch, original.OriginModelName); setupErr != nil {
				fork.Close()
				cancel()
				return setupErr
			}
		}
		if len(common.GetContextKeyStringMap(ctx, constant.ContextKeyChannelParamOverride)) != 0 {
			fork.Close()
			cancel()
			return types.NewError(errors.New("parameter overrides require serial routing"), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
		}
		request, requestErr := helper.GetAndValidateRequest(ctx, original.RelayFormat)
		if requestErr != nil {
			fork.Close()
			cancel()
			return types.NewError(requestErr, types.ErrorCodeInvalidRequest)
		}
		info, infoErr := relaycommon.GenRelayInfo(ctx, original.RelayFormat, request, nil)
		if infoErr != nil {
			fork.Close()
			cancel()
			return types.NewError(infoErr, types.ErrorCodeGenRelayInfoFailed)
		}
		info.SetEstimatePromptTokens(tokens)
		info.InitChannelMeta(ctx)
		info.ForcePreConsume = true
		info.DisablePing = true
		ordinal++
		info.Hedge = &relaycommon.HedgeAttempt{ID: common.NewRequestId(), Number: ordinal, PrefixMu: &prefixMu}
		// Billing identities are independent; the public Gin request ID is unchanged.
		info.RequestId = info.Hedge.ID
		price, priceErr := helper.ModelPriceHelper(ctx, info, tokens, meta)
		if priceErr != nil {
			fork.Close()
			cancel()
			return types.NewError(priceErr, types.ErrorCodeModelPriceError)
		}
		if !price.FreeModel {
			if billingErr := service.PreConsumeBilling(ctx, price.QuotaToPreConsume, info); billingErr != nil {
				fork.Close()
				cancel()
				return billingErr
			}
		}
		attempt := state.BeginAttempt(ch.Id)
		ctx.Set("resilient_route", true)
		ctx.Set("resilient_route_started", true)
		ctx.Set("use_channel", []string{fmt.Sprint(ch.Id)})
		w := &hedgeWorker{c: ctx, info: info, channel: ch, cancel: cancel, done: make(chan struct{}), accounting: make(chan struct{}), started: time.Now(), attempt: attempt}
		w.writer = &hedgeResponseWriter{header: c.Writer.Header().Clone(), status: http.StatusOK, size: -1}
		ctx.Writer = w.writer
		info.Hedge.Commit = func() bool {
			accepted := make(chan bool, 1)
			select {
			case offers <- hedgeGateOffer{w, accepted}:
				select {
				case yes := <-accepted:
					return yes
				case <-finished:
					return false
				case <-attemptCtx.Done():
					return false
				}
			case <-finished:
				return false
			case <-attemptCtx.Done():
				return false
			}
		}
		var dispatchOnce sync.Once
		info.Hedge.Dispatched = func() {
			dispatchOnce.Do(func() {
				if clientContext.Err() != nil {
					cancel()
					return
				}
				settings := ch.GetReliabilitySettings()
				if settings.FirstContentTimeoutSeconds == nil || *settings.FirstContentTimeoutSeconds <= 0 {
					return
				}
				go func() {
					timer := time.NewTimer(time.Duration(*settings.FirstContentTimeoutSeconds) * time.Second)
					defer timer.Stop()
					select {
					case <-timer.C:
						select {
						case thresholds <- w:
						case <-finished:
						case <-w.done:
						}
					case <-w.done:
					case <-finished:
					}
				}()
			})
		}
		active[ch.Id] = w
		state.Record(ch.Id, attempt, "hedge_launched", "")
		logger.LogInfo(ctx, fmt.Sprintf("hedge attempt %d launched on channel %d", ordinal, ch.Id))
		go func() {
			defer close(w.done)
			defer cancel()
			defer fork.Close()
			defer service.CleanupFileSources(ctx)
			if err := clientContext.Err(); err != nil {
				// Admission may have blocked on billing after the launch check.
				// Release that reservation through normal finalization, without
				// allowing the newly admitted worker to dispatch.
				w.err = types.NewError(err, types.ErrorCodeDoRequestFailed, types.ErrOptionWithSkipRetry())
			} else {
				switch original.RelayFormat {
				case types.RelayFormatClaude:
					w.err = relay.ClaudeHelper(ctx, info)
				case types.RelayFormatGemini:
					w.err = geminiRelayHandler(ctx, info)
				default:
					w.err = relayHandler(ctx, info)
				}
			}
			if w.err == nil && !info.Hedge.Winner.Load() && (info.StreamStatus == nil || !info.StreamStatus.IsCommitted()) {
				w.err = types.NewOpenAIError(errors.New("hedge candidate did not produce a gated stream"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
			}
			// Report execution first. Finalization must wait for the serialized
			// winner/cancellation decision even if this worker finishes early.
			results <- w
			<-w.accounting
			outcome := service.RouteAttemptOutcome("hedge_loser")
			if info.Hedge.Winner.Load() {
				outcome = service.RouteAttemptSucceeded
			}
			ctx.Set("route_history", append(w.history, service.RouteAttempt{ChannelID: ch.Id, ChannelName: ch.Name, Attempt: attempt, Outcome: outcome}))
			service.FinalizeHedgeAttempt(ctx, info)
		}()
		return nil
	}

	if apiErr := launch(initial, baseline, true); apiErr != nil {
		return apiErr
	}
	for len(active) > 0 {
		select {
		case offer := <-offers:
			w := offer.worker
			prefixMu.Lock()
			if winner == nil && c.Request.Context().Err() == nil {
				winner = w
				w.info.Hedge.Winner.Store(true)
				w.writer.target = c.Writer
				for key, values := range w.writer.header {
					c.Writer.Header()[key] = append([]string(nil), values...)
				}
				state.Record(w.channel.Id, w.attempt, "hedge_winner", "first_content")
				for _, peer := range active {
					if peer != winner {
						if billLosers && peer.info.Hedge.ValidPrefix.Load() {
							state.Record(peer.channel.Id, peer.attempt, "hedge_draining", "")
							peer.decideBilling(true, state.History)
							detachHedgeLoser(peer, hedgeDrainTimeout)
						} else {
							state.Record(peer.channel.Id, peer.attempt, "hedge_cancelled", "loser_billing_policy")
							peer.decideBilling(false, state.History)
							peer.cancel()
						}
					}
				}
				w.decideBilling(false, state.History)
			}
			prefixMu.Unlock()
			offer.accepted <- winner == w
		case w := <-thresholds:
			if winner != nil || clientContext.Err() != nil || active[w.channel.Id] != w || len(active) >= 2 {
				continue
			}
			state.Record(w.channel.Id, w.attempt, "hedge_threshold", "first_content_timeout")
			excluded := make(map[int]struct{}, len(state.ExcludedChannelIDs)+len(active))
			for id := range state.ExcludedChannelIDs {
				excluded[id] = struct{}{}
			}
			for id := range active {
				excluded[id] = struct{}{}
			}
			if !state.CanSelectMoreChannels() {
				continue
			}
			candidate, _, selectErr := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{Ctx: selection, TokenGroup: original.TokenGroup, ModelName: original.OriginModelName, RequestPath: c.Request.URL.Path, ExcludedChannelIDs: excluded, CandidateAllowed: hedgeChannelAllowed})
			if selectErr != nil || candidate == nil {
				state.Record(w.channel.Id, w.attempt, "hedge_skipped", "no_eligible_backup")
				c.Set("route_history", append([]service.RouteAttempt(nil), state.History...))
				continue
			}
			if admissionErr := launch(candidate, selection, false); admissionErr != nil {
				state.Record(candidate.Id, 0, "hedge_skipped", "backup_admission_failed")
				c.Set("route_history", append([]service.RouteAttempt(nil), state.History...))
				logger.LogWarn(c, "hedge backup not admitted: "+admissionErr.Error())
			}
		case w := <-results:
			// Actual failures before a winner have no eligible losing response
			// at winner selection. Return only this attempt's reservation.
			w.decideBilling(false, state.History)
			<-w.done
			delete(active, w.channel.Id)
			if winner != nil {
				if w != winner {
					continue
				}
				// Promote only a completed winner's routing/affinity state. No worker
				// accesses the original pooled Gin context after this function returns.
				for key, value := range w.c.Keys {
					if key != common.KeyBodyStorage && key != common.KeyRequestBody && key != string(constant.ContextKeyFileSourcesToCleanup) {
						c.Set(key, value)
					}
				}
				logRouteHistory(c, state)
				completed := w.err == nil && w.info.StreamStatus != nil && w.info.StreamStatus.IsNormalEnd() && !w.info.StreamStatus.HasErrors()
				perfmetrics.RecordChannelAttempt(w.info, w.channel.Id, w.started, completed)
				if completed {
					service.RecordAutoBanSuccess(w.channel.Id)
				}
				return w.err
			}
			if w.err == nil {
				w.err = types.NewOpenAIError(errors.New("stream ended before winner selection"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
			}
			lastErr = w.err
			decision := service.ClassifyRelayRetry(c, w.err, false)
			perfmetrics.RecordChannelAttempt(w.info, w.channel.Id, w.started, false)
			if !decision.Retryable {
				for _, peer := range active {
					peer.decideBilling(false, state.History)
					peer.cancel()
				}
				for _, peer := range active {
					<-peer.done
				}
				return lastErr
			}
			service.ClearCurrentChannelAffinityCache(selection)
			if state.CanAttempt(w.channel.Id, w.channel.GetReliabilitySettings().MaxAttempts) {
				if admissionErr := launch(w.channel, w.c, false); admissionErr == nil {
					continue
				} else {
					lastErr = admissionErr
				}
			}
			state.Exclude(w.channel.Id)
			state.Record(w.channel.Id, w.attempt, service.RouteAttemptExcluded, decision.Reason)
			if decision.AutoBanEligible {
				service.RecordAutoBanFailure(*types.NewChannelError(w.channel.Id, w.channel.Type, w.channel.Name, w.channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(w.c, constant.ContextKeyChannelKey), w.channel.GetAutoBan()), w.err.ErrorWithStatusCode())
			}
			excluded := make(map[int]struct{}, len(state.ExcludedChannelIDs)+len(active))
			for id := range state.ExcludedChannelIDs {
				excluded[id] = struct{}{}
			}
			for id := range active {
				excluded[id] = struct{}{}
			}
			if !state.CanSelectMoreChannels() {
				continue
			}
			candidate, _, selectErr := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{Ctx: selection, TokenGroup: original.TokenGroup, ModelName: original.OriginModelName, RequestPath: c.Request.URL.Path, ExcludedChannelIDs: excluded, CandidateAllowed: hedgeChannelAllowed})
			if selectErr == nil && candidate != nil {
				if admissionErr := launch(candidate, selection, false); admissionErr != nil {
					lastErr = admissionErr
				}
			}
		case <-clientDone:
			clientDone = nil
			if winner != nil {
				winner.cancel()
				continue
			}
			for _, w := range active {
				w.decideBilling(false, state.History)
				w.cancel()
			}
			for _, w := range active {
				<-w.done
			}
			return types.NewOpenAIError(c.Request.Context().Err(), types.ErrorCodeDoRequestFailed, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
		}
	}
	logRouteHistory(c, state)
	return lastErr
}

func detachHedgeLoser(w *hedgeWorker, timeout time.Duration) {
	w.info.Hedge.Detached.Store(true)
	select {
	case hedgeDrainSlots <- struct{}{}:
		go func() {
			defer func() { <-hedgeDrainSlots }()
			timer := time.NewTimer(timeout)
			defer timer.Stop()
			select {
			case <-w.done:
			case <-timer.C:
				w.cancel()
				<-w.done
			}
		}()
	default:
		w.cancel()
	}
}

func cloneHedgeContext(source *gin.Context) *gin.Context {
	ctx := source.Copy()
	for key, value := range ctx.Keys {
		switch value := value.(type) {
		case map[string]any:
			data, err := common.Marshal(value)
			var cloned map[string]any
			if err == nil && common.Unmarshal(data, &cloned) == nil {
				ctx.Keys[key] = cloned
			}
		case []string:
			ctx.Keys[key] = append([]string(nil), value...)
		}
	}
	delete(ctx.Keys, "route_state")
	delete(ctx.Keys, "route_history")
	delete(ctx.Keys, string(constant.ContextKeyFileSourcesToCleanup))
	return ctx
}

// A private response sink prevents even adapter trailers and error paths from
// leaking losing bytes. Only semantic winner admission installs target.
type hedgeResponseWriter struct {
	target gin.ResponseWriter
	header http.Header
	status int
	size   int
}

func (w *hedgeResponseWriter) Header() http.Header {
	if w.target != nil {
		return w.target.Header()
	}
	return w.header
}
func (w *hedgeResponseWriter) WriteHeader(code int) {
	if w.target != nil {
		w.target.WriteHeader(code)
	}
	w.status = code
}
func (w *hedgeResponseWriter) WriteHeaderNow() {
	if w.target != nil {
		w.target.WriteHeaderNow()
	}
	if w.size < 0 {
		w.size = 0
	}
}
func (w *hedgeResponseWriter) Write(p []byte) (int, error) {
	if w.target != nil {
		return w.target.Write(p)
	}
	w.WriteHeaderNow()
	w.size += len(p)
	return len(p), nil
}
func (w *hedgeResponseWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }
func (w *hedgeResponseWriter) Status() int {
	if w.target != nil {
		return w.target.Status()
	}
	return w.status
}
func (w *hedgeResponseWriter) Size() int {
	if w.target != nil {
		return w.target.Size()
	}
	return w.size
}
func (w *hedgeResponseWriter) Written() bool { return w.Size() >= 0 }
func (w *hedgeResponseWriter) Flush() {
	if w.target != nil {
		w.target.Flush()
	}
}
func (w *hedgeResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, errors.New("hedged streams cannot hijack")
}
func (w *hedgeResponseWriter) CloseNotify() <-chan bool { return nil }
func (w *hedgeResponseWriter) Pusher() http.Pusher      { return nil }
func (w *hedgeResponseWriter) SetWriteDeadline(t time.Time) error {
	if w.target != nil {
		return http.NewResponseController(w.target).SetWriteDeadline(t)
	}
	return nil
}
