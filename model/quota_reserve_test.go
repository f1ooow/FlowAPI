package model

import (
	"math"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func createReserveTestUser(t *testing.T, quota int) User {
	t.Helper()
	user := User{
		Username:    "reserve-user-" + common.GetRandomString(6),
		Password:    "unused-password-hash",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AuthVersion: 1,
		Quota:       quota,
		AffCode:     "reserve-aff-" + common.GetRandomString(8),
	}
	require.NoError(t, DB.Create(&user).Error)
	return user
}

func createReserveTestToken(t *testing.T, remainQuota int) Token {
	t.Helper()
	token := Token{
		UserId:      1,
		Key:         "reserve-token-" + common.GetRandomString(8),
		Name:        "reserve-test",
		Status:      common.TokenStatusEnabled,
		ExpiredTime: -1,
		RemainQuota: remainQuota,
	}
	require.NoError(t, token.Insert())
	return token
}

func getUserQuotaFromDB(t *testing.T, id int) int {
	t.Helper()
	var user User
	require.NoError(t, DB.Select("quota").First(&user, id).Error)
	return user.Quota
}

func getTokenFromDB(t *testing.T, id int) Token {
	t.Helper()
	var token Token
	require.NoError(t, DB.First(&token, id).Error)
	return token
}

func resetBatchUpdateTestState(t *testing.T) {
	t.Helper()
	oldBatchEnabled := common.BatchUpdateEnabled
	common.BatchUpdateEnabled = false
	for i := 0; i < BatchUpdateTypeCount; i++ {
		batchUpdateLocks[i].Lock()
		batchUpdateStores[i] = make(map[int]int)
		batchUpdateLocks[i].Unlock()
	}
	t.Cleanup(func() {
		common.BatchUpdateEnabled = oldBatchEnabled
		for i := 0; i < BatchUpdateTypeCount; i++ {
			batchUpdateLocks[i].Lock()
			batchUpdateStores[i] = make(map[int]int)
			batchUpdateLocks[i].Unlock()
		}
	})
}

func TestTryReserveQuotaWithoutRedis(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)

	user := createReserveTestUser(t, 100)
	reserved, err := TryReserveUserQuota(user.Id, 60)
	require.NoError(t, err)
	assert.True(t, reserved)
	assert.Equal(t, 40, getUserQuotaFromDB(t, user.Id))

	reserved, err = TryReserveUserQuota(user.Id, 41)
	require.NoError(t, err)
	assert.False(t, reserved)
	assert.Equal(t, 40, getUserQuotaFromDB(t, user.Id))

	token := createReserveTestToken(t, 80)
	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 25, false)
	require.NoError(t, err)
	assert.True(t, reserved)
	reloaded := getTokenFromDB(t, token.Id)
	assert.Equal(t, 55, reloaded.RemainQuota)
	assert.Equal(t, 25, reloaded.UsedQuota)

	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 56, false)
	require.NoError(t, err)
	assert.False(t, reserved)
	assert.Equal(t, 55, getTokenFromDB(t, token.Id).RemainQuota)
}

func TestH3AdmissionAfterEvictionHonorsGenericReservationsInBatchMode(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)
	common.BatchUpdateEnabled = true

	user := createReserveTestUser(t, 10)
	reserved, err := TryReserveUserQuota(user.Id, 8)
	require.NoError(t, err)
	assert.True(t, reserved)
	assert.Equal(t, 2, getUserQuotaFromDB(t, user.Id))
	require.NoError(t, common.RDB.Del(t.Context(), getUserCacheKey(user.Id)).Err())
	h3 := newH3AdmissionTask(user.Id, 0, 3)
	require.ErrorIs(t, h3.ReserveH3Dispatch("", false), ErrH3WalletQuota)

	reserved, err = TryReserveUserQuota(user.Id, 3)
	require.NoError(t, err)
	assert.False(t, reserved, "stale DB balance must not authorize a second spend")
	cachedUser, err := GetUserCache(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 2, cachedUser.Quota)

	token := createReserveTestToken(t, 9)
	token.UserId = user.Id
	require.NoError(t, DB.Save(&token).Error)
	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 7, false)
	require.NoError(t, err)
	assert.True(t, reserved)
	require.NoError(t, common.RDB.Del(t.Context(), getTokenCacheKey(token.Key)).Err())
	require.NoError(t, DB.Model(&user).Update("unlimited_quota", true).Error)
	h3.PrivateData.TokenId, h3.PrivateData.BillingSource = token.Id, "unlimited"
	require.ErrorIs(t, h3.ReserveH3Dispatch(token.Key, false), ErrH3TokenQuota)
	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 3, false)
	require.NoError(t, err)
	assert.False(t, reserved)
	assert.Equal(t, 2, getTokenFromDB(t, token.Id).RemainQuota)

	batchUpdate()
	assert.Equal(t, 2, getUserQuotaFromDB(t, user.Id))
	reloadedToken := getTokenFromDB(t, token.Id)
	assert.Equal(t, 2, reloadedToken.RemainQuota)
	assert.Equal(t, 7, reloadedToken.UsedQuota)
}

func TestBatchUpdateAccumulatesTwoMaximumRequestCharges(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	common.BatchUpdateEnabled = true

	user := createReserveTestUser(t, common.MaxQuota*2+100)
	require.NoError(t, DecreaseUserQuota(user.Id, common.MaxQuota, false))
	require.NoError(t, DecreaseUserQuota(user.Id, common.MaxQuota, false))

	batchUpdate()
	assert.Equal(t, 100, getUserQuotaFromDB(t, user.Id))
}

func TestBatchUpdateAccumulatorSaturatesOverflow(t *testing.T) {
	resetBatchUpdateTestState(t)

	addNewRecord(BatchUpdateTypeUserQuota, 1, math.MaxInt)
	addNewRecord(BatchUpdateTypeUserQuota, 1, 1)
	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	assert.Equal(t, math.MaxInt, batchUpdateStores[BatchUpdateTypeUserQuota][1])
	batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()

	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	batchUpdateStores[BatchUpdateTypeUserQuota] = make(map[int]int)
	batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()
	addNewRecord(BatchUpdateTypeUserQuota, 1, math.MinInt)
	addNewRecord(BatchUpdateTypeUserQuota, 1, -1)
	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	assert.Equal(t, math.MinInt, batchUpdateStores[BatchUpdateTypeUserQuota][1])
	batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()
}

func TestReserveFallsBackToDatabaseWhenRedisIsUnavailable(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	server := useUserCacheMiniRedis(t)

	user := createReserveTestUser(t, 20)
	require.NoError(t, populateUserCache(user))
	server.Close()

	// Redis 故障时降级为数据库条件更新：服务保持可用且不会超扣。
	reserved, err := TryReserveUserQuota(user.Id, 5)
	require.NoError(t, err)
	assert.True(t, reserved)
	assert.Equal(t, 15, getUserQuotaFromDB(t, user.Id))

	reserved, err = TryReserveUserQuota(user.Id, 16)
	require.NoError(t, err)
	assert.False(t, reserved)
	assert.Equal(t, 15, getUserQuotaFromDB(t, user.Id))
}

func TestSynchronousReserveCompensatesCacheWhenPersistenceFails(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)

	user := createReserveTestUser(t, 10)
	require.NoError(t, populateUserCache(user))
	require.NoError(t, DB.Delete(&user).Error)

	reserved, err := TryReserveUserQuota(user.Id, 6)
	assert.False(t, reserved)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	cached, cacheErr := cacheGetUserBase(user.Id)
	require.NoError(t, cacheErr)
	assert.Equal(t, 10, cached.Quota)

	token := createReserveTestToken(t, 12)
	_, err = GetTokenByKey(token.Key, true)
	require.NoError(t, err)
	require.NoError(t, DB.Delete(&token).Error)
	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 7, false)
	assert.False(t, reserved)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	cachedToken, cacheErr := cacheGetTokenByKey(token.Key)
	require.NoError(t, cacheErr)
	assert.Equal(t, 12, cachedToken.RemainQuota)
	assert.Zero(t, cachedToken.UsedQuota)
}

func TestTokenCacheInitPreservesLiveQuotaAndFenceBlocksStaleSnapshot(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	server := useUserCacheMiniRedis(t)

	token := createReserveTestToken(t, 100)
	loaded, err := GetTokenByKey(token.Key, true)
	require.NoError(t, err)
	stale := *loaded

	result, err := cacheApplyTokenQuotaDelta(token.Id, token.Key, -70)
	require.NoError(t, err)
	require.Equal(t, cacheQuotaOK, result)

	// 已存在的哈希只刷新 TTL：数据库快照不得覆盖已被原子预扣的余额。
	code, err := cacheInitToken(stale)
	require.NoError(t, err)
	assert.Equal(t, 2, code)
	cached, err := cacheGetTokenByKey(token.Key)
	require.NoError(t, err)
	assert.Equal(t, 30, cached.RemainQuota)

	// 变更期间：fence 删除缓存并拦截并发读者手中的过期快照。
	require.NoError(t, invalidateTokenCacheForMutation(token.Key))
	code, err = cacheInitToken(stale)
	require.NoError(t, err)
	assert.Zero(t, code, "the pre-mutation snapshot must not be published while fenced")
	_, err = cacheGetTokenByKey(token.Key)
	assert.Error(t, err)

	// fence 过期后可重新从数据库水合。
	server.FastForward(time.Duration(tokenCacheFenceSeconds+1) * time.Second)
	fresh, err := GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	assert.Equal(t, 100, fresh.RemainQuota)
	cached, err = cacheGetTokenByKey(token.Key)
	require.NoError(t, err)
	assert.Equal(t, 100, cached.RemainQuota)
}

func TestH3AdmissionAndRefundRejectDelayedColdFills(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)
	user := createReserveTestUser(t, 100)
	token := createReserveTestToken(t, 100)
	token.UserId = user.Id
	require.NoError(t, DB.Save(&token).Error)
	task := newH3AdmissionTask(user.Id, token.Id, 30)
	require.NoError(t, task.ReserveH3Dispatch(token.Key, false))
	require.NoError(t, populateUserCache(user))
	_, err := cacheGetUserBase(user.Id)
	require.Error(t, err)
	code, err := cacheInitToken(token)
	require.NoError(t, err)
	require.Zero(t, code)
	reservedUser, err := GetUserById(user.Id, true)
	require.NoError(t, err)
	reservedToken := getTokenFromDB(t, token.Id)
	require.Equal(t, 70, reservedUser.Quota)
	require.Equal(t, 70, reservedToken.RemainQuota)
	finishH3AdmissionTask(t, &task, 0)
	require.NoError(t, populateUserCache(*reservedUser))
	_, err = cacheGetUserBase(user.Id)
	require.Error(t, err)
	code, err = cacheInitToken(reservedToken)
	require.NoError(t, err)
	require.Zero(t, code)
	_, err = GetUserCache(user.Id)
	require.NoError(t, err)
	_, err = GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	require.NoError(t, populateUserCache(user))
	_, err = cacheInitToken(token)
	require.NoError(t, err)
	cachedUser, err := cacheGetUserBase(user.Id)
	require.NoError(t, err)
	require.Equal(t, 100, cachedUser.Quota)
	cachedToken, err := cacheGetTokenByKey(token.Key)
	require.NoError(t, err)
	require.Equal(t, 100, cachedToken.RemainQuota)
	require.Zero(t, cachedToken.UsedQuota)
}

func TestH3AdmissionRequiresBothDurableAndCachedBalance(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)
	user := createReserveTestUser(t, 9)
	token := createReserveTestToken(t, 9)
	token.UserId = user.Id
	require.NoError(t, DB.Save(&token).Error)
	user.Quota, token.RemainQuota = 100, 100
	require.NoError(t, populateUserCache(user))
	_, err := cacheInitToken(token)
	require.NoError(t, err)
	task := newH3AdmissionTask(user.Id, token.Id, 10)
	require.ErrorIs(t, task.ReserveH3Dispatch(token.Key, false), ErrH3WalletQuota)
	require.NoError(t, DB.Model(&user).Update("quota", 100).Error)
	require.ErrorIs(t, task.ReserveH3Dispatch(token.Key, false), ErrH3TokenQuota)
	require.Equal(t, 100, getUserQuotaFromDB(t, user.Id))
	require.Equal(t, 9, getTokenFromDB(t, token.Id).RemainQuota)
	require.NoError(t, DB.Model(&token).Update("remain_quota", 100).Error)
	_, err = cacheApplyTokenQuotaDelta(token.Id, token.Key, -91)
	require.NoError(t, err)
	require.ErrorIs(t, task.ReserveH3Dispatch(token.Key, false), ErrH3TokenQuota)
	_, err = cacheApplyUserQuotaDelta(user.Id, -91)
	require.NoError(t, err)
	require.ErrorIs(t, task.ReserveH3Dispatch(token.Key, false), ErrH3WalletQuota)
	require.Equal(t, 100, getUserQuotaFromDB(t, user.Id))
	require.Equal(t, 100, getTokenFromDB(t, token.Id).RemainQuota)
}

func pendingH3Refund(t *testing.T) (User, Token, Task) {
	t.Helper()
	truncateTables(t)
	useUserCacheMiniRedis(t)
	user := createReserveTestUser(t, 100)
	token := createReserveTestToken(t, 100)
	token.UserId = user.Id
	require.NoError(t, DB.Save(&token).Error)
	require.NoError(t, populateUserCache(user))
	_, err := cacheInitToken(token)
	require.NoError(t, err)
	task := newH3AdmissionTask(user.Id, token.Id, 30)
	require.NoError(t, task.ReserveH3Dispatch(token.Key, false))
	require.NoError(t, task.BeginH3Dispatch())
	final := 5
	task.Status, task.FinishTime, task.Progress, task.PendingQuota = TaskStatusSuccess, 1, "100%", &final
	won, err := task.UpdateWithStatus(TaskStatusDispatching)
	require.NoError(t, err)
	require.True(t, won)
	return user, token, task
}

func TestH3RefundLostCommitAckRetainsPublication(t *testing.T) {
	user, token, task := pendingH3Refund(t)
	original := DB
	sqlDB, err := original.DB()
	require.NoError(t, err)
	DB = original.WithContext(t.Context())
	DB.Statement.ConnPool = &h3CommitFaultPool{DB: sqlDB}
	t.Cleanup(func() { DB = original })
	won, err := task.FinalizeH3Quota()
	DB = original
	require.ErrorContains(t, err, "lost COMMIT response")
	require.False(t, won)
	require.Equal(t, 95, getUserQuotaFromDB(t, user.Id))
	wallet, err := cacheGetUserBase(user.Id)
	require.NoError(t, err)
	require.Equal(t, 70, wallet.Quota)
	won, err = task.UpdateWithStatus(TaskStatusSuccess)
	require.NoError(t, err)
	require.False(t, won)
	require.Error(t, task.Update())
	require.True(t, HasUnfinishedSyncTasks())
	var retry Task
	require.NoError(t, DB.First(&retry, task.ID).Error)
	require.Equal(t, common.GetPointer(5), retry.SettledQuota)
	won, err = task.FinalizeH3Quota()
	require.NoError(t, err)
	require.False(t, won)
	require.NoError(t, retry.PublishH3Refund())
	require.NoError(t, task.PublishH3Refund())
	require.False(t, HasUnfinishedSyncTasks())
	wallet, err = cacheGetUserBase(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 95, wallet.Quota)
	cached, err := cacheGetTokenByKey(token.Key)
	require.NoError(t, err)
	assert.Equal(t, 95, cached.RemainQuota)
	assert.Equal(t, 5, cached.UsedQuota)
	assert.Equal(t, 5, getTokenFromDB(t, token.Id).UsedQuota)
}

func TestH3RefundRetryAfterPartialPublication(t *testing.T) {
	for _, scenario := range []string{"newer reservation", "revoked token"} {
		t.Run(scenario, func(t *testing.T) {
			user, token, task := pendingH3Refund(t)
			floor := getTokenCacheKey(token.Key) + ":h3-seq"
			require.NoError(t, common.RDB.Del(t.Context(), floor).Err())
			require.NoError(t, common.RDB.LPush(t.Context(), floor, "wrong type").Err())
			won, err := task.FinalizeH3Quota()
			require.Error(t, err)
			require.True(t, won)
			require.True(t, HasUnfinishedSyncTasks())
			wallet, err := cacheGetUserBase(user.Id)
			require.NoError(t, err)
			require.Equal(t, 95, wallet.Quota)
			cached, err := cacheGetTokenByKey(token.Key)
			require.NoError(t, err)
			require.Equal(t, 70, cached.RemainQuota)
			require.NoError(t, common.RDB.Set(t.Context(), floor, "1", 0).Err())
			if scenario == "revoked token" {
				require.NoError(t, DB.Delete(&token).Error)
				require.NoError(t, task.PublishH3Refund())
				require.NoError(t, common.RDB.Del(t.Context(), getTokenCacheFenceKey(token.Key)).Err())
				filled, err := cacheInitToken(*cached)
				require.NoError(t, err)
				require.Zero(t, filled)
				_, err = GetTokenByKey(token.Key, false)
				require.ErrorIs(t, err, gorm.ErrRecordNotFound)
				require.False(t, HasUnfinishedSyncTasks())
				return
			}

			barrier := &h3PublicationBarrier{key: getUserCacheKey(user.Id), entered: make(chan struct{}), resume: make(chan struct{})}
			common.RDB.AddHook(barrier)
			done := make(chan error, 1)
			go func() { done <- task.PublishH3Refund() }()
			select {
			case err := <-done:
				t.Fatalf("refund replay returned before publication: %v", err)
			case <-barrier.entered:
			}
			later := newH3AdmissionTask(user.Id, token.Id, 7)
			err = later.ReserveH3Dispatch(token.Key, false)
			close(barrier.resume)
			require.NoError(t, <-done)
			require.NoError(t, err)
			require.NoError(t, task.PublishH3Refund())
			wallet, err = cacheGetUserBase(user.Id)
			require.NoError(t, err)
			assert.Equal(t, 88, wallet.Quota)
			cached, err = cacheGetTokenByKey(token.Key)
			require.NoError(t, err)
			assert.Equal(t, 88, cached.RemainQuota)
			assert.Equal(t, 12, cached.UsedQuota)
			assert.Equal(t, 88, getUserQuotaFromDB(t, user.Id))
			assert.Equal(t, 12, getTokenFromDB(t, token.Id).UsedQuota)
		})
	}
}
