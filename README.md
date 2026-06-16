# mysqlcli

`mysqlcli` 是一个使用 Go 编写的 MySQL 命令行客户端。它兼容官方 `mysql` 客户端的常用连接参数，并增加类似 `psql` 的数据库对象和运维诊断快捷命令。

## 当前能力

- 支持 TCP 和 Unix Socket 连接。
- 支持 `-h`、`-P`、`-u`、`-p`、`-D`、`-S`、`-e` 等常用参数。
- `-p` 使用终端无回显读取密码。
- 支持交互式多行 SQL、`;`、`\g`、`\G` 和 `DELIMITER`。
- 支持交互式执行 SQL 文件，以及查看变量、告警和健康状态快照。
- 支持方向键编辑、历史记录、`Ctrl+C` 清空输入及取消正在执行的查询。
- 支持 `~/.my.cnf`、`--defaults-file` 和 `--defaults-extra-file`。
- 支持 MySQL 风格 TLS 模式及 CA、客户端证书配置。
- 支持表格、纵向和批处理输出。
- 支持数据库对象、会话、锁等待、复制、用户和权限快捷命令。
- 支持配置化自定义快捷命令，可覆盖内置命令。
- 对 MySQL 5.7 和 8.x 的锁、复制、用户与角色差异提供基础回退。

## 构建

```shell
go build -o mysqlcli ./cmd/mysqlcli
```

## 使用

完整使用说明见 [docs/usage.md](docs/usage.md)。

```shell
./mysqlcli -h 127.0.0.1 -P 3306 -u root -p
./mysqlcli -h 127.0.0.1 -u root -p -D app -e "SELECT VERSION()"
./mysqlcli --socket /tmp/mysql.sock -u root -p mysql
./mysqlcli --defaults-file ~/.my.cnf
./mysqlcli -h db.example.com -u app -p --ssl-mode VERIFY_IDENTITY --ssl-ca ca.pem
```

非交互环境中，`-p` 无法安全读取密码。此时可使用 MySQL 配置文件，或临时使用 `MYSQL_PWD`。

## 快捷命令

| 命令 | 用途 |
| --- | --- |
| `\l` | 查看数据库 |
| `\d [object]` | 列出或描述数据库对象 |
| `\dt`、`\dv`、`\di`、`\df` | 查看表、视图、索引和例程 |
| `\size [table]` | 查看表空间大小 |
| `\tableinfo table` | 查看表元数据详情 |
| `\tablesize [pattern]` | 按空间占用排序查看表 |
| `\connect db` | 切换数据库 |
| `\sessions` | 查看活跃会话 |
| `\locks` | 查看锁等待 |
| `\locks --all` | 查看全部 InnoDB 锁 |
| `\locks --tree` | 查看阻塞关系树 |
| `\repl` | 查看复制状态 |
| `\repl channels` | 查看复制通道 |
| `\variables [pattern]` | 查看全局服务器变量 |
| `\variables --session [pattern]` | 查看当前会话变量 |
| `\warnings` | 查看最近 SQL 告警 |
| `\W` | 切换每条语句后自动显示告警 |
| `\innodb` | 查看 InnoDB 状态摘要 |
| `\slowlog [N]` | 查看慢日志配置和最近慢查询 |
| `\binlog` | 查看二进制日志摘要 |
| `\deadlocks` | 查看最近死锁信息 |
| `\charset` | 查看字符集和排序规则 |
| `\du` | 查看用户 |
| `\grants user@host` | 查看授权 |
| `\roles user@host` | 查看角色 |
| `\whoami` | 查看当前身份 |
| `\privileges` | 查看服务器权限类型 |
| `\reconnect` | 使用原连接参数重新连接 |
| `\kill id` | 终止查询 |
| `\x` | 切换纵向输出 |
| `\timing` | 切换耗时显示 |
| `\pager command` | 设置分页器 |
| `\i file`、`\. file` | 执行 SQL 文件 |
| `\status`、`\s` | 查看连接和健康状态 |
| `\show`、`\p` | 打印当前输入缓冲区 |
| `\e`、`\edit` | 使用外部编辑器编辑缓冲区 |
| `\q` | 退出 |

在客户端内执行 `\?` 可查看完整帮助。

## 自定义快捷命令

可以在 `~/.my.cnf`、`--defaults-file` 或 `--defaults-extra-file` 指定的配置文件中增加 `[command.<name>]` 分组。自定义命令优先于内置命令，因此下面示例会把 `\du table_name` 配成查看指定表空间大小：

```ini
[command.du]
description = 查看表空间
sql = SELECT TABLE_NAME AS TableName, ENGINE AS Engine, ROUND((DATA_LENGTH + INDEX_LENGTH) / 1024 / 1024, 2) AS TotalMB, ROUND(DATA_FREE / 1024 / 1024, 2) AS FreeMB FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?
```

使用：

```text
\du orders
```

自定义 SQL 中的 `?` 会按命令参数顺序绑定，参数数量必须和 `?` 数量一致。

## 开发状态

当前版本已经具备基础连接、查询、可编辑 REPL、历史、TLS、MySQL 配置文件和诊断命令。容器集成测试与更精细的复制摘要仍在后续阶段。

完整设计见 [docs/design.md](docs/design.md)。
