package sqlparser

import "testing"

func TestBufferComplete(t *testing.T) {
	tests := []struct {
		name      string
		statement string
		complete  bool
	}{
		{name: "简单语句", statement: "SELECT 1;", complete: true},
		{name: "字符串中的分号", statement: "SELECT ';'", complete: false},
		{name: "字符串后终止", statement: "SELECT ';';", complete: true},
		{name: "注释中的分号", statement: "SELECT 1 -- ;", complete: false},
		{name: "纵向执行", statement: "SELECT 1\\G", complete: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buffer := NewBuffer()
			buffer.Append(test.statement)
			if got := buffer.Complete(); got != test.complete {
				t.Fatalf("Complete() = %v, 期望 %v", got, test.complete)
			}
		})
	}
}

func TestBufferCustomDelimiter(t *testing.T) {
	buffer := NewBuffer()
	if !buffer.SetDelimiter("//") {
		t.Fatal("SetDelimiter() 应成功")
	}
	buffer.Append("CREATE PROCEDURE p() BEGIN SELECT 1; END//")
	if !buffer.Complete() {
		t.Fatal("自定义分隔符未被识别")
	}
}
