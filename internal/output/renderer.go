package output

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"unicode/utf8"

	"github.com/linchongguang/mysqlcli/internal/query"
)

type Options struct {
	Batch           bool
	SkipColumnNames bool
	Silent          bool
	Vertical        bool
	Timing          bool
}

type Renderer struct {
	writer       io.Writer
	options      Options
	pagerCommand string
}

func NewRenderer(writer io.Writer, options Options) *Renderer {
	return &Renderer{writer: writer, options: options}
}

func (r *Renderer) SetVertical(enabled bool) {
	r.options.Vertical = enabled
}

func (r *Renderer) Vertical() bool {
	return r.options.Vertical
}

func (r *Renderer) SetTiming(enabled bool) {
	r.options.Timing = enabled
}

func (r *Renderer) Timing() bool {
	return r.options.Timing
}

func (r *Renderer) SetPager(command string) {
	r.pagerCommand = strings.TrimSpace(command)
}

func (r *Renderer) Pager() string {
	return r.pagerCommand
}

func (r *Renderer) Render(result query.Result) error {
	if r.pagerCommand != "" && result.HasRows && !r.options.Batch {
		return r.renderWithPager(result)
	}

	return r.renderDirect(result)
}

func (r *Renderer) renderWithPager(result query.Result) error {
	reader, writer := io.Pipe()
	errCh := make(chan error, 1)
	go func() {
		pagerRenderer := *r
		pagerRenderer.writer = writer
		pagerRenderer.pagerCommand = ""
		err := pagerRenderer.Render(result)
		errCh <- writer.CloseWithError(err)
	}()

	pagerErr := runPager(r.pagerCommand, reader, r.writer)
	closeErr := <-errCh
	if pagerErr != nil {
		return pagerErr
	}
	return closeErr
}

func (r *Renderer) renderDirect(result query.Result) error {
	if !result.HasRows {
		if !r.options.Silent {
			_, err := fmt.Fprintf(r.writer, "Query OK, %d rows affected\n", result.RowsAffected)
			return err
		}
		return nil
	}

	var err error
	switch {
	case r.options.Vertical:
		err = r.renderVertical(result)
	case r.options.Batch:
		err = r.renderBatch(result)
	default:
		err = r.renderTable(result)
	}
	if err != nil {
		return err
	}
	if !r.options.Silent {
		fmt.Fprintf(r.writer, "%d rows in set", len(result.Rows))
		if r.options.Timing {
			fmt.Fprintf(r.writer, " (%.3f sec)", result.Duration.Seconds())
		}
		fmt.Fprintln(r.writer)
	}
	return nil
}

func runPager(commandText string, input io.Reader, output io.Writer) error {
	command := exec.Command("sh", "-c", commandText)
	command.Stdin = input
	command.Stdout = output
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("执行分页器 %q: %w", commandText, err)
	}
	return nil
}

func (r *Renderer) renderBatch(result query.Result) error {
	if !r.options.SkipColumnNames {
		fmt.Fprintln(r.writer, strings.Join(result.Columns, "\t"))
	}
	for _, row := range result.Rows {
		fmt.Fprintln(r.writer, strings.Join(row, "\t"))
	}
	return nil
}

func (r *Renderer) renderVertical(result query.Result) error {
	maxWidth := 0
	for _, column := range result.Columns {
		maxWidth = max(maxWidth, utf8.RuneCountInString(column))
	}
	for rowIndex, row := range result.Rows {
		fmt.Fprintf(r.writer, "*************************** %d. row ***************************\n", rowIndex+1)
		for index, column := range result.Columns {
			fmt.Fprintf(r.writer, "%*s: %s\n", maxWidth, column, row[index])
		}
	}
	return nil
}

func (r *Renderer) renderTable(result query.Result) error {
	widths := make([]int, len(result.Columns))
	for index, column := range result.Columns {
		widths[index] = utf8.RuneCountInString(column)
	}
	for _, row := range result.Rows {
		for index, value := range row {
			widths[index] = max(widths[index], utf8.RuneCountInString(value))
		}
	}

	writeBorder := func() {
		fmt.Fprint(r.writer, "+")
		for _, width := range widths {
			fmt.Fprint(r.writer, strings.Repeat("-", width+2), "+")
		}
		fmt.Fprintln(r.writer)
	}
	writeRow := func(row []string) {
		fmt.Fprint(r.writer, "|")
		for index, value := range row {
			padding := widths[index] - utf8.RuneCountInString(value)
			fmt.Fprintf(r.writer, " %s%s |", value, strings.Repeat(" ", padding))
		}
		fmt.Fprintln(r.writer)
	}

	writeBorder()
	if !r.options.SkipColumnNames {
		writeRow(result.Columns)
		writeBorder()
	}
	for _, row := range result.Rows {
		writeRow(row)
	}
	writeBorder()
	return nil
}
