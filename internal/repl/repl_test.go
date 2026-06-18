package repl

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/linchongguang/mysqlcli/internal/database"
	"github.com/linchongguang/mysqlcli/internal/output"
	"github.com/linchongguang/mysqlcli/internal/testutil"
)

type replClient struct {
	db       *sql.DB
	database string
}

func (c *replClient) DB() *sql.DB { return c.db }

func (c *replClient) CurrentDatabase() string { return c.database }

func (c *replClient) UseDatabase(_ context.Context, databaseName string) error {
	c.database = databaseName
	return nil
}

func (c *replClient) Reconnect(context.Context) error { return nil }

func (c *replClient) SessionInfo(context.Context) (database.SessionInfo, error) {
	return database.SessionInfo{ConnectionID: 1, Version: "8.0.test", Database: c.database}, nil
}

func TestREPLExecutesStatementAndQuits(t *testing.T) {
	db := testutil.NewDatabase(testutil.DatabaseOptions{
		Columns: []string{"value"},
		Rows:    [][]driver.Value{{int64(1)}},
	})
	defer db.Close()
	client := &replClient{db: db, database: "demo"}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	renderer := output.NewRenderer(&stdout, output.Options{Silent: true})
	repl := New(client, renderer, strings.NewReader("SELECT 1;\n\\q\n"), &stdout, &stderr, false, "", false, nil)

	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("Run() 返回错误: %v", err)
	}
	if !strings.Contains(stdout.String(), "value") || !strings.Contains(stdout.String(), "1") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestREPLUseStatementUpdatesCurrentDatabase(t *testing.T) {
	db := testutil.NewDatabase(testutil.DatabaseOptions{})
	defer db.Close()
	client := &replClient{db: db}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	repl := New(client, output.NewRenderer(&stdout, output.Options{}), strings.NewReader("USE testdb;\n\\q\n"), &stdout, &stderr, false, "", false, nil)

	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("Run() 返回错误: %v", err)
	}
	if client.CurrentDatabase() != "testdb" {
		t.Fatalf("CurrentDatabase() = %q", client.CurrentDatabase())
	}
	if !strings.Contains(stdout.String(), "Database changed to testdb") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestParseUseStatement(t *testing.T) {
	tests := []struct {
		statement string
		want      string
		ok        bool
	}{
		{statement: "USE testdb", want: "testdb", ok: true},
		{statement: "use `test-db`", want: "test-db", ok: true},
		{statement: "USE `test``db`", want: "test`db", ok: true},
		{statement: "USEFUL testdb", ok: false},
		{statement: "USE", ok: false},
		{statement: "USE `testdb` trailing", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.statement, func(t *testing.T) {
			got, ok := parseUseStatement(tt.statement)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("parseUseStatement(%q) = %q, %v", tt.statement, got, ok)
			}
		})
	}
}

func TestREPLSourceFile(t *testing.T) {
	sqlFile := filepath.Join(t.TempDir(), "script.sql")
	if err := os.WriteFile(sqlFile, []byte("SELECT 1;\n"), 0600); err != nil {
		t.Fatalf("写入 SQL 文件: %v", err)
	}
	db := testutil.NewDatabase(testutil.DatabaseOptions{
		Columns: []string{"value"},
		Rows:    [][]driver.Value{{int64(1)}},
	})
	defer db.Close()
	client := &replClient{db: db, database: "demo"}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	repl := New(client, output.NewRenderer(&stdout, output.Options{Silent: true}), strings.NewReader("\\i "+sqlFile+"\n\\q\n"), &stdout, &stderr, false, "", false, nil)
	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("Run() 返回错误: %v", err)
	}
	if !strings.Contains(stdout.String(), "value") || !strings.Contains(stdout.String(), "1") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestREPLAutoWarnings(t *testing.T) {
	db := testutil.NewDatabase(testutil.DatabaseOptions{
		QueryHandlers: []testutil.QueryHandler{
			{
				SQL:     "SELECT 1",
				Columns: []string{"value"},
				Rows:    [][]driver.Value{{int64(1)}},
			},
			{
				SQL:     "SHOW WARNINGS",
				Columns: []string{"Level", "Code", "Message"},
				Rows:    [][]driver.Value{{"Warning", int64(1265), "Data truncated"}},
			},
		},
	})
	defer db.Close()
	client := &replClient{db: db, database: "demo"}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	repl := New(client, output.NewRenderer(&stdout, output.Options{Silent: true}), strings.NewReader("\\W\nSELECT 1;\n\\q\n"), &stdout, &stderr, false, "", false, nil)
	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("Run() 返回错误: %v", err)
	}
	if !strings.Contains(stdout.String(), "Data truncated") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestREPLShowPrintsCurrentBuffer(t *testing.T) {
	db := testutil.NewDatabase(testutil.DatabaseOptions{})
	defer db.Close()
	client := &replClient{db: db, database: "demo"}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	repl := New(client, output.NewRenderer(&stdout, output.Options{Silent: true}), strings.NewReader("SELECT 1\n\\show\n\\c\n\\q\n"), &stdout, &stderr, false, "", false, nil)
	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("Run() 返回错误: %v", err)
	}
	if !strings.Contains(stdout.String(), "SELECT 1") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestREPLEditorExecutesEditedStatement(t *testing.T) {
	editorScript := filepath.Join(t.TempDir(), "editor.sh")
	if err := os.WriteFile(editorScript, []byte("#!/bin/sh\nprintf 'SELECT 1;\\n' > \"$1\"\n"), 0700); err != nil {
		t.Fatalf("写入编辑器脚本: %v", err)
	}
	t.Setenv("EDITOR", editorScript)

	db := testutil.NewDatabase(testutil.DatabaseOptions{
		Columns: []string{"value"},
		Rows:    [][]driver.Value{{int64(1)}},
	})
	defer db.Close()
	client := &replClient{db: db, database: "demo"}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	repl := New(client, output.NewRenderer(&stdout, output.Options{Silent: true}), strings.NewReader("\\e\n\\q\n"), &stdout, &stderr, false, "", false, nil)
	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("Run() 返回错误: %v", err)
	}
	if !strings.Contains(stdout.String(), "value") || !strings.Contains(stdout.String(), "1") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestREPLReportsIncompleteStatement(t *testing.T) {
	db := testutil.NewDatabase(testutil.DatabaseOptions{})
	defer db.Close()
	repl := New(&replClient{db: db}, output.NewRenderer(&bytes.Buffer{}, output.Options{}), strings.NewReader("SELECT 1\n"), &bytes.Buffer{}, &bytes.Buffer{}, false, "", false, nil)
	if err := repl.Run(context.Background()); err == nil || !strings.Contains(err.Error(), "未完成") {
		t.Fatalf("Run() 错误 = %v", err)
	}
}

func TestRunCancelableCancelsOnInterrupt(t *testing.T) {
	db := testutil.NewDatabase(testutil.DatabaseOptions{WaitCancel: true})
	defer db.Close()
	var stderr bytes.Buffer
	repl := New(&replClient{db: db}, output.NewRenderer(&bytes.Buffer{}, output.Options{}), strings.NewReader(""), &bytes.Buffer{}, &stderr, false, "", false, nil)
	interrupts := make(chan os.Signal, 1)
	interrupts <- os.Interrupt

	err := repl.runCancelable(context.Background(), interrupts, func(ctx context.Context) error {
		_, queryErr := db.QueryContext(ctx, "SELECT SLEEP(10)")
		return queryErr
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runCancelable() 错误 = %v", err)
	}
	if !strings.Contains(stderr.String(), "正在取消") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunCancelableCompletesNormally(t *testing.T) {
	repl := New(&replClient{}, output.NewRenderer(&bytes.Buffer{}, output.Options{}), strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, false, "", false, nil)
	interrupts := make(chan os.Signal, 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := repl.runCancelable(ctx, interrupts, func(context.Context) error { return nil }); err != nil {
		t.Fatalf("runCancelable() 返回错误: %v", err)
	}
}
