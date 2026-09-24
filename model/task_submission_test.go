package model

import (
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskSubmissionConcurrentReservationAndOwnerIsolation(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&TaskSubmission{}))
	t.Cleanup(func() { DB.Exec("DELETE FROM task_submissions"); DB.Exec("DELETE FROM tasks") })
	start := make(chan struct{})
	type result struct {
		submission *TaskSubmission
		owner      bool
		err        error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			s, o, e := ReserveTaskSubmission(1, 2, "CaseSensitive-Key", "fingerprint")
			results <- result{s, o, e}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	owners := 0
	taskID := ""
	for r := range results {
		require.NoError(t, r.err)
		if r.owner {
			owners++
		}
		if taskID != "" {
			assert.Equal(t, taskID, r.submission.TaskID)
		}
		taskID = r.submission.TaskID
	}
	assert.Equal(t, 1, owners)
	_, _, err := ReserveTaskSubmission(1, 2, "CaseSensitive-Key", "different payload")
	require.ErrorIs(t, err, ErrSubmissionConflict)
	for _, identity := range []struct {
		user, token int
		key         string
	}{{2, 2, "CaseSensitive-Key"}, {1, 3, "CaseSensitive-Key"}, {1, 2, "casesensitive-key"}} {
		s, owner, err := ReserveTaskSubmission(identity.user, identity.token, identity.key, "fingerprint")
		require.NoError(t, err)
		assert.True(t, owner)
		assert.NotEqual(t, taskID, s.TaskID)
	}
}

func TestTaskSubmissionCrashRemainsUnknownAndAcceptanceIsAtomic(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&TaskSubmission{}))
	t.Cleanup(func() { DB.Exec("DELETE FROM task_submissions"); DB.Exec("DELETE FROM tasks") })
	s, owner, err := ReserveTaskSubmission(1, 2, "crash-key", "body")
	require.NoError(t, err)
	require.True(t, owner)
	require.NoError(t, DB.Model(s).Update("created_at", time.Now().Unix()-601).Error)
	read, err := GetTaskSubmission(1, 2, "crash-key")
	require.NoError(t, err)
	assert.Equal(t, "unknown", read.Status)
	replay, owner, err := ReserveTaskSubmission(1, 2, "crash-key", "body")
	require.NoError(t, err)
	assert.False(t, owner)
	assert.Equal(t, s.TaskID, replay.TaskID)
	task := &Task{TaskID: s.TaskID, UserId: 1, PrivateData: TaskPrivateData{TokenId: 2}}
	require.Error(t, InsertTaskForSubmission(task, s.ID+100))
	var count int64
	require.NoError(t, DB.Model(&Task{}).Where("task_id = ?", s.TaskID).Count(&count).Error)
	assert.Zero(t, count, "failed association must roll back task insertion")
	task.ID = 0
	require.NoError(t, InsertTaskForSubmission(task, s.ID))
	require.NoError(t, FinishTaskSubmission(s.ID, true))
	read, err = GetTaskSubmission(1, 2, "crash-key")
	require.NoError(t, err)
	assert.Equal(t, "accepted", read.Status, "finish must not erase a durable accepted result")
}

func TestTaskSubmissionDispatchIsDurableAndBindsAccount(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&TaskSubmission{}))
	t.Cleanup(func() { DB.Exec("DELETE FROM task_submissions"); DB.Exec("DELETE FROM tasks") })
	s, owner, err := ReserveTaskSubmission(21, 22, "dispatch-route", "digest")
	require.NoError(t, err)
	require.True(t, owner)
	route := TaskSubmissionRoute{ChannelID: 42, ChannelType: 1, APIKey: "private-account", BaseURL: "https://provider.example", UpstreamModelName: "wan3.0-video-720p", ReservedQuota: 100}
	require.Error(t, PrepareTaskSubmissionDispatch(s.ID, 21, 23, route))
	require.NoError(t, PrepareTaskSubmissionDispatch(s.ID, 21, 22, route))
	require.Error(t, PrepareTaskSubmissionDispatch(s.ID, 21, 22, route))
	require.NoError(t, FinishTaskSubmission(s.ID, false))
	saved, err := GetTaskSubmission(21, 22, "dispatch-route")
	require.NoError(t, err)
	assert.Equal(t, "unknown", saved.Status)
	assert.NotEmpty(t, saved.RouteJSON)
	task := &Task{TaskID: s.TaskID, UserId: 21, ChannelId: 43, PrivateData: TaskPrivateData{TokenId: 22, Key: route.APIKey}, Properties: Properties{UpstreamModelName: route.UpstreamModelName}}
	require.Error(t, InsertTaskForSubmission(task, s.ID))
	task.ChannelId = 42
	require.NoError(t, InsertTaskForSubmission(task, s.ID))
	require.NoError(t, FinishTaskSubmission(s.ID, true))
	saved, err = GetTaskSubmission(21, 22, "dispatch-route")
	require.NoError(t, err)
	assert.Equal(t, "accepted", saved.Status)
}

func TestRecordTaskSubmissionDispatchSnapshotsRelayRouteAndKeepsCredentialPrivate(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&TaskSubmission{}))
	t.Cleanup(func() { DB.Exec("DELETE FROM task_submissions") })
	submission, owner, err := ReserveTaskSubmission(31, 32, "relay-info-route", "digest")
	require.NoError(t, err)
	require.True(t, owner)
	effectiveRatio := 2.4
	info := &relaycommon.RelayInfo{
		UserId:          31,
		TokenId:         32,
		UsingGroup:      "pro",
		OriginModelName: "wan-origin",
		BillingSource:   "subscription",
		SubscriptionId:  44,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:            42,
			ChannelType:          101,
			ChannelBaseUrl:       "https://wan.example",
			ApiKey:               "provider-secret",
			UpstreamModelName:    "wan-3.0-time",
			ChannelOtherSettings: dto.ChannelOtherSettings{VideoProtocol: "wan-rolldek-v1"},
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{BillingUnit: "output_seconds"},
		PriceData: types.PriceData{
			Quota:          123,
			ModelPrice:     .6,
			GroupRatioInfo: types.GroupRatioInfo{GroupRatio: effectiveRatio, BaseGroupRatio: 2, UserGroupRatio: .8, ChannelRatio: 1.5, IncludeChannelRatio: true},
		},
	}

	require.NoError(t, RecordTaskSubmissionDispatch(submission.ID, info))
	saved, err := GetTaskSubmission(31, 32, "relay-info-route")
	require.NoError(t, err)
	assert.Equal(t, 42, saved.ChannelID)
	assert.Equal(t, 101, saved.ChannelType)
	assert.Equal(t, "provider-secret", saved.APIKey)
	assert.Equal(t, "wan-3.0-time", saved.UpstreamModelName)
	assert.Equal(t, 123, saved.ReservedQuota)
	assert.Equal(t, "subscription", saved.FundingSource)
	assert.Equal(t, 44, saved.SubscriptionID)
	assert.Equal(t, "pro", saved.Group)
	assert.NotZero(t, saved.DispatchedAt)

	var route TaskSubmissionRoute
	require.NoError(t, common.UnmarshalJsonStr(saved.RouteJSON, &route))
	assert.Equal(t, "provider-secret", route.APIKey)
	assert.Equal(t, "wan-rolldek-v1", route.ChannelOtherSettings.VideoProtocol)
	require.NotNil(t, route.BillingContext)
	assert.Equal(t, "output_seconds", route.BillingContext.BillingUnit)
	assert.Equal(t, effectiveRatio, *route.BillingContext.EffectiveRatio)
	var billingContext TaskBillingContext
	require.NoError(t, common.UnmarshalJsonStr(saved.BillingContextJSON, &billingContext))
	assert.Equal(t, effectiveRatio, *billingContext.EffectiveRatio)

	encoded, err := common.Marshal(saved)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "provider-secret")
	assert.Error(t, RecordTaskSubmissionDispatch(submission.ID, info), "a recorded dispatch must never permit a second paid POST")
}

func TestTaskSubmissionDefiniteRejectionDoesNotStayUnknown(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&TaskSubmission{}))
	t.Cleanup(func() { DB.Exec("DELETE FROM task_submissions") })
	s, _, err := ReserveTaskSubmission(1, 2, "rejected-request", "fingerprint")
	require.NoError(t, err)
	require.NoError(t, PrepareTaskSubmissionDispatch(s.ID, 1, 2, TaskSubmissionRoute{ChannelID: 1, APIKey: "key", UpstreamModelName: "wan3.0-video-720p"}))
	require.NoError(t, FinishTaskSubmission(s.ID, true, true))
	saved, err := GetTaskSubmission(1, 2, "rejected-request")
	require.NoError(t, err)
	assert.Equal(t, "failed", saved.Status)
	_, owner, err := ReserveTaskSubmission(1, 2, "rejected-request", "fingerprint")
	require.NoError(t, err)
	assert.False(t, owner)
}
