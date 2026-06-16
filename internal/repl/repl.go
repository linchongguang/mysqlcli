package repl

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"

	"github.com/linchongguang/mysqlcli/internal/config"
	"github.com/linchongguang/mysqlcli/internal/lineeditor"
	"github.com/linchongguang/mysqlcli/internal/metacommand"
	"github.com/linchongguang/mysqlcli/internal/output"
	"github.com/linchongguang/mysqlcli/internal/query"
	"github.com/linchongguang/mysqlcli/internal/sqlparser"
)

type REPL struct {
	client      Client
	renderer    *output.Renderer
	commands    *metacommand.Registry
	input       io.Reader
	output      io.Writer
	errorOutput io.Writer
	interactive bool
	historyFile string
	history     bool
}

type Client interface {
	metacommand.Client
	CurrentDatabase() string
}

func New(client Client, renderer *output.Renderer, input io.Reader, outputWriter io.Writer, errorWriter io.Writer, interactive bool, historyFile string, history bool, customCommands map[string]config.CustomCommand) *REPL {
	return &REPL{
		client:      client,
		renderer:    renderer,
		commands:    metacommand.NewRegistry(client, renderer, outputWriter, customCommands),
		input:       input,
		output:      outputWriter,
		errorOutput: errorWriter,
		interactive: interactive,
		historyFile: historyFile,
		history:     history,
	}
}

func (r *REPL) Run(ctx context.Context) error {
	readLine, addHistory, closeInput, err := r.inputReader()
	if err != nil {
		return err
	}
	defer closeInput()

	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)

	buffer := sqlparser.NewBuffer()

	for {
		line, readErr := readLine(r.prompt(buffer.Empty()))
		if errors.Is(readErr, lineeditor.ErrInterrupt) {
			buffer.Clear()
			continue
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return fmt.Errorf("读取输入: %w", readErr)
		}
		trimmed := strings.TrimSpace(line)

		if trimmed == "\\c" {
			buffer.Clear()
			continue
		}
		if trimmed == "\\p" || trimmed == "\\show" {
			fmt.Fprintln(r.output, buffer.String())
			continue
		}
		if trimmed == "\\e" || trimmed == "\\edit" {
			edited, err := r.editBuffer(buffer.String())
			if err != nil {
				r.printExecutionError(err)
				continue
			}
			buffer.Clear()
			if strings.TrimSpace(edited) == "" {
				continue
			}
			buffer.Append(edited)
			if !buffer.Complete() {
				continue
			}
			statement, vertical := buffer.Statement()
			buffer.Clear()
			if statement == "" {
				continue
			}
			if err := r.executeStatement(ctx, interrupts, statement, vertical); err != nil {
				r.printExecutionError(err)
			} else if historyErr := addHistory(statement); historyErr != nil {
				fmt.Fprintf(r.errorOutput, "保存历史记录: %v\n", historyErr)
			}
			continue
		}
		if buffer.Empty() && strings.HasPrefix(strings.ToUpper(trimmed), "DELIMITER ") {
			if !buffer.SetDelimiter(strings.TrimSpace(trimmed[len("DELIMITER "):])) {
				fmt.Fprintln(r.errorOutput, "无效的 DELIMITER")
			}
			continue
		}
		if buffer.Empty() && strings.HasPrefix(trimmed, "\\") {
			var commandResult metacommand.Result
			commandErr := r.runCancelable(ctx, interrupts, func(commandCtx context.Context) error {
				var executeErr error
				commandResult, executeErr = r.commands.Execute(commandCtx, trimmed)
				return executeErr
			})
			if commandErr != nil {
				r.printExecutionError(commandErr)
				continue
			}
			_ = addHistory(trimmed)
			if commandResult.Quit {
				return nil
			}
			if commandResult.SourceFile != "" {
				if err := r.executeFile(ctx, interrupts, commandResult.SourceFile, addHistory); err != nil {
					r.printExecutionError(err)
				}
				continue
			}
			if commandResult.Handled {
				continue
			}
		}

		buffer.Append(line)
		if !buffer.Complete() {
			continue
		}
		statement, vertical := buffer.Statement()
		buffer.Clear()
		if statement == "" {
			continue
		}
		if err := r.executeStatement(ctx, interrupts, statement, vertical); err != nil {
			r.printExecutionError(err)
		} else if historyErr := addHistory(statement); historyErr != nil {
			fmt.Fprintf(r.errorOutput, "保存历史记录: %v\n", historyErr)
		}
	}

	if !buffer.Empty() {
		return fmt.Errorf("输入结束时仍有未完成的 SQL 语句")
	}
	return nil
}

func (r *REPL) editBuffer(initial string) (string, error) {
	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		editor = "vim"
	}
	file, err := os.CreateTemp("", "mysqlcli-*.sql")
	if err != nil {
		return "", fmt.Errorf("创建临时编辑文件: %w", err)
	}
	defer os.Remove(file.Name())
	if initial != "" {
		if _, err := file.WriteString(initial); err != nil {
			file.Close()
			return "", fmt.Errorf("写入临时编辑文件: %w", err)
		}
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("关闭临时编辑文件: %w", err)
	}

	command := exec.Command("sh", "-c", editor+" "+shellQuote(file.Name()))
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("执行编辑器 %q: %w", editor, err)
	}
	content, err := os.ReadFile(file.Name())
	if err != nil {
		return "", fmt.Errorf("读取临时编辑文件: %w", err)
	}
	return string(content), nil
}

func (r *REPL) executeFile(ctx context.Context, interrupts <-chan os.Signal, fileName string, addHistory func(string) error) error {
	file, err := os.Open(fileName)
	if err != nil {
		return fmt.Errorf("读取 SQL 文件 %s: %w", fileName, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	buffer := sqlparser.NewBuffer()
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if buffer.Empty() && strings.HasPrefix(strings.ToUpper(trimmed), "DELIMITER ") {
			if !buffer.SetDelimiter(strings.TrimSpace(trimmed[len("DELIMITER "):])) {
				return fmt.Errorf("%s: 无效的 DELIMITER", fileName)
			}
			continue
		}
		buffer.Append(line)
		if !buffer.Complete() {
			continue
		}
		statement, vertical := buffer.Statement()
		buffer.Clear()
		if statement == "" {
			continue
		}
		if err := r.executeStatement(ctx, interrupts, statement, vertical); err != nil {
			return fmt.Errorf("%s: %w", fileName, err)
		}
		if historyErr := addHistory(statement); historyErr != nil {
			fmt.Fprintf(r.errorOutput, "保存历史记录: %v\n", historyErr)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("读取 SQL 文件 %s: %w", fileName, err)
	}
	if !buffer.Empty() {
		return fmt.Errorf("%s: 文件结束时仍有未完成的 SQL 语句", fileName)
	}
	return nil
}

func (r *REPL) executeStatement(ctx context.Context, interrupts <-chan os.Signal, statement string, vertical bool) error {
	previousVertical := r.renderer.Vertical()
	if vertical {
		r.renderer.SetVertical(true)
		defer r.renderer.SetVertical(previousVertical)
	}

	var result query.Result
	executeErr := r.runCancelable(ctx, interrupts, func(queryCtx context.Context) error {
		var queryErr error
		result, queryErr = query.Execute(queryCtx, r.client.DB(), statement)
		return queryErr
	})
	if executeErr != nil {
		return executeErr
	}
	if renderErr := r.renderer.Render(result); renderErr != nil {
		return renderErr
	}
	if r.commands.AutoWarnings() {
		warnings, err := r.commands.Warnings(ctx)
		if err != nil {
			fmt.Fprintf(r.errorOutput, "读取 warnings: %v\n", err)
		} else if len(warnings.Rows) > 0 {
			return r.renderer.Render(warnings)
		}
	}
	return nil
}

func (r *REPL) prompt(primary bool) string {
	if !r.interactive {
		return ""
	}
	if primary {
		databaseName := r.client.CurrentDatabase()
		if databaseName == "" {
			databaseName = "(none)"
		}
		return databaseName + "> "
	}
	return "    -> "
}

func (r *REPL) inputReader() (func(string) (string, error), func(string) error, func(), error) {
	if r.interactive {
		file, ok := r.input.(*os.File)
		if !ok {
			return nil, nil, nil, fmt.Errorf("交互模式需要终端文件")
		}
		editor, err := lineeditor.New(file, r.output, r.historyFile, r.history)
		if err != nil {
			return nil, nil, nil, err
		}
		return editor.ReadLine, editor.AddHistory, func() {}, nil
	}

	scanner := bufio.NewScanner(r.input)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	readLine := func(_ string) (string, error) {
		if scanner.Scan() {
			return scanner.Text(), nil
		}
		if err := scanner.Err(); err != nil {
			return "", err
		}
		return "", io.EOF
	}
	return readLine, func(string) error { return nil }, func() {}, nil
}

func (r *REPL) runCancelable(ctx context.Context, interrupts <-chan os.Signal, execute func(context.Context) error) error {
	executeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	go func() {
		select {
		case <-interrupts:
			fmt.Fprintln(r.errorOutput, "正在取消当前操作...")
			cancel()
		case <-done:
		case <-ctx.Done():
			cancel()
		}
	}()
	err := execute(executeCtx)
	close(done)
	return err
}

func (r *REPL) printExecutionError(err error) {
	if errors.Is(err, context.Canceled) {
		fmt.Fprintln(r.errorOutput, "操作已取消")
		return
	}
	fmt.Fprintln(r.errorOutput, err)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
