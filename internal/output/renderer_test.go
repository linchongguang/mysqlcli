package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/linchongguang/mysqlcli/internal/query"
)

func TestRendererTable(t *testing.T) {
	var output bytes.Buffer
	renderer := NewRenderer(&output, Options{Silent: true})
	result := query.Result{
		Columns: []string{"name", "value"},
		Rows:    [][]string{{"mysql", "8.0"}},
		HasRows: true,
	}
	if err := renderer.Render(result); err != nil {
		t.Fatalf("Render() 返回错误: %v", err)
	}
	for _, expected := range []string{"| name  | value |", "| mysql | 8.0   |"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("输出未包含 %q:\n%s", expected, output.String())
		}
	}
}

func TestRendererBatch(t *testing.T) {
	var output bytes.Buffer
	renderer := NewRenderer(&output, Options{Batch: true, SkipColumnNames: true, Silent: true})
	result := query.Result{Columns: []string{"id"}, Rows: [][]string{{"1"}, {"2"}}, HasRows: true}
	if err := renderer.Render(result); err != nil {
		t.Fatalf("Render() 返回错误: %v", err)
	}
	if output.String() != "1\n2\n" {
		t.Fatalf("输出 = %q", output.String())
	}
}
