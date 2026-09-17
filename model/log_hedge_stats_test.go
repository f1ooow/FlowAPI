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

func setupHedgeLogDB(t *testing.T) *gorm.DB {
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

func TestSumUsedQuotaHedgeAttempts(t *testing.T) {
	db := setupHedgeLogDB(t)
	now := time.Now().Unix()
	logs := []Log{
		{RequestId: "race", ChannelId: 11, Quota: 10, PromptTokens: 100, CompletionTokens: 20, Other: `{"hedge":{"role":"winner"}}`},
		{RequestId: "race", ChannelId: 12, Quota: 20, PromptTokens: 200, CompletionTokens: 30, Other: `{"hedge":{"role":"loser"}}`},
		{RequestId: "another", ChannelId: 11, Quota: 7, PromptTokens: 40, CompletionTokens: 10},
		{ChannelId: 11, Quota: 3, PromptTokens: 10, CompletionTokens: 1},
		{ChannelId: 11, Quota: 4, PromptTokens: 20, CompletionTokens: 2},
		{ChannelId: 11, Quota: 5, PromptTokens: 30, CompletionTokens: 3},
		{ChannelId: 11, Quota: 6, PromptTokens: 40, CompletionTokens: 4},
		{RequestId: "bob-request", ChannelId: 12, Quota: 9, PromptTokens: 50, CompletionTokens: 5},
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
	logs[7].Username = "bob"
	logs[7].ModelName = "other-model"
	logs[7].TokenName = "second"
	logs[7].Group = "vip"
	logs[8].CreatedAt = now - 120
	logs[9].Type = LogTypeError
	logs[10].Type = LogTypeRefund
	require.NoError(t, db.Create(&logs).Error)
	require.NoError(t, db.Model(&Log{}).Where("id IN ?", []int{logs[5].Id, logs[6].Id}).Update("request_id", nil).Error)

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
		{name: "all attempts additive", want: Stat{Quota: 164, Rpm: 7, Tpm: 565}},
		{name: "user filter", username: "alice", want: Stat{Quota: 155, Rpm: 6, Tpm: 510}},
		{name: "first channel", channel: 11, want: Stat{Quota: 135, Rpm: 6, Tpm: 280}},
		{name: "second channel", channel: 12, want: Stat{Quota: 29, Rpm: 1, Tpm: 285}},
		{name: "user and channel", username: "alice", channel: 12, want: Stat{Quota: 20, Rpm: 0, Tpm: 230}},
		{name: "model filter", model: "other-model", want: Stat{Quota: 9, Rpm: 1, Tpm: 55}},
		{name: "token filter", token: "second", want: Stat{Quota: 9, Rpm: 1, Tpm: 55}},
		{name: "group filter", group: "vip", want: Stat{Quota: 9, Rpm: 1, Tpm: 55}},
		{name: "recent quota window", start: now - 30, end: now + 30, want: Stat{Quota: 64, Rpm: 7, Tpm: 565}},
		{name: "historical quota preserves live rates", start: now - 180, end: now - 90, want: Stat{Quota: 100, Rpm: 7, Tpm: 565}},
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

func TestSumUsedQuotaLateLoserDoesNotRenewLogicalRPM(t *testing.T) {
	db := setupHedgeLogDB(t)
	now := time.Now().Unix()
	logs := []Log{
		{Type: LogTypeConsume, CreatedAt: now - 120, RequestId: "late-race", ChannelId: 11, Group: "default", Quota: 10, PromptTokens: 100, CompletionTokens: 20, Other: `{"hedge":{"role":"winner"}}`},
		{Type: LogTypeConsume, CreatedAt: now - 5, RequestId: "late-race", ChannelId: 12, Group: "backup", Quota: 20, PromptTokens: 200, CompletionTokens: 30, Other: `{"hedge":{"role":"loser"}}`},
	}
	require.NoError(t, db.Create(&logs).Error)

	stat, err := SumUsedQuota(LogTypeConsume, 0, 0, "", "", "", 0, "")
	require.NoError(t, err)
	assert.Equal(t, Stat{Quota: 30, Rpm: 0, Tpm: 230}, stat)

	stat, err = SumUsedQuota(LogTypeConsume, now-30, now+30, "", "", "", 12, "backup")
	require.NoError(t, err)
	assert.Equal(t, Stat{Quota: 20, Rpm: 0, Tpm: 230}, stat, "channel/group filters must not turn a loser into another logical request")
}

func TestSumUsedQuotaPreservesLegacyRowsAndParsesHedgeRoles(t *testing.T) {
	db := setupHedgeLogDB(t)
	metadata := []string{
		"",
		`not-json`,
		`{"hedge":`,
		`{"hedge":null}`,
		`{"hedge":{"other":"value"}}`,
		`{"admin_info":{"hedge":{"role":"loser"}}}`,
		`{"hedge":{"role":"winner"}}`,
		`{ "hedge": { "role": "loser", "settlement_status": "settled" } }`,
		`{"hedge":{"role":"loser","settlement_status":"unmetered"}}`,
	}
	logs := make([]Log, len(metadata))
	for i, other := range metadata {
		// Legacy consume rows remain separate requests even if an old caller
		// reused its request ID. Neither malformed nor nested metadata is a loser.
		logs[i] = Log{Type: LogTypeConsume, CreatedAt: time.Now().Unix() - 5, RequestId: "reused-legacy-id", Quota: 3, PromptTokens: 10, CompletionTokens: 2, Other: other}
	}
	require.NoError(t, db.Create(&logs).Error)
	require.NoError(t, db.Model(&Log{}).Where("id = ?", logs[0].Id).Update("other", nil).Error)

	stat, err := SumUsedQuota(LogTypeConsume, 0, 0, "", "", "", 0, "")
	require.NoError(t, err)
	assert.Equal(t, Stat{Quota: 27, Rpm: 7, Tpm: 108}, stat)
}
