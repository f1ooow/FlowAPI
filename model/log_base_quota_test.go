package model

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// useIsolatedLogDB points LOG_DB at a private in-memory SQLite database so a
// test can insert and scan logs without seeing rows from other tests.
func useIsolatedLogDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousLogDB := LOG_DB
	previousLogDatabaseType := common.LogDatabaseType()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	LOG_DB = db
	common.SetLogDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		LOG_DB = previousLogDB
		common.SetLogDatabaseType(previousLogDatabaseType)
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&Log{}))
	return db
}

func billingRatiosOther(effectiveRatio float64) map[string]interface{} {
	return map[string]interface{}{
		"group_ratio": effectiveRatio,
		"admin_info": map[string]interface{}{
			"billing_ratios": map[string]interface{}{"effective_ratio": effectiveRatio},
		},
	}
}

// The base quota is the charge before group and channel ratios, i.e. the
// official-price amount a channel's cost ratio applies to.
func TestConsumeLogBaseQuota(t *testing.T) {
	tests := []struct {
		name  string
		quota int
		other map[string]interface{}
		want  int64
	}{
		{name: "group ratio only", quota: 1_141_862, other: billingRatiosOther(1.5), want: 761_241},
		{name: "channel ratio included in the price", quota: 1000, other: billingRatiosOther(0.2), want: 5000},
		{name: "effective ratio wins over group ratio", quota: 600, other: map[string]interface{}{
			"group_ratio": 1.0,
			"admin_info": map[string]interface{}{
				"billing_ratios": map[string]interface{}{"effective_ratio": 3.0},
			},
		}, want: 200},
		{name: "legacy log without billing snapshot", quota: 70, other: map[string]interface{}{"group_ratio": 0.35}, want: 200},
		{name: "no recorded ratio", quota: 40, other: map[string]interface{}{}, want: 0},
		{name: "free group", quota: 0, other: billingRatiosOther(0), want: 0},
		{name: "zero ratio with a charge is not divided", quota: 10, other: billingRatiosOther(0), want: 0},
		{name: "nil other", quota: 10, other: nil, want: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, consumeLogBaseQuota(test.quota, test.other))
		})
	}
}

func TestRecordConsumeLogStoresBaseQuota(t *testing.T) {
	db := useIsolatedLogDB(t)
	previousDataExport := common.DataExportEnabled
	common.DataExportEnabled = false
	t.Cleanup(func() { common.DataExportEnabled = previousDataExport })

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Set(common.RequestIdKey, "base-quota-consume")

	RecordConsumeLog(c, 0, RecordConsumeLogParams{
		ChannelId: 91,
		ModelName: "claude-fable-5",
		Quota:     300,
		Other:     billingRatiosOther(1.5),
	})

	var log Log
	require.NoError(t, db.Where("request_id = ?", "base-quota-consume").First(&log).Error)
	assert.EqualValues(t, 300, log.Quota)
	assert.EqualValues(t, 200, log.BaseQuota)
}

func TestRecordTaskBillingLogStoresBaseQuotaForConsume(t *testing.T) {
	db := useIsolatedLogDB(t)
	previousDataExport := common.DataExportEnabled
	common.DataExportEnabled = false
	t.Cleanup(func() { common.DataExportEnabled = previousDataExport })

	RecordTaskBillingLog(RecordTaskBillingLogParams{LogType: LogTypeConsume, ChannelId: 92, ModelName: "veo", Quota: 1000, Other: billingRatiosOther(0.5)})
	RecordTaskBillingLog(RecordTaskBillingLogParams{LogType: LogTypeRefund, ChannelId: 93, ModelName: "veo", Quota: 1000, Other: billingRatiosOther(0.5)})

	var consume, refund Log
	require.NoError(t, db.Where("channel_id = ?", 92).First(&consume).Error)
	require.NoError(t, db.Where("channel_id = ?", 93).First(&refund).Error)
	assert.EqualValues(t, 2000, consume.BaseQuota)
	assert.Zero(t, refund.BaseQuota)
}

// Logs written before base_quota existed must be filled from their stored
// billing snapshot so channel cost totals cover history.
func TestBackfillLogBaseQuota(t *testing.T) {
	db := useIsolatedLogDB(t)
	logs := []Log{
		{ChannelId: 94, Type: LogTypeConsume, Quota: 300, Other: `{"admin_info":{"billing_ratios":{"effective_ratio":1.5}}}`},
		{ChannelId: 94, Type: LogTypeConsume, Quota: 70, Other: `{"group_ratio":0.35}`},
		{ChannelId: 94, Type: LogTypeConsume, Quota: 50, BaseQuota: 999, Other: `{"group_ratio":1}`},
		{ChannelId: 94, Type: LogTypeConsume, Quota: 40, Other: `{}`},
		{ChannelId: 94, Type: LogTypeConsume, Quota: 40, Other: `not json`},
		{ChannelId: 94, Type: LogTypeRefund, Quota: 30, Other: `{"group_ratio":1}`},
	}
	require.NoError(t, db.Create(&logs).Error)

	updated, err := BackfillLogBaseQuota(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, updated)

	var stored []Log
	require.NoError(t, db.Order("id").Find(&stored).Error)
	baseQuotas := make([]int64, 0, len(stored))
	for _, log := range stored {
		baseQuotas = append(baseQuotas, log.BaseQuota)
	}
	assert.Equal(t, []int64{200, 200, 999, 0, 0, 0}, baseQuotas)

	updated, err = BackfillLogBaseQuota(context.Background())
	require.NoError(t, err)
	assert.Zero(t, updated)
}
