package model

import (
	"context"
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
	var walletTotal int64
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
			updates["h3_refund_total"] = gorm.Expr("h3_refund_total + ?", refund)
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
		if common.RedisEnabled && task.PrivateData.BillingSource == "wallet" {
			if err := tx.Model(&User{}).Where("id = ?", task.UserId).Pluck("h3_refund_total", &walletTotal).Error; err != nil {
				return err
			}
		}
		if task.PrivateData.TokenId > 0 {
			owner := tx.Unscoped().Model(&Token{}).Where("id = ? AND user_id = ?", task.PrivateData.TokenId, task.UserId)
			if err := requireAccountingRow(owner.Updates(map[string]any{
				"h3_refund_total": gorm.Expr("h3_refund_total + ?", refund),
				"remain_quota":    gorm.Expr("remain_quota + ?", refund), "used_quota": gorm.Expr("used_quota - ?", refund), "accessed_time": common.GetTimestamp(),
			})); err != nil {
				return err
			}
			if common.RedisEnabled {
				if err := tx.Unscoped().First(&token, task.PrivateData.TokenId).Error; err != nil {
					return err
				}
			}
		}
		return tx.Model(&Channel{}).Where("id = ?", task.ChannelId).Update("used_quota", gorm.Expr("used_quota - ?", refund)).Error
	})
	if err != nil || !won {
		return false, err
	}
	task.Quota, task.PendingQuota, task.SettledQuota = final, nil, &final
	if refund > 0 && common.RedisEnabled {
		if task.PrivateData.BillingSource == "wallet" {
			publishH3Refund(getUserCacheKey(task.UserId), task.UserId, "Quota", walletTotal)
		}
		if token.Id > 0 && !token.DeletedAt.Valid {
			publishH3Refund(getTokenCacheKey(token.Key), token.Id, "RemainQuota", token.H3RefundTotal)
		}
	}
	return true, nil
}

func publishH3Refund(key string, id int, field string, total int64) {
	const script = `
local total = tonumber(ARGV[3])
local floor = tonumber(redis.call('GET', KEYS[2]) or '0')
if total > floor then redis.call('SET', KEYS[2], ARGV[3]) end
if redis.call('HGET', KEYS[1], 'Id') ~= ARGV[1] or redis.call('HEXISTS', KEYS[1], ARGV[2]) == 0 then return 0 end
local delta = total - tonumber(redis.call('HGET', KEYS[1], 'H3RefundTotal') or '0')
if delta <= 0 then return 0 end
redis.call('HINCRBY', KEYS[1], ARGV[2], delta)
if ARGV[2] == 'RemainQuota' then redis.call('HINCRBY', KEYS[1], 'UsedQuota', -delta) end
redis.call('HSET', KEYS[1], 'H3RefundTotal', ARGV[3])
return 1`
	if err := common.RDB.Eval(context.Background(), script, []string{key, key + ":h3-refunded"}, id, field, total).Err(); err != nil {
		common.SysError("publish H3 refund: " + err.Error())
	}
}
