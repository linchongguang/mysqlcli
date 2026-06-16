package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestParseConnectionOptions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	tests := []struct {
		name         string
		args         []string
		wantHost     string
		wantPort     int
		wantUser     string
		wantPassword string
		wantDatabase string
	}{
		{
			name:         "短参数",
			args:         []string{"-h", "db.local", "-P", "3307", "-u", "app", "-psecret", "demo"},
			wantHost:     "db.local",
			wantPort:     3307,
			wantUser:     "app",
			wantPassword: "secret",
			wantDatabase: "demo",
		},
		{
			name:         "长参数",
			args:         []string{"--host", "127.0.0.1", "--user", "root", "--password=pass", "--database", "mysql"},
			wantHost:     "127.0.0.1",
			wantPort:     DEFAULT_PORT,
			wantUser:     "root",
			wantPassword: "pass",
			wantDatabase: "mysql",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Parse(test.args, bytes.NewReader(nil), &bytes.Buffer{})
			if err != nil {
				t.Fatalf("Parse() 返回错误: %v", err)
			}
			if got.Host != test.wantHost || got.Port != test.wantPort || got.User != test.wantUser || got.Password != test.wantPassword || got.Database != test.wantDatabase {
				t.Fatalf("Parse() = %+v", got)
			}
		})
	}
}

func TestParseRejectsInvalidProtocol(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, err := Parse([]string{"--protocol", "udp"}, bytes.NewReader(nil), &bytes.Buffer{})
	if err == nil {
		t.Fatal("期望返回协议错误")
	}
}

func TestParseHelp(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	got, err := Parse([]string{"--help"}, bytes.NewReader(nil), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Parse() 返回错误: %v", err)
	}
	if !got.ShowHelp {
		t.Fatal("ShowHelp 应为 true")
	}
}

func TestParseDefaultsFileAndCommandLineOverride(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	configPath := filepath.Join(t.TempDir(), "client.cnf")
	content := `[client]
host = db.internal
port = 3307
user = config_user
password = config_secret
ssl-mode = VERIFY_CA
ssl-ca = /tmp/ca.pem

[mysqlcli]
history = off
`
	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		t.Fatalf("写入配置文件: %v", err)
	}

	got, err := Parse([]string{"--defaults-file", configPath, "--host", "cli.internal", "--user", "cli_user"}, bytes.NewReader(nil), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Parse() 返回错误: %v", err)
	}
	if got.Host != "cli.internal" || got.Port != 3307 || got.User != "cli_user" || got.Password != "config_secret" {
		t.Fatalf("Parse() = %+v", got)
	}
	if got.SSLMode != "VERIFY_CA" || got.SSLCA != "/tmp/ca.pem" || got.HistoryEnabled {
		t.Fatalf("配置文件字段未生效: %+v", got)
	}
}

func TestParseCustomCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	configPath := filepath.Join(t.TempDir(), "client.cnf")
	content := `[mysqlcli]

[command.du]
description = 查看表空间
sql = SELECT TABLE_NAME, DATA_LENGTH + INDEX_LENGTH AS total_bytes FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?
`
	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		t.Fatalf("写入配置文件: %v", err)
	}

	got, err := Parse([]string{"--defaults-file", configPath}, bytes.NewReader(nil), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Parse() 返回错误: %v", err)
	}
	command, ok := got.CustomCommands["du"]
	if !ok {
		t.Fatal("未解析 command.du")
	}
	if command.Description != "查看表空间" || command.SQL == "" {
		t.Fatalf("自定义命令 = %+v", command)
	}
}

func TestParseDefaultMyCnf(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	if err := os.WriteFile(filepath.Join(homeDir, ".my.cnf"), []byte("[client]\nhost=from-home\nuser=home_user\n"), 0600); err != nil {
		t.Fatalf("写入 .my.cnf: %v", err)
	}
	got, err := Parse(nil, bytes.NewReader(nil), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Parse() 返回错误: %v", err)
	}
	if got.Host != "from-home" || got.User != "home_user" {
		t.Fatalf("Parse() = %+v", got)
	}
}

func TestParseRejectsIncompleteClientCertificate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, err := Parse([]string{"--ssl-cert", "client.pem"}, bytes.NewReader(nil), &bytes.Buffer{})
	if err == nil {
		t.Fatal("期望返回证书参数错误")
	}
}
