package model

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ChannelMetric aggregates channel availability per time bucket for the admin
// channel monitoring page.
//
// 口径：AttemptCount 是「渠道尝试次数」而不是「用户请求数」。一个用户请求
// failover 三次会写出 3 条分属不同渠道的记录，这正是渠道可用率需要的语义 ——
// 请求级打点只会把成功记在最后一个渠道上，被绕过的故障渠道的失败完全不可见，
// 可用率会恒定接近 100%。
//
// 这张表刻意与 perf_metrics 分开：给 perf_metrics 的唯一索引加 channel_id 在
// 老库上不会生效（GORM 只按索引名建索引、从不修改已有索引），MySQL 会静默把
// 不同渠道的计数累加进同一行，PG/SQLite 则每次 flush 报 42P10。
//
// 缓存计数列（Cache*）与上面的可用率计数列**口径不同**，不可互相当分母：
// 失败的 attempt 没有 usage，所以缓存量只能在成功结算路径打点，是「成功请求级」，
// 而 AttemptCount 是「渠道尝试级」。命中率的分母永远是 CacheSignalCount /
// CacheInputTokens，绝不是 AttemptCount。
//
// 这五列都是普通 int64 列：新增普通列 GORM AutoMigrate 会 ALTER TABLE ADD COLUMN，
// 三库都支持；上面那条唯一索引刻意不碰，因为 GORM 从不修改已有索引（见 R1 注释）。
type ChannelMetric struct {
	Id             int   `json:"id" gorm:"primaryKey"`
	ChannelId      int   `json:"channel_id" gorm:"uniqueIndex:idx_channel_metric_channel_bucket,priority:1"`
	BucketTs       int64 `json:"bucket_ts" gorm:"uniqueIndex:idx_channel_metric_channel_bucket,priority:2;index:idx_channel_metric_bucket_ts"`
	AttemptCount   int64 `json:"-" gorm:"default:0"`
	SuccessCount   int64 `json:"-" gorm:"default:0"`
	TotalLatencyMs int64 `json:"-" gorm:"default:0"`
	// CacheRequestCount 是成功结算且上游给了 usage 的请求数（CCH 的 totalRequests），
	// 只作为 engagement 的分母。
	CacheRequestCount int64 `json:"-" gorm:"default:0"`
	// CacheSignalCount 是通过前置过滤（cache_read>0 || cache_write>0）的请求数
	// （CCH 的 cacheSignalRequests），即命中率的样本量。
	CacheSignalCount int64 `json:"-" gorm:"default:0"`
	// 以下三列只累加「有缓存信号」的请求，与 CacheSignalCount 同一批样本。
	CacheReadTokens  int64 `json:"-" gorm:"default:0"`
	CacheWriteTokens int64 `json:"-" gorm:"default:0"`
	// CacheInputTokens 是命中率的分母，按 usage semantic 归一化后的总输入长度
	// （与 service 层的 inputLen 同口径），不是各家裸 prompt_tokens 的和。
	CacheInputTokens int64 `json:"-" gorm:"default:0"`
}

func (ChannelMetric) TableName() string {
	return "channel_metrics"
}

// hasSamples reports whether the row carries anything worth writing. The two
// counter families arrive independently: an availability-only flush has no
// cache sample, and a cache-only flush (a bucket where every attempt landed in
// the previous bucket but the settlement landed in this one) has no attempt.
// Checking AttemptCount alone would silently drop the latter.
func (metric *ChannelMetric) hasSamples() bool {
	return metric.AttemptCount != 0 || metric.CacheRequestCount != 0
}

func UpsertChannelMetric(metric *ChannelMetric) error {
	if metric == nil || !metric.hasSamples() {
		return nil
	}
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "channel_id"},
			{Name: "bucket_ts"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"attempt_count":       gorm.Expr("channel_metrics.attempt_count + ?", metric.AttemptCount),
			"success_count":       gorm.Expr("channel_metrics.success_count + ?", metric.SuccessCount),
			"total_latency_ms":    gorm.Expr("channel_metrics.total_latency_ms + ?", metric.TotalLatencyMs),
			"cache_request_count": gorm.Expr("channel_metrics.cache_request_count + ?", metric.CacheRequestCount),
			"cache_signal_count":  gorm.Expr("channel_metrics.cache_signal_count + ?", metric.CacheSignalCount),
			"cache_read_tokens":   gorm.Expr("channel_metrics.cache_read_tokens + ?", metric.CacheReadTokens),
			"cache_write_tokens":  gorm.Expr("channel_metrics.cache_write_tokens + ?", metric.CacheWriteTokens),
			"cache_input_tokens":  gorm.Expr("channel_metrics.cache_input_tokens + ?", metric.CacheInputTokens),
		}),
	}).Create(metric).Error
}

// ChannelMetricBucket is one display bucket for one channel: the storage rows
// have already been rolled up by the SQL query below.
type ChannelMetricBucket struct {
	ChannelId         int   `json:"channel_id"`
	BucketTs          int64 `json:"bucket_ts"`
	AttemptCount      int64 `json:"attempt_count"`
	SuccessCount      int64 `json:"success_count"`
	TotalLatencyMs    int64 `json:"total_latency_ms"`
	CacheRequestCount int64 `json:"cache_request_count"`
	CacheSignalCount  int64 `json:"cache_signal_count"`
	CacheReadTokens   int64 `json:"cache_read_tokens"`
	CacheWriteTokens  int64 `json:"cache_write_tokens"`
	CacheInputTokens  int64 `json:"cache_input_tokens"`
}

// channelMetricBucketExpr rolls storage buckets up into wider display buckets
// inside SQL. 7d at a 5 minute storage width times 100 channels is ~200k rows,
// too many to pull into memory just to sum them.
//
// This is integer arithmetic on a column that already holds whole seconds, not
// a SQL time function: strftime / DATE_FORMAT / date_trunc disagree across the
// three supported dialects and stay banned. MySQL is the dialect that needs
// FLOOR because its "/" operator yields a DECIMAL; PostgreSQL and SQLite both
// truncate integer division, and bucket_ts is never negative. Same shape as
// rankingBucketExpr in usedata_rankings.go.
func channelMetricBucketExpr(stepSeconds int64) string {
	if common.UsingMainDatabase(common.DatabaseTypeMySQL) {
		return fmt.Sprintf("FLOOR(bucket_ts / %d) * %d", stepSeconds, stepSeconds)
	}
	return fmt.Sprintf("(bucket_ts / %d) * %d", stepSeconds, stepSeconds)
}

func GetChannelMetricBuckets(startTs int64, endTs int64, stepSeconds int64) ([]ChannelMetricBucket, error) {
	buckets := make([]ChannelMetricBucket, 0)
	if stepSeconds <= 0 {
		stepSeconds = 300
	}
	bucketExpr := channelMetricBucketExpr(stepSeconds)
	err := DB.Model(&ChannelMetric{}).
		Select(fmt.Sprintf("channel_id, %s as bucket_ts, SUM(attempt_count) as attempt_count, SUM(success_count) as success_count, SUM(total_latency_ms) as total_latency_ms, SUM(cache_request_count) as cache_request_count, SUM(cache_signal_count) as cache_signal_count, SUM(cache_read_tokens) as cache_read_tokens, SUM(cache_write_tokens) as cache_write_tokens, SUM(cache_input_tokens) as cache_input_tokens", bucketExpr)).
		Where("bucket_ts >= ? AND bucket_ts <= ?", startTs, endTs).
		Group(fmt.Sprintf("channel_id, %s", bucketExpr)).
		Order("bucket_ts ASC").
		Find(&buckets).Error
	return buckets, err
}

// ChannelIdentity is the minimal channel row the monitoring page needs. The
// names are resolved with a separate query and mapped back in Go rather than
// joined, matching how GetAllLogs enriches channel names.
type ChannelIdentity struct {
	Id   int    `json:"id" gorm:"column:id"`
	Name string `json:"name" gorm:"column:name"`
}

func GetChannelIdentities() ([]ChannelIdentity, error) {
	identities := make([]ChannelIdentity, 0)
	err := DB.Table("channels").Select("id, name").Order("id ASC").Find(&identities).Error
	return identities, err
}

func DeleteChannelMetricsBefore(cutoffTs int64) error {
	if cutoffTs <= 0 {
		return nil
	}
	return DB.Where("bucket_ts < ?", cutoffTs).Delete(&ChannelMetric{}).Error
}
