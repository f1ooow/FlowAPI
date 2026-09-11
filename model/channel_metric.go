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
type ChannelMetric struct {
	Id             int   `json:"id" gorm:"primaryKey"`
	ChannelId      int   `json:"channel_id" gorm:"uniqueIndex:idx_channel_metric_channel_bucket,priority:1"`
	BucketTs       int64 `json:"bucket_ts" gorm:"uniqueIndex:idx_channel_metric_channel_bucket,priority:2;index:idx_channel_metric_bucket_ts"`
	AttemptCount   int64 `json:"-" gorm:"default:0"`
	SuccessCount   int64 `json:"-" gorm:"default:0"`
	TotalLatencyMs int64 `json:"-" gorm:"default:0"`
}

func (ChannelMetric) TableName() string {
	return "channel_metrics"
}

func UpsertChannelMetric(metric *ChannelMetric) error {
	if metric == nil || metric.AttemptCount == 0 {
		return nil
	}
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "channel_id"},
			{Name: "bucket_ts"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"attempt_count":    gorm.Expr("channel_metrics.attempt_count + ?", metric.AttemptCount),
			"success_count":    gorm.Expr("channel_metrics.success_count + ?", metric.SuccessCount),
			"total_latency_ms": gorm.Expr("channel_metrics.total_latency_ms + ?", metric.TotalLatencyMs),
		}),
	}).Create(metric).Error
}

// ChannelMetricBucket is one display bucket for one channel: the storage rows
// have already been rolled up by the SQL query below.
type ChannelMetricBucket struct {
	ChannelId      int   `json:"channel_id"`
	BucketTs       int64 `json:"bucket_ts"`
	AttemptCount   int64 `json:"attempt_count"`
	SuccessCount   int64 `json:"success_count"`
	TotalLatencyMs int64 `json:"total_latency_ms"`
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
		Select(fmt.Sprintf("channel_id, %s as bucket_ts, SUM(attempt_count) as attempt_count, SUM(success_count) as success_count, SUM(total_latency_ms) as total_latency_ms", bucketExpr)).
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
