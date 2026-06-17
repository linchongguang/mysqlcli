package metacommand

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"

	"github.com/linchongguang/mysqlcli/internal/config"
	"github.com/linchongguang/mysqlcli/internal/database"
	"github.com/linchongguang/mysqlcli/internal/output"
	"github.com/linchongguang/mysqlcli/internal/testutil"
)

type fakeClient struct {
	db             *sql.DB
	reconnectCalls int
}

func TestRegistryCustomCommandOverridesBuiltin(t *testing.T) {
	calls := &testutil.Calls{}
	db := testutil.NewDatabase(testutil.DatabaseOptions{
		Columns: []string{"TableName", "TotalMB"},
		Rows:    [][]driver.Value{{"orders", float64(12.5)}},
		Calls:   calls,
	})
	defer db.Close()

	customCommands := map[string]config.CustomCommand{
		"du": {
			Name: "du",
			SQL:  "SELECT TABLE_NAME AS TableName, 12.5 AS TotalMB FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?",
		},
	}
	var destination bytes.Buffer
	registry := NewRegistry(&fakeClient{db: db}, output.NewRenderer(&destination, output.Options{Silent: true}), &destination, customCommands)
	if _, err := registry.Execute(context.Background(), `\du orders`); err != nil {
		t.Fatalf("Execute() 返回错误: %v", err)
	}
	if !strings.Contains(destination.String(), "orders") || !strings.Contains(destination.String(), "12.5") {
		t.Fatalf("输出 = %q", destination.String())
	}
	queries, _ := calls.Snapshot()
	if len(queries) != 1 || len(queries[0].Args) != 1 || queries[0].Args[0] != "orders" {
		t.Fatalf("查询记录 = %#v", queries)
	}
}

func TestRegistryDuWithoutArgsKeepsBuiltinWhenCustomDuExists(t *testing.T) {
	calls := &testutil.Calls{}
	db := testutil.NewDatabase(testutil.DatabaseOptions{
		QueryHandlers: []testutil.QueryHandler{
			{
				SQL:     userList8SQL,
				Args:    []driver.Value{"%"},
				Columns: []string{"UserName", "HostName"},
				Rows:    [][]driver.Value{{"root", "localhost"}},
			},
		},
		Calls: calls,
	})
	defer db.Close()
	customCommands := map[string]config.CustomCommand{
		"du": {
			Name: "du",
			SQL:  "SELECT ?",
		},
	}
	var destination bytes.Buffer
	registry := NewRegistry(&fakeClient{db: db}, output.NewRenderer(&destination, output.Options{Silent: true}), &destination, customCommands)
	if _, err := registry.Execute(context.Background(), `\du`); err != nil {
		t.Fatalf("Execute() 返回错误: %v", err)
	}
	queries, _ := calls.Snapshot()
	if len(queries) != 1 || queries[0].SQL != userList8SQL {
		t.Fatalf("查询记录 = %#v", queries)
	}
	if !strings.Contains(destination.String(), "root") {
		t.Fatalf("输出 = %q", destination.String())
	}
}

func TestRegistryCustomCommandValidatesArguments(t *testing.T) {
	db := testutil.NewDatabase(testutil.DatabaseOptions{})
	defer db.Close()
	registry := NewRegistry(&fakeClient{db: db}, output.NewRenderer(&bytes.Buffer{}, output.Options{}), &bytes.Buffer{}, map[string]config.CustomCommand{
		"dx": {Name: "dx", SQL: "SELECT ?"},
	})
	_, err := registry.Execute(context.Background(), `\dx`)
	if err == nil || !strings.Contains(err.Error(), "需要 1 个参数") {
		t.Fatalf("错误 = %v", err)
	}
}

func TestRegistryHelpShowsCustomCommands(t *testing.T) {
	var destination bytes.Buffer
	registry := NewRegistry(&fakeClient{}, output.NewRenderer(&destination, output.Options{}), &destination, map[string]config.CustomCommand{
		"du": {Name: "du", SQL: "SELECT ?", Description: "查看表空间"},
	})
	if _, err := registry.Execute(context.Background(), `\?`); err != nil {
		t.Fatalf("Execute() 返回错误: %v", err)
	}
	if !strings.Contains(destination.String(), `\du`) || !strings.Contains(destination.String(), "查看表空间") {
		t.Fatalf("帮助输出 = %q", destination.String())
	}
}

func TestRegistryCommandHelp(t *testing.T) {
	var destination bytes.Buffer
	registry := NewRegistry(&fakeClient{}, output.NewRenderer(&destination, output.Options{}), &destination, nil)
	if _, err := registry.Execute(context.Background(), `\h locks`); err != nil {
		t.Fatalf("Execute() 返回错误: %v", err)
	}
	if !strings.Contains(destination.String(), `\locks [--all|--tree]`) {
		t.Fatalf("帮助输出 = %q", destination.String())
	}
}

func TestRegistryCommandHelpSessions(t *testing.T) {
	var destination bytes.Buffer
	registry := NewRegistry(&fakeClient{}, output.NewRenderer(&destination, output.Options{}), &destination, nil)
	if _, err := registry.Execute(context.Background(), `\h sessions`); err != nil {
		t.Fatalf("Execute() 返回错误: %v", err)
	}
	if !strings.Contains(destination.String(), `\sessions [--all]`) || !strings.Contains(destination.String(), "--min-seconds") {
		t.Fatalf("帮助输出 = %q", destination.String())
	}
}

func TestCountPlaceholdersIgnoresQuotedQuestionMarks(t *testing.T) {
	statement := "SELECT '?', `?`, col FROM t WHERE a = ? AND b = ? -- ?\n/* ? */"
	if got := countPlaceholders(statement); got != 2 {
		t.Fatalf("countPlaceholders() = %d, 期望 2", got)
	}
}

func TestRegistryDDLQuotesIdentifier(t *testing.T) {
	calls := &testutil.Calls{}
	db := testutil.NewDatabase(testutil.DatabaseOptions{
		Columns: []string{"Table", "Create Table"},
		Rows:    [][]driver.Value{{"weird`table", "CREATE TABLE `weird``table` (...)"}},
		Calls:   calls,
	})
	defer db.Close()
	var destination bytes.Buffer
	registry := NewRegistry(&fakeClient{db: db}, output.NewRenderer(&destination, output.Options{}), &destination, nil)
	if _, err := registry.Execute(context.Background(), "\\ddl weird`table"); err != nil {
		t.Fatalf("Execute() 返回错误: %v", err)
	}
	queries, _ := calls.Snapshot()
	if len(queries) != 1 || queries[0].SQL != "SHOW CREATE TABLE `weird``table`" {
		t.Fatalf("查询记录 = %#v", queries)
	}
}

func TestRegistryDescribeTableIncludesMetadataIndexesAndForeignKeys(t *testing.T) {
	db := testutil.NewDatabase(testutil.DatabaseOptions{
		QueryHandlers: []testutil.QueryHandler{
			{
				SQL:     tableInfoSQL,
				Args:    []driver.Value{"orders"},
				Columns: []string{"TableName", "Engine", "TableComment"},
				Rows:    [][]driver.Value{{"orders", "InnoDB", "订单表"}},
			},
			{
				SQL:     describeObjectSQL,
				Args:    []driver.Value{"orders"},
				Columns: []string{"Field", "Type"},
				Rows:    [][]driver.Value{{"id", "bigint"}},
			},
			{
				SQL:     indexListSQL,
				Args:    []driver.Value{"orders"},
				Columns: []string{"IndexName", "ColumnName"},
				Rows:    [][]driver.Value{{"PRIMARY", "id"}},
			},
			{
				SQL:     foreignKeySQL,
				Args:    []driver.Value{"orders"},
				Columns: []string{"ConstraintName", "ReferencedTable"},
				Rows:    [][]driver.Value{{"fk_orders_user", "users"}},
			},
		},
	})
	defer db.Close()
	var destination bytes.Buffer
	registry := NewRegistry(&fakeClient{db: db}, output.NewRenderer(&destination, output.Options{}), &destination, nil)
	if _, err := registry.Execute(context.Background(), `\desc orders`); err != nil {
		t.Fatalf("Execute() 返回错误: %v", err)
	}
	outputText := destination.String()
	for _, want := range []string{"Table:", "Columns:", "Indexes:", "Foreign Keys:", "订单表", "PRIMARY", "fk_orders_user"} {
		if !strings.Contains(outputText, want) {
			t.Fatalf("输出缺少 %q: %q", want, outputText)
		}
	}
}

func (c *fakeClient) DB() *sql.DB { return c.db }

func (c *fakeClient) UseDatabase(context.Context, string) error { return nil }

func (c *fakeClient) Reconnect(context.Context) error {
	c.reconnectCalls++
	return nil
}

func (c *fakeClient) SessionInfo(context.Context) (database.SessionInfo, error) {
	return database.SessionInfo{ConnectionID: 7, Version: "8.0.test", CurrentUser: "root@localhost"}, nil
}

func TestRegistryReconnect(t *testing.T) {
	client := &fakeClient{db: testutil.NewDatabase(testutil.DatabaseOptions{})}
	defer client.db.Close()
	var destination bytes.Buffer
	registry := NewRegistry(client, output.NewRenderer(&destination, output.Options{Silent: true}), &destination, nil)

	result, err := registry.Execute(context.Background(), `\reconnect`)
	if err != nil {
		t.Fatalf("Execute() 返回错误: %v", err)
	}
	if !result.Handled || client.reconnectCalls != 1 || !strings.Contains(destination.String(), "Reconnected") {
		t.Fatalf("重连结果异常: result=%+v calls=%d output=%q", result, client.reconnectCalls, destination.String())
	}
}

func TestRegistryDescMissingTableReturnsError(t *testing.T) {
	db := testutil.NewDatabase(testutil.DatabaseOptions{
		QueryHandlers: []testutil.QueryHandler{
			{
				SQL:     tableInfoSQL,
				Args:    []driver.Value{"not_exist"},
				Columns: []string{"TableName"},
				Rows:    nil,
			},
			{
				SQL:     objectListSQL,
				Args:    []driver.Value{"not_exist"},
				Columns: []string{"ObjectName"},
				Rows:    nil,
			},
		},
	})
	defer db.Close()
	registry := NewRegistry(&fakeClient{db: db}, output.NewRenderer(&bytes.Buffer{}, output.Options{}), &bytes.Buffer{}, nil)
	_, err := registry.Execute(context.Background(), `\desc not_exist`)
	if err == nil || !strings.Contains(err.Error(), "表不存在: not_exist") {
		t.Fatalf("错误 = %v", err)
	}
}

func TestRegistryDPatternKeepsSearchBehavior(t *testing.T) {
	db := testutil.NewDatabase(testutil.DatabaseOptions{
		QueryHandlers: []testutil.QueryHandler{
			{
				SQL:     tableInfoSQL,
				Args:    []driver.Value{"not_exist"},
				Columns: []string{"TableName"},
				Rows:    nil,
			},
			{
				SQL:     objectListSQL,
				Args:    []driver.Value{"not_exist"},
				Columns: []string{"ObjectName"},
				Rows:    nil,
			},
		},
	})
	defer db.Close()
	var destination bytes.Buffer
	registry := NewRegistry(&fakeClient{db: db}, output.NewRenderer(&destination, output.Options{}), &destination, nil)
	if _, err := registry.Execute(context.Background(), `\d not_exist`); err != nil {
		t.Fatalf("Execute() 返回错误: %v", err)
	}
	if !strings.Contains(destination.String(), "ObjectName") {
		t.Fatalf("输出 = %q", destination.String())
	}
}

func TestRegistrySourceFileCommand(t *testing.T) {
	registry := NewRegistry(&fakeClient{}, output.NewRenderer(&bytes.Buffer{}, output.Options{}), &bytes.Buffer{}, nil)
	result, err := registry.Execute(context.Background(), `\i /tmp/schema.sql`)
	if err != nil {
		t.Fatalf("Execute() 返回错误: %v", err)
	}
	if !result.Handled || result.SourceFile != "/tmp/schema.sql" {
		t.Fatalf("结果 = %+v", result)
	}
}

func TestRegistryVariables(t *testing.T) {
	calls := &testutil.Calls{}
	db := testutil.NewDatabase(testutil.DatabaseOptions{
		Columns: []string{"VariableName", "VariableValue"},
		Rows:    [][]driver.Value{{"innodb_buffer_pool_size", "134217728"}},
		Calls:   calls,
	})
	defer db.Close()
	var destination bytes.Buffer
	registry := NewRegistry(&fakeClient{db: db}, output.NewRenderer(&destination, output.Options{Silent: true}), &destination, nil)
	if _, err := registry.Execute(context.Background(), `\variables --session innodb`); err != nil {
		t.Fatalf("Execute() 返回错误: %v", err)
	}
	queries, _ := calls.Snapshot()
	if len(queries) != 1 || !strings.Contains(queries[0].SQL, "session_variables") || queries[0].Args[0] != "%innodb%" {
		t.Fatalf("查询记录 = %#v", queries)
	}
}

func TestRegistryWarningsToggle(t *testing.T) {
	var destination bytes.Buffer
	registry := NewRegistry(&fakeClient{}, output.NewRenderer(&destination, output.Options{}), &destination, nil)
	if registry.AutoWarnings() {
		t.Fatal("初始 AutoWarnings 应为 false")
	}
	if _, err := registry.Execute(context.Background(), `\W`); err != nil {
		t.Fatalf("Execute() 返回错误: %v", err)
	}
	if !registry.AutoWarnings() || !strings.Contains(destination.String(), "on") {
		t.Fatalf("AutoWarnings = %v, 输出 = %q", registry.AutoWarnings(), destination.String())
	}
}

func TestRegistryEnhancedStatus(t *testing.T) {
	db := testutil.NewDatabase(testutil.DatabaseOptions{
		Columns: []string{"UptimeSeconds", "Questions", "SlowQueries", "QuestionsPerSecond"},
		Rows:    [][]driver.Value{{"100", "250", "2", "2.5"}},
	})
	defer db.Close()
	var destination bytes.Buffer
	registry := NewRegistry(&fakeClient{db: db}, output.NewRenderer(&destination, output.Options{Silent: true}), &destination, nil)
	if _, err := registry.Execute(context.Background(), `\status`); err != nil {
		t.Fatalf("Execute() 返回错误: %v", err)
	}
	outputText := destination.String()
	if !strings.Contains(outputText, "Connection id: 7") || !strings.Contains(outputText, "UptimeSeconds") || !strings.Contains(outputText, "250") {
		t.Fatalf("输出 = %q", outputText)
	}
}

func TestRegistryDBADiagnosticCommands(t *testing.T) {
	tests := []struct {
		name           string
		command        string
		columns        []string
		rows           [][]driver.Value
		wantSQL        string
		wantArg        driver.Value
		wantOutputText string
	}{
		{
			name:           "tableinfo",
			command:        `\tableinfo orders`,
			columns:        []string{"TableName", "Engine", "TotalMB"},
			rows:           [][]driver.Value{{"orders", "InnoDB", "42.00"}},
			wantSQL:        tableInfoSQL,
			wantArg:        "orders",
			wantOutputText: "orders",
		},
		{
			name:           "tablesize",
			command:        `\tablesize log`,
			columns:        []string{"TableName", "TotalMB"},
			rows:           [][]driver.Value{{"app_log", "1024.00"}},
			wantSQL:        tableSizeSQL,
			wantArg:        "%log%",
			wantOutputText: "app_log",
		},
		{
			name:           "charset",
			command:        `\charset`,
			columns:        []string{"VariableName", "VariableValue"},
			rows:           [][]driver.Value{{"character_set_server", "utf8mb4"}},
			wantSQL:        charsetSQL,
			wantOutputText: "utf8mb4",
		},
		{
			name:           "binlog",
			command:        `\binlog`,
			columns:        []string{"Log_name", "File_size"},
			rows:           [][]driver.Value{{"mysql-bin.000001", int64(1234)}},
			wantSQL:        "SHOW BINARY LOGS",
			wantOutputText: "mysql-bin.000001",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := &testutil.Calls{}
			db := testutil.NewDatabase(testutil.DatabaseOptions{
				Columns: tt.columns,
				Rows:    tt.rows,
				Calls:   calls,
			})
			defer db.Close()
			var destination bytes.Buffer
			registry := NewRegistry(&fakeClient{db: db}, output.NewRenderer(&destination, output.Options{Silent: true}), &destination, nil)
			if _, err := registry.Execute(context.Background(), tt.command); err != nil {
				t.Fatalf("Execute() 返回错误: %v", err)
			}
			queries, _ := calls.Snapshot()
			if len(queries) != 1 || queries[0].SQL != tt.wantSQL {
				t.Fatalf("查询记录 = %#v", queries)
			}
			if tt.wantArg != nil && (len(queries[0].Args) != 1 || queries[0].Args[0] != tt.wantArg) {
				t.Fatalf("查询参数 = %#v", queries[0].Args)
			}
			if !strings.Contains(destination.String(), tt.wantOutputText) {
				t.Fatalf("输出 = %q", destination.String())
			}
		})
	}
}

func TestRegistrySlowLog(t *testing.T) {
	calls := &testutil.Calls{}
	db := testutil.NewDatabase(testutil.DatabaseOptions{
		QueryHandlers: []testutil.QueryHandler{
			{
				SQL:     slowLogVariablesSQL,
				Columns: []string{"VariableName", "VariableValue"},
				Rows:    [][]driver.Value{{"slow_query_log", "ON"}},
			},
			{
				SQL:     slowLogRecentSQL,
				Args:    []driver.Value{int64(3)},
				Columns: []string{"StartTime", "SqlText"},
				Rows:    [][]driver.Value{{"2026-06-16 10:00:00", "SELECT SLEEP(1)"}},
			},
		},
		Calls: calls,
	})
	defer db.Close()
	var destination bytes.Buffer
	registry := NewRegistry(&fakeClient{db: db}, output.NewRenderer(&destination, output.Options{Silent: true}), &destination, nil)
	if _, err := registry.Execute(context.Background(), `\slowlog 3`); err != nil {
		t.Fatalf("Execute() 返回错误: %v", err)
	}
	queries, _ := calls.Snapshot()
	if len(queries) != 2 || queries[0].SQL != slowLogVariablesSQL || queries[1].SQL != slowLogRecentSQL {
		t.Fatalf("查询记录 = %#v", queries)
	}
	outputText := destination.String()
	if !strings.Contains(outputText, "slow_query_log") || !strings.Contains(outputText, "SELECT SLEEP(1)") {
		t.Fatalf("输出 = %q", outputText)
	}
}

func TestRegistryInnoDBAndDeadlocks(t *testing.T) {
	statusText := `=====================================
2026-06-16 10:00:00 INNODB MONITOR OUTPUT
=====================================
SEMAPHORES
----------
OS WAIT ARRAY INFO: reservation count 2, signal count 3
Mutex spin waits 4, rounds 5, OS waits 6
------------
TRANSACTIONS
------------
History list length 9
------------------------
LATEST DETECTED DEADLOCK
------------------------
*** (1) TRANSACTION:
TRANSACTION 123, ACTIVE 1 sec
*** WE ROLL BACK TRANSACTION (1)
------------
BUFFER POOL AND MEMORY
----------------------
Buffer pool hit rate 999 / 1000
--------------
ROW OPERATIONS
--------------
0 queries inside InnoDB, 0 queries in queue
Number of rows inserted 1, updated 2, deleted 3, read 4`

	db := testutil.NewDatabase(testutil.DatabaseOptions{
		Columns: []string{"Type", "Name", "Status"},
		Rows:    [][]driver.Value{{"InnoDB", "", statusText}},
	})
	defer db.Close()
	var destination bytes.Buffer
	registry := NewRegistry(&fakeClient{db: db}, output.NewRenderer(&destination, output.Options{Silent: true}), &destination, nil)
	if _, err := registry.Execute(context.Background(), `\innodb`); err != nil {
		t.Fatalf("Execute(innodb) 返回错误: %v", err)
	}
	if _, err := registry.Execute(context.Background(), `\deadlocks`); err != nil {
		t.Fatalf("Execute(deadlocks) 返回错误: %v", err)
	}
	outputText := destination.String()
	for _, want := range []string{"HistoryListLength", "BufferPoolHitRate", "SemaphoreSpinWaits", "LATEST DETECTED DEADLOCK", "TRANSACTION 123"} {
		if !strings.Contains(outputText, want) {
			t.Fatalf("输出缺少 %q: %q", want, outputText)
		}
	}
}

func TestRegistryLocksRejectsUnknownOption(t *testing.T) {
	registry := NewRegistry(&fakeClient{}, output.NewRenderer(&bytes.Buffer{}, output.Options{}), &bytes.Buffer{}, nil)
	_, err := registry.Execute(context.Background(), `\locks --unknown`)
	if err == nil || !strings.Contains(err.Error(), "未知 locks 参数") {
		t.Fatalf("错误 = %v", err)
	}
}

func TestRegistryLocksUsesMySQL8Metadata(t *testing.T) {
	calls := &testutil.Calls{}
	db := testutil.NewDatabase(testutil.DatabaseOptions{
		QueryHandlers: []testutil.QueryHandler{
			{
				SQL:     lockWaits8CapabilitySQL,
				Columns: []string{"TableCount"},
				Rows:    [][]driver.Value{{int64(2)}},
			},
			{
				SQL:     locks8SQL,
				Columns: []string{"WaitingTransaction", "BlockingTransaction"},
				Rows:    [][]driver.Value{{"trx1", "trx2"}},
			},
		},
		Calls: calls,
	})
	defer db.Close()
	var destination bytes.Buffer
	registry := NewRegistry(&fakeClient{db: db}, output.NewRenderer(&destination, output.Options{Silent: true}), &destination, nil)
	if _, err := registry.Execute(context.Background(), `\locks`); err != nil {
		t.Fatalf("Execute() 返回错误: %v", err)
	}
	queries, _ := calls.Snapshot()
	if len(queries) != 2 || queries[0].SQL != lockWaits8CapabilitySQL || queries[1].SQL != locks8SQL {
		t.Fatalf("查询记录 = %#v", queries)
	}
	if strings.Contains(destination.String(), "trx1") == false {
		t.Fatalf("输出 = %q", destination.String())
	}
}

func TestRegistryLocksDoesNotFallbackFromMySQL8PermissionError(t *testing.T) {
	db := testutil.NewDatabase(testutil.DatabaseOptions{
		QueryHandlers: []testutil.QueryHandler{
			{
				SQL:     lockWaits8CapabilitySQL,
				Columns: []string{"TableCount"},
				Rows:    [][]driver.Value{{int64(2)}},
			},
			{
				SQL: locks8SQL,
				Err: errors.New("SELECT command denied"),
			},
			{
				SQL:     locks57SQL,
				Columns: []string{"WaitingTransaction"},
				Rows:    [][]driver.Value{{"should-not-run"}},
			},
		},
	})
	defer db.Close()
	registry := NewRegistry(&fakeClient{db: db}, output.NewRenderer(&bytes.Buffer{}, output.Options{}), &bytes.Buffer{}, nil)
	_, err := registry.Execute(context.Background(), `\locks`)
	if err == nil || !strings.Contains(err.Error(), "MySQL 8 锁信息失败") {
		t.Fatalf("错误 = %v", err)
	}
}

func TestRegistryLocksFallsBackToMySQL57Metadata(t *testing.T) {
	calls := &testutil.Calls{}
	db := testutil.NewDatabase(testutil.DatabaseOptions{
		QueryHandlers: []testutil.QueryHandler{
			{
				SQL:     lockWaits8CapabilitySQL,
				Columns: []string{"TableCount"},
				Rows:    [][]driver.Value{{int64(0)}},
			},
			{
				SQL:     lockWaits57CapabilitySQL,
				Columns: []string{"TableCount"},
				Rows:    [][]driver.Value{{int64(2)}},
			},
			{
				SQL:     locks57SQL,
				Columns: []string{"WaitingTransaction", "BlockingTransaction"},
				Rows:    [][]driver.Value{{"trx57a", "trx57b"}},
			},
		},
		Calls: calls,
	})
	defer db.Close()
	var destination bytes.Buffer
	registry := NewRegistry(&fakeClient{db: db}, output.NewRenderer(&destination, output.Options{Silent: true}), &destination, nil)
	if _, err := registry.Execute(context.Background(), `\locks`); err != nil {
		t.Fatalf("Execute() 返回错误: %v", err)
	}
	queries, _ := calls.Snapshot()
	if len(queries) != 3 || queries[0].SQL != lockWaits8CapabilitySQL || queries[1].SQL != lockWaits57CapabilitySQL || queries[2].SQL != locks57SQL {
		t.Fatalf("查询记录 = %#v", queries)
	}
	if !strings.Contains(destination.String(), "trx57a") {
		t.Fatalf("输出 = %q", destination.String())
	}
}

func TestRegistryToggleVertical(t *testing.T) {
	var destination bytes.Buffer
	renderer := output.NewRenderer(&destination, output.Options{})
	registry := NewRegistry(&fakeClient{}, renderer, &destination, nil)
	if _, err := registry.Execute(context.Background(), `\x on`); err != nil {
		t.Fatalf("Execute() 返回错误: %v", err)
	}
	if !renderer.Vertical() {
		t.Fatal("纵向输出应已启用")
	}
}

func TestRegistryPager(t *testing.T) {
	var destination bytes.Buffer
	renderer := output.NewRenderer(&destination, output.Options{})
	registry := NewRegistry(&fakeClient{}, renderer, &destination, nil)

	if _, err := registry.Execute(context.Background(), `\pager less -S`); err != nil {
		t.Fatalf("Execute() 返回错误: %v", err)
	}
	if renderer.Pager() != "less -S" {
		t.Fatalf("Pager() = %q", renderer.Pager())
	}
	if _, err := registry.Execute(context.Background(), `\pager off`); err != nil {
		t.Fatalf("Execute() 返回错误: %v", err)
	}
	if renderer.Pager() != "" {
		t.Fatalf("Pager() = %q", renderer.Pager())
	}
}

func TestRegistryReplicationChannels(t *testing.T) {
	db := testutil.NewDatabase(testutil.DatabaseOptions{
		Columns: []string{"ChannelName", "SourceHost", "SourcePort"},
		Rows:    [][]driver.Value{{"channel_1", "source.local", int64(3306)}},
	})
	defer db.Close()

	var destination bytes.Buffer
	registry := NewRegistry(&fakeClient{db: db}, output.NewRenderer(&destination, output.Options{Silent: true}), &destination, nil)
	if _, err := registry.Execute(context.Background(), `\repl channels`); err != nil {
		t.Fatalf("Execute() 返回错误: %v", err)
	}
	if !strings.Contains(destination.String(), "channel_1") || !strings.Contains(destination.String(), "source.local") {
		t.Fatalf("输出 = %q", destination.String())
	}
}

func TestRegistrySessionsPassesArguments(t *testing.T) {
	calls := &testutil.Calls{}
	db := testutil.NewDatabase(testutil.DatabaseOptions{
		Columns: []string{"ConnectionId"},
		Rows:    [][]driver.Value{{int64(10)}},
		Calls:   calls,
	})
	defer db.Close()

	var destination bytes.Buffer
	registry := NewRegistry(&fakeClient{db: db}, output.NewRenderer(&destination, output.Options{Silent: true}), &destination, nil)
	if _, err := registry.Execute(context.Background(), `\sessions --all --min-seconds 5 --user app`); err != nil {
		t.Fatalf("Execute() 返回错误: %v", err)
	}
	queries, _ := calls.Snapshot()
	if len(queries) != 1 {
		t.Fatalf("查询调用次数 = %d", len(queries))
	}
	wantArgs := []driver.Value{true, int64(5), "app", "app"}
	if len(queries[0].Args) != len(wantArgs) {
		t.Fatalf("参数 = %#v", queries[0].Args)
	}
	for index := range wantArgs {
		if queries[0].Args[index] != wantArgs[index] {
			t.Fatalf("参数[%d] = %#v, 期望 %#v", index, queries[0].Args[index], wantArgs[index])
		}
	}
}
