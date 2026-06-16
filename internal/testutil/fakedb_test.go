package testutil

import (
	"context"
	"database/sql/driver"
	"testing"
)

func TestNewDatabaseRecordsQueryArguments(t *testing.T) {
	calls := &Calls{}
	db := NewDatabase(DatabaseOptions{
		Columns: []string{"id"},
		Rows:    [][]driver.Value{{int64(1)}},
		Calls:   calls,
	})
	defer db.Close()

	if _, err := db.QueryContext(context.Background(), "SELECT ?", "value"); err != nil {
		t.Fatalf("QueryContext() 返回错误: %v", err)
	}
	queries, _ := calls.Snapshot()
	if len(queries) != 1 || len(queries[0].Args) != 1 || queries[0].Args[0] != "value" {
		t.Fatalf("查询记录 = %#v", queries)
	}
}

func TestNewDatabaseUsesMatchingHandler(t *testing.T) {
	db := NewDatabase(DatabaseOptions{
		QueryHandlers: []QueryHandler{
			{
				SQL:     "SELECT ?",
				Args:    []driver.Value{"matched"},
				Columns: []string{"value"},
				Rows:    [][]driver.Value{{"ok"}},
			},
		},
	})
	defer db.Close()

	rows, err := db.QueryContext(context.Background(), "SELECT ?", "matched")
	if err != nil {
		t.Fatalf("QueryContext() 返回错误: %v", err)
	}
	defer rows.Close()

	var value string
	if !rows.Next() {
		t.Fatal("期望有一行结果")
	}
	if err := rows.Scan(&value); err != nil {
		t.Fatalf("Scan() 返回错误: %v", err)
	}
	if value != "ok" {
		t.Fatalf("value = %q", value)
	}
}
