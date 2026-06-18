package database

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/linchongguang/mysqlcli/internal/config"
)

type Client struct {
	mu       sync.RWMutex
	db       *sql.DB
	config   config.Config
	database string
	tlsName  string
}

type SessionInfo struct {
	ConnectionID uint64
	Version      string
	CurrentUser  string
	Database     string
}

func Open(ctx context.Context, appConfig config.Config) (*Client, error) {
	db, tlsName, err := openDB(ctx, appConfig)
	if err != nil {
		return nil, err
	}
	return &Client{db: db, config: appConfig, database: appConfig.Database, tlsName: tlsName}, nil
}

func openDB(ctx context.Context, appConfig config.Config) (*sql.DB, string, error) {
	driverConfig := mysql.NewConfig()
	driverConfig.User = appConfig.User
	driverConfig.Passwd = appConfig.Password
	driverConfig.DBName = appConfig.Database
	driverConfig.Timeout = appConfig.ConnectTimeout
	driverConfig.ReadTimeout = 0
	driverConfig.WriteTimeout = 0
	driverConfig.ParseTime = true
	driverConfig.MultiStatements = false
	tlsName, err := configureTLS(appConfig, driverConfig)
	if err != nil {
		return nil, "", err
	}

	if appConfig.Socket != "" || appConfig.Protocol == "socket" {
		driverConfig.Net = "unix"
		driverConfig.Addr = appConfig.Socket
	} else {
		driverConfig.Net = "tcp"
		driverConfig.Addr = net.JoinHostPort(appConfig.Host, strconv.Itoa(appConfig.Port))
	}

	db, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		deregisterTLS(tlsName)
		return nil, "", fmt.Errorf("创建 MySQL 连接: %w", err)
	}
	// 命令行客户端必须保持单会话语义，确保 USE、会话变量和临时表始终作用于同一连接。
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxIdleTime(5 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		deregisterTLS(tlsName)
		return nil, "", fmt.Errorf("连接 MySQL %s: %w", appConfig.Address(), err)
	}
	return db, tlsName, nil
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	err := c.db.Close()
	deregisterTLS(c.tlsName)
	c.tlsName = ""
	return err
}

func (c *Client) DB() *sql.DB {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.db
}

func (c *Client) CurrentDatabase() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.database
}

func (c *Client) UseDatabase(ctx context.Context, databaseName string) error {
	if databaseName == "" {
		return fmt.Errorf("数据库名称不能为空")
	}
	c.mu.RLock()
	db := c.db
	c.mu.RUnlock()
	if _, err := db.ExecContext(ctx, "USE "+quoteIdentifier(databaseName)); err != nil {
		return fmt.Errorf("切换数据库 %s: %w", databaseName, err)
	}
	c.mu.Lock()
	c.database = databaseName
	c.config.Database = databaseName
	c.mu.Unlock()
	return nil
}

func (c *Client) Reconnect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	appConfig := c.config
	appConfig.Database = c.database

	newDB, tlsName, err := openDB(ctx, appConfig)
	if err != nil {
		return err
	}

	oldDB := c.db
	oldTLSName := c.tlsName
	c.db = newDB
	c.tlsName = tlsName
	c.config = appConfig

	_ = oldDB.Close()
	deregisterTLS(oldTLSName)
	return nil
}

func (c *Client) SessionInfo(ctx context.Context) (SessionInfo, error) {
	var info SessionInfo
	var database sql.NullString
	c.mu.RLock()
	db := c.db
	c.mu.RUnlock()
	err := db.QueryRowContext(ctx, "SELECT CONNECTION_ID(), VERSION(), CURRENT_USER(), DATABASE()").Scan(
		&info.ConnectionID,
		&info.Version,
		&info.CurrentUser,
		&database,
	)
	if err != nil {
		return SessionInfo{}, fmt.Errorf("读取会话信息: %w", err)
	}
	if database.Valid {
		info.Database = database.String
	}
	return info, nil
}

var tlsSequence atomic.Uint64

func configureTLS(appConfig config.Config, driverConfig *mysql.Config) (string, error) {
	switch appConfig.SSLMode {
	case "DISABLED":
		return "", nil
	case "PREFERRED":
		driverConfig.TLSConfig = "preferred"
		return "", nil
	case "REQUIRED":
		driverConfig.TLSConfig = "skip-verify"
		return "", nil
	}

	tlsConfig, err := buildTLSConfig(appConfig)
	if err != nil {
		return "", err
	}
	tlsName := "mysqlcli-" + strconv.FormatUint(tlsSequence.Add(1), 10)
	if err := mysql.RegisterTLSConfig(tlsName, tlsConfig); err != nil {
		return "", fmt.Errorf("注册 TLS 配置: %w", err)
	}
	driverConfig.TLSConfig = tlsName
	return tlsName, nil
}

func buildTLSConfig(appConfig config.Config) (*tls.Config, error) {
	rootCAs, err := x509.SystemCertPool()
	if err != nil || rootCAs == nil {
		rootCAs = x509.NewCertPool()
	}
	if appConfig.SSLCA != "" {
		pemData, err := os.ReadFile(appConfig.SSLCA)
		if err != nil {
			return nil, fmt.Errorf("读取 CA 证书: %w", err)
		}
		if !rootCAs.AppendCertsFromPEM(pemData) {
			return nil, fmt.Errorf("CA 证书 %s 不包含有效证书", appConfig.SSLCA)
		}
	}

	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: rootCAs}
	if appConfig.SSLCert != "" {
		certificate, err := tls.LoadX509KeyPair(appConfig.SSLCert, appConfig.SSLKey)
		if err != nil {
			return nil, fmt.Errorf("读取客户端证书: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{certificate}
	}

	if appConfig.SSLMode == "VERIFY_IDENTITY" {
		tlsConfig.ServerName = appConfig.Host
		return tlsConfig, nil
	}

	// VERIFY_CA 只校验证书链，不校验主机名。
	tlsConfig.InsecureSkipVerify = true
	tlsConfig.VerifyConnection = func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) == 0 {
			return fmt.Errorf("服务器未提供证书")
		}
		intermediates := x509.NewCertPool()
		for _, certificate := range state.PeerCertificates[1:] {
			intermediates.AddCert(certificate)
		}
		_, err := state.PeerCertificates[0].Verify(x509.VerifyOptions{
			Roots:         rootCAs,
			Intermediates: intermediates,
		})
		return err
	}
	return tlsConfig, nil
}

func deregisterTLS(name string) {
	if name != "" {
		mysql.DeregisterTLSConfig(name)
	}
}

func quoteIdentifier(value string) string {
	result := "`"
	for _, char := range value {
		if char == '`' {
			result += "``"
		} else {
			result += string(char)
		}
	}
	return result + "`"
}
