package testutil

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"sync"
	"sync/atomic"
)

type DatabaseOptions struct {
	Columns       []string
	Rows          [][]driver.Value
	RowsAffected  int64
	LastInsertID  int64
	WaitCancel    bool
	QueryHandlers []QueryHandler
	ExecHandlers  []ExecHandler
	Calls         *Calls
}

type QueryHandler struct {
	SQL     string
	Args    []driver.Value
	Columns []string
	Rows    [][]driver.Value
}

type ExecHandler struct {
	SQL          string
	Args         []driver.Value
	RowsAffected int64
	LastInsertID int64
}

type Calls struct {
	mu      sync.Mutex
	Queries []Call
	Execs   []Call
}

type Call struct {
	SQL  string
	Args []driver.Value
}

func (c *Calls) Snapshot() (queries []Call, execs []Call) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Call(nil), c.Queries...), append([]Call(nil), c.Execs...)
}

var driverSequence atomic.Uint64

func NewDatabase(options DatabaseOptions) *sql.DB {
	if options.Calls == nil {
		options.Calls = &Calls{}
	}
	driverName := "mysqlcli-test-" + stringID(driverSequence.Add(1))
	sql.Register(driverName, fakeDriver{options: options})
	db, err := sql.Open(driverName, "")
	if err != nil {
		panic(err)
	}
	return db
}

type fakeDriver struct {
	options DatabaseOptions
}

func (d fakeDriver) Open(string) (driver.Conn, error) {
	return &fakeConnection{options: d.options}, nil
}

type fakeConnection struct {
	options DatabaseOptions
}

func (c *fakeConnection) Prepare(string) (driver.Stmt, error) {
	return nil, driver.ErrSkip
}

func (c *fakeConnection) Close() error { return nil }

func (c *fakeConnection) Begin() (driver.Tx, error) { return nil, driver.ErrSkip }

func (c *fakeConnection) Ping(context.Context) error { return nil }

func (c *fakeConnection) QueryContext(ctx context.Context, statement string, args []driver.NamedValue) (driver.Rows, error) {
	if c.options.WaitCancel {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	values := namedValues(args)
	c.options.Calls.mu.Lock()
	c.options.Calls.Queries = append(c.options.Calls.Queries, Call{SQL: statement, Args: append([]driver.Value(nil), values...)})
	c.options.Calls.mu.Unlock()
	for _, handler := range c.options.QueryHandlers {
		if handler.matches(statement, values) {
			return &fakeRows{columns: handler.Columns, rows: handler.Rows}, nil
		}
	}
	return &fakeRows{columns: c.options.Columns, rows: c.options.Rows}, nil
}

func (c *fakeConnection) ExecContext(ctx context.Context, statement string, args []driver.NamedValue) (driver.Result, error) {
	if c.options.WaitCancel {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	values := namedValues(args)
	c.options.Calls.mu.Lock()
	c.options.Calls.Execs = append(c.options.Calls.Execs, Call{SQL: statement, Args: append([]driver.Value(nil), values...)})
	c.options.Calls.mu.Unlock()
	for _, handler := range c.options.ExecHandlers {
		if handler.matches(statement, values) {
			return fakeResult{rowsAffected: handler.RowsAffected, lastInsertID: handler.LastInsertID}, nil
		}
	}
	return fakeResult{rowsAffected: c.options.RowsAffected, lastInsertID: c.options.LastInsertID}, nil
}

func namedValues(args []driver.NamedValue) []driver.Value {
	values := make([]driver.Value, len(args))
	for index, arg := range args {
		values[index] = arg.Value
	}
	return values
}

func (h QueryHandler) matches(statement string, args []driver.Value) bool {
	return matches(h.SQL, h.Args, statement, args)
}

func (h ExecHandler) matches(statement string, args []driver.Value) bool {
	return matches(h.SQL, h.Args, statement, args)
}

func matches(expectedSQL string, expectedArgs []driver.Value, actualSQL string, actualArgs []driver.Value) bool {
	if expectedSQL != "" && expectedSQL != actualSQL {
		return false
	}
	if expectedArgs == nil {
		return true
	}
	if len(expectedArgs) != len(actualArgs) {
		return false
	}
	for index := range expectedArgs {
		if expectedArgs[index] != actualArgs[index] {
			return false
		}
	}
	return true
}

type fakeResult struct {
	rowsAffected int64
	lastInsertID int64
}

func (r fakeResult) LastInsertId() (int64, error) { return r.lastInsertID, nil }

func (r fakeResult) RowsAffected() (int64, error) { return r.rowsAffected, nil }

type fakeRows struct {
	columns []string
	rows    [][]driver.Value
	index   int
}

func (r *fakeRows) Columns() []string { return r.columns }

func (r *fakeRows) Close() error { return nil }

func (r *fakeRows) Next(destination []driver.Value) error {
	if r.index >= len(r.rows) {
		return io.EOF
	}
	copy(destination, r.rows[r.index])
	r.index++
	return nil
}

func stringID(value uint64) string {
	if value == 0 {
		return "0"
	}
	buffer := make([]byte, 0, 20)
	for value > 0 {
		buffer = append(buffer, byte('0'+value%10))
		value /= 10
	}
	for left, right := 0, len(buffer)-1; left < right; left, right = left+1, right-1 {
		buffer[left], buffer[right] = buffer[right], buffer[left]
	}
	return string(buffer)
}
