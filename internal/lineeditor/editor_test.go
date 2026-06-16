package lineeditor

import "testing"

func TestSensitive(t *testing.T) {
	tests := []struct {
		statement string
		want      bool
	}{
		{"SELECT 1", false},
		{"CREATE USER app IDENTIFIED BY 'secret'", true},
		{"SET PASSWORD = 'secret'", true},
		{"CHANGE REPLICATION SOURCE TO SOURCE_PASSWORD='secret'", true},
	}
	for _, test := range tests {
		if got := Sensitive(test.statement); got != test.want {
			t.Errorf("Sensitive(%q) = %v, 期望 %v", test.statement, got, test.want)
		}
	}
}
