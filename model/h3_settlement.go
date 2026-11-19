package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

func requireAccountingRow(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("accounting row missing or balance limit exceeded")
	}
	return nil
}

func (task *Task) FinalizeH3Quota() (bool, error) {
	if task.PrivateData.H3ReservationConfirmed == nil || !*task.PrivateData.H3ReservationConfirmed || task.PendingQuota == nil || task.FinishTime <= 0 {
		return false, errors.New("missing confirmed H3 reservation or terminal decision")
	}
	final := *task.PendingQuota
	if final < 0 || final > task.Quota || !(task.Status == TaskStatusSuccess || task.Status == TaskStatusFailure && final == 0) {
		return false, errors.New("invalid H3 settlement")
	}
	refund := task.Quota - final
	var won bool
	var token Token
	err := DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Task{}).Where("id = ? AND status = ? AND quota = ? AND pending_quota = ? AND settled_quota IS NULL", task.ID, task.Status, task.Quota, final).
			Updates(map[string]any{"quota": final, "pending_quota": nil, "settled_quota": final})
		won = result.RowsAffected == 1
		if result.Error != nil || !won || refund == 0 {
			return result.Error
		}
		user := tx.Model(&User{}).Where("id = ?", task.UserId)
		updates := map[string]any{"used_quota": gorm.Expr("used_quota - ?", refund)}
		switch task.PrivateData.BillingSource {
		case "wallet":
			user = user.Where("quota <= ?", common.MaxWalletQuota-refund)
			updates["quota"] = gorm.Expr("quota + ?", refund)
		case "subscription":
			if err := requireAccountingRow(tx.Model(&UserSubscription{}).Where("id = ? AND user_id = ? AND amount_used >= ?", task.PrivateData.SubscriptionId, task.UserId, refund).
				Update("amount_used", gorm.Expr("amount_used - ?", refund))); err != nil {
				return err
			}
		case "unlimited":
		default:
			return fmt.Errorf("unknown H3 funding source %q", task.PrivateData.BillingSource)
		}
		if err := requireAccountingRow(user.Updates(updates)); err != nil {
			return err
		}
		if task.PrivateData.TokenId > 0 {
			if err := tx.Where("id = ? AND user_id = ?", task.PrivateData.TokenId, task.UserId).First(&token).Error; err != nil {
				return err
			}
			if err := requireAccountingRow(tx.Model(&token).Updates(map[string]any{
				"remain_quota": gorm.Expr("remain_quota + ?", refund), "used_quota": gorm.Expr("used_quota - ?", refund), "accessed_time": common.GetTimestamp(),
			})); err != nil {
				return err
			}
		}
		return requireAccountingRow(tx.Model(&Channel{}).Where("id = ?", task.ChannelId).Update("used_quota", gorm.Expr("used_quota - ?", refund)))
	})
	if err != nil || !won {
		return false, err
	}
	task.Quota, task.PendingQuota, task.SettledQuota = final, nil, &final
	if refund > 0 && common.RedisEnabled {
		if task.PrivateData.BillingSource == "wallet" {
			if err := cacheIncrUserQuota(task.UserId, int64(refund)); err != nil {
				common.SysError("refresh H3 wallet quota: " + err.Error())
			}
		}
		if token.Id > 0 {
			if _, err := cacheApplyTokenQuotaDelta(token.Id, token.Key, int64(refund)); err != nil {
				common.SysError("refresh H3 token quota: " + err.Error())
			}
		}
	}
	return true, nil
}
