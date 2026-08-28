package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnlimitedBillingKeepsWalletAndSettlesTokenQuota(t *testing.T) {
	truncate(t)
	const (
		userID   = 8101
		tokenID  = 8102
		tokenKey = "unlimited-billing-token"
	)
	seedUser(t, userID, 25)
	seedToken(t, tokenID, userID, tokenKey, 1_000)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	info := &relaycommon.RelayInfo{
		UserId:             userID,
		UserQuota:          25,
		UserUnlimitedQuota: true,
		TokenId:            tokenID,
		TokenKey:           tokenKey,
	}

	session, apiErr := NewBillingSession(c, info, 200)
	require.Nil(t, apiErr)
	require.NotNil(t, session)
	assert.Equal(t, BillingSourceUnlimited, info.BillingSource)

	walletQuota, err := model.GetUserQuota(userID, false)
	require.NoError(t, err)
	assert.Equal(t, 25, walletQuota)
	token, err := model.GetTokenById(tokenID)
	require.NoError(t, err)
	assert.Equal(t, 800, token.RemainQuota)

	require.NoError(t, session.Settle(300))
	walletQuota, err = model.GetUserQuota(userID, false)
	require.NoError(t, err)
	assert.Equal(t, 25, walletQuota)
	token, err = model.GetTokenById(tokenID)
	require.NoError(t, err)
	assert.Equal(t, 700, token.RemainQuota)
}

func TestUnlimitedTaskRefundRestoresAccountingWithoutCreditingWallet(t *testing.T) {
	truncate(t)
	const (
		userID    = 8111
		channelID = 8112
		tokenID   = 8113
		quota     = 300
	)
	seedUser(t, userID, 25)
	seedChannel(t, channelID)
	seedToken(t, tokenID, userID, "unlimited-task-token", 700)
	seedChargedAccounting(t, userID, channelID, tokenID, quota, 1)
	task := makeTask(userID, channelID, quota, tokenID, BillingSourceUnlimited, 0)
	require.NoError(t, task.Insert())

	assert.True(t, RefundTaskQuota(t.Context(), task, "upstream failed"))
	walletQuota, err := model.GetUserQuota(userID, false)
	require.NoError(t, err)
	assert.Equal(t, 25, walletQuota)
	token, err := model.GetTokenById(tokenID)
	require.NoError(t, err)
	assert.Equal(t, 1_000, token.RemainQuota)

	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, 0, user.UsedQuota)
	assert.Equal(t, 1, user.RequestCount)
}
