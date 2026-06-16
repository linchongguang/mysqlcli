package metacommand

import "testing"

func TestParse(t *testing.T) {
	command, err := Parse(`\d "Order Detail"`)
	if err != nil {
		t.Fatalf("Parse() 返回错误: %v", err)
	}
	if command.Name != "d" || len(command.Args) != 1 || command.Args[0] != "Order Detail" {
		t.Fatalf("Parse() = %+v", command)
	}
}
