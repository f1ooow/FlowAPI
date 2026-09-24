package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrSubmissionConflict = errors.New("submission key has a different request")

// The digest preserves case-sensitive key identity on all supported DB collations.
type TaskSubmission struct {
	ID           int    `gorm:"primaryKey"`
	UserID       int    `gorm:"uniqueIndex:idx_task_submission_owner"`
	TokenID      int    `gorm:"uniqueIndex:idx_task_submission_owner"`
	KeyHash      string `gorm:"size:64;uniqueIndex:idx_task_submission_owner"`
	ClientKey    string `gorm:"size:200;not null"`
	Fingerprint  string `gorm:"size:64;not null"`
	TaskID       string `gorm:"size:191;uniqueIndex;not null"`
	Status       string `gorm:"size:20;not null"`
	CreatedAt    int64
	UpdatedAt    int64
	DispatchedAt int64
	RouteJSON    string `gorm:"type:text" json:"-"`

	// Dispatch fields are a private, queryable copy of the route snapshot. The
	// complete protocol/settings snapshot remains in RouteJSON for forward
	// compatibility, while the selected credential is never exposed by model
	// JSON serialization.
	ChannelID          int    `gorm:"column:dispatch_channel_id" json:"-"`
	ChannelType        int    `gorm:"column:dispatch_channel_type" json:"-"`
	APIKey             string `gorm:"column:dispatch_api_key;type:text" json:"-"`
	BaseURL            string `gorm:"column:dispatch_base_url;type:text" json:"-"`
	UpstreamModelName  string `gorm:"column:dispatch_upstream_model;type:varchar(191)" json:"-"`
	BillingContextJSON string `gorm:"column:dispatch_billing_context;type:text" json:"-"`
	FundingSource      string `gorm:"column:dispatch_funding_source;type:varchar(20)" json:"-"`
	SubscriptionID     int    `gorm:"column:dispatch_subscription_id" json:"-"`
	Group              string `gorm:"column:dispatch_group;type:varchar(50)" json:"-"`
	ReservedQuota      int    `gorm:"column:dispatch_reserved_quota" json:"-"`
}

func ValidTaskSubmissionKey(key string) bool {
	if len(key) == 0 || len(key) > 200 {
		return false
	}
	for i := range key {
		if key[i] < 33 || key[i] > 126 {
			return false
		}
	}
	return true
}

func ReserveTaskSubmission(userID, tokenID int, key, fingerprint string) (*TaskSubmission, bool, error) {
	if userID <= 0 || tokenID <= 0 || !ValidTaskSubmissionKey(key) || fingerprint == "" {
		return nil, false, errors.New("invalid submission identity")
	}
	candidate := &TaskSubmission{UserID: userID, TokenID: tokenID, ClientKey: key, KeyHash: fmt.Sprintf("%x", sha256.Sum256([]byte(key))), Fingerprint: fingerprint, TaskID: GenerateTaskID(), Status: "submitting", CreatedAt: time.Now().Unix(), UpdatedAt: time.Now().Unix()}
	if err := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(candidate).Error; err != nil {
		return nil, false, err
	}
	saved, err := GetTaskSubmission(userID, tokenID, key)
	if err != nil {
		return nil, false, err
	}
	if saved.Fingerprint != fingerprint {
		return nil, false, ErrSubmissionConflict
	}
	return saved, saved.TaskID == candidate.TaskID, nil
}

func GetTaskSubmission(userID, tokenID int, key string) (*TaskSubmission, error) {
	var submission TaskSubmission
	err := DB.Where("user_id = ? AND token_id = ? AND key_hash = ?", userID, tokenID, fmt.Sprintf("%x", sha256.Sum256([]byte(key)))).First(&submission).Error
	if err != nil {
		return nil, err
	}
	// A crashed process must never make the reservation eligible for another paid POST.
	if submission.Status == "submitting" && time.Now().Unix()-submission.CreatedAt > 600 {
		submission.Status = "unknown"
	}
	return &submission, nil
}

func FinishTaskSubmission(id int, dispatched bool, definiteFailure ...bool) error {
	status := "failed"
	if dispatched {
		status = "unknown"
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var submission TaskSubmission
		if err := lockForUpdate(tx).First(&submission, id).Error; err != nil {
			return err
		}
		if submission.DispatchedAt > 0 || dispatched {
			status = "unknown"
		}
		if len(definiteFailure) > 0 && definiteFailure[0] {
			status = "failed"
		}
		return tx.Model(&TaskSubmission{}).Where("id = ? AND status = ?", id, "submitting").Updates(map[string]any{"status": status, "updated_at": time.Now().Unix()}).Error
	})
}

// Task persistence and acceptance are atomic: lookup never advertises an absent task.
func InsertTaskForSubmission(task *Task, submissionID int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var submission TaskSubmission
		if err := lockForUpdate(tx).First(&submission, submissionID).Error; err != nil {
			return err
		}
		if submission.UserID != task.UserId || submission.TokenID != task.PrivateData.TokenId || submission.TaskID != task.TaskID {
			return errors.New("submission owner mismatch")
		}
		if submission.RouteJSON != "" {
			var route TaskSubmissionRoute
			if err := common.UnmarshalJsonStr(submission.RouteJSON, &route); err != nil {
				return err
			}
			if route.ChannelID != task.ChannelId || route.APIKey != task.PrivateData.Key || route.UpstreamModelName != task.Properties.UpstreamModelName {
				return errors.New("submission route mismatch")
			}
		}
		if err := tx.Create(task).Error; err != nil {
			return err
		}
		result := tx.Model(&TaskSubmission{}).Where("id = ? AND task_id = ? AND status IN ?", submissionID, task.TaskID, []string{"submitting", "unknown"}).Updates(map[string]any{"status": "accepted", "updated_at": time.Now().Unix()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("submission acceptance conflict")
		}
		return nil
	})
}

// TaskSubmissionRoute records the exact account and protocol before paid dispatch.
// RouteJSON is private, just like Task.PrivateData; never return it to clients.
type TaskSubmissionRoute struct {
	ChannelID            int                      `json:"channel_id"`
	ChannelType          int                      `json:"channel_type"`
	APIKey               string                   `json:"api_key"`
	BaseURL              string                   `json:"base_url"`
	UpstreamModelName    string                   `json:"upstream_model_name"`
	ChannelOtherSettings dto.ChannelOtherSettings `json:"channel_other_settings"`
	BillingContext       *TaskBillingContext      `json:"billing_context,omitempty"`
	FundingSource        string                   `json:"funding_source,omitempty"`
	SubscriptionID       int                      `json:"subscription_id,omitempty"`
	Group                string                   `json:"group,omitempty"`
	ReservedQuota        int                      `json:"reserved_quota"`
}

// RecordTaskSubmissionDispatch snapshots the selected route and billing
// reservation after pre-consume and before the first paid provider request.
// It is deliberately keyed by RelayInfo so callers cannot accidentally persist
// a route different from the credentials that will be used by DoRequest.
func RecordTaskSubmissionDispatch(submissionID int, info *relaycommon.RelayInfo) error {
	if submissionID <= 0 || info == nil || info.ChannelMeta == nil || info.TaskRelayInfo == nil {
		return errors.New("invalid task submission dispatch info")
	}
	if info.UserId <= 0 || info.TokenId <= 0 {
		return errors.New("invalid submission owner")
	}
	if info.ChannelId <= 0 || info.ChannelType <= 0 || info.ApiKey == "" || info.UpstreamModelName == "" {
		return errors.New("invalid submission route")
	}
	if info.PriceData.Quota < 0 {
		return errors.New("invalid submission reservation")
	}

	route := TaskSubmissionRoute{
		ChannelID:            info.ChannelId,
		ChannelType:          info.ChannelType,
		APIKey:               info.ApiKey,
		BaseURL:              info.ChannelBaseUrl,
		UpstreamModelName:    info.UpstreamModelName,
		ChannelOtherSettings: info.ChannelOtherSettings,
		BillingContext:       NewTaskBillingContext(info),
		FundingSource:        info.BillingSource,
		SubscriptionID:       info.SubscriptionId,
		Group:                info.UsingGroup,
		ReservedQuota:        info.PriceData.Quota,
	}
	return recordTaskSubmissionDispatch(submissionID, info.UserId, info.TokenId, route)
}

// PrepareTaskSubmissionDispatch must succeed after quota reservation and before
// the first POST. Even a process crash immediately after this call cannot replay it.
func PrepareTaskSubmissionDispatch(id, userID, tokenID int, route TaskSubmissionRoute) error {
	return recordTaskSubmissionDispatch(id, userID, tokenID, route)
}

func recordTaskSubmissionDispatch(id, userID, tokenID int, route TaskSubmissionRoute) error {
	if route.ChannelID <= 0 || route.APIKey == "" || route.UpstreamModelName == "" || route.ReservedQuota < 0 {
		return errors.New("invalid submission route")
	}
	data, err := common.Marshal(route)
	if err != nil {
		return err
	}
	billingContextData, err := common.Marshal(route.BillingContext)
	if err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var submission TaskSubmission
		if err := lockForUpdate(tx).First(&submission, id).Error; err != nil {
			return err
		}
		if submission.UserID != userID || submission.TokenID != tokenID {
			return errors.New("submission owner mismatch")
		}
		result := tx.Model(&TaskSubmission{}).Where("id = ? AND status = ? AND dispatched_at = ?", id, "submitting", 0).Updates(map[string]any{
			"route_json":               string(data),
			"dispatch_channel_id":      route.ChannelID,
			"dispatch_channel_type":    route.ChannelType,
			"dispatch_api_key":         route.APIKey,
			"dispatch_base_url":        route.BaseURL,
			"dispatch_upstream_model":  route.UpstreamModelName,
			"dispatch_billing_context": string(billingContextData),
			"dispatch_funding_source":  route.FundingSource,
			"dispatch_subscription_id": route.SubscriptionID,
			"dispatch_group":           route.Group,
			"dispatch_reserved_quota":  route.ReservedQuota,
			"dispatched_at":            time.Now().Unix(),
			"updated_at":               time.Now().Unix(),
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("submission already dispatched")
		}
		return nil
	})
}
