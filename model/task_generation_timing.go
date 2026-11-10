package model

import (
	"context"

	"github.com/QuantumNous/new-api/common"
)

// TaskGenerationTiming describes observed acceptance-to-success duration, not
// provider progress or a task execution deadline.
type TaskGenerationTiming struct {
	AverageSeconds float64 `json:"average_seconds"`
	SampleCount    int64   `json:"sample_count"`
}

// GetTaskGenerationTiming aggregates only the latest successful tasks for an
// actual route. Origin model separates operations sharing an upstream model,
// such as video generation and H3 Context IR. No raw task data leaves the DB.
func GetTaskGenerationTiming(ctx context.Context, channelID int, upstreamModel, originModel string, since, until int64, sampleLimit int) (*TaskGenerationTiming, error) {
	if channelID <= 0 || upstreamModel == "" || originModel == "" || since <= 0 || until <= since || sampleLimit <= 0 {
		return nil, nil
	}
	upstreamExpr := "json_extract(properties, '$.upstream_model_name')"
	originExpr := "json_extract(properties, '$.origin_model_name')"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		upstreamExpr = "properties->>'upstream_model_name'"
		originExpr = "properties->>'origin_model_name'"
	} else if common.UsingMainDatabase(common.DatabaseTypeMySQL) {
		upstreamExpr = "JSON_UNQUOTE(JSON_EXTRACT(properties, '$.upstream_model_name'))"
		originExpr = "JSON_UNQUOTE(JSON_EXTRACT(properties, '$.origin_model_name'))"
	}
	samples := DB.WithContext(ctx).Model(&Task{}).
		Select("finish_time - submit_time AS duration_seconds").
		Where("channel_id = ? AND status = ?", channelID, TaskStatusSuccess).
		Where("submit_time > 0 AND finish_time > submit_time").
		Where("finish_time >= ? AND finish_time <= ?", since, until).
		Where(upstreamExpr+" = ? AND "+originExpr+" = ?", upstreamModel, originModel).
		Order("finish_time DESC, id DESC").Limit(sampleLimit)
	var result TaskGenerationTiming
	err := DB.WithContext(ctx).Table("(?) AS timing_samples", samples).
		Select("COUNT(*) AS sample_count, COALESCE(AVG(duration_seconds * 1.0), 0) AS average_seconds").
		Scan(&result).Error
	if err != nil {
		return nil, err
	}
	if result.SampleCount == 0 {
		return nil, nil
	}
	return &result, nil
}
