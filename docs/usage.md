# mysqlcli 使用文档

`mysqlcli` 是一个免安装的 MySQL 命令行客户端，兼容常用 `mysql` 客户端连接参数，并提供数据库对象、会话、锁、复制、用户权限和自定义快捷命令。

## 1. 获取二进制

当前仓库已生成两个本地构建产物：

```text
bin/mysqlcli              macOS amd64
bin/mysqlcli-linux-amd64  Linux x86_64
```

Linux 服务器免安装使用：

```shell
chmod +x mysqlcli-linux-amd64
./mysqlcli-linux-amd64 --help
```

如果放到固定路径，可以改名为 `mysqlcli`：

```shell
mv mysqlcli-linux-amd64 /usr/local/bin/mysqlcli
mysqlcli --help
```

已经上传到测试服务器的路径：

```shell
/root/mysqlcli
```

## 2. 基本连接

TCP 连接：

```shell
mysqlcli -h 127.0.0.1 -P 3306 -u root -p
```

指定数据库：

```shell
mysqlcli -h 127.0.0.1 -P 3306 -u app -p app_db
mysqlcli -h 127.0.0.1 -u app -p -D app_db
```

Unix Socket 连接：

```shell
mysqlcli --socket /tmp/mysql.sock -u root -p
mysqlcli -S /var/lib/mysql/mysql.sock -u root -p mysql
```

非交互执行 SQL：

```shell
mysqlcli -h 127.0.0.1 -u root -p -e "SELECT VERSION()"
```

批处理输出：

```shell
mysqlcli -h 127.0.0.1 -u root -p -B -N -e "SELECT user, host FROM mysql.user"
```

说明：`-p` 不带密码时会从终端安全读取密码。非交互环境无法读取密码，建议使用配置文件。

## 3. 配置文件

默认读取 `~/.my.cnf` 的 `[client]` 和 `[mysqlcli]` 分组。

示例：

```ini
[client]
host = 127.0.0.1
port = 3306
user = root
password = secret
database = app_db

[mysqlcli]
history = on
history-file = /root/.mysqlcli_history
```

指定配置文件：

```shell
mysqlcli --defaults-file /root/.my.cnf
```

加载附加配置文件：

```shell
mysqlcli --defaults-extra-file /root/mysqlcli-extra.cnf
```

优先级从低到高：

1. 默认值
2. `~/.my.cnf` 或 `--defaults-file`
3. `--defaults-extra-file`
4. 环境变量 `MYSQL_PWD`
5. 命令行参数

## 4. TLS 连接

支持 MySQL 风格 TLS 模式：

```shell
mysqlcli -h db.example.com -u app -p --ssl-mode REQUIRED
mysqlcli -h db.example.com -u app -p --ssl-mode VERIFY_CA --ssl-ca ca.pem
mysqlcli -h db.example.com -u app -p --ssl-mode VERIFY_IDENTITY --ssl-ca ca.pem
```

客户端证书：

```shell
mysqlcli -h db.example.com -u app -p \
    --ssl-mode VERIFY_IDENTITY \
    --ssl-ca ca.pem \
    --ssl-cert client-cert.pem \
    --ssl-key client-key.pem
```

模式说明：

| 模式 | 行为 |
| --- | --- |
| `DISABLED` | 不使用 TLS |
| `PREFERRED` | 优先 TLS，兼容回退 |
| `REQUIRED` | 必须使用 TLS，但不校验证书 |
| `VERIFY_CA` | 校验证书链 |
| `VERIFY_IDENTITY` | 校验证书链和主机名 |

## 5. 交互使用

进入客户端后可以直接执行 SQL：

```sql
SELECT DATABASE();
SHOW TABLES;
```

多行 SQL：

```sql
SELECT
    user,
    host
FROM mysql.user;
```

纵向输出：

```text
SELECT * FROM mysql.user LIMIT 1\G
```

切换纵向模式：

```text
\x
\x on
\x off
```

显示耗时：

```text
\timing
\timing on
\timing off
```

分页器：

```text
\pager less -S
\pager off
```

执行 SQL 文件：

```text
\i /path/to/schema.sql
\. migration.sql
```

说明：`\i` 和 `\.` 会在当前连接、当前数据库和当前分隔符语义下执行文件内容，支持多语句、`\G` 纵向输出和 `DELIMITER`。

查看或编辑当前输入缓冲区：

```text
\show
\p
\e
\edit
```

说明：`\e` / `\edit` 会调用 `$EDITOR`，未设置时默认使用 `vim`。编辑后如果 SQL 已完整会直接执行，否则保留在缓冲区继续输入。

退出：

```text
\q
```

终端快捷键：

| 快捷键 | 说明 |
| --- | --- |
| 方向键 | 移动光标或查看历史 |
| `Ctrl+A` | 跳到行首 |
| `Ctrl+E` | 跳到行尾 |
| `Ctrl+C` | 清空当前输入或取消正在执行的查询 |
| `Ctrl+D` | 空输入时退出 |

历史记录默认保存到用户配置目录。包含密码修改、创建用户密码等敏感 SQL 的语句不会写入历史。

## 6. 数据库对象快捷命令

| 命令 | 说明 |
| --- | --- |
| `\l [pattern]` | 列出数据库 |
| `\connect <db>` | 切换数据库 |
| `\use <db>` | `\connect` 的别名 |
| `\d` | 列出当前库表和视图 |
| `\d <table>` | 查看表字段 |
| `\dt [pattern]` | 查看表 |
| `\dv [pattern]` | 查看视图 |
| `\di [table]` | 查看索引 |
| `\df [pattern]` | 查看存储过程和函数 |
| `\triggers [pattern]` | 查看触发器 |
| `\events [pattern]` | 查看事件 |
| `\size [table]` | 查看表空间大小 |
| `\tablesize [pattern]` | 按表空间占用排序 |
| `\tableinfo <table>` | 查看表元数据详情 |

示例：

```text
\l
\connect app_db
\dt
\d orders
\di orders
\size orders
\tablesize log
\tableinfo orders
```

## 7. 会话、锁和复制

活跃会话：

```text
\sessions
\sessions --all
\sessions --min-seconds 10
\sessions --user app
\ps
```

终止查询或连接：

```text
\kill 12345
\kill --connection 12345
```

锁等待：

```text
\locks
\locks --all
\locks --tree
```

复制状态：

```text
\repl
\repl status
\repl channels
\repl errors
\repl source
```

说明：这些命令依赖 MySQL 元数据表和权限。权限不足时会显示错误原因，不会修改数据库状态。

## 8. 变量、告警和状态

查看服务器变量：

```text
\variables
\variables innodb
\variables buffer_pool
\variables --session sql_mode
```

查看最近 SQL 告警：

```text
\warnings
```

切换每条语句后自动显示告警：

```text
\W
```

查看连接和健康状态快照：

```text
\status
\s
```

`\status` 会显示连接 ID、服务器版本、当前用户、当前数据库，并补充 `Uptime`、`Questions`、`Slow queries`、打开表数量、线程连接数和每秒查询数等轻量健康指标。

## 9. DBA 诊断命令

InnoDB 状态摘要：

```text
\innodb
```

最近死锁信息：

```text
\deadlocks
```

慢日志配置和最近慢查询：

```text
\slowlog
\slowlog 10
```

二进制日志摘要：

```text
\binlog
```

字符集和排序规则：

```text
\charset
```

说明：

- `\innodb` 和 `\deadlocks` 基于 `SHOW ENGINE INNODB STATUS`，通常需要 `PROCESS` 权限。
- `\slowlog` 默认只展示慢日志相关配置；带数字时会读取 `mysql.slow_log` 最近 N 条记录，要求 MySQL 慢日志输出包含 `TABLE` 且当前用户有读取权限。
- `\binlog` 基于 `SHOW BINARY LOGS`，未启用 binlog 或权限不足时会返回 MySQL 原始错误。

## 10. 用户和权限

| 命令 | 说明 |
| --- | --- |
| `\du [pattern]` | 查看用户 |
| `\user <user@host>` | 查看用户详情 |
| `\grants [user@host]` | 查看授权 |
| `\roles [user@host]` | 查看角色 |
| `\whoami` | 查看当前身份 |
| `\privileges` | 查看权限类型 |

示例：

```text
\du
\user app@%
\grants app@%
\whoami
```

## 11. 自定义快捷命令

可以在配置文件中增加 `[command.<name>]` 分组。自定义命令可以新增命令，也可以覆盖内置命令。

字段：

| 字段 | 说明 |
| --- | --- |
| `description` | 命令说明，会显示在 `\?` 中 |
| `sql` | 要执行的 SQL |

自定义 SQL 中的 `?` 会按命令参数顺序绑定，参数数量必须一致。

### 示例：查看大表

```ini
[command.bigtable]
description = 查看当前库最大的 20 张表
sql = SELECT TABLE_NAME AS TableName, ROUND((DATA_LENGTH + INDEX_LENGTH) / 1024 / 1024, 2) AS TotalMB FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() ORDER BY DATA_LENGTH + INDEX_LENGTH DESC LIMIT 20
```

使用：

```text
\bigtable
```

### 示例：查看指定表空间

```ini
[command.table-size]
description = 查看指定表空间
sql = SELECT TABLE_NAME AS TableName, ENGINE AS Engine, ROUND((DATA_LENGTH + INDEX_LENGTH) / 1024 / 1024, 2) AS TotalMB, ROUND(DATA_FREE / 1024 / 1024, 2) AS FreeMB FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?
```

使用：

```text
\table-size orders
```

### 示例：覆盖内置命令

下面示例会把 `\du <table>` 改成查看指定表大小。`du` 只是示例，机制本身支持任意命令名。

```ini
[command.du]
description = 查看表空间
sql = SELECT TABLE_NAME AS TableName, ROUND((DATA_LENGTH + INDEX_LENGTH) / 1024 / 1024, 2) AS TotalMB FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?
```

使用：

```text
\du orders
```

## 12. 常见问题

### `-p` 在脚本中失败

`-p` 需要交互式终端。脚本中建议使用配置文件：

```shell
mysqlcli --defaults-file /root/.my.cnf -e "SELECT 1"
```

### 诊断命令提示权限不足

`\sessions`、`\locks`、`\repl`、`\du` 等命令可能需要访问 `information_schema`、`performance_schema` 或 `mysql` 系统表。请使用有相应权限的账号执行。

### 自定义命令参数数量错误

如果 SQL 中有 2 个 `?`，命令就必须传入 2 个参数：

```ini
[command.find-table]
sql = SELECT TABLE_SCHEMA, TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
```

```text
\find-table app_db orders
```

## 13. 版本验证

查看版本：

```shell
mysqlcli --version
```

查看帮助：

```shell
mysqlcli --help
```

Linux 文件校验：

```shell
sha256sum mysqlcli-linux-amd64
```

当前 Linux x86_64 构建校验值：

```text
dc0f9611251cf7a75520152153e6a5fde5528b9f3da48376617386c7ddfe0d64
```
