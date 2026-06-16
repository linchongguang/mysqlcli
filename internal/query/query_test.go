package query

import (
	"context"
	"database/sql/driver"
	"errors"
	"testing"

	"github.com/linchongguang/mysqlcli/internal/testutil"
)

func TestExecuteQuery(t *testing.T) {
	db := testutil.NewDatabase(testutil.DatabaseOptions{
		Columns: []string{"id", "name", "optional"},
		Rows:    [][]driver.Value{{int64(1), []byte("mysql"), nil}},
	})
	defer db.Close()

	result, err := Execute(context.Background(), db, "SELECT id, name, optional FROM demo")
	if err != nil {
		t.Fatalf("Execute() 返回错误: %v", err)
	}
	if len(result.Rows) != 1 || result.Rows[0][1] != "mysql" || result.Rows[0][2] != "NULL" {
		t.Fatalf("Execute() = %+v", result)
	}
}

func TestExecuteStatement(t *testing.T) {
	db := testutil.NewDatabase(testutil.DatabaseOptions{RowsAffected: 3})
	defer db.Close()

	result, err := Execute(context.Background(), db, "UPDATE demo SET active = 1")
	if err != nil {
		t.Fatalf("Execute() 返回错误: %v", err)
	}
	if result.RowsAffected != 3 || result.HasRows {
		t.Fatalf("Execute() = %+v", result)
	}
}

func TestExecuteCancellation(t *testing.T) {
	db := testutil.NewDatabase(testutil.DatabaseOptions{WaitCancel: true})
	defer db.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Execute(ctx, db, "SELECT SLEEP(10)")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() 错误 = %v", err)
	}
}

func TestIsQuery(t *testing.T) {
	for _, statement := range []string{"SELECT 1", "SHOW DATABASES", "WITH cte AS (SELECT 1) SELECT * FROM cte"} {
		if !IsQuery(statement) {
			t.Errorf("IsQuery(%q) = false", statement)
		}
	}
	if IsQuery("DELETE FROM demo") {
		t.Fatal("DELETE 不应识别为结果集查询")
	}
}
