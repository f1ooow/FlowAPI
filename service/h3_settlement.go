package service

import (
	"context"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

func failH3PollingTask(ctx context.Context, task *model.Task, reason string) {
	previous := task.Status
	if previous == model.TaskStatusSuccess || previous == model.TaskStatusFailure {
		return
	}
	task.Status, task.FailReason, task.Progress = model.TaskStatusFailure, reason, "100%"
	task.FinishTime = time.Now().Unix()
	if err := recordH3Terminal(task, previous, 0); err != nil {
		logger.LogError(ctx, fmt.Sprintf("H3 task %s: %v", task.TaskID, err))
	}
}

func recordH3Terminal(task *model.Task, previous model.TaskStatus, final int) error {
	if *task.PrivateData.H3ReservationConfirmed {
		task.PendingQuota = &final
	}
	won, err := task.UpdateWithStatus(previous)
	if err != nil || !won || task.PendingQuota == nil {
		return err
	}
	return settleH3Quota(task)
}

func settleH3Quota(task *model.Task) error {
	reserved := task.Quota
	won, err := task.FinalizeH3Quota()
	if err != nil || !won || reserved == task.Quota {
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
	return nil
}

func retryH3Settlements(ctx context.Context) {
	var after int64
	for ctx.Err() == nil {
		var tasks []model.Task
		err := model.DB.WithContext(ctx).Where("pending_quota IS NOT NULL AND id > ?", after).Order("id").Limit(constant.TaskQueryLimit).Find(&tasks).Error
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("load pending H3 settlements: %v", err))
			return
		}
		if len(tasks) == 0 {
			return
		}
		for index := range tasks {
			task := &tasks[index]
			if err := settleH3Quota(task); err != nil {
				logger.LogError(ctx, fmt.Sprintf("H3 settlement %s: %v", task.TaskID, err))
			}
			after = task.ID
		}
	}
}
