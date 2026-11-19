package model

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	commonRelay "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"gorm.io/gorm"
)

type TaskStatus string

func (t TaskStatus) ToVideoStatus() string {
	var status string
	switch t {
	case TaskStatusQueued, TaskStatusSubmitted:
		status = dto.VideoStatusQueued
	case TaskStatusInProgress:
		status = dto.VideoStatusInProgress
	case TaskStatusSuccess:
		status = dto.VideoStatusCompleted
	case TaskStatusFailure, TaskStatusPollReview:
		status = dto.VideoStatusFailed
	default:
		status = dto.VideoStatusUnknown // Default fallback
	}
	return status
}

const (
	TaskStatusAdmitted        TaskStatus = "ADMITTED"
	TaskStatusDispatching     TaskStatus = "DISPATCHING"
	TaskStatusDispatchUnknown TaskStatus = "DISPATCH_UNKNOWN"
	TaskStatusPollReview      TaskStatus = "POLL_REVIEW"
	TaskStatusNotStart        TaskStatus = "NOT_START"
	TaskStatusSubmitted                  = "SUBMITTED"
	TaskStatusQueued                     = "QUEUED"
	TaskStatusInProgress                 = "IN_PROGRESS"
	TaskStatusFailure                    = "FAILURE"
	TaskStatusSuccess                    = "SUCCESS"
	TaskStatusUnknown                    = "UNKNOWN"
)

// TaskRefundLegacyCutoff separates tasks created before timeout refunds were
// introduced. Those legacy tasks are failed without an automatic refund.
const TaskRefundLegacyCutoff int64 = 1771718400 // 2026-02-22 00:00:00 UTC

type Task struct {
	ID           int64                 `json:"id" gorm:"primary_key;AUTO_INCREMENT"`
	CreatedAt    int64                 `json:"created_at" gorm:"index"`
	UpdatedAt    int64                 `json:"updated_at"`
	TaskID       string                `json:"task_id" gorm:"type:varchar(191);index"` // 第三方id，不一定有/ song id\ Task id
	H3UpstreamID *string               `json:"-" gorm:"column:h3_upstream_id;type:text"`
	H3LastPolledAt *int64              `json:"-" gorm:"column:h3_last_polled_at"`
	Platform     constant.TaskPlatform `json:"platform" gorm:"type:varchar(30);index"` // 平台
	UserId       int                   `json:"user_id" gorm:"index"`
	Group        string                `json:"group" gorm:"type:varchar(50)"` // 修正计费用
	ChannelId    int                   `json:"channel_id" gorm:"index"`
	Quota        int                   `json:"quota"`
	PendingQuota *int                  `json:"-" gorm:"index"`
	SettledQuota *int                  `json:"-"`
	Action       string                `json:"action" gorm:"type:varchar(40);index"` // 任务类型, song, lyrics, description-mode
	Status       TaskStatus            `json:"status" gorm:"type:varchar(20);index"` // 任务状态
	FailReason   string                `json:"fail_reason"`
	SubmitTime   int64                 `json:"submit_time" gorm:"index"`
	StartTime    int64                 `json:"start_time" gorm:"index"`
	FinishTime   int64                 `json:"finish_time" gorm:"index"`
	Progress     string                `json:"progress" gorm:"type:varchar(20);index"`
	Properties   Properties            `json:"properties" gorm:"type:json"`
	Username     string                `json:"username,omitempty" gorm:"-"`
	// 禁止返回给用户，内部可能包含key等隐私信息
	PrivateData TaskPrivateData `json:"-" gorm:"column:private_data;type:json"`
	Data        json.RawMessage `json:"data" gorm:"type:json"`
}

func (t *Task) SetData(data any) {
	b, _ := common.Marshal(data)
	t.Data = json.RawMessage(b)
}

func (t *Task) GetData(v any) error {
	return common.Unmarshal(t.Data, &v)
}

type Properties struct {
	Input             string `json:"input"`
	UpstreamModelName string `json:"upstream_model_name,omitempty"`
	OriginModelName   string `json:"origin_model_name,omitempty"`
}

func (m *Properties) Scan(val interface{}) error {
	bytesValue, _ := val.([]byte)
	if len(bytesValue) == 0 {
		*m = Properties{}
		return nil
	}
	return common.Unmarshal(bytesValue, m)
}

func (m Properties) Value() (driver.Value, error) {
	if m == (Properties{}) {
		return nil, nil
	}
	return common.Marshal(m)
}

type TaskPrivateData struct {
	H3ReservationConfirmed *bool                     `json:"h3_reservation_confirmed,omitempty"`
	ChannelGeneration      string                    `json:"channel_generation,omitempty"`
	ChannelOtherInfo       *string                   `json:"channel_other_info,omitempty"`
	ChannelBaseURL         string                    `json:"channel_base_url,omitempty"`
	ChannelSettings        *dto.ChannelOtherSettings `json:"channel_settings,omitempty"`
	Key                    string                    `json:"key,omitempty"`
	UpstreamTaskID         string                    `json:"upstream_task_id,omitempty"` // 上游真实 task ID
	ResultURL              string                    `json:"result_url,omitempty"`       // 任务成功后的结果 URL（视频地址等）
	// 计费上下文：用于异步退款/差额结算（轮询阶段读取）
	BillingSource  string              `json:"billing_source,omitempty"`  // "wallet"、"subscription" 或 "unlimited"
	SubscriptionId int                 `json:"subscription_id,omitempty"` // 订阅 ID，用于订阅退款
	TokenId        int                 `json:"token_id,omitempty"`        // 令牌 ID，用于令牌额度退款
	NodeName       string              `json:"node_name,omitempty"`       // 发起任务的节点名，轮询结算阶段据此归属日志而非最后查询节点
	BillingContext *TaskBillingContext `json:"billing_context,omitempty"` // 计费参数快照（用于轮询阶段重新计算）
}

// TaskBillingContext 记录任务提交时的计费参数，以便轮询阶段可以重新计算额度。
type TaskBillingContext struct {
	BillingUnit         string             `json:"billing_unit,omitempty"`
	Resolution          string             `json:"resolution,omitempty"`
	OutputVideoDuration float64            `json:"output_video_duration,omitempty"`
	InputVideoDuration  float64            `json:"input_video_duration,omitempty"`
	InputImageCount     int                `json:"input_image_count,omitempty"`
	InputImagePrice     float64            `json:"input_image_price,omitempty"`
	ModelPrice          float64            `json:"model_price,omitempty"`           // 模型单价
	GroupRatio          float64            `json:"group_ratio,omitempty"`           // 历史兼容：旧任务中是最终分组倍率
	BaseGroupRatio      float64            `json:"base_group_ratio,omitempty"`      // 提交时计费分组倍率
	UserGroupRatio      float64            `json:"user_group_ratio,omitempty"`      // 提交时用户分组倍率
	ChannelRatio        float64            `json:"channel_ratio,omitempty"`         // 提交时渠道成本倍率
	IncludeChannelRatio bool               `json:"include_channel_ratio,omitempty"` // 是否计入渠道倍率
	EffectiveRatio      *float64           `json:"effective_ratio,omitempty"`       // 提交时最终倍率；nil 表示历史快照
	ModelRatio          float64            `json:"model_ratio,omitempty"`           // 模型倍率
	OtherRatios         map[string]float64 `json:"other_ratios,omitempty"`          // 附加倍率（时长、分辨率等）
	OriginModelName     string             `json:"origin_model_name,omitempty"`     // 模型名称，必须为OriginModelName
	PerCallBilling      bool               `json:"per_call_billing,omitempty"`      // 按次计费：跳过轮询阶段的差额结算
	// WanInputBilled 标记预扣已含参考视频秒数；旧 Wan 快照缺此标记，结算只按成片秒数。
	WanInputBilled bool `json:"wan_input_billed,omitempty"`
	// OutputDurationUnverified 标记上游未给出可信成片时长、按请求时长结算（审计用）。
	OutputDurationUnverified bool `json:"output_duration_unverified,omitempty"`
}

// NewTaskBillingContext freezes the prices used by submission and later settlement.
func NewTaskBillingContext(info *commonRelay.RelayInfo) *TaskBillingContext {
	effectiveRatio := info.PriceData.GroupRatioInfo.GroupRatio
	return &TaskBillingContext{
		BillingUnit:         info.BillingUnit,
		ModelPrice:          info.PriceData.ModelPrice,
		InputImageCount:     info.PriceData.TaskInputImageCount,
		InputImagePrice:     info.PriceData.TaskInputImagePrice,
		GroupRatio:          effectiveRatio,
		BaseGroupRatio:      info.PriceData.GroupRatioInfo.BaseGroupRatio,
		UserGroupRatio:      info.PriceData.GroupRatioInfo.UserGroupRatio,
		ChannelRatio:        info.PriceData.GroupRatioInfo.ChannelRatio,
		IncludeChannelRatio: info.PriceData.GroupRatioInfo.IncludeChannelRatio,
		EffectiveRatio:      &effectiveRatio,
		ModelRatio:          info.PriceData.ModelRatio,
		OtherRatios:         info.PriceData.OtherRatios(),
		OriginModelName:     info.OriginModelName,
		PerCallBilling:      common.StringsContains(constant.TaskPricePatches, info.OriginModelName) || info.PriceData.UsePrice,
	}
}

// GetUpstreamTaskID 获取上游真实 task ID（用于与 provider 通信）
// 旧数据没有 UpstreamTaskID 时，TaskID 本身就是上游 ID
func (t *Task) GetUpstreamTaskID() string {
	if t.H3UpstreamID != nil {
		return *t.H3UpstreamID
	}
	if t.PrivateData.UpstreamTaskID != "" {
		return t.PrivateData.UpstreamTaskID
	}
	if t.PrivateData.H3ReservationConfirmed != nil {
		return ""
	}
	return t.TaskID
}

// GetResultURL 获取任务结果 URL（视频地址等）
// 新数据存在 PrivateData.ResultURL 中；旧数据回退到 FailReason（历史兼容）
func (t *Task) GetResultURL() string {
	if t.PrivateData.ResultURL != "" {
		return t.PrivateData.ResultURL
	}
	return t.FailReason
}

// GenerateTaskID 生成对外暴露的 task_xxxx 格式 ID
func GenerateTaskID() string {
	key, _ := common.GenerateRandomCharsKey(32)
	return "task_" + key
}

func (p *TaskPrivateData) Scan(val interface{}) error {
	bytesValue, _ := val.([]byte)
	if len(bytesValue) == 0 {
		return nil
	}
	return common.Unmarshal(bytesValue, p)
}

func (p TaskPrivateData) Value() (driver.Value, error) {
	if (p == TaskPrivateData{}) {
		return nil, nil
	}
	return common.Marshal(p)
}

// SyncTaskQueryParams 用于包含所有搜索条件的结构体，可以根据需求添加更多字段
type SyncTaskQueryParams struct {
	Platform       constant.TaskPlatform
	ChannelID      string
	TaskID         string
	UserID         string
	Action         string
	Status         string
	StartTimestamp int64
	EndTimestamp   int64
	UserIDs        []int
}

func InitTask(platform constant.TaskPlatform, relayInfo *commonRelay.RelayInfo) *Task {
	properties := Properties{}
	privateData := TaskPrivateData{}
	if relayInfo != nil && relayInfo.ChannelMeta != nil {
		if relayInfo.H3Billing {
			privateData.ChannelGeneration = relayInfo.ChannelGeneration
		}
		// Provider task IDs are scoped to the submitting account, including
		// multi-key channels. Do not rotate credentials during polling.
		privateData.Key = relayInfo.ChannelMeta.ApiKey
		privateData.ChannelBaseURL = relayInfo.ChannelMeta.ChannelBaseUrl
		otherInfo := relayInfo.ChannelMeta.ChannelOtherInfo
		privateData.ChannelOtherInfo = &otherInfo
		settings := relayInfo.ChannelMeta.ChannelOtherSettings
		privateData.ChannelSettings = &dto.ChannelOtherSettings{
			VideoProtocol: settings.VideoProtocol,
		}
		if settings.TaskEndpointOverride != nil {
			override := *settings.TaskEndpointOverride
			privateData.ChannelSettings.TaskEndpointOverride = &override
		}
		if relayInfo.UpstreamModelName != "" {
			properties.UpstreamModelName = relayInfo.UpstreamModelName
		}
		if relayInfo.OriginModelName != "" {
			properties.OriginModelName = relayInfo.OriginModelName
		}
	}

	// 使用预生成的公开 ID（如果有），否则新生成
	taskID := ""
	if relayInfo.TaskRelayInfo != nil && relayInfo.TaskRelayInfo.PublicTaskID != "" {
		taskID = relayInfo.TaskRelayInfo.PublicTaskID
	} else {
		taskID = GenerateTaskID()
	}

	t := &Task{
		TaskID:      taskID,
		UserId:      relayInfo.UserId,
		Group:       relayInfo.UsingGroup,
		SubmitTime:  time.Now().Unix(),
		Status:      TaskStatusNotStart,
		Progress:    "0%",
		ChannelId:   relayInfo.ChannelId,
		Platform:    platform,
		Properties:  properties,
		PrivateData: privateData,
	}
	return t
}

func TaskGetAllUserTask(userId int, startIdx int, num int, queryParams SyncTaskQueryParams) []*Task {
	var tasks []*Task
	var err error

	// 初始化查询构建器
	query := DB.Where("user_id = ?", userId)

	if queryParams.TaskID != "" {
		query = query.Where("task_id = ?", queryParams.TaskID)
	}
	if queryParams.Action != "" {
		query = query.Where("action = ?", queryParams.Action)
	}
	if queryParams.Status != "" {
		query = query.Where("status = ?", queryParams.Status)
	}
	if queryParams.Platform != "" {
		query = query.Where("platform = ?", queryParams.Platform)
	}
	if queryParams.StartTimestamp != 0 {
		// 假设您已将前端传来的时间戳转换为数据库所需的时间格式，并处理了时间戳的验证和解析
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != 0 {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}

	// 获取数据
	err = query.Omit("channel_id").Order("id desc").Limit(num).Offset(startIdx).Find(&tasks).Error
	if err != nil {
		return nil
	}

	return tasks
}

func TaskGetAllTasks(startIdx int, num int, queryParams SyncTaskQueryParams) []*Task {
	var tasks []*Task
	var err error

	// 初始化查询构建器
	query := DB

	// 添加过滤条件
	if queryParams.ChannelID != "" {
		query = query.Where("channel_id = ?", queryParams.ChannelID)
	}
	if queryParams.Platform != "" {
		query = query.Where("platform = ?", queryParams.Platform)
	}
	if queryParams.UserID != "" {
		query = query.Where("user_id = ?", queryParams.UserID)
	}
	if len(queryParams.UserIDs) != 0 {
		query = query.Where("user_id in (?)", queryParams.UserIDs)
	}
	if queryParams.TaskID != "" {
		query = query.Where("task_id = ?", queryParams.TaskID)
	}
	if queryParams.Action != "" {
		query = query.Where("action = ?", queryParams.Action)
	}
	if queryParams.Status != "" {
		query = query.Where("status = ?", queryParams.Status)
	}
	if queryParams.StartTimestamp != 0 {
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != 0 {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}

	// 获取数据
	err = query.Order("id desc").Limit(num).Offset(startIdx).Find(&tasks).Error
	if err != nil {
		return nil
	}

	return tasks
}

func GetTimedOutUnfinishedTasks(cutoffUnix int64, limit int) []*Task {
	var tasks []*Task
	err := DB.Where("progress != ?", "100%").
		Where("status NOT IN ?", []TaskStatus{TaskStatusFailure, TaskStatusSuccess, TaskStatusPollReview}).
		Where("(status != ? OR h3_upstream_id IS NOT NULL)", TaskStatusDispatchUnknown).
		Where("(h3_upstream_id IS NULL OR updated_at < ?)", cutoffUnix).
		Where("submit_time < ?", cutoffUnix).
		Order("submit_time").
		Limit(limit).
		Find(&tasks).Error
	if err != nil {
		return nil
	}
	return tasks
}

func GetAllUnFinishSyncTasks(limit int) []*Task {
	var tasks []*Task
	var err error
	// get all tasks progress is not 100%
	err = DB.Where("progress != ? AND status NOT IN ?", "100%", []TaskStatus{TaskStatusFailure, TaskStatusSuccess, TaskStatusPollReview}).
		Where("(status NOT IN ? OR h3_upstream_id IS NOT NULL)", []TaskStatus{TaskStatusDispatching, TaskStatusDispatchUnknown}).
		Limit(limit).Order("COALESCE(h3_last_polled_at, created_at * 1000000, 0), id").Find(&tasks).Error
	if err != nil {
		return nil
	}
	if len(tasks) == limit {
		var acknowledged []int64
		for _, task := range tasks {
			if task.H3UpstreamID != nil {
				acknowledged = append(acknowledged, task.ID)
			}
		}
		if len(acknowledged) > 0 {
			if err := DB.Model(&Task{}).Where("id IN ?", acknowledged).UpdateColumn("h3_last_polled_at", time.Now().UnixMicro()).Error; err != nil {
				common.SysError("record H3 polling batch: " + err.Error())
			}
		}
	}
	return tasks
}

// HasUnfinishedSyncTasks reports whether at least one async (Suno/video) task is
// still in progress. It is a cheap existence check (LIMIT 1) used to decide
// whether the async_task_poll system task needs to run; when no task is pending
// the scheduler skips creating a row entirely.
func HasUnfinishedSyncTasks() bool {
	var id int64
	query := DB.Model(&Task{}).
		Where("progress != ?", "100%").
		Where("status NOT IN ?", []TaskStatus{TaskStatusFailure, TaskStatusSuccess, TaskStatusPollReview}).
		Where("(status != ? OR h3_upstream_id IS NOT NULL)", TaskStatusDispatchUnknown)
	if constant.TaskTimeoutMinutes <= 0 {
		query = query.Where("(status != ? OR h3_upstream_id IS NOT NULL)", TaskStatusDispatching)
	}
	err := query.Limit(1).Pluck("id", &id).Error
	if err != nil || id != 0 {
		return err == nil
	}
	return DB.Model(&Task{}).Where("pending_quota IS NOT NULL").Limit(1).Pluck("id", &id).Error == nil && id != 0
}

func GetByTaskId(userId int, taskId string) (*Task, bool, error) {
	if taskId == "" {
		return nil, false, nil
	}
	var task *Task
	var err error
	err = DB.Where("user_id = ? and task_id = ?", userId, taskId).
		First(&task).Error
	exist, err := RecordExist(err)
	if err != nil {
		return nil, false, err
	}
	return task, exist, err
}

func GetByTaskIds(userId int, taskIds []any) ([]*Task, error) {
	if len(taskIds) == 0 {
		return nil, nil
	}
	var task []*Task
	var err error
	err = DB.Where("user_id = ? and task_id in (?)", userId, taskIds).
		Find(&task).Error
	if err != nil {
		return nil, err
	}
	return task, nil
}

// GetTaskForDashboardPreview allows the same admin visibility as the task log,
// but refuses ambiguous legacy IDs rather than serving an arbitrary result.
func GetTaskForDashboardPreview(userID int, taskID string, isAdmin bool) (*Task, bool, error) {
	query := DB.Where("task_id = ?", taskID)
	if !isAdmin {
		query = query.Where("user_id = ?", userID)
	}
	var tasks []*Task
	if err := query.Limit(2).Find(&tasks).Error; err != nil {
		return nil, false, err
	}
	if len(tasks) != 1 {
		return nil, false, nil
	}
	return tasks[0], true, nil
}

func (task *Task) Insert() error {
	if task.PrivateData.H3ReservationConfirmed != nil {
		return DB.Transaction(func(tx *gorm.DB) error { return task.insertWithUsage(tx) })
	}
	return DB.Create(task).Error
}

func (task *Task) h3Channel(tx *gorm.DB) *gorm.DB {
	channel := tx.Model(&Channel{}).Where("id = ?", task.ChannelId)
	if task.PrivateData.ChannelGeneration == "" {
		return channel.Where("generation LIKE ? AND (created_time IS NULL OR created_time <= ?)", legacyChannelGenerationPrefix+"%", task.SubmitTime)
	}
	return channel.Where("generation = ?", task.PrivateData.ChannelGeneration)
}

func (task *Task) GetH3PollingChannel() (*Channel, error) {
	var channel Channel
	err := task.h3Channel(DB).Take(&channel).Error
	return &channel, err
}

func (task *Task) insertWithUsage(tx *gorm.DB) error {
	if err := tx.Create(task).Error; err != nil {
		return err
	}
	if task.PrivateData.H3ReservationConfirmed == nil || !*task.PrivateData.H3ReservationConfirmed {
		return nil
	}
	if err := requireAccountingRow(tx.Model(&User{}).Where("id = ?", task.UserId).Updates(map[string]any{
		"used_quota": gorm.Expr("used_quota + ?", task.Quota), "request_count": gorm.Expr("request_count + 1"),
	})); err != nil {
		return err
	}
	if task.ChannelId == 0 {
		return nil
	}
	if task.PrivateData.ChannelGeneration == "" {
		return fmt.Errorf("missing selected channel generation for channel %d", task.ChannelId)
	}
	channel := task.h3Channel(tx)
	if task.Quota == 0 {
		return lockForUpdate(channel).Select("id").Take(&Channel{}).Error
	}
	return requireAccountingRow(channel.Update("used_quota", gorm.Expr("used_quota + ?", task.Quota)))
}

type taskSnapshot struct {
	Status     TaskStatus
	Progress   string
	StartTime  int64
	FinishTime int64
	FailReason string
	ResultURL  string
	Data       json.RawMessage
}

func (s taskSnapshot) Equal(other taskSnapshot) bool {
	return s.Status == other.Status &&
		s.Progress == other.Progress &&
		s.StartTime == other.StartTime &&
		s.FinishTime == other.FinishTime &&
		s.FailReason == other.FailReason &&
		s.ResultURL == other.ResultURL &&
		bytes.Equal(s.Data, other.Data)
}

func (t *Task) Snapshot() taskSnapshot {
	return taskSnapshot{
		Status:     t.Status,
		Progress:   t.Progress,
		StartTime:  t.StartTime,
		FinishTime: t.FinishTime,
		FailReason: t.FailReason,
		ResultURL:  t.PrivateData.ResultURL,
		Data:       t.Data,
	}
}

func (Task *Task) Update() error {
	var err error
	err = DB.Save(Task).Error
	return err
}

func (t *Task) UpdateQuota() error {
	return DB.Model(t).Update("quota", t.Quota).Error
}

func (t *Task) BeginH3Dispatch() error {
	if t.ID == 0 || t.Status != TaskStatusAdmitted {
		return fmt.Errorf("H3 task %s is not admitted for dispatch", t.TaskID)
	}
	db := DB
	var claimed bool
	err := db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Task{}).Where("id = ? AND user_id = ? AND task_id = ? AND status = ? AND h3_upstream_id IS NULL",
			t.ID, t.UserId, t.TaskID, TaskStatusAdmitted).Update("status", TaskStatusDispatching)
		if result.Error != nil {
			return result.Error
		}
		claimed = result.RowsAffected == 1
		if !claimed {
			return fmt.Errorf("H3 task %s no longer owns the admission", t.TaskID)
		}
		return nil
	})
	if err != nil {
		if claimed {
			id, userID, publicID := t.ID, t.UserId, t.TaskID
			unused := db.Model(&Task{}).Where("id = ? AND user_id = ? AND task_id = ? AND h3_upstream_id IS NULL AND status IN ?",
				id, userID, publicID, []TaskStatus{TaskStatusAdmitted, TaskStatusDispatching, TaskStatusDispatchUnknown}).Session(&gorm.Session{})
			if cancelErr := failUnsentH3Admissions(unused); cancelErr != nil {
				common.SysError(fmt.Sprintf("H3 %s: retrying unused send intent after commit error: %v", publicID, cancelErr))
				go func() {
					for delay := time.Second; ; delay = min(delay*2, time.Minute) {
						time.Sleep(delay)
						if failUnsentH3Admissions(unused) == nil {
							return
						}
					}
				}()
			}
		}
		return err
	}
	t.Status = TaskStatusDispatching
	return nil
}

func (t *Task) AcceptH3Dispatch(upstreamID string, data json.RawMessage) (acknowledged bool, err error) {
	if upstreamID == "" {
		return false, fmt.Errorf("empty H3 provider acknowledgement")
	}
	dispatch := DB.Model(&Task{}).Where("id = ? AND status IN ? AND (h3_upstream_id IS NULL OR h3_upstream_id = ?)",
		t.ID, []TaskStatus{TaskStatusDispatching, TaskStatusDispatchUnknown}, upstreamID).Session(&gorm.Session{})
	ack := dispatch.Update("h3_upstream_id", upstreamID)
	promotion := dispatch.Updates(map[string]any{"status": TaskStatusNotStart, "h3_upstream_id": upstreamID, "data": data})
	if promotion.Error == nil && promotion.RowsAffected == 1 {
		return true, nil
	}
	return ack.Error == nil && ack.RowsAffected == 1, fmt.Errorf("ack rows=%d err=%v; promotion rows=%d err=%v", ack.RowsAffected, ack.Error, promotion.RowsAffected, promotion.Error)
}

func (t *Task) MarkH3DispatchUnknown() error {
	if t.Status == TaskStatusAdmitted || t.Status == TaskStatusSuccess || t.Status == TaskStatusFailure || t.GetUpstreamTaskID() != "" {
		return fmt.Errorf("H3 dispatch %s already has a provider ID or terminal decision", t.TaskID)
	}
	return DB.Model(&Task{}).Where("id = ? AND status = ? AND h3_upstream_id IS NULL", t.ID, t.Status).
		Update("status", TaskStatusDispatchUnknown).Error
}

func (t *Task) MarkH3PollingForReview(cutoff int64, reason string) error {
	return DB.Model(&Task{}).Where("id = ? AND status = ? AND updated_at < ? AND h3_upstream_id IS NOT NULL", t.ID, t.Status, cutoff).
		Updates(map[string]any{"status": TaskStatusPollReview, "fail_reason": reason, "progress": "100%", "finish_time": common.GetTimestamp()}).Error
}

// UpdateWithStatus performs a conditional UPDATE guarded by fromStatus (CAS).
// Returns (true, nil) if this caller won the update, (false, nil) if
// another process already moved the task out of fromStatus. MySQL commonly
// reports changed rows rather than matched rows, so a same-value no-op update
// can also return false even when the status predicate still matched.
//
// Uses Model().Select("*").Updates() instead of Save() because GORM's Save
// falls back to INSERT ON CONFLICT when the WHERE-guarded UPDATE matches
// zero rows, which silently bypasses the CAS guard.
func (t *Task) UpdateWithStatus(fromStatus TaskStatus) (bool, error) {
	query := DB.Model(t).Where("status = ?", fromStatus)
	if (fromStatus == TaskStatusDispatching || fromStatus == TaskStatusDispatchUnknown) && t.H3UpstreamID == nil {
		query = query.Where("h3_upstream_id IS NULL")
	}
	result := query.Select("*").Omit("H3UpstreamID", "H3LastPolledAt").Updates(t)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// TaskBulkUpdateByID performs an unconditional bulk UPDATE by primary key IDs.
// WARNING: This function has NO CAS (Compare-And-Swap) guard — it will overwrite
// any concurrent status changes. DO NOT use in billing/quota lifecycle flows
// (e.g., timeout, success, failure transitions that trigger refunds or settlements).
// For status transitions that involve billing, use Task.UpdateWithStatus() instead.
func TaskBulkUpdateByID(ids []int64, params map[string]any) error {
	if len(ids) == 0 {
		return nil
	}
	return DB.Model(&Task{}).
		Where("id in (?)", ids).
		Updates(params).Error
}

type TaskQuotaUsage struct {
	Mode  string  `json:"mode"`
	Count float64 `json:"count"`
}

// TaskCountAllTasks returns total tasks that match the given query params (admin usage)
func TaskCountAllTasks(queryParams SyncTaskQueryParams) int64 {
	var total int64
	query := DB.Model(&Task{})
	if queryParams.ChannelID != "" {
		query = query.Where("channel_id = ?", queryParams.ChannelID)
	}
	if queryParams.Platform != "" {
		query = query.Where("platform = ?", queryParams.Platform)
	}
	if queryParams.UserID != "" {
		query = query.Where("user_id = ?", queryParams.UserID)
	}
	if len(queryParams.UserIDs) != 0 {
		query = query.Where("user_id in (?)", queryParams.UserIDs)
	}
	if queryParams.TaskID != "" {
		query = query.Where("task_id = ?", queryParams.TaskID)
	}
	if queryParams.Action != "" {
		query = query.Where("action = ?", queryParams.Action)
	}
	if queryParams.Status != "" {
		query = query.Where("status = ?", queryParams.Status)
	}
	if queryParams.StartTimestamp != 0 {
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != 0 {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}
	_ = query.Count(&total).Error
	return total
}

// TaskCountAllUserTask returns total tasks for given user
func TaskCountAllUserTask(userId int, queryParams SyncTaskQueryParams) int64 {
	var total int64
	query := DB.Model(&Task{}).Where("user_id = ?", userId)
	if queryParams.TaskID != "" {
		query = query.Where("task_id = ?", queryParams.TaskID)
	}
	if queryParams.Action != "" {
		query = query.Where("action = ?", queryParams.Action)
	}
	if queryParams.Status != "" {
		query = query.Where("status = ?", queryParams.Status)
	}
	if queryParams.Platform != "" {
		query = query.Where("platform = ?", queryParams.Platform)
	}
	if queryParams.StartTimestamp != 0 {
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != 0 {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}
	_ = query.Count(&total).Error
	return total
}
func (t *Task) ToOpenAIVideo() *dto.OpenAIVideo {
	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = t.TaskID
	openAIVideo.Status = t.Status.ToVideoStatus()
	openAIVideo.Model = t.Properties.OriginModelName
	openAIVideo.SetProgressStr(t.Progress)
	openAIVideo.CreatedAt = t.CreatedAt
	openAIVideo.CompletedAt = t.UpdatedAt
	openAIVideo.SetMetadata("url", t.GetResultURL())
	return openAIVideo
}
