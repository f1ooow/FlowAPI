package service

import (
	"context"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

func FailH3Task(ctx context.Context, task *model.Task, reason string) bool {
	previous := task.Status
	if previous == model.TaskStatusSuccess || previous == model.TaskStatusFailure {
		return previous == model.TaskStatusFailure
	}
	task.Status, task.FailReason, task.Progress = model.TaskStatusFailure, reason, "100%"
	task.FinishTime = time.Now().Unix()
	recorded, err := persistAndSettleH3Terminal(task, previous, 0)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("H3 task %s: %v", task.TaskID, err))
	}
	return recorded
}

func recordH3Terminal(task *model.Task, previous model.TaskStatus, final int) error {
	_, err := persistAndSettleH3Terminal(task, previous, final)
	return err
}

func persistAndSettleH3Terminal(task *model.Task, previous model.TaskStatus, final int) (bool, error) {
	if *task.PrivateData.H3ReservationConfirmed {
		task.PendingQuota = &final
	}
	won, err := task.UpdateWithStatus(previous)
	if err == nil && !won && previous == model.TaskStatusDispatching && task.H3UpstreamID == nil && task.Status == model.TaskStatusFailure {
		won, err = task.UpdateWithStatus(model.TaskStatusDispatchUnknown)
	}
	if err != nil || !won || task.PendingQuota == nil {
		return won, err
	}
	return true, settleH3Quota(task)
}

func settleH3Quota(task *model.Task) error {
	if task.SettledQuota != nil {
		return task.PublishH3Refund()
	}
	reserved := task.Quota
	won, err := task.FinalizeH3Quota()
	if !won || reserved == task.Quota {
		return err
	}
	other := taskBillingOther(task)
	other["task_id"], other["pre_consumed_quota"], other["actual_quota"] = task.TaskID, reserved, task.Quota
	other["reason"] = task.FailReason
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId: task.UserId, LogType: model.LogTypeRefund, Content: "H3 settlement", ChannelId: task.ChannelId,
		ModelName: taskModelName(task), Quota: reserved - task.Quota, TokenId: task.PrivateData.TokenId,
		Group: task.Group, Other: other, NodeName: task.PrivateData.NodeName,
	})
	return err
}

func retryH3Settlements(ctx context.Context) {
	var after int64
	for ctx.Err() == nil {
		var tasks []model.Task
		err := model.PendingH3Settlements().WithContext(ctx).Where("id > ?", after).Order("id").Limit(constant.TaskQueryLimit).Find(&tasks).Error
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("load pending H3 settlements: %v", err))
			return
		}
		if len(tasks) == 0 {
			return
		}
		for index := range tasks {
			task := &tasks[index]
			if task.H3RejectionCode != nil && (task.Status == model.TaskStatusDispatching || task.Status == model.TaskStatusDispatchUnknown) {
				FailH3Task(ctx, task, *task.H3RejectionCode)
			} else if err := settleH3Quota(task); err != nil {
				logger.LogError(ctx, fmt.Sprintf("H3 settlement %s: %v", task.TaskID, err))
			}
			after = task.ID
		}
	}
}
