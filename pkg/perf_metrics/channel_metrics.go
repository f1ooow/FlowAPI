package perfmetrics

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
)

// channelHotBuckets holds the channel-availability counters that have not been
// flushed yet. It lives next to hotBuckets and is drained by the same
// flushLoop: both share the bucket width, the flush interval and the retention
// period, so a second ticker would only duplicate lifecycle management. The
// keys, the table and the queries stay separate.
var channelHotBuckets sync.Map

type channelBucketKey struct {
	channelId int
	bucketTs  int64
}

type channelCounters struct {
	attemptCount   int64
	successCount   int64
	totalLatencyMs int64

	cacheRequestCount int64
	cacheSignalCount  int64
	cacheReadTokens   int64
	cacheWriteTokens  int64
	cacheInputTokens  int64
}

// hasSamples mirrors model.ChannelMetric.hasSamples: the availability counters
// and the cache counters are fed by two different call sites, so a drained
// bucket can legitimately carry only one of the two families.
func (c channelCounters) hasSamples() bool {
	return c.attemptCount != 0 || c.cacheRequestCount != 0
}

type atomicChannelBucket struct {
	attemptCount   atomic.Int64
	successCount   atomic.Int64
	totalLatencyMs atomic.Int64

	cacheRequestCount atomic.Int64
	cacheSignalCount  atomic.Int64
	cacheReadTokens   atomic.Int64
	cacheWriteTokens  atomic.Int64
	cacheInputTokens  atomic.Int64
}

func (b *atomicChannelBucket) add(latencyMs int64, success bool) {
	b.attemptCount.Add(1)
	if success {
		b.successCount.Add(1)
	}
	if latencyMs > 0 {
		b.totalLatencyMs.Add(latencyMs)
	}
}

// addCacheUsage applies the CCH pre-filter: a request whose upstream reported
// neither a cache read nor a cache write says nothing about cache effectiveness
// and would only dilute the ratio, so it counts towards cacheRequestCount (the
// engagement denominator) and nothing else. Token sums stay restricted to the same sample
// set as cacheSignalCount, which keeps `read / input` a ratio of two figures
// drawn from the same requests.
func (b *atomicChannelBucket) addCacheUsage(sample ChannelCacheSample) {
	b.cacheRequestCount.Add(1)
	readTokens := clampChannelCacheTokens(sample.CacheReadTokens)
	writeTokens := clampChannelCacheTokens(sample.CacheWriteTokens)
	if readTokens == 0 && writeTokens == 0 {
		return
	}
	b.cacheSignalCount.Add(1)
	b.cacheReadTokens.Add(readTokens)
	b.cacheWriteTokens.Add(writeTokens)
	b.cacheInputTokens.Add(clampChannelCacheTokens(sample.CacheInputTokens))
}

func (b *atomicChannelBucket) drain() channelCounters {
	return channelCounters{
		attemptCount:      b.attemptCount.Swap(0),
		successCount:      b.successCount.Swap(0),
		totalLatencyMs:    b.totalLatencyMs.Swap(0),
		cacheRequestCount: b.cacheRequestCount.Swap(0),
		cacheSignalCount:  b.cacheSignalCount.Swap(0),
		cacheReadTokens:   b.cacheReadTokens.Swap(0),
		cacheWriteTokens:  b.cacheWriteTokens.Swap(0),
		cacheInputTokens:  b.cacheInputTokens.Swap(0),
	}
}

// addCounters puts a failed flush back into the hot bucket. Every counter has
// to come back, otherwise a failed flush would keep the attempts and lose the
// cache sample and the two families would disagree from then on.
func (b *atomicChannelBucket) addCounters(c channelCounters) {
	if c.attemptCount != 0 {
		b.attemptCount.Add(c.attemptCount)
	}
	if c.successCount != 0 {
		b.successCount.Add(c.successCount)
	}
	if c.totalLatencyMs != 0 {
		b.totalLatencyMs.Add(c.totalLatencyMs)
	}
	if c.cacheRequestCount != 0 {
		b.cacheRequestCount.Add(c.cacheRequestCount)
	}
	if c.cacheSignalCount != 0 {
		b.cacheSignalCount.Add(c.cacheSignalCount)
	}
	if c.cacheReadTokens != 0 {
		b.cacheReadTokens.Add(c.cacheReadTokens)
	}
	if c.cacheWriteTokens != 0 {
		b.cacheWriteTokens.Add(c.cacheWriteTokens)
	}
	if c.cacheInputTokens != 0 {
		b.cacheInputTokens.Add(c.cacheInputTokens)
	}
}

// channelCacheTokenSampleLimit bounds one request's contribution. Usage token
// counts come straight out of an upstream response and are never validated
// anywhere in the relay path, so a single absurd value (a wrapped negative
// arriving as 18446744073686646784, for instance) would otherwise ruin the
// whole bucket's ratio. The columns are int64 and this is the same magnitude
// the request validators use for max_tokens, so the accumulation itself cannot
// realistically overflow. These are plain statistics counters, not quota, so
// they deliberately do not go through common/quota_math.go: its bounds mean
// "quota" and its clamps raise billing-saturation audit events.
const channelCacheTokenSampleLimit = int64(math.MaxInt32)

func clampChannelCacheTokens(tokens int64) int64 {
	if tokens <= 0 {
		return 0
	}
	if tokens > channelCacheTokenSampleLimit {
		common.SysError(fmt.Sprintf("channel cache usage sample %d exceeds the per-request bound, clamped to %d", tokens, channelCacheTokenSampleLimit))
		return channelCacheTokenSampleLimit
	}
	return tokens
}

// RecordChannelAttempt records exactly one attempt against one channel.
//
// 口径：attempt 级，不是请求级。一个用户请求 failover 三次会调用三次，写出 3 条
// 分属不同渠道的记录 —— 只有这样，被 failover 绕过的故障渠道才会真的掉可用率。
// 请求级打点（RecordRelaySample 的做法）只把结果记在最后一个渠道上，渠道 A 500
// 之后重试渠道 B 成功时 A 的失败完全不可见，可用率恒定接近 100%。
//
// 任务类 relay（Midjourney / Suno / 视频）只记提交阶段的成败与耗时；异步执行结果
// 属于业务结果而非网关到上游的可用性，不回写渠道可用率。
//
// 两类情形刻意不打点：
//   - channelId <= 0，即还没选中渠道就失败（无可用渠道、鉴权失败、模型不存在）。
//     这类失败不归属任何具体渠道，计进任意一个渠道都是错误归因。注意此时
//     info.ChannelMeta 还是 nil（GenRelayInfo 不初始化这个嵌入指针），所以本函数
//     只用调用方显式传入的 channelId，绝不读 info.ChannelId。IsChannelTest 是
//     RelayInfo 自身的字段而不是 ChannelMeta 上的，nil 时读它是安全的。
//   - 渠道测试等合成流量：它不是真实用户请求，不应移动渠道的真实可用率。
func RecordChannelAttempt(info *relaycommon.RelayInfo, channelId int, attemptStart time.Time, success bool) {
	if info == nil || info.IsChannelTest || channelId <= 0 {
		return
	}
	if !perf_metrics_setting.GetSetting().Enabled {
		return
	}
	latencyMs := time.Since(attemptStart).Milliseconds()
	if latencyMs < 0 {
		latencyMs = 0
	}
	key := channelBucketKey{
		channelId: channelId,
		bucketTs:  bucketStart(time.Now().Unix()),
	}
	actual, _ := channelHotBuckets.LoadOrStore(key, &atomicChannelBucket{})
	actual.(*atomicChannelBucket).add(latencyMs, success)
}

// ChannelCacheSample is one settled request's contribution to the channel cache
// statistics. The caller owns the usage-semantic normalization: CacheInputTokens
// must already be the normalized total input context length, because
// prompt_tokens means different things depending on the upstream format and this
// package cannot tell them apart.
type ChannelCacheSample struct {
	CacheReadTokens  int64
	CacheWriteTokens int64
	CacheInputTokens int64
}

// RecordChannelCacheUsage records the cache side of one successfully settled
// request against one channel.
//
// 口径与 RecordChannelAttempt **不同**，两者共用一张表但不共用分母：
// 失败的 attempt 根本没有 usage（RelayInfo 不携带最终 usage，结算在 relay helper
// 内部完成），所以缓存统计只能挂在成功结算路径上，是**成功请求级**。
// attempt_count 不是缓存命中率的分母，cache_request_count / cache_signal_count 才是。
//
// 本轮刻意不覆盖 Realtime 与 Audio：这两条路径的日志生成器（GenerateWssOtherInfo /
// GenerateAudioOtherInfo）传的 cacheTokens 恒为 0，即使 realtime 上游确实返回了
// cached_tokens，缓存量没有归一化进 dto.Usage。补齐那条链路是另一件事，不在这里做。
//
// 与 RecordChannelAttempt 一样跳过渠道测试等合成流量与未归属渠道的样本。
func RecordChannelCacheUsage(info *relaycommon.RelayInfo, channelId int, sample ChannelCacheSample) {
	if info == nil || info.IsChannelTest || channelId <= 0 {
		return
	}
	if !perf_metrics_setting.GetSetting().Enabled {
		return
	}
	key := channelBucketKey{
		channelId: channelId,
		bucketTs:  bucketStart(time.Now().Unix()),
	}
	actual, _ := channelHotBuckets.LoadOrStore(key, &atomicChannelBucket{})
	actual.(*atomicChannelBucket).addCacheUsage(sample)
}

func flushCompletedChannelBuckets() {
	currentBucket := bucketStart(time.Now().Unix())
	channelHotBuckets.Range(func(key, value any) bool {
		k := key.(channelBucketKey)
		if k.bucketTs >= currentBucket {
			return true
		}

		bucket := value.(*atomicChannelBucket)
		drained := bucket.drain()
		if !drained.hasSamples() {
			deleteOldEmptyChannelBucket(k, key)
			return true
		}

		err := model.UpsertChannelMetric(&model.ChannelMetric{
			ChannelId:         k.channelId,
			BucketTs:          k.bucketTs,
			AttemptCount:      drained.attemptCount,
			SuccessCount:      drained.successCount,
			TotalLatencyMs:    drained.totalLatencyMs,
			CacheRequestCount: drained.cacheRequestCount,
			CacheSignalCount:  drained.cacheSignalCount,
			CacheReadTokens:   drained.cacheReadTokens,
			CacheWriteTokens:  drained.cacheWriteTokens,
			CacheInputTokens:  drained.cacheInputTokens,
		})
		if err != nil {
			bucket.addCounters(drained)
			common.SysError(fmt.Sprintf("failed to flush channel metric bucket channel=%d bucket=%d: %s", k.channelId, k.bucketTs, err.Error()))
			return true
		}

		deleteOldEmptyChannelBucket(k, key)
		return true
	})
}

func deleteOldEmptyChannelBucket(k channelBucketKey, rawKey any) {
	if k.bucketTs < bucketStart(time.Now().Add(-24*time.Hour).Unix()) {
		channelHotBuckets.Delete(rawKey)
	}
}

func cleanupExpiredChannelMetrics(retentionDays int) {
	if retentionDays <= 0 {
		return
	}
	cutoff := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour).Unix()
	if err := model.DeleteChannelMetricsBefore(cutoff); err != nil {
		common.SysError("failed to cleanup expired channel metrics: " + err.Error())
	}
}
