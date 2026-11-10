package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVideoTaskGenerationTimingCachesSamplesAndInvalidatesOnConfigChange(t *testing.T) {
	truncate(t)
	if videoTimingCache != nil {
		require.NoError(t, videoTimingCache.Purge())
	}
	t.Cleanup(func() {
		if videoTimingCache != nil {
			require.NoError(t, videoTimingCache.Purge())
		}
	})
	settings := config.GlobalConfig.Get("perf_metrics_setting").(*perf_metrics_setting.PerfMetricsSetting)
	old := *settings
	t.Cleanup(func() { *settings = old })
	settings.Enabled = true
	settings.RetentionDays = 7
	settings.TaskTimingSampleLimit = 100
	settings.FlushInterval = 5
	task := &model.Task{ChannelId: 901, Properties: model.Properties{UpstreamModelName: "timing-upstream", OriginModelName: "timing-video"}}
	history := *task
	history.TaskID = "cached-history"
	history.Status = model.TaskStatusSuccess
	history.FinishTime = time.Now().Unix() - 10
	history.SubmitTime = history.FinishTime - 25
	require.NoError(t, model.DB.Create(&history).Error)
	got, err := GetVideoTaskGenerationTiming(context.Background(), task)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 25.0, got.AverageSeconds)
	require.NoError(t, model.DB.Delete(&history).Error)
	cached, err := GetVideoTaskGenerationTiming(context.Background(), task)
	require.NoError(t, err)
	assert.Equal(t, got, cached)

	settings.TaskTimingSampleLimit = 1
	got, err = GetVideoTaskGenerationTiming(context.Background(), task)
	require.NoError(t, err)
	assert.Nil(t, got)
	history.ID = 0
	require.NoError(t, model.DB.Create(&history).Error)
	got, err = GetVideoTaskGenerationTiming(context.Background(), task)
	require.NoError(t, err)
	assert.Nil(t, got, "empty history must also be cached")

	settings.RetentionDays = 0
	got, err = GetVideoTaskGenerationTiming(context.Background(), task)
	require.NoError(t, err)
	assert.Nil(t, got, "invalid retention must not enable an unbounded scan")
	settings.RetentionDays = 7
	settings.Enabled = false
	got, err = GetVideoTaskGenerationTiming(context.Background(), task)
	require.NoError(t, err)
	assert.Nil(t, got)
}
