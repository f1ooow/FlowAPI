package perfmetrics

import (
	"fmt"
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
}

type atomicChannelBucket struct {
	attemptCount   atomic.Int64
	successCount   atomic.Int64
	totalLatencyMs atomic.Int64
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

func (b *atomicChannelBucket) drain() channelCounters {
	return channelCounters{
		attemptCount:   b.attemptCount.Swap(0),
		successCount:   b.successCount.Swap(0),
		totalLatencyMs: b.totalLatencyMs.Swap(0),
	}
}

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

func flushCompletedChannelBuckets() {
	currentBucket := bucketStart(time.Now().Unix())
	channelHotBuckets.Range(func(key, value any) bool {
		k := key.(channelBucketKey)
		if k.bucketTs >= currentBucket {
			return true
		}

		bucket := value.(*atomicChannelBucket)
		drained := bucket.drain()
		if drained.attemptCount == 0 {
			deleteOldEmptyChannelBucket(k, key)
			return true
		}

		err := model.UpsertChannelMetric(&model.ChannelMetric{
			ChannelId:      k.channelId,
			BucketTs:       k.bucketTs,
			AttemptCount:   drained.attemptCount,
			SuccessCount:   drained.successCount,
			TotalLatencyMs: drained.totalLatencyMs,
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
