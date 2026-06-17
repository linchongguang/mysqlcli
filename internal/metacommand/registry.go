package metacommand

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/linchongguang/mysqlcli/internal/config"
	"github.com/linchongguang/mysqlcli/internal/database"
	"github.com/linchongguang/mysqlcli/internal/output"
	"github.com/linchongguang/mysqlcli/internal/query"
)

type Result struct {
	Handled    bool
	Quit       bool
	SourceFile string
}

type Client interface {
	DB() *sql.DB
	UseDatabase(context.Context, string) error
	Reconnect(context.Context) error
	SessionInfo(context.Context) (database.SessionInfo, error)
}

type Registry struct {
	client         Client
	renderer       *output.Renderer
	writer         io.Writer
	customCommands map[string]config.CustomCommand
	autoWarnings   bool
}

func NewRegistry(client Client, renderer *output.Renderer, writer io.Writer, customCommands map[string]config.CustomCommand) *Registry {
	return &Registry{client: client, renderer: renderer, writer: writer, customCommands: customCommands}
}

func (r *Registry) Execute(ctx context.Context, line string) (Result, error) {
	command, err := Parse(line)
	if err != nil {
		return Result{Handled: true}, err
	}
	if command.Name == "du" && len(command.Args) == 0 {
		return r.users(ctx, command.Args)
	}
	if customCommand, ok := r.customCommands[command.Name]; ok {
		return r.custom(ctx, customCommand, command.Args)
	}

	switch command.Name {
	case "q", "quit", "exit":
		return Result{Handled: true, Quit: true}, nil
	case "?", "h", "help":
		if len(command.Args) == 0 {
			r.printHelp()
		} else if len(command.Args) == 1 {
			r.printCommandHelp(command.Args[0])
		} else {
			return Result{Handled: true}, fmt.Errorf("用法: \\h [command]")
		}
		return Result{Handled: true}, nil
	case "i", ".":
		if len(command.Args) != 1 {
			return Result{Handled: true}, fmt.Errorf("用法: \\%s <filename>", command.Name)
		}
		return Result{Handled: true, SourceFile: command.Args[0]}, nil
	case "l":
		return r.runQuery(ctx, databaseListSQL, patternArg(command.Args))
	case "connect", "use":
		if len(command.Args) != 1 {
			return Result{Handled: true}, fmt.Errorf("用法: \\%s <database>", command.Name)
		}
		if err := r.client.UseDatabase(ctx, command.Args[0]); err != nil {
			return Result{Handled: true}, err
		}
		fmt.Fprintf(r.writer, "Database changed to %s\n", command.Args[0])
		return Result{Handled: true}, nil
	case "d", "desc", "describe":
		return r.describe(ctx, command.Args)
	case "dt":
		return r.runQuery(ctx, tableListSQL, patternArg(command.Args))
	case "dv":
		return r.runQuery(ctx, viewListSQL, patternArg(command.Args))
	case "di":
		return r.runQuery(ctx, indexListSQL, patternArg(command.Args))
	case "size", "table-size", "tablesize":
		return r.runQuery(ctx, tableSizeSQL, patternArg(command.Args))
	case "tableinfo":
		if len(command.Args) != 1 {
			return Result{Handled: true}, fmt.Errorf("用法: \\tableinfo <table>")
		}
		return r.runQuery(ctx, tableInfoSQL, command.Args[0])
	case "ddl":
		return r.ddl(ctx, command.Args)
	case "df":
		return r.runQuery(ctx, routineListSQL, patternArg(command.Args))
	case "triggers":
		return r.runQuery(ctx, triggerListSQL, patternArg(command.Args))
	case "events":
		return r.runQuery(ctx, eventListSQL, patternArg(command.Args))
	case "sessions", "ps":
		return r.sessions(ctx, command.Args)
	case "locks":
		return r.locks(ctx, command.Args)
	case "repl":
		return r.replication(ctx, command.Args)
	case "variables", "vars":
		return r.variables(ctx, command.Args)
	case "warnings":
		if len(command.Args) != 0 {
			return Result{Handled: true}, fmt.Errorf("用法: \\warnings")
		}
		return r.runQuery(ctx, "SHOW WARNINGS")
	case "charset":
		if len(command.Args) != 0 {
			return Result{Handled: true}, fmt.Errorf("用法: \\charset")
		}
		return r.runQuery(ctx, charsetSQL)
	case "binlog":
		if len(command.Args) != 0 {
			return Result{Handled: true}, fmt.Errorf("用法: \\binlog")
		}
		return r.runQuery(ctx, "SHOW BINARY LOGS")
	case "slowlog":
		return r.slowLog(ctx, command.Args)
	case "innodb":
		if len(command.Args) != 0 {
			return Result{Handled: true}, fmt.Errorf("用法: \\innodb")
		}
		return r.innodbStatus(ctx)
	case "deadlocks":
		if len(command.Args) != 0 {
			return Result{Handled: true}, fmt.Errorf("用法: \\deadlocks")
		}
		return r.deadlocks(ctx)
	case "w":
		if len(command.Args) != 0 {
			return Result{Handled: true}, fmt.Errorf("用法: \\W")
		}
		r.autoWarnings = !r.autoWarnings
		fmt.Fprintf(r.writer, "Show warnings is %s.\n", onOff(r.autoWarnings))
		return Result{Handled: true}, nil
	case "du":
		return r.users(ctx, command.Args)
	case "user":
		return r.user(ctx, command.Args)
	case "grants":
		return r.grants(ctx, command.Args)
	case "roles":
		return r.roles(ctx, command.Args)
	case "whoami":
		return r.whoAmI(ctx)
	case "privileges":
		return r.runQuery(ctx, "SHOW PRIVILEGES")
	case "kill":
		return r.kill(ctx, command.Args)
	case "x":
		return r.toggleVertical(command.Args)
	case "timing":
		return r.toggleTiming(command.Args)
	case "pager":
		return r.pager(command.Args)
	case "status", "s":
		return r.status(ctx)
	case "reconnect":
		if len(command.Args) != 0 {
			return Result{Handled: true}, fmt.Errorf("用法: \\reconnect")
		}
		if err := r.client.Reconnect(ctx); err != nil {
			return Result{Handled: true}, fmt.Errorf("重新连接失败: %w", err)
		}
		fmt.Fprintln(r.writer, "Reconnected.")
		return r.status(ctx)
	default:
		return Result{Handled: true}, fmt.Errorf("未知快捷命令: \\%s，使用 \\? 查看帮助", command.Name)
	}
}

func (r *Registry) AutoWarnings() bool {
	return r.autoWarnings
}

func (r *Registry) Warnings(ctx context.Context) (query.Result, error) {
	return query.Execute(ctx, r.client.DB(), "SHOW WARNINGS")
}

func (r *Registry) custom(ctx context.Context, customCommand config.CustomCommand, args []string) (Result, error) {
	if strings.TrimSpace(customCommand.SQL) == "" {
		return Result{Handled: true}, fmt.Errorf("自定义命令 \\%s 未配置 sql", customCommand.Name)
	}
	placeholderCount := countPlaceholders(customCommand.SQL)
	if len(args) != placeholderCount {
		return Result{Handled: true}, fmt.Errorf("自定义命令 \\%s 需要 %d 个参数，实际 %d 个", customCommand.Name, placeholderCount, len(args))
	}
	queryArgs := make([]any, len(args))
	for index, arg := range args {
		queryArgs[index] = arg
	}
	return r.runQuery(ctx, customCommand.SQL, queryArgs...)
}

func countPlaceholders(statement string) int {
	count := 0
	var quote rune
	inLineComment := false
	inBlockComment := false
	escaped := false
	runes := []rune(statement)
	for index := 0; index < len(runes); index++ {
		char := runes[index]
		next := rune(0)
		if index+1 < len(runes) {
			next = runes[index+1]
		}
		if inLineComment {
			if char == '\n' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			if char == '*' && next == '/' {
				inBlockComment = false
				index++
			}
			continue
		}
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if char == '\\' {
				escaped = true
				continue
			}
			if char == quote {
				if next == quote {
					index++
					continue
				}
				quote = 0
			}
			continue
		}
		if char == '#' || (char == '-' && next == '-' && (index+2 >= len(runes) || runes[index+2] == ' ')) {
			inLineComment = true
			continue
		}
		if char == '/' && next == '*' {
			inBlockComment = true
			index++
			continue
		}
		if char == '\'' || char == '"' || char == '`' {
			quote = char
			continue
		}
		if char == '?' {
			count++
		}
	}
	return count
}

func (r *Registry) runQuery(ctx context.Context, statement string, args ...any) (Result, error) {
	queryResult, err := query.Execute(ctx, r.client.DB(), statement, args...)
	if err != nil {
		return Result{Handled: true}, err
	}
	return Result{Handled: true}, r.renderer.Render(queryResult)
}

func (r *Registry) describe(ctx context.Context, args []string) (Result, error) {
	if len(args) == 0 {
		return r.runQuery(ctx, objectListSQL, "%")
	}
	if len(args) != 1 {
		return Result{Handled: true}, fmt.Errorf("用法: \\d [table|pattern]")
	}
	tableName := args[0]
	tableInfo, err := query.Execute(ctx, r.client.DB(), tableInfoSQL, tableName)
	if err != nil {
		return Result{Handled: true}, err
	}
	if len(tableInfo.Rows) == 0 {
		return r.runQuery(ctx, objectListSQL, patternArg(args))
	}
	if err := r.renderSection("Table", tableInfo); err != nil {
		return Result{Handled: true}, err
	}
	columns, err := query.Execute(ctx, r.client.DB(), describeObjectSQL, tableName)
	if err != nil {
		return Result{Handled: true}, err
	}
	if err := r.renderSection("Columns", columns); err != nil {
		return Result{Handled: true}, err
	}
	indexes, err := query.Execute(ctx, r.client.DB(), indexListSQL, tableName)
	if err != nil {
		return Result{Handled: true}, err
	}
	if err := r.renderSection("Indexes", indexes); err != nil {
		return Result{Handled: true}, err
	}
	foreignKeys, err := query.Execute(ctx, r.client.DB(), foreignKeySQL, tableName)
	if err != nil {
		return Result{Handled: true}, err
	}
	return Result{Handled: true}, r.renderSection("Foreign Keys", foreignKeys)
}

func (r *Registry) renderSection(title string, result query.Result) error {
	fmt.Fprintf(r.writer, "\n%s:\n", title)
	return r.renderer.Render(result)
}

func (r *Registry) ddl(ctx context.Context, args []string) (Result, error) {
	if len(args) != 1 {
		return Result{Handled: true}, fmt.Errorf("用法: \\ddl <table>")
	}
	return r.runQuery(ctx, "SHOW CREATE TABLE "+quoteIdentifierPath(args[0]))
}

func (r *Registry) sessions(ctx context.Context, args []string) (Result, error) {
	showAll := false
	minSeconds := 0
	userName := ""
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--all":
			showAll = true
		case "--min-seconds":
			if index+1 >= len(args) {
				return Result{Handled: true}, fmt.Errorf("--min-seconds 缺少数值")
			}
			index++
			value, err := strconv.Atoi(args[index])
			if err != nil || value < 0 {
				return Result{Handled: true}, fmt.Errorf("--min-seconds 必须是非负整数")
			}
			minSeconds = value
		case "--user":
			if index+1 >= len(args) {
				return Result{Handled: true}, fmt.Errorf("--user 缺少用户名")
			}
			index++
			userName = args[index]
		default:
			return Result{Handled: true}, fmt.Errorf("未知 sessions 参数: %s", args[index])
		}
	}
	return r.runQuery(ctx, sessionsSQL, showAll, minSeconds, userName, userName)
}

func (r *Registry) variables(ctx context.Context, args []string) (Result, error) {
	scope := "global"
	var pattern string
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--global":
			scope = "global"
		case "--session":
			scope = "session"
		default:
			if pattern != "" {
				return Result{Handled: true}, fmt.Errorf("用法: \\variables [--global|--session] [pattern]")
			}
			pattern = args[index]
		}
	}
	statement := globalVariablesSQL
	if scope == "session" {
		statement = sessionVariablesSQL
	}
	return r.runQuery(ctx, statement, patternArg([]string{pattern}))
}

func (r *Registry) slowLog(ctx context.Context, args []string) (Result, error) {
	limit := 0
	if len(args) > 1 {
		return Result{Handled: true}, fmt.Errorf("用法: \\slowlog [N]")
	}
	if len(args) == 1 {
		value, err := strconv.Atoi(args[0])
		if err != nil || value <= 0 {
			return Result{Handled: true}, fmt.Errorf("\\slowlog 的 N 必须是正整数")
		}
		limit = value
	}
	variables, err := query.Execute(ctx, r.client.DB(), slowLogVariablesSQL)
	if err != nil {
		return Result{Handled: true}, fmt.Errorf("读取慢日志配置失败，可能缺少 performance_schema 权限: %w", err)
	}
	if err := r.renderer.Render(variables); err != nil {
		return Result{Handled: true}, err
	}
	if limit == 0 {
		return Result{Handled: true}, nil
	}
	recent, err := query.Execute(ctx, r.client.DB(), slowLogRecentSQL, limit)
	if err != nil {
		return Result{Handled: true}, fmt.Errorf("读取 mysql.slow_log 失败，可能未使用 TABLE 输出或权限不足: %w", err)
	}
	return Result{Handled: true}, r.renderer.Render(recent)
}

func (r *Registry) innodbStatus(ctx context.Context) (Result, error) {
	statusText, err := r.showInnoDBStatus(ctx)
	if err != nil {
		return Result{Handled: true}, err
	}
	return Result{Handled: true}, r.renderer.Render(parseInnoDBSummary(statusText))
}

func (r *Registry) deadlocks(ctx context.Context) (Result, error) {
	statusText, err := r.showInnoDBStatus(ctx)
	if err != nil {
		return Result{Handled: true}, err
	}
	deadlockText := latestDeadlock(statusText)
	if deadlockText == "" {
		deadlockText = "未发现最近死锁信息"
	}
	result := query.Result{
		Columns: []string{"LatestDeadlock"},
		Rows:    [][]string{{deadlockText}},
		HasRows: true,
	}
	return Result{Handled: true}, r.renderer.Render(result)
}

func (r *Registry) showInnoDBStatus(ctx context.Context) (string, error) {
	result, err := query.Execute(ctx, r.client.DB(), "SHOW ENGINE INNODB STATUS")
	if err != nil {
		return "", fmt.Errorf("读取 InnoDB 状态失败，可能缺少 PROCESS 权限: %w", err)
	}
	if len(result.Rows) == 0 {
		return "", fmt.Errorf("InnoDB 状态为空")
	}
	statusIndex := len(result.Columns) - 1
	for index, column := range result.Columns {
		if strings.EqualFold(column, "Status") {
			statusIndex = index
			break
		}
	}
	if statusIndex < 0 || statusIndex >= len(result.Rows[0]) {
		return "", fmt.Errorf("InnoDB 状态结果缺少 Status 列")
	}
	return result.Rows[0][statusIndex], nil
}

func (r *Registry) locks(ctx context.Context, args []string) (Result, error) {
	mode := "waits"
	if len(args) > 1 {
		return Result{Handled: true}, fmt.Errorf("用法: \\locks [--all|--tree]")
	}
	if len(args) == 1 {
		switch args[0] {
		case "--all":
			mode = "all"
		case "--tree":
			mode = "tree"
		default:
			return Result{Handled: true}, fmt.Errorf("未知 locks 参数: %s", args[0])
		}
	}
	query8 := locks8SQL
	query57 := locks57SQL
	capability8SQL := lockWaits8CapabilitySQL
	capability57SQL := lockWaits57CapabilitySQL
	required8Tables := 2
	required57Tables := 2
	if mode == "all" {
		query8 = allLocks8SQL
		query57 = allLocks57SQL
		capability8SQL = allLocks8CapabilitySQL
		capability57SQL = allLocks57CapabilitySQL
		required8Tables = 1
		required57Tables = 1
	} else if mode == "tree" {
		query8 = lockTree8SQL
		query57 = lockTree57SQL
	}

	if r.hasMetadataTables(ctx, capability8SQL, required8Tables) {
		result, err := query.Execute(ctx, r.client.DB(), query8)
		if err != nil {
			return Result{Handled: true}, fmt.Errorf("读取 MySQL 8 锁信息失败，可能缺少 performance_schema.data_locks/data_lock_waits 权限: %w", err)
		}
		return Result{Handled: true}, r.renderer.Render(result)
	}

	if r.hasMetadataTables(ctx, capability57SQL, required57Tables) {
		result, err := query.Execute(ctx, r.client.DB(), query57)
		if err != nil {
			return Result{Handled: true}, fmt.Errorf("读取 MySQL 5.7 锁信息失败，可能缺少 information_schema InnoDB 锁表权限: %w", err)
		}
		return Result{Handled: true}, r.renderer.Render(result)
	}

	return Result{Handled: true}, fmt.Errorf("当前服务器未提供可用锁元数据表：MySQL 8 需要 performance_schema.data_locks/data_lock_waits，MySQL 5.7 需要 information_schema.INNODB_LOCKS/INNODB_LOCK_WAITS")
}

func (r *Registry) hasMetadataTables(ctx context.Context, statement string, requiredCount int) bool {
	result, err := query.Execute(ctx, r.client.DB(), statement)
	if err != nil || len(result.Rows) == 0 || len(result.Rows[0]) == 0 {
		return false
	}
	count, err := strconv.Atoi(result.Rows[0][0])
	return err == nil && count >= requiredCount
}

func (r *Registry) replication(ctx context.Context, args []string) (Result, error) {
	if len(args) > 1 {
		return Result{Handled: true}, fmt.Errorf("用法: \\repl [status|channels|errors|source]")
	}
	mode := "status"
	if len(args) == 1 {
		mode = strings.ToLower(args[0])
	}
	if mode == "source" {
		return r.runQuery(ctx, "SHOW MASTER STATUS")
	}
	if mode != "status" && mode != "channels" && mode != "errors" {
		return Result{Handled: true}, fmt.Errorf("未知复制子命令: %s", mode)
	}
	if mode == "channels" {
		result, err := query.Execute(ctx, r.client.DB(), replicationChannelsSQL)
		if err == nil {
			return Result{Handled: true}, r.renderer.Render(result)
		}
		return r.replicationStatusFallback(ctx, []string{"Channel_Name", "Master_Host", "Master_Port", "Source_Host", "Source_Port", "Auto_Position"})
	}
	return r.replicationStatus(ctx, mode)
}

func (r *Registry) replicationStatus(ctx context.Context, mode string) (Result, error) {
	result, err := query.Execute(ctx, r.client.DB(), "SHOW REPLICA STATUS")
	if err != nil {
		result, err = query.Execute(ctx, r.client.DB(), "SHOW SLAVE STATUS")
	}
	if err != nil {
		return Result{Handled: true}, fmt.Errorf("读取复制状态失败，可能未配置复制或权限不足: %w", err)
	}
	if mode == "errors" {
		result = filterColumns(result, []string{"Channel_Name", "Last_IO_Error", "Last_SQL_Error", "Last_Error"})
	}
	return Result{Handled: true}, r.renderer.Render(result)
}

func (r *Registry) replicationStatusFallback(ctx context.Context, columns []string) (Result, error) {
	result, err := query.Execute(ctx, r.client.DB(), "SHOW REPLICA STATUS")
	if err != nil {
		result, err = query.Execute(ctx, r.client.DB(), "SHOW SLAVE STATUS")
	}
	if err != nil {
		return Result{Handled: true}, fmt.Errorf("读取复制通道失败，可能未配置复制或权限不足: %w", err)
	}
	return Result{Handled: true}, r.renderer.Render(filterColumns(result, columns))
}

func (r *Registry) user(ctx context.Context, args []string) (Result, error) {
	if len(args) != 1 {
		return Result{Handled: true}, fmt.Errorf("用法: \\user <user@host>")
	}
	userName, hostName, err := parseAccount(args[0])
	if err != nil {
		return Result{Handled: true}, err
	}
	result, queryErr := query.Execute(ctx, r.client.DB(), userDetail8SQL, userName, hostName)
	if queryErr != nil {
		result, queryErr = query.Execute(ctx, r.client.DB(), userDetail57SQL, userName, hostName)
	}
	if queryErr != nil {
		return Result{Handled: true}, fmt.Errorf("读取用户详情失败，可能缺少 mysql.user 权限: %w", queryErr)
	}
	return Result{Handled: true}, r.renderer.Render(result)
}

func (r *Registry) users(ctx context.Context, args []string) (Result, error) {
	pattern := patternArg(args)
	result, err := query.Execute(ctx, r.client.DB(), userList8SQL, pattern)
	if err != nil {
		result, err = query.Execute(ctx, r.client.DB(), userList57SQL, pattern)
	}
	if err != nil {
		return Result{Handled: true}, fmt.Errorf("读取用户列表失败，可能缺少 mysql.user 权限: %w", err)
	}
	return Result{Handled: true}, r.renderer.Render(result)
}

func (r *Registry) grants(ctx context.Context, args []string) (Result, error) {
	statement := "SHOW GRANTS"
	if len(args) > 1 {
		return Result{Handled: true}, fmt.Errorf("用法: \\grants [user@host]")
	}
	if len(args) == 1 {
		userName, hostName, err := parseAccount(args[0])
		if err != nil {
			return Result{Handled: true}, err
		}
		statement = "SHOW GRANTS FOR " + quoteString(userName) + "@" + quoteString(hostName)
	}
	return r.runQuery(ctx, statement)
}

func (r *Registry) roles(ctx context.Context, args []string) (Result, error) {
	if len(args) == 0 {
		return r.runQuery(ctx, currentRolesSQL)
	}
	if len(args) != 1 {
		return Result{Handled: true}, fmt.Errorf("用法: \\roles [user@host]")
	}
	userName, hostName, err := parseAccount(args[0])
	if err != nil {
		return Result{Handled: true}, err
	}
	result, err := query.Execute(ctx, r.client.DB(), roleEdgesSQL, userName, hostName)
	if err != nil {
		return Result{Handled: true}, fmt.Errorf("当前服务器可能不支持角色或权限不足: %w", err)
	}
	return Result{Handled: true}, r.renderer.Render(result)
}

func (r *Registry) whoAmI(ctx context.Context) (Result, error) {
	result, err := query.Execute(ctx, r.client.DB(), whoAmI8SQL)
	if err != nil {
		result, err = query.Execute(ctx, r.client.DB(), whoAmI57SQL)
	}
	if err != nil {
		return Result{Handled: true}, err
	}
	return Result{Handled: true}, r.renderer.Render(result)
}

func (r *Registry) kill(ctx context.Context, args []string) (Result, error) {
	connectionMode := false
	if len(args) == 2 && args[0] == "--connection" {
		connectionMode = true
		args = args[1:]
	}
	if len(args) != 1 {
		return Result{Handled: true}, fmt.Errorf("用法: \\kill [--connection] <connectionId>")
	}
	connectionID, err := strconv.ParseUint(args[0], 10, 64)
	if err != nil || connectionID == 0 {
		return Result{Handled: true}, fmt.Errorf("connectionId 必须是正整数")
	}
	statement := "KILL QUERY " + strconv.FormatUint(connectionID, 10)
	if connectionMode {
		statement = "KILL CONNECTION " + strconv.FormatUint(connectionID, 10)
	}
	return r.runQuery(ctx, statement)
}

func (r *Registry) toggleVertical(args []string) (Result, error) {
	enabled, err := toggleValue(args, r.renderer.Vertical())
	if err != nil {
		return Result{Handled: true}, err
	}
	r.renderer.SetVertical(enabled)
	fmt.Fprintf(r.writer, "Expanded display is %s.\n", onOff(enabled))
	return Result{Handled: true}, nil
}

func (r *Registry) toggleTiming(args []string) (Result, error) {
	enabled, err := toggleValue(args, r.renderer.Timing())
	if err != nil {
		return Result{Handled: true}, err
	}
	r.renderer.SetTiming(enabled)
	fmt.Fprintf(r.writer, "Timing is %s.\n", onOff(enabled))
	return Result{Handled: true}, nil
}

func (r *Registry) pager(args []string) (Result, error) {
	if len(args) == 0 {
		currentPager := r.renderer.Pager()
		if currentPager == "" {
			fmt.Fprintln(r.writer, "Pager is off.")
		} else {
			fmt.Fprintf(r.writer, "Pager is %s.\n", currentPager)
		}
		return Result{Handled: true}, nil
	}
	commandText := strings.Join(args, " ")
	if strings.EqualFold(commandText, "off") {
		r.renderer.SetPager("")
		fmt.Fprintln(r.writer, "Pager is off.")
		return Result{Handled: true}, nil
	}
	r.renderer.SetPager(commandText)
	fmt.Fprintf(r.writer, "Pager is %s.\n", commandText)
	return Result{Handled: true}, nil
}

func (r *Registry) status(ctx context.Context) (Result, error) {
	info, err := r.client.SessionInfo(ctx)
	if err != nil {
		return Result{Handled: true}, err
	}
	fmt.Fprintf(r.writer, "Connection id: %d\nServer version: %s\nCurrent user: %s\nCurrent database: %s\n", info.ConnectionID, info.Version, info.CurrentUser, emptyDefault(info.Database, "(none)"))
	summary, err := query.Execute(ctx, r.client.DB(), statusSummarySQL)
	if err != nil {
		fmt.Fprintf(r.writer, "Status summary unavailable: %v\n", err)
		return Result{Handled: true}, nil
	}
	return Result{Handled: true}, r.renderer.Render(summary)
}

func (r *Registry) printHelp() {
	fmt.Fprintln(r.writer, `mysqlcli 快捷命令:
  \l [pattern]              列出数据库
  \i FILE | \. FILE          执行 SQL 文件
  \d [object]               列出、搜索或描述对象
  \desc TABLE | \describe TABLE  描述表
  \ddl TABLE                查看建表 DDL
  \dt \dv \di \df          查看表、视图、索引和例程
  \size [table]            查看表空间大小
  \tableinfo TABLE          查看表元数据详情
  \tablesize [pattern]      按空间占用排序查看表
  \connect DB | \use DB     切换数据库
  \sessions [--all]         查看活跃会话
  \locks [--all|--tree]     查看锁等待、全部锁或阻塞树
  \repl [status|channels|errors|source]  查看复制状态
  \variables [--session] [pattern]  查看服务器变量
  \warnings                 查看最近 SQL 告警
  \W                        切换自动显示告警
  \innodb                   查看 InnoDB 状态摘要
  \slowlog [N]              查看慢日志配置和最近慢查询
  \binlog                   查看二进制日志摘要
  \deadlocks                查看最近死锁信息
  \charset                  查看字符集和排序规则
  \du [pattern]             查看用户
  \user USER@HOST           查看用户详情
  \grants [USER@HOST]       查看授权
  \roles [USER@HOST]        查看角色
  \whoami                   查看当前身份
  \privileges               查看权限类型
  \kill [--connection] ID   终止查询或连接
  \x [on|off]               切换纵向显示
  \timing [on|off]          切换耗时显示
  \pager [command|off]      设置或关闭分页器
  \status                   查看连接状态
  \reconnect                重新建立连接
  \c                        清空输入缓冲区
  \p | \show                打印输入缓冲区
  \e | \edit                使用外部编辑器编辑缓冲区
  \q                        退出`)
	if len(r.customCommands) == 0 {
		return
	}
	names := make([]string, 0, len(r.customCommands))
	for name := range r.customCommands {
		names = append(names, name)
	}
	sort.Strings(names)
	fmt.Fprintln(r.writer, "\n自定义快捷命令:")
	for _, name := range names {
		description := r.customCommands[name].Description
		if description == "" {
			description = "执行自定义 SQL"
		}
		fmt.Fprintf(r.writer, "  \\%-24s %s\n", name, description)
	}
}

func (r *Registry) printCommandHelp(name string) {
	name = strings.TrimPrefix(strings.ToLower(name), "\\")
	help := map[string]string{
		"d": `\d [table|pattern]
  不带参数时列出当前库对象。
  参数精确命中表名时展示表信息、字段、索引和外键。
  参数未精确命中时按包含匹配搜索对象。
  示例: \d orders`,
		"desc":     `\desc TABLE：\d TABLE 的别名。示例: \desc orders`,
		"describe": `\describe TABLE：\d TABLE 的别名。示例: \describe orders`,
		"ddl":      `\ddl TABLE：显示 SHOW CREATE TABLE 输出。示例: \ddl orders`,
		"locks": `\locks [--all|--tree]
  查看当前锁等待、全部 InnoDB 锁或阻塞树。
  MySQL 8 使用 performance_schema.data_locks/data_lock_waits。
  MySQL 5.7 使用 information_schema.INNODB_LOCKS/INNODB_LOCK_WAITS。`,
		"du":        `\du [pattern]：查看用户列表。注意: \du 无参数始终保留内置用户列表入口。`,
		"variables": `\variables [--global|--session] [pattern]：查看服务器变量。示例: \variables innodb`,
		"slowlog":   `\slowlog [N]：查看慢日志配置；带 N 时查看 mysql.slow_log 最近 N 条。`,
		"tableinfo": `\tableinfo TABLE：查看表引擎、行格式、空间、自增、排序规则、注释等元数据。`,
		"tablesize": `\tablesize [pattern]：按数据和索引空间占用排序查看表。`,
	}
	if text, ok := help[name]; ok {
		fmt.Fprintln(r.writer, text)
		return
	}
	fmt.Fprintf(r.writer, "未找到 \\%s 的单命令帮助，使用 \\? 查看完整命令列表。\n", name)
}

func patternArg(args []string) string {
	if len(args) == 0 || args[0] == "" {
		return "%"
	}
	if strings.ContainsAny(args[0], "%_") {
		return args[0]
	}
	return "%" + args[0] + "%"
}

func parseAccount(value string) (string, string, error) {
	parts := strings.SplitN(value, "@", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("账户必须使用 user@host 格式")
	}
	return parts[0], parts[1], nil
}

func quoteString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func quoteIdentifierPath(value string) string {
	parts := strings.Split(value, ".")
	for index, part := range parts {
		parts[index] = "`" + strings.ReplaceAll(part, "`", "``") + "`"
	}
	return strings.Join(parts, ".")
}

func toggleValue(args []string, current bool) (bool, error) {
	if len(args) == 0 {
		return !current, nil
	}
	if len(args) != 1 {
		return false, fmt.Errorf("只允许指定 on 或 off")
	}
	switch strings.ToLower(args[0]) {
	case "on":
		return true, nil
	case "off":
		return false, nil
	default:
		return false, fmt.Errorf("只允许指定 on 或 off")
	}
}

func onOff(value bool) string {
	if value {
		return "on"
	}
	return "off"
}

func emptyDefault(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func parseInnoDBSummary(statusText string) query.Result {
	rows := [][]string{}
	lines := strings.Split(statusText, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		switch {
		case strings.HasPrefix(trimmed, "History list length"):
			rows = append(rows, []string{"HistoryListLength", strings.TrimSpace(strings.TrimPrefix(trimmed, "History list length"))})
		case strings.Contains(lower, "buffer pool hit rate"):
			rows = append(rows, []string{"BufferPoolHitRate", trimmed})
		case strings.Contains(lower, "os file reads") && strings.Contains(lower, "os file writes"):
			rows = append(rows, []string{"FileIO", trimmed})
		case strings.Contains(lower, "rows inserted") && strings.Contains(lower, "updated"):
			rows = append(rows, []string{"RowOperations", trimmed})
		case strings.Contains(lower, "spin waits") && strings.Contains(lower, "rounds"):
			rows = append(rows, []string{"SemaphoreSpinWaits", trimmed})
		case strings.Contains(lower, "reservation count") && strings.Contains(lower, "signal count"):
			rows = append(rows, []string{"SemaphoreSignals", trimmed})
		case strings.Contains(lower, "adaptive hash index"):
			rows = append(rows, []string{"AdaptiveHashIndex", trimmed})
		}
	}
	if len(rows) == 0 {
		rows = append(rows, []string{"Summary", "未能从 SHOW ENGINE INNODB STATUS 中提取摘要，请直接执行原始 SQL 查看完整输出"})
	}
	return query.Result{
		Columns: []string{"Metric", "Value"},
		Rows:    rows,
		HasRows: true,
	}
}

func latestDeadlock(statusText string) string {
	lines := strings.Split(statusText, "\n")
	start := -1
	for index, line := range lines {
		if strings.Contains(line, "LATEST DETECTED DEADLOCK") {
			start = index
			break
		}
	}
	if start == -1 {
		return ""
	}
	end := len(lines)
	for index := start + 1; index < len(lines); index++ {
		line := strings.TrimSpace(lines[index])
		if strings.HasPrefix(line, "------------") && index > start+2 {
			end = index
			break
		}
	}
	section := strings.TrimSpace(strings.Join(lines[start:end], "\n"))
	const maxDeadlockLength = 4000
	if len(section) > maxDeadlockLength {
		return section[:maxDeadlockLength] + "\n... 已截断 ..."
	}
	return section
}

func filterColumns(result query.Result, names []string) query.Result {
	indexes := make([]int, 0, len(names))
	columns := make([]string, 0, len(names))
	for _, name := range names {
		for index, column := range result.Columns {
			if strings.EqualFold(column, name) {
				indexes = append(indexes, index)
				columns = append(columns, column)
				break
			}
		}
	}
	if len(indexes) == 0 {
		return result
	}
	rows := make([][]string, 0, len(result.Rows))
	for _, row := range result.Rows {
		filtered := make([]string, len(indexes))
		for index, sourceIndex := range indexes {
			filtered[index] = row[sourceIndex]
		}
		rows = append(rows, filtered)
	}
	result.Columns = columns
	result.Rows = rows
	return result
}
