package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	DEFAULT_HOST            = "localhost"
	DEFAULT_PORT            = 3306
	DEFAULT_CONNECT_TIMEOUT = 10 * time.Second
)

type Config struct {
	Host            string
	Port            int
	User            string
	Password        string
	Database        string
	Socket          string
	Protocol        string
	Execute         string
	Batch           bool
	SkipColumnNames bool
	Silent          bool
	ConnectTimeout  time.Duration
	SSLMode         string
	SSLCA           string
	SSLCert         string
	SSLKey          string
	DefaultsFile    string
	DefaultsExtra   string
	HistoryFile     string
	HistoryEnabled  bool
	CustomCommands  map[string]CustomCommand
	ShowVersion     bool
	ShowHelp        bool
	PasswordPrompt  bool
}

type CustomCommand struct {
	Name        string
	SQL         string
	Description string
}

func Parse(args []string, stdin io.Reader, stderr io.Writer) (Config, error) {
	var result Config
	result.Host = DEFAULT_HOST
	result.Port = DEFAULT_PORT
	result.ConnectTimeout = DEFAULT_CONNECT_TIMEOUT
	result.SSLMode = "preferred"
	result.HistoryEnabled = true
	result.HistoryFile = defaultHistoryFile()
	result.CustomCommands = make(map[string]CustomCommand)

	normalizedArgs, password, passwordPrompt, err := normalizePasswordArgs(args)
	if err != nil {
		return Config{}, err
	}

	defaultsFile, defaultsExtra := findDefaultsFiles(normalizedArgs)
	if err := loadConfigFiles(&result, defaultsFile, defaultsExtra); err != nil {
		return Config{}, err
	}

	flags := flag.NewFlagSet("mysqlcli", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&result.Host, "h", result.Host, "MySQL 主机地址")
	flags.StringVar(&result.Host, "host", result.Host, "MySQL 主机地址")
	flags.IntVar(&result.Port, "P", result.Port, "MySQL TCP 端口")
	flags.IntVar(&result.Port, "port", result.Port, "MySQL TCP 端口")
	flags.StringVar(&result.User, "u", result.User, "登录用户")
	flags.StringVar(&result.User, "user", result.User, "登录用户")
	flags.StringVar(&result.Database, "D", result.Database, "初始数据库")
	flags.StringVar(&result.Database, "database", result.Database, "初始数据库")
	flags.StringVar(&result.Socket, "S", result.Socket, "Unix Socket 路径")
	flags.StringVar(&result.Socket, "socket", result.Socket, "Unix Socket 路径")
	flags.StringVar(&result.Protocol, "protocol", result.Protocol, "连接协议: tcp 或 socket")
	flags.StringVar(&result.Execute, "e", "", "执行 SQL 后退出")
	flags.StringVar(&result.Execute, "execute", "", "执行 SQL 后退出")
	flags.BoolVar(&result.Batch, "B", false, "批处理输出")
	flags.BoolVar(&result.Batch, "batch", false, "批处理输出")
	flags.BoolVar(&result.SkipColumnNames, "N", false, "不输出列名")
	flags.BoolVar(&result.SkipColumnNames, "skip-column-names", false, "不输出列名")
	flags.BoolVar(&result.Silent, "s", false, "精简输出")
	flags.BoolVar(&result.Silent, "silent", false, "精简输出")
	flags.DurationVar(&result.ConnectTimeout, "connect-timeout", result.ConnectTimeout, "连接超时")
	flags.StringVar(&result.SSLMode, "ssl-mode", result.SSLMode, "TLS 模式")
	flags.StringVar(&result.SSLCA, "ssl-ca", result.SSLCA, "CA 证书文件")
	flags.StringVar(&result.SSLCert, "ssl-cert", result.SSLCert, "客户端证书文件")
	flags.StringVar(&result.SSLKey, "ssl-key", result.SSLKey, "客户端私钥文件")
	flags.StringVar(&result.DefaultsFile, "defaults-file", defaultsFile, "MySQL 配置文件")
	flags.StringVar(&result.DefaultsExtra, "defaults-extra-file", defaultsExtra, "附加 MySQL 配置文件")
	flags.StringVar(&result.HistoryFile, "history-file", result.HistoryFile, "历史记录文件")
	flags.BoolVar(&result.HistoryEnabled, "history", result.HistoryEnabled, "启用历史记录")
	flags.BoolVar(&result.ShowVersion, "version", false, "显示版本")
	flags.BoolVar(&result.ShowHelp, "?", false, "显示帮助")
	flags.BoolVar(&result.ShowHelp, "help", false, "显示帮助")

	if err := flags.Parse(normalizedArgs); err != nil {
		return Config{}, err
	}
	if flags.NArg() > 1 {
		return Config{}, fmt.Errorf("只允许指定一个数据库名称")
	}
	if flags.NArg() == 1 {
		databaseFlagSet := false
		flags.Visit(func(item *flag.Flag) {
			if item.Name == "D" || item.Name == "database" {
				databaseFlagSet = true
			}
		})
		if databaseFlagSet {
			return Config{}, errors.New("数据库名称不能同时通过位置参数和 -D 指定")
		}
		result.Database = flags.Arg(0)
	}

	result.PasswordPrompt = passwordPrompt
	if password != "" || passwordPrompt {
		result.Password = password
	}
	if result.Password == "" && !result.PasswordPrompt {
		result.Password = os.Getenv("MYSQL_PWD")
	}
	if result.PasswordPrompt {
		result.Password, err = readPassword(stdin, stderr)
		if err != nil {
			return Config{}, err
		}
	}

	if result.Port < 1 || result.Port > 65535 {
		return Config{}, fmt.Errorf("端口必须在 1 到 65535 之间")
	}
	if result.Protocol != "" && result.Protocol != "tcp" && result.Protocol != "socket" {
		return Config{}, fmt.Errorf("不支持的协议 %q", result.Protocol)
	}
	if result.Protocol == "socket" && result.Socket == "" {
		return Config{}, errors.New("使用 socket 协议时必须指定 --socket")
	}
	result.SSLMode = normalizeSSLMode(result.SSLMode)
	if !validSSLMode(result.SSLMode) {
		return Config{}, fmt.Errorf("不支持的 ssl-mode %q", result.SSLMode)
	}
	if (result.SSLCert == "") != (result.SSLKey == "") {
		return Config{}, errors.New("--ssl-cert 和 --ssl-key 必须同时指定")
	}

	return result, nil
}

func PrintUsage(writer io.Writer) {
	fmt.Fprintln(writer, `Usage: mysqlcli [OPTIONS] [database]

连接选项:
  -h, --host HOST              MySQL 主机地址
  -P, --port PORT              MySQL TCP 端口
  -u, --user USER              登录用户
  -p[PASSWORD]                 读取密码或直接指定密码
  -D, --database DATABASE      初始数据库
  -S, --socket PATH            Unix Socket 路径
      --protocol PROTOCOL      tcp 或 socket
      --connect-timeout TIME   连接超时，例如 10s
      --ssl-mode MODE          DISABLED/PREFERRED/REQUIRED/VERIFY_CA/VERIFY_IDENTITY
      --ssl-ca PATH            CA 证书文件
      --ssl-cert PATH          客户端证书文件
      --ssl-key PATH           客户端私钥文件
      --defaults-file PATH     只读取指定 MySQL 配置文件
      --defaults-extra-file P  额外读取 MySQL 配置文件

执行与输出:
  -e, --execute SQL            执行 SQL 后退出
  -B, --batch                  批处理输出
  -N, --skip-column-names      不输出列名
  -s, --silent                 精简输出
      --version                显示版本
  -?, --help                   显示帮助`)
}

func normalizePasswordArgs(args []string) ([]string, string, bool, error) {
	result := make([]string, 0, len(args))
	var password string
	var prompt bool

	for _, arg := range args {
		switch {
		case arg == "-p" || arg == "--password":
			prompt = true
		case strings.HasPrefix(arg, "-p") && len(arg) > 2:
			password = arg[2:]
			prompt = false
		case strings.HasPrefix(arg, "--password="):
			password = strings.TrimPrefix(arg, "--password=")
			prompt = password == ""
		default:
			result = append(result, arg)
		}
	}

	return result, password, prompt, nil
}

func readPassword(stdin io.Reader, stderr io.Writer) (string, error) {
	file, ok := stdin.(*os.File)
	if !ok || !isTerminal(file) {
		return "", errors.New("-p 需要从交互式终端读取密码；非交互模式请使用配置文件或 MYSQL_PWD")
	}

	fmt.Fprint(stderr, "Enter password: ")
	if err := setEcho(file, false); err != nil {
		return "", err
	}
	defer setEcho(file, true)

	var password string
	_, err := fmt.Fscanln(file, &password)
	fmt.Fprintln(stderr)
	if err != nil {
		return "", fmt.Errorf("读取密码: %w", err)
	}
	return password, nil
}

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func setEcho(file *os.File, enabled bool) error {
	argument := "-echo"
	if enabled {
		argument = "echo"
	}
	command := exec.Command("stty", argument)
	command.Stdin = file
	if err := command.Run(); err != nil {
		return fmt.Errorf("设置终端密码回显: %w", err)
	}
	return nil
}

func (c Config) Address() string {
	if c.Socket != "" || c.Protocol == "socket" {
		return c.Socket
	}
	return c.Host + ":" + strconv.Itoa(c.Port)
}

func findDefaultsFiles(args []string) (string, string) {
	var defaultsFile string
	var defaultsExtra string
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case strings.HasPrefix(arg, "--defaults-file="):
			defaultsFile = strings.TrimPrefix(arg, "--defaults-file=")
		case arg == "--defaults-file" && index+1 < len(args):
			index++
			defaultsFile = args[index]
		case strings.HasPrefix(arg, "--defaults-extra-file="):
			defaultsExtra = strings.TrimPrefix(arg, "--defaults-extra-file=")
		case arg == "--defaults-extra-file" && index+1 < len(args):
			index++
			defaultsExtra = args[index]
		}
	}
	return defaultsFile, defaultsExtra
}

func loadConfigFiles(result *Config, defaultsFile string, defaultsExtra string) error {
	var paths []string
	if defaultsFile != "" {
		paths = append(paths, defaultsFile)
	} else if homeDir, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(homeDir, ".my.cnf"))
	}
	if defaultsExtra != "" {
		paths = append(paths, defaultsExtra)
	}
	for _, path := range paths {
		if err := loadConfigFile(result, path, defaultsFile != "" || path == defaultsExtra); err != nil {
			return err
		}
	}
	return nil
}

func loadConfigFile(result *Config, path string, required bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && !required {
			return nil
		}
		return fmt.Errorf("读取配置文件 %s: %w", path, err)
	}
	section := ""
	for lineNumber, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			continue
		}
		if strings.HasPrefix(section, "command.") {
			key, value, ok := splitConfigLine(line)
			if !ok {
				return fmt.Errorf("配置文件 %s 第 %d 行格式错误", path, lineNumber+1)
			}
			commandName := strings.TrimSpace(strings.TrimPrefix(section, "command."))
			if err := applyCustomCommandValue(result, commandName, strings.ToLower(strings.TrimSpace(key)), unquote(strings.TrimSpace(value))); err != nil {
				return fmt.Errorf("配置文件 %s 第 %d 行: %w", path, lineNumber+1, err)
			}
			continue
		}
		if section != "client" && section != "mysqlcli" {
			continue
		}
		key, value, ok := splitConfigLine(line)
		if !ok {
			return fmt.Errorf("配置文件 %s 第 %d 行格式错误", path, lineNumber+1)
		}
		if err := applyConfigValue(result, strings.ToLower(strings.TrimSpace(key)), unquote(strings.TrimSpace(value))); err != nil {
			return fmt.Errorf("配置文件 %s 第 %d 行: %w", path, lineNumber+1, err)
		}
	}
	return nil
}

func splitConfigLine(line string) (string, string, bool) {
	key, value, ok := strings.Cut(line, "=")
	if !ok {
		key, value, ok = strings.Cut(line, " ")
	}
	return key, value, ok
}

func applyConfigValue(result *Config, key string, value string) error {
	switch strings.ReplaceAll(key, "_", "-") {
	case "host":
		result.Host = value
	case "port":
		port, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("port 不是有效整数")
		}
		result.Port = port
	case "user":
		result.User = value
	case "password":
		result.Password = value
	case "database":
		result.Database = value
	case "socket":
		result.Socket = value
	case "protocol":
		result.Protocol = strings.ToLower(value)
	case "ssl-mode":
		result.SSLMode = value
	case "ssl-ca":
		result.SSLCA = value
	case "ssl-cert":
		result.SSLCert = value
	case "ssl-key":
		result.SSLKey = value
	case "connect-timeout":
		duration, err := time.ParseDuration(value)
		if err != nil {
			if seconds, parseErr := strconv.Atoi(value); parseErr == nil {
				duration = time.Duration(seconds) * time.Second
			} else {
				return fmt.Errorf("connect-timeout 格式错误")
			}
		}
		result.ConnectTimeout = duration
	case "history-file":
		result.HistoryFile = value
	case "history":
		result.HistoryEnabled = value != "0" && !strings.EqualFold(value, "false") && !strings.EqualFold(value, "off")
	}
	return nil
}

func applyCustomCommandValue(result *Config, name string, key string, value string) error {
	if name == "" {
		return fmt.Errorf("自定义命令名称不能为空")
	}
	normalizedName := strings.ToLower(strings.TrimPrefix(name, "\\"))
	command := result.CustomCommands[normalizedName]
	command.Name = normalizedName
	switch strings.ReplaceAll(key, "_", "-") {
	case "sql":
		command.SQL = value
	case "description", "desc":
		command.Description = value
	default:
		return fmt.Errorf("未知自定义命令字段 %q", key)
	}
	result.CustomCommands[normalizedName] = command
	return nil
}

func unquote(value string) string {
	if len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"')) {
		return value[1 : len(value)-1]
	}
	return value
}

func normalizeSSLMode(value string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(value), "-", "_"))
}

func validSSLMode(value string) bool {
	switch value {
	case "DISABLED", "PREFERRED", "REQUIRED", "VERIFY_CA", "VERIFY_IDENTITY":
		return true
	default:
		return false
	}
}

func defaultHistoryFile() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(configDir, "mysqlcli", "history")
}
