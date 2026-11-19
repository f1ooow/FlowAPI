package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (task *Task) BeforeUpdate(tx *gorm.DB) error {
	if _, snapshot := tx.Statement.Dest.(*Task); !snapshot || task.PrivateData.H3ReservationConfirmed == nil || !*task.PrivateData.H3ReservationConfirmed {
		return nil
	}
	// Save can upsert a stale snapshot after a zero-row update. H3 status
	// writers use UpdateWithStatus; only settlement may change a settled row.
	if _, guarded := tx.Statement.Clauses["WHERE"]; !guarded {
		return errors.New("H3 task snapshots require a conditional update")
	}
	tx.Statement.AddClause(clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "settled_quota IS NULL"}}})
	return nil
}

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
	publish := refund > 0 && common.RedisEnabled && (task.PrivateData.BillingSource == "wallet" || task.PrivateData.TokenId > 0)
	var won bool
	err := DB.Transaction(func(tx *gorm.DB) error {
		settlement := map[string]any{"quota": final, "settled_quota": final}
		if !publish {
			settlement["pending_quota"] = nil
		}
		result := tx.Model(&Task{}).Where("id = ? AND status = ? AND quota = ? AND pending_quota = ? AND settled_quota IS NULL", task.ID, task.Status, task.Quota, final).
			Updates(settlement)
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
			updates["h3_quota_seq"] = gorm.Expr("h3_quota_seq + 1")
			updates["h3_quota_net"] = gorm.Expr("h3_quota_net + ?", refund)
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
			owner := tx.Unscoped().Model(&Token{}).Where("id = ? AND user_id = ?", task.PrivateData.TokenId, task.UserId)
			if err := requireAccountingRow(owner.Updates(map[string]any{
				"h3_quota_seq": gorm.Expr("h3_quota_seq + 1"), "h3_quota_net": gorm.Expr("h3_quota_net + ?", refund),
				"remain_quota": gorm.Expr("remain_quota + ?", refund), "used_quota": gorm.Expr("used_quota - ?", refund), "accessed_time": common.GetTimestamp(),
			})); err != nil {
				return err
			}
		}
		return task.h3Channel(tx).Update("used_quota", gorm.Expr("used_quota - ?", refund)).Error
	})
	if err != nil || !won {
		return false, err
	}
	task.Quota, task.SettledQuota = final, &final
	if publish {
		return true, task.PublishH3Refund()
	}
	task.PendingQuota = nil
	return true, nil
}

// PublishH3Refund reconciles only committed accounting. Both quota columns
// stay set until publication succeeds, keeping the existing poller runnable.
func (task *Task) PublishH3Refund() error {
	var pending Task
	err := DB.Select("id", "user_id", "private_data").Where("id = ? AND pending_quota = settled_quota", task.ID).Take(&pending).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !common.RedisEnabled {
		return errors.New("Redis is disabled with an H3 refund awaiting publication")
	}
	if pending.PrivateData.BillingSource == "wallet" {
		var wallet User
		err := DB.Select("id", "h3_quota_seq", "h3_quota_net").First(&wallet, pending.UserId).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil {
			if err := wallet.h3Wallet().publish(""); err != nil {
				return err
			}
		}
	}
	if pending.PrivateData.TokenId > 0 {
		var token Token
		err := DB.Unscoped().Where("user_id = ?", pending.UserId).First(&token, pending.PrivateData.TokenId).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil {
			if token.DeletedAt.Valid {
				if err := invalidateTokenCacheForMutation(token.Key); err != nil {
					return err
				}
			}
			if err := token.h3Token().publish(""); err != nil {
				return err
			}
		}
	}
	if err := DB.Model(&Task{}).Where("id = ? AND pending_quota = settled_quota", task.ID).UpdateColumn("pending_quota", nil).Error; err != nil {
		return err
	}
	task.PendingQuota = nil
	return nil
}
