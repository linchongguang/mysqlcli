package query

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Result struct {
	Columns      []string
	Rows         [][]string
	RowsAffected int64
	LastInsertID int64
	Duration     time.Duration
	HasRows      bool
}

func Execute(ctx context.Context, db *sql.DB, statement string, args ...any) (Result, error) {
	startedAt := time.Now()
	if !IsQuery(statement) {
		execResult, err := db.ExecContext(ctx, statement, args...)
		if err != nil {
			return Result{}, fmt.Errorf("执行 SQL: %w", err)
		}
		rowsAffected, _ := execResult.RowsAffected()
		lastInsertID, _ := execResult.LastInsertId()
		return Result{
			RowsAffected: rowsAffected,
			LastInsertID: lastInsertID,
			Duration:     time.Since(startedAt),
		}, nil
	}

	rows, err := db.QueryContext(ctx, statement, args...)
	if err != nil {
		return Result{}, fmt.Errorf("执行 SQL: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return Result{}, fmt.Errorf("读取结果列: %w", err)
	}
	result := Result{Columns: columns, HasRows: len(columns) > 0}

	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for index := range values {
			pointers[index] = &values[index]
		}
		if err := rows.Scan(pointers...); err != nil {
			return Result{}, fmt.Errorf("读取结果行: %w", err)
		}
		row := make([]string, len(columns))
		for index, value := range values {
			row[index] = stringify(value)
		}
		result.Rows = append(result.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return Result{}, fmt.Errorf("遍历结果: %w", err)
	}
	result.Duration = time.Since(startedAt)
	result.RowsAffected = int64(len(result.Rows))
	return result, nil
}

func stringify(value any) string {
	switch typedValue := value.(type) {
	case nil:
		return "NULL"
	case []byte:
		return string(typedValue)
	case time.Time:
		return typedValue.Format("2006-01-02 15:04:05.999999")
	default:
		return fmt.Sprint(typedValue)
	}
}

func IsQuery(statement string) bool {
	fields := strings.Fields(strings.TrimSpace(statement))
	if len(fields) == 0 {
		return false
	}
	switch strings.ToUpper(fields[0]) {
	case "SELECT", "SHOW", "DESC", "DESCRIBE", "EXPLAIN", "WITH", "CALL", "TABLE", "VALUES":
		return true
	default:
		return false
	}
}
