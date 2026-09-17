package model

import (
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsClickHouseDSN(t *testing.T) {
	cases := []struct {
		dsn  string
		want bool
	}{
		{"clickhouse://default:pass@localhost:9000/logs", true},
		{"tcp://localhost:9000/logs", true},
		{"http://localhost:8123/logs", true},
		{"https://localhost:8443/logs", true},
		{"postgres://root:pass@localhost:5432/db", false},
		{"postgresql://root:pass@localhost:5432/db", false},
		{"root:pass@tcp(localhost:3306)/db", false},
		{"local", false},
		{"", false},
	}
	for _, c := range cases {
		assert.Equalf(t, c.want, isClickHouseDSN(c.dsn), "dsn=%q", c.dsn)
	}
}

func TestNormalizeClickHouseDSN(t *testing.T) {
	// https without secure gets secure=true appended
	normalized := normalizeClickHouseDSN("https://default:pass@localhost:8443/logs")
	assert.Contains(t, normalized, "secure=true")
	assert.True(t, strings.HasPrefix(normalized, "https://"))

	// https that already specifies secure is left untouched
	assert.Equal(t,
		"https://localhost:8443/logs?secure=false",
		normalizeClickHouseDSN("https://localhost:8443/logs?secure=false"),
	)

	// non-https schemes are returned verbatim
	assert.Equal(t, "clickhouse://localhost:9000/logs", normalizeClickHouseDSN("clickhouse://localhost:9000/logs"))
	assert.Equal(t, "tcp://localhost:9000/logs", normalizeClickHouseDSN("tcp://localhost:9000/logs"))
}

func TestChooseDBRejectsClickHouseForMainDatabase(t *testing.T) {
	original, had := os.LookupEnv("SQL_DSN")
	t.Cleanup(func() {
		if had {
			require.NoError(t, os.Setenv("SQL_DSN", original))
		} else {
			require.NoError(t, os.Unsetenv("SQL_DSN"))
		}
	})
	require.NoError(t, os.Setenv("SQL_DSN", "clickhouse://default:pass@localhost:9000/logs"))

	db, dbType, err := chooseDB("SQL_DSN", false)
	require.Error(t, err)
	assert.Nil(t, db)
	assert.Equal(t, common.DatabaseType(""), dbType)
	assert.Contains(t, err.Error(), "does not support ClickHouse")
}

func TestClickHouseLogTTLExpression(t *testing.T) {
	assert.Equal(t, "", clickHouseLogTTLExpression(0))
	assert.Equal(t, "", clickHouseLogTTLExpression(-5))
	assert.Equal(t, "toDateTime(created_at) + INTERVAL 30 DAY DELETE", clickHouseLogTTLExpression(30))
}

func TestClickHouseLogTTLClause(t *testing.T) {
	assert.Equal(t, "", clickHouseLogTTLClause(0))
	assert.Equal(t, "\nTTL toDateTime(created_at) + INTERVAL 7 DAY DELETE", clickHouseLogTTLClause(7))
}

func TestClickHouseLogCreateTableSQL(t *testing.T) {
	withoutTTL := clickHouseLogCreateTableSQL(0)
	assert.Contains(t, withoutTTL, "CREATE TABLE IF NOT EXISTS logs")
	assert.Contains(t, withoutTTL, "ENGINE = MergeTree()")
	assert.Contains(t, withoutTTL, "PARTITION BY toYYYYMM(toDateTime(created_at))")
	assert.Contains(t, withoutTTL, "ORDER BY (created_at, request_id)")
	assert.NotContains(t, withoutTTL, "TTL ")

	withTTL := clickHouseLogCreateTableSQL(30)
	assert.Contains(t, withTTL, "ORDER BY (created_at, request_id)")
	assert.Contains(t, withTTL, "TTL toDateTime(created_at) + INTERVAL 30 DAY DELETE")
}

func TestClickHouseCreateTableHasTTL(t *testing.T) {
	assert.True(t, clickHouseCreateTableHasTTL("CREATE TABLE logs (...)\nTTL toDateTime(created_at) + INTERVAL 30 DAY DELETE"))
	assert.True(t, clickHouseCreateTableHasTTL("CREATE TABLE logs (...) TTL toDateTime(created_at)"))
	assert.False(t, clickHouseCreateTableHasTTL("CREATE TABLE logs (...)\nORDER BY (created_at, request_id)"))
}

func TestClickHouseLogOrder(t *testing.T) {
	db := setupHedgeLogDB(t)
	logs := []Log{
		{CreatedAt: 100, RequestId: "race", ChannelId: 11, UpstreamRequestId: "upstream", Other: `{"hedge":{"attempt_id":"a"}}`},
		{CreatedAt: 100, RequestId: "race", ChannelId: 12, UpstreamRequestId: "upstream", Other: `{"hedge":{"attempt_id":"c"}}`},
		{CreatedAt: 100, RequestId: "race", ChannelId: 11, UpstreamRequestId: "upstream", Other: `{"hedge":{"attempt_id":"b"}}`},
		{CreatedAt: 100, RequestId: "race", ChannelId: 11, UpstreamRequestId: "upstream-z", Other: `{"hedge":{"attempt_id":"d"}}`},
	}
	require.NoError(t, db.Create(&logs).Error)
	for _, prefix := range []string{"", "logs."} {
		t.Run("prefix="+prefix, func(t *testing.T) {
			var ids []int
			for offset := 0; offset < len(logs); offset += 2 {
				var page []Log
				require.NoError(t, db.Table("logs").Order(clickHouseLogOrder(prefix)).Offset(offset).Limit(2).Find(&page).Error)
				for _, entry := range page {
					ids = append(ids, entry.Id)
				}
			}
			assert.Equal(t, []int{logs[1].Id, logs[3].Id, logs[2].Id, logs[0].Id}, ids)
		})
	}
}

func TestBuildLogLikeConditionUsesStandardEscape(t *testing.T) {
	originalLogDatabaseType := common.LogDatabaseType()
	t.Cleanup(func() {
		common.SetLogDatabaseType(originalLogDatabaseType)
	})
	common.SetLogDatabaseType(common.DatabaseTypeSQLite)

	condition, pattern, err := buildLogLikeCondition("logs.model_name", "gpt_4%")

	require.NoError(t, err)
	assert.Equal(t, "logs.model_name LIKE ? ESCAPE '!'", condition)
	assert.Equal(t, "gpt!_4%", pattern)
}

func TestBuildLogLikeConditionUsesClickHouseEscaping(t *testing.T) {
	originalLogDatabaseType := common.LogDatabaseType()
	t.Cleanup(func() {
		common.SetLogDatabaseType(originalLogDatabaseType)
	})
	common.SetLogDatabaseType(common.DatabaseTypeClickHouse)

	condition, pattern, err := buildLogLikeCondition("logs.model_name", `gpt_4\mini%`)

	require.NoError(t, err)
	assert.Equal(t, "logs.model_name LIKE ?", condition)
	assert.Equal(t, `gpt\_4\\mini%`, pattern)
}

func TestEnsureLogRequestId(t *testing.T) {
	empty := &Log{}
	ensureLogRequestId(empty)
	assert.NotEmpty(t, empty.RequestId, "empty request id should be backfilled")

	existing := &Log{RequestId: "fixed-request-id"}
	ensureLogRequestId(existing)
	assert.Equal(t, "fixed-request-id", existing.RequestId, "existing request id must be preserved")

	assert.NotPanics(t, func() { ensureLogRequestId(nil) })
}

func TestAssignDisplayLogIds(t *testing.T) {
	logs := []*Log{{}, {}, {}}

	assignDisplayLogIds(logs, 0)
	assert.Equal(t, []int{1, 2, 3}, []int{logs[0].Id, logs[1].Id, logs[2].Id})

	assignDisplayLogIds(logs, 20)
	assert.Equal(t, []int{21, 22, 23}, []int{logs[0].Id, logs[1].Id, logs[2].Id})

	assert.NotPanics(t, func() { assignDisplayLogIds(nil, 0) })
}
