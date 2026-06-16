package database

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/linchongguang/mysqlcli/internal/config"
)

func TestConfigureTLSModes(t *testing.T) {
	tests := []struct {
		mode string
		want string
	}{
		{mode: "DISABLED", want: ""},
		{mode: "PREFERRED", want: "preferred"},
		{mode: "REQUIRED", want: "skip-verify"},
	}
	for _, test := range tests {
		t.Run(test.mode, func(t *testing.T) {
			driverConfig := mysql.NewConfig()
			tlsName, err := configureTLS(config.Config{SSLMode: test.mode}, driverConfig)
			if err != nil {
				t.Fatalf("configureTLS() 返回错误: %v", err)
			}
			defer deregisterTLS(tlsName)
			if driverConfig.TLSConfig != test.want {
				t.Fatalf("TLSConfig = %q, 期望 %q", driverConfig.TLSConfig, test.want)
			}
		})
	}
}

func TestBuildTLSConfigRejectsInvalidCA(t *testing.T) {
	caPath := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caPath, []byte("invalid certificate"), 0600); err != nil {
		t.Fatalf("写入测试证书: %v", err)
	}
	_, err := buildTLSConfig(config.Config{SSLMode: "VERIFY_CA", SSLCA: caPath})
	if err == nil {
		t.Fatal("期望返回无效 CA 错误")
	}
}
