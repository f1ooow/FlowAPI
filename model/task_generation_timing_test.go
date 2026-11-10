package model

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskGenerationTimingUsesRecentSuccessfulRouteSamples(t *testing.T) {
	truncateTables(t)
	const since, until = int64(1000), int64(2000)
	fixtures := []Task{
		{SubmitTime: 1880, StartTime: 1899, FinishTime: 1900}, // 20s, including queued time
		{SubmitTime: 1759, FinishTime: 1800},                  // 41s, never observed running
		{SubmitTime: 1640, FinishTime: 1700},                  // excluded by sample limit
		{SubmitTime: 1850, FinishTime: 1950, Status: TaskStatusFailure},
		{SubmitTime: 1850, FinishTime: 1950, Status: TaskStatusInProgress},
		{SubmitTime: 1850, FinishTime: 1950, ChannelId: 2},
		{SubmitTime: 1850, FinishTime: 1950, Properties: Properties{UpstreamModelName: "other"}},
		{SubmitTime: 1850, FinishTime: 1950, Properties: Properties{OriginModelName: "MiniMax-H3-Context-IR"}},
		{SubmitTime: 0, FinishTime: 1950},
		{SubmitTime: 1950, FinishTime: 1950},
		{SubmitTime: 1951, FinishTime: 1950},
		{SubmitTime: 800, FinishTime: 999},
		{SubmitTime: 1900, FinishTime: 2001},
	}
	for i := range fixtures {
		task := &fixtures[i]
		task.TaskID = fmt.Sprintf("timing-%d", i)
		task.UserId = i + 1 // statistics are aggregated across users, not raw tasks
		if task.Status == "" {
			task.Status = TaskStatusSuccess
		}
		if task.ChannelId == 0 {
			task.ChannelId = 1
		}
		if task.Properties.UpstreamModelName == "" {
			task.Properties.UpstreamModelName = "MiniMax-H3"
		}
		if task.Properties.OriginModelName == "" {
			task.Properties.OriginModelName = "MiniMax-H3-video"
		}
		insertTask(t, task)
	}
	got, err := GetTaskGenerationTiming(context.Background(), 1, "MiniMax-H3", "MiniMax-H3-video", since, until, 2)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, int64(2), got.SampleCount)
	assert.Equal(t, 30.5, got.AverageSeconds)

	got, err = GetTaskGenerationTiming(context.Background(), 1, "missing", "MiniMax-H3-video", since, until, 2)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestTaskGenerationTimingRejectsUnboundedOrUnknownIdentity(t *testing.T) {
	for _, tc := range []struct {
		channel, limit   int
		upstream, origin string
		since, until     int64
	}{
		{0, 100, "upstream", "origin", 1000, 2000},
		{1, 100, "", "origin", 1000, 2000},
		{1, 100, "upstream", "", 1000, 2000},
		{1, 0, "upstream", "origin", 1000, 2000},
		{1, 100, "upstream", "origin", 0, 2000},
		{1, 100, "upstream", "origin", 2000, 1000},
	} {
		got, err := GetTaskGenerationTiming(context.Background(), tc.channel, tc.upstream, tc.origin, tc.since, tc.until, tc.limit)
		require.NoError(t, err)
		assert.Nil(t, got)
	}
}
