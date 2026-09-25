package model

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupSQLiteLogDB points LOG_DB at an isolated in-memory SQLite database and
// restores the previous log database when the test ends.
func setupSQLiteLogDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := LOG_DB
	previousType := common.LogDatabaseType()
	previousGroupCol := logGroupCol
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	LOG_DB = db
	common.SetLogDatabaseType(common.DatabaseTypeSQLite)
	logGroupCol = "`group`"
	t.Cleanup(func() {
		LOG_DB = previousDB
		common.SetLogDatabaseType(previousType)
		logGroupCol = previousGroupCol
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&Log{}))
	return db
}

// SumUsedQuota feeds the usage-log statistics header: quota follows the
// requested time window, while RPM/TPM always cover consume rows of the last
// minute. Both honor the user, token, model, channel and group filters.
func TestSumUsedQuotaFilters(t *testing.T) {
	db := setupSQLiteLogDB(t)
	now := time.Now().Unix()
	logs := []Log{
		{RequestId: "a", ChannelId: 11, Quota: 10, PromptTokens: 100, CompletionTokens: 20},
		{RequestId: "b", ChannelId: 12, Quota: 20, PromptTokens: 200, CompletionTokens: 30},
		{RequestId: "c", ChannelId: 11, Quota: 7, PromptTokens: 40, CompletionTokens: 10},
		{RequestId: "d", ChannelId: 11, Quota: 3, PromptTokens: 10, CompletionTokens: 1},
		{RequestId: "bob", ChannelId: 12, Quota: 9, PromptTokens: 50, CompletionTokens: 5},
		{RequestId: "older", ChannelId: 11, Quota: 100, PromptTokens: 1000, CompletionTokens: 100},
		{RequestId: "error", ChannelId: 11, Quota: 999, PromptTokens: 999, CompletionTokens: 999},
		{RequestId: "refund", ChannelId: 11, Quota: 999, PromptTokens: 999, CompletionTokens: 999},
	}
	for i := range logs {
		logs[i].Type = LogTypeConsume
		logs[i].CreatedAt = now - 5
		logs[i].Username = "alice"
		logs[i].ModelName = "gpt-test"
		logs[i].TokenName = "first"
		logs[i].Group = "default"
	}
	logs[4].Username = "bob"
	logs[4].ModelName = "other-model"
	logs[4].TokenName = "second"
	logs[4].Group = "vip"
	logs[5].CreatedAt = now - 120
	logs[6].Type = LogTypeError
	logs[7].Type = LogTypeRefund
	require.NoError(t, db.Create(&logs).Error)

	cases := []struct {
		name     string
		username string
		channel  int
		model    string
		token    string
		group    string
		start    int64
		end      int64
		want     Stat
	}{
		{name: "all consume rows", want: Stat{Quota: 149, Rpm: 5, Tpm: 466}},
		{name: "user filter", username: "alice", want: Stat{Quota: 140, Rpm: 4, Tpm: 411}},
		{name: "first channel", channel: 11, want: Stat{Quota: 120, Rpm: 3, Tpm: 181}},
		{name: "second channel", channel: 12, want: Stat{Quota: 29, Rpm: 2, Tpm: 285}},
		{name: "user and channel", username: "alice", channel: 12, want: Stat{Quota: 20, Rpm: 1, Tpm: 230}},
		{name: "model filter", model: "other-model", want: Stat{Quota: 9, Rpm: 1, Tpm: 55}},
		{name: "token filter", token: "second", want: Stat{Quota: 9, Rpm: 1, Tpm: 55}},
		{name: "group filter", group: "vip", want: Stat{Quota: 9, Rpm: 1, Tpm: 55}},
		{name: "recent quota window", start: now - 30, end: now + 30, want: Stat{Quota: 49, Rpm: 5, Tpm: 466}},
		{name: "historical quota keeps live rates", start: now - 180, end: now - 90, want: Stat{Quota: 100, Rpm: 5, Tpm: 466}},
		{name: "empty result", username: "missing", want: Stat{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stat, err := SumUsedQuota(LogTypeConsume, tc.start, tc.end, tc.model, tc.username, tc.token, tc.channel, tc.group)
			require.NoError(t, err)
			assert.Equal(t, tc.want, stat)
		})
	}
}
