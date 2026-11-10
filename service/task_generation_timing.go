package service

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
	"github.com/samber/hot"
	"golang.org/x/sync/singleflight"
)

var videoTimingCacheOnce sync.Once
var videoTimingCache *cachex.HybridCache[*model.TaskGenerationTiming]
var videoTimingLoads singleflight.Group

// GetVideoTaskGenerationTiming is optional display metadata. Call only after
// authenticating ownership of task; route identity must come from that task.
func GetVideoTaskGenerationTiming(ctx context.Context, task *model.Task) (*model.TaskGenerationTiming, error) {
	settings := perf_metrics_setting.GetSetting()
	now := time.Now().Unix()
	if task == nil || task.ChannelId <= 0 || task.Properties.UpstreamModelName == "" || task.Properties.OriginModelName == "" ||
		!settings.Enabled || settings.RetentionDays <= 0 || int64(settings.RetentionDays) >= now/(24*60*60) || settings.TaskTimingSampleLimit <= 0 {
		return nil, nil
	}
	minutes := perf_metrics_setting.GetFlushIntervalMinutes()
	if int64(minutes) > math.MaxInt64/int64(time.Minute) {
		return nil, nil
	}
	ttl := time.Duration(minutes) * time.Minute
	videoTimingCacheOnce.Do(func() {
		// Operator-configurable memory bound, shared by all route identities.
		capacity := common.GetEnvOrDefault("VIDEO_TASK_TIMING_CACHE_CAP", 1000)
		if capacity <= 0 {
			capacity = 1000
		}
		videoTimingCache = cachex.NewHybridCache[*model.TaskGenerationTiming](cachex.HybridCacheConfig[*model.TaskGenerationTiming]{
			Namespace:    "new-api:video_task_timing:v1",
			Redis:        common.RDB,
			RedisEnabled: func() bool { return common.RedisEnabled && common.RDB != nil },
			RedisCodec:   cachex.JSONCodec[*model.TaskGenerationTiming]{},
			Memory: func() *hot.HotCache[string, *model.TaskGenerationTiming] {
				return hot.NewHotCache[string, *model.TaskGenerationTiming](hot.LRU, capacity).WithTTL(ttl).WithJanitor().Build()
			},
		})
	})
	key := fmt.Sprintf("%d:%q:%q:%d:%d:%d", task.ChannelId, task.Properties.UpstreamModelName, task.Properties.OriginModelName, settings.RetentionDays, settings.TaskTimingSampleLimit, minutes)
	if value, found, err := videoTimingCache.Get(key); err != nil || found {
		return value, err
	}
	value, err, _ := videoTimingLoads.Do(key, func() (any, error) {
		if cached, found, err := videoTimingCache.Get(key); err != nil || found {
			return cached, err
		}
		timing, queryErr := model.GetTaskGenerationTiming(ctx, task.ChannelId, task.Properties.UpstreamModelName, task.Properties.OriginModelName,
			now-int64(settings.RetentionDays)*24*60*60, now, settings.TaskTimingSampleLimit)
		// Cache empty results (including query failures) so polling cannot hammer
		// an unavailable statistics query. A cancelled request may retry normally.
		if ctx.Err() != nil {
			return (*model.TaskGenerationTiming)(nil), ctx.Err()
		}
		cacheErr := videoTimingCache.SetWithTTL(key, timing, ttl)
		if queryErr != nil {
			return (*model.TaskGenerationTiming)(nil), queryErr
		}
		return timing, cacheErr
	})
	if err != nil {
		return nil, err
	}
	return value.(*model.TaskGenerationTiming), nil
}
