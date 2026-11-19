package model

import (
	"context"
	"database/sql"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newH3AdmissionTask(userID, tokenID, quota int) Task {
	confirmed := true
	return Task{TaskID: GenerateTaskID(), UserId: userID, Quota: quota, Status: TaskStatusAdmitted,
		PrivateData: TaskPrivateData{TokenId: tokenID, BillingSource: "wallet", H3ReservationConfirmed: &confirmed}}
}

func finishH3AdmissionTask(t *testing.T, task *Task, final int) {
	t.Helper()
	require.NoError(t, task.BeginH3Dispatch())
	task.Status, task.FinishTime, task.PendingQuota = TaskStatusSuccess, 1, &final
	won, err := task.UpdateWithStatus(TaskStatusDispatching)
	require.NoError(t, err)
	require.True(t, won)
	won, err = task.FinalizeH3Quota()
	require.NoError(t, err)
	require.True(t, won)
}

type h3CommitFaultPool struct {
	*sql.DB
	rollback bool
	unavailable bool
}

func (pool *h3CommitFaultPool) BeginTx(ctx context.Context, opts *sql.TxOptions) (gorm.ConnPool, error) {
	if pool.unavailable {
		return nil, errors.New("injected database outage")
	}
	tx, err := pool.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &h3CommitFaultTx{Tx: tx, pool: pool}, nil
}

func (pool *h3CommitFaultPool) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if pool.unavailable {
		return nil, errors.New("injected database outage")
	}
	return pool.DB.QueryContext(ctx, query, args...)
}

type h3CommitFaultTx struct {
	*sql.Tx
	pool *h3CommitFaultPool
}

func (tx *h3CommitFaultTx) Commit() error {
	var err error
	if tx.pool.rollback {
		err = tx.Tx.Rollback()
	} else {
		err = tx.Tx.Commit()
	}
	if err != nil {
		return err
	}
	tx.pool.unavailable = true
	return errors.New("injected lost COMMIT response")
}

func TestH3LostAdmissionCommitRecoversAfterDatabaseRestoration(t *testing.T) {
	for _, tc := range []struct {
		name string
		rollback bool
	}{{"committed", false}, {"rolled back", true}} {
		t.Run(tc.name, func(t *testing.T) {
			truncateTables(t)
			user := createReserveTestUser(t, 100)
			token := createReserveTestToken(t, 80)
			token.UserId = user.Id
			require.NoError(t, DB.Save(&token).Error)
			task := newH3AdmissionTask(user.Id, token.Id, 30)
			publicID := task.TaskID
			original := DB
			sqlDB, err := original.DB()
			require.NoError(t, err)
			DB = original.WithContext(t.Context())
			DB.Statement.ConnPool = &h3CommitFaultPool{DB: sqlDB, rollback: tc.rollback}
			t.Cleanup(func() { DB = original })
			err = task.ReserveH3Dispatch(token.Key, false)
			require.ErrorIs(t, err, ErrH3AdmissionUnconfirmed)
			require.ErrorContains(t, err, "lost COMMIT response")
			assert.Equal(t, publicID, task.TaskID)
			assert.Zero(t, task.ID)
			_, _, err = GetByTaskId(user.Id, publicID)
			require.ErrorContains(t, err, "injected database outage")
			DB = original
			stored, exists, err := GetByTaskId(user.Id, publicID)
			require.NoError(t, err)
			require.Equal(t, !tc.rollback, exists)
			if exists {
				require.Equal(t, TaskStatusAdmitted, stored.Status)
				require.NoError(t, RecoverUnsentH3Admissions(t.Context(), stored.CreatedAt-1))
				require.Equal(t, 70, getUserQuotaFromDB(t, user.Id))
				require.NoError(t, RecoverUnsentH3Admissions(t.Context(), stored.CreatedAt))
				require.Error(t, stored.BeginH3Dispatch())
				stored, _, err = GetByTaskId(user.Id, publicID)
				require.NoError(t, err)
				require.Equal(t, TaskStatus(TaskStatusFailure), stored.Status)
				require.Equal(t, common.GetPointer(0), stored.PendingQuota)
				won, err := stored.FinalizeH3Quota()
				require.NoError(t, err)
				require.True(t, won)
			}
			require.NoError(t, RecoverUnsentH3Admissions(t.Context(), common.GetTimestamp()))
			assert.Equal(t, 100, getUserQuotaFromDB(t, user.Id))
			assert.Equal(t, 80, getTokenFromDB(t, token.Id).RemainQuota)
		})
	}
}

func TestH3AdmissionRollsBackTaskFundsAndUsageOnSQLFailure(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)
	user := createReserveTestUser(t, 100)
	token := createReserveTestToken(t, 80)
	token.UserId = user.Id
	require.NoError(t, DB.Save(&token).Error)
	require.NoError(t, populateUserCache(user))
	_, err := cacheInitToken(token)
	require.NoError(t, err)
	channel := Channel{Name: "h3-atomic-usage"}
	require.NoError(t, DB.Create(&channel).Error)
	task := newH3AdmissionTask(user.Id, token.Id, 30)
	task.ChannelId = channel.Id
	task.PrivateData.ChannelGeneration = channel.Generation
	require.NoError(t, DB.Exec(`CREATE TRIGGER fail_h3_usage BEFORE UPDATE OF used_quota ON channels
BEGIN SELECT RAISE(ABORT, 'injected channel usage failure'); END`).Error)
	t.Cleanup(func() { require.NoError(t, DB.Exec("DROP TRIGGER IF EXISTS fail_h3_usage").Error) })
	require.ErrorContains(t, task.ReserveH3Dispatch(token.Key, false), "injected channel usage failure")
	assert.Zero(t, task.ID)
	var count int64
	require.NoError(t, DB.Model(&Task{}).Where("task_id = ?", task.TaskID).Count(&count).Error)
	assert.Zero(t, count)
	wallet, err := GetUserById(user.Id, true)
	require.NoError(t, err)
	assert.Equal(t, 100, wallet.Quota)
	assert.Zero(t, wallet.RequestCount)
	assert.Zero(t, wallet.UsedQuota)
	assert.Zero(t, wallet.H3QuotaSeq)
	stored := getTokenFromDB(t, token.Id)
	assert.Equal(t, 80, stored.RemainQuota)
	assert.Zero(t, stored.UsedQuota)
	assert.Zero(t, stored.H3QuotaSeq)
	require.NoError(t, DB.First(&channel, channel.Id).Error)
	assert.Zero(t, channel.UsedQuota)
	cachedUser, err := cacheGetUserBase(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 100, cachedUser.Quota)
	cachedToken, err := cacheGetTokenByKey(token.Key)
	require.NoError(t, err)
	assert.Equal(t, 80, cachedToken.RemainQuota)
	assert.Zero(t, cachedToken.UsedQuota)
	require.NoError(t, DB.Exec("DROP TRIGGER fail_h3_usage").Error)
	require.NoError(t, task.ReserveH3Dispatch(token.Key, false))
	duplicate := task
	duplicate.ID = 0
	require.ErrorIs(t, duplicate.ReserveH3Dispatch(token.Key, false), ErrH3DispatchExists)
	wallet, err = GetUserById(user.Id, true)
	require.NoError(t, err)
	assert.Equal(t, 70, wallet.Quota)
	assert.Equal(t, 30, wallet.UsedQuota)
	assert.Equal(t, 1, wallet.RequestCount)
	assert.Equal(t, 50, getTokenFromDB(t, token.Id).RemainQuota)
	require.NoError(t, DB.First(&channel, channel.Id).Error)
	assert.EqualValues(t, 30, channel.UsedQuota)
}

type h3PublicationBarrier struct {
	key             string
	paused          atomic.Bool
	entered, resume chan struct{}
}

func (barrier *h3PublicationBarrier) BeforeProcess(ctx context.Context, cmd redis.Cmder) (context.Context, error) {
	if cmd.Name() == "hget" && cmd.Args()[1] == barrier.key && cmd.Args()[2] == "H3QuotaNet" && barrier.paused.CompareAndSwap(false, true) {
		close(barrier.entered)
		<-barrier.resume
	}
	return ctx, nil
}

func (*h3PublicationBarrier) AfterProcess(context.Context, redis.Cmder) error { return nil }
func (*h3PublicationBarrier) BeforeProcessPipeline(ctx context.Context, _ []redis.Cmder) (context.Context, error) {
	return ctx, nil
}
func (*h3PublicationBarrier) AfterProcessPipeline(context.Context, []redis.Cmder) error { return nil }

func TestH3OverlappingReservationsKeepOutstandingBatchDebits(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)
	common.BatchUpdateEnabled = true
	user := createReserveTestUser(t, 100)
	token := createReserveTestToken(t, 100)
	token.UserId = user.Id
	require.NoError(t, DB.Save(&token).Error)
	ok, err := TryReserveUserQuota(user.Id, 20)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = TryReserveTokenQuota(token.Id, token.Key, 20, false)
	require.NoError(t, err)
	require.True(t, ok)
	barrier := &h3PublicationBarrier{key: getUserCacheKey(user.Id), entered: make(chan struct{}), resume: make(chan struct{})}
	common.RDB.AddHook(barrier)
	task := newH3AdmissionTask(user.Id, token.Id, 50)
	done := make(chan error, 1)
	go func() { done <- task.ReserveH3Dispatch(token.Key, false) }()
	select {
	case err := <-done:
		t.Fatalf("admission returned before publication: %v", err)
	case <-barrier.entered:
	}
	released := false
	defer func() {
		if !released {
			close(barrier.resume)
			<-done
		}
	}()
	overlap := newH3AdmissionTask(user.Id, token.Id, 31)
	require.ErrorIs(t, overlap.ReserveH3Dispatch(token.Key, false), ErrH3WalletQuota)
	ok, err = TryReserveTokenQuota(token.Id, token.Key, 31, false)
	require.NoError(t, err)
	require.False(t, ok)
	close(barrier.resume)
	released = true
	require.NoError(t, <-done)
	reservedUser, err := GetUserById(user.Id, true)
	require.NoError(t, err)
	reservedToken := getTokenFromDB(t, token.Id)
	require.Equal(t, 50, reservedUser.Quota)
	require.Equal(t, 50, reservedToken.RemainQuota)
	finishH3AdmissionTask(t, &task, 30)
	refundedUser, err := GetUserById(user.Id, true)
	require.NoError(t, err)
	refundedToken := getTokenFromDB(t, token.Id)
	for _, replay := range []h3QuotaAccount{refundedUser.h3Wallet(), reservedToken.h3Token(), reservedUser.h3Wallet(), refundedToken.h3Token()} {
		require.NoError(t, replay.publish(""))
	}
	cachedUser, err := cacheGetUserBase(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 50, cachedUser.Quota)
	cachedToken, err := cacheGetTokenByKey(token.Key)
	require.NoError(t, err)
	assert.Equal(t, 50, cachedToken.RemainQuota)
	assert.Equal(t, 50, cachedToken.UsedQuota)
	batchUpdate()
	assert.Equal(t, 50, getUserQuotaFromDB(t, user.Id))
	assert.Equal(t, 50, getTokenFromDB(t, token.Id).RemainQuota)
}

func TestH3AdmissionChecksFrozenFundingAgainstLiveAccounts(t *testing.T) {
	truncateTables(t)
	user := createReserveTestUser(t, 0)
	token := createReserveTestToken(t, 0)
	token.UserId = user.Id
	require.NoError(t, DB.Save(&token).Error)
	task := newH3AdmissionTask(user.Id, token.Id, 30)
	task.PrivateData.BillingSource = "unlimited"
	require.ErrorIs(t, task.ReserveH3Dispatch(token.Key, true), ErrH3WalletQuota)
	require.NoError(t, DB.Model(&user).Update("unlimited_quota", true).Error)
	require.ErrorIs(t, task.ReserveH3Dispatch(token.Key, true), ErrH3TokenQuota)
	require.NoError(t, DB.Model(&token).Update("unlimited_quota", true).Error)
	require.ErrorIs(t, task.ReserveH3Dispatch("wrong-key", true), ErrH3TokenQuota)
	other := createReserveTestUser(t, 50)
	foreign := newH3AdmissionTask(other.Id, token.Id, 30)
	require.ErrorIs(t, foreign.ReserveH3Dispatch(token.Key, true), ErrH3TokenQuota)
	require.Equal(t, 50, getUserQuotaFromDB(t, other.Id))
	require.NoError(t, task.ReserveH3Dispatch(token.Key, true))
	assert.Zero(t, getUserQuotaFromDB(t, user.Id))
	assert.Equal(t, -30, getTokenFromDB(t, token.Id).RemainQuota)
	require.NoError(t, DB.Delete(&token).Error)
	deletedToken := newH3AdmissionTask(other.Id, token.Id, 30)
	deletedToken.UserId, deletedToken.PrivateData.BillingSource = user.Id, "unlimited"
	require.ErrorIs(t, deletedToken.ReserveH3Dispatch(token.Key, true), ErrH3TokenQuota)
	free := newH3AdmissionTask(other.Id, 0, 0)
	free.PrivateData.BillingSource = ""
	require.NoError(t, free.ReserveH3Dispatch("", false))
	require.NoError(t, DB.Delete(&other).Error)
	playground := newH3AdmissionTask(other.Id, 0, 10)
	require.ErrorIs(t, playground.ReserveH3Dispatch("", false), ErrH3WalletQuota)
}
