package model

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var (
	ErrH3WalletQuota    = errors.New("H3 wallet funding unavailable")
	ErrH3TokenQuota     = errors.New("H3 token funding unavailable")
	ErrH3DispatchExists = errors.New("H3 dispatch already exists")
)

type h3QuotaAccount struct {
	id         int
	key, field string
	seq, net   int64
}

func (user *User) h3Wallet() h3QuotaAccount {
	return h3QuotaAccount{user.Id, getUserCacheKey(user.Id), "Quota", user.H3QuotaSeq, user.H3QuotaNet}
}

func (token *Token) h3Token() h3QuotaAccount {
	return h3QuotaAccount{token.Id, getTokenCacheKey(token.Key), "RemainQuota", token.H3QuotaSeq, token.H3QuotaNet}
}

const h3CacheFloorLua = `
local function older(seq, floor)
  if #seq ~= #floor then return #seq < #floor end
  return seq < floor
end
local function allowH3Fill(floorKey, pendingKey, seq)
  local floor = redis.call('GET', floorKey) or '0'
  if redis.call('EXISTS', pendingKey) == 1 or older(seq, floor) then return false end
  if older(floor, seq) then redis.call('SET', floorKey, seq) end
  return true
end
`

func (account h3QuotaAccount) checkAdmission(ticket string, quota int, limited bool) (bool, error) {
	ctx := context.Background()
	if err := common.RDB.Set(ctx, account.key+":h3-pending", ticket, 0).Err(); err != nil {
		return false, err
	}
	if !limited {
		return true, nil
	}
	values, err := common.RDB.HMGet(ctx, account.key, "Id", account.field, "H3QuotaNet").Result()
	if err != nil || values[0] == nil {
		return err == nil, err
	}
	if values[0] != strconv.Itoa(account.id) {
		return false, errors.New("H3 cached quota owner mismatch")
	}
	balance, err := strconv.ParseInt(fmt.Sprint(values[1]), 10, 64)
	if err != nil {
		return false, err
	}
	var cachedNet int64
	if values[2] != nil {
		cachedNet, err = strconv.ParseInt(fmt.Sprint(values[2]), 10, 64)
		if err != nil {
			return false, err
		}
	}
	available := decimal.NewFromInt(balance).Add(decimal.NewFromInt(account.net)).Sub(decimal.NewFromInt(cachedNet))
	return available.GreaterThanOrEqual(decimal.NewFromInt(int64(quota))), nil
}

func (account h3QuotaAccount) publish(ticket string) error {
	const script = h3CacheFloorLua + `
local floor = redis.call('GET', KEYS[2]) or '0'
local seq = ARGV[3]
if not older(seq, floor) then
  local cached = redis.call('HGET', KEYS[1], 'H3QuotaSeq') or '0'
  if redis.call('HGET', KEYS[1], 'Id') == ARGV[1] and older(cached, seq) and redis.call('HEXISTS', KEYS[1], ARGV[2]) == 1 then
    if (redis.call('HGET', KEYS[1], 'H3QuotaNet') or '0') ~= ARGV[5] then return 0 end
    redis.call('HINCRBY', KEYS[1], ARGV[2], ARGV[6])
    if ARGV[2] == 'RemainQuota' then redis.call('HINCRBY', KEYS[1], 'UsedQuota', ARGV[7]) end
    redis.call('HSET', KEYS[1], 'H3QuotaSeq', seq, 'H3QuotaNet', ARGV[4])
  end
  if older(floor, seq) then redis.call('SET', KEYS[2], seq) end
end
if redis.call('GET', KEYS[3]) == ARGV[8] then redis.call('DEL', KEYS[3]) end
return 1`
	ctx := context.Background()
	for {
		cachedNet, err := common.RDB.HGet(ctx, account.key, "H3QuotaNet").Int64()
		if err != nil && err != redis.Nil {
			return err
		}
		delta := decimal.NewFromInt(account.net).Sub(decimal.NewFromInt(cachedNet))
		result, err := common.RDB.Eval(ctx, script, []string{account.key, account.key + ":h3-seq", account.key + ":h3-pending"},
			account.id, account.field, account.seq, account.net, cachedNet, delta.String(), delta.Neg().String(), ticket).Int()
		if err != nil || result == 1 {
			return err
		}
	}
}

func (account h3QuotaAccount) publishCommitted(ticket string) {
	if err := account.publish(ticket); err != nil {
		common.SysError("publish H3 quota: " + err.Error())
	}
}

func repairH3QuotaCache(key string, id int, wallet bool) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var account h3QuotaAccount
		var row any = &Token{}
		if wallet {
			row = &User{}
		}
		if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
			if err := tx.Model(row).Where("id = ?", id).UpdateColumn("h3_quota_seq", gorm.Expr("h3_quota_seq")).Error; err != nil {
				return err
			}
		}
		if err := lockForUpdate(tx).First(row, id).Error; err != nil {
			return err
		}
		switch owner := row.(type) {
		case *User:
			account = owner.h3Wallet()
		case *Token:
			account = owner.h3Token()
		}
		if account.key != key {
			return errors.New("H3 quota cache owner changed")
		}
		ticket, err := common.RDB.Get(context.Background(), key+":h3-pending").Result()
		if err == redis.Nil {
			return nil
		}
		if err != nil {
			return err
		}
		return account.publish(ticket)
	})
}

func (task *Task) ReserveH3Dispatch(tokenKey string, tokenUnlimited bool) error {
	if task == nil || task.Status != TaskStatusDispatching || task.PrivateData.H3ReservationConfirmed == nil || !*task.PrivateData.H3ReservationConfirmed || task.TaskID == "" || task.Quota < 0 || task.Quota > common.MaxQuota || task.PrivateData.TokenId < 0 || task.PendingQuota != nil || task.SettledQuota != nil {
		return errors.New("invalid H3 dispatch reservation")
	}
	if task.ID != 0 {
		return ErrH3DispatchExists
	}
	source := task.PrivateData.BillingSource
	if source != "wallet" && source != "unlimited" && !(source == "" && task.Quota == 0 && task.PrivateData.TokenId == 0) {
		return errors.New("invalid H3 dispatch funding source")
	}
	dispatch := *task
	var accounts []h3QuotaAccount
	ticket := common.GetUUID()
	var readyToCommit bool
	err := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).Select("id", "quota", "unlimited_quota", "status", "h3_quota_seq", "h3_quota_net").Where("id = ?", task.UserId).First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrH3WalletQuota
			}
			return err
		}
		if user.Status != common.UserStatusEnabled || source != "" && (source == "unlimited") != user.UnlimitedQuota || source == "wallet" && user.Quota < task.Quota {
			return ErrH3WalletQuota
		}
		var duplicates int64
		if err := tx.Model(&Task{}).Where("user_id = ? AND task_id = ?", task.UserId, task.TaskID).Count(&duplicates).Error; err != nil {
			return err
		}
		if duplicates != 0 {
			return ErrH3DispatchExists
		}
		if source == "wallet" && task.Quota > 0 {
			if err := requireAccountingRow(tx.Model(&user).Updates(map[string]any{
				"quota": gorm.Expr("quota - ?", task.Quota), "h3_quota_seq": gorm.Expr("h3_quota_seq + 1"), "h3_quota_net": gorm.Expr("h3_quota_net - ?", task.Quota),
			})); err != nil {
				return err
			}
			if common.RedisEnabled {
				account := user.h3Wallet()
				accounts = append(accounts, account)
				ok, err := account.checkAdmission(ticket, task.Quota, true)
				if err != nil {
					return err
				}
				if !ok {
					return ErrH3WalletQuota
				}
			}
		}
		if task.PrivateData.TokenId > 0 {
			var token Token
			if err := lockForUpdate(tx).Where("id = ? AND user_id = ?", task.PrivateData.TokenId, user.Id).First(&token).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrH3TokenQuota
				}
				return err
			}
			if token.Key != tokenKey || token.Status != common.TokenStatusEnabled || token.ExpiredTime != -1 && token.ExpiredTime < common.GetTimestamp() || token.UnlimitedQuota != tokenUnlimited || !token.UnlimitedQuota && token.RemainQuota < task.Quota {
				return ErrH3TokenQuota
			}
			if task.Quota > 0 {
				if err := requireAccountingRow(tx.Model(&token).Updates(map[string]any{
					"remain_quota": gorm.Expr("remain_quota - ?", task.Quota), "used_quota": gorm.Expr("used_quota + ?", task.Quota), "accessed_time": common.GetTimestamp(),
					"h3_quota_seq": gorm.Expr("h3_quota_seq + 1"), "h3_quota_net": gorm.Expr("h3_quota_net - ?", task.Quota),
				})); err != nil {
					return err
				}
				if common.RedisEnabled {
					account := token.h3Token()
					accounts = append(accounts, account)
					ok, err := account.checkAdmission(ticket, task.Quota, !token.UnlimitedQuota)
					if err != nil {
						return err
					}
					if !ok {
						return ErrH3TokenQuota
					}
				}
			}
		}
		if err := dispatch.insertWithUsage(tx); err != nil {
			return err
		}
		readyToCommit = true
		return nil
	})
	if err != nil && readyToCommit {
		return err
	}
	for _, account := range accounts {
		if err == nil {
			account.seq++
			account.net -= int64(task.Quota)
		}
		account.publishCommitted(ticket)
	}
	if err == nil {
		*task = dispatch
	}
	return err
}
