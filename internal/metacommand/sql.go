package metacommand

const databaseListSQL = `SELECT SCHEMA_NAME AS DatabaseName, DEFAULT_CHARACTER_SET_NAME AS CharacterSet, DEFAULT_COLLATION_NAME AS CollationName
FROM information_schema.SCHEMATA
WHERE SCHEMA_NAME LIKE ?
ORDER BY SCHEMA_NAME`

const objectListSQL = `SELECT TABLE_NAME AS ObjectName, TABLE_TYPE AS ObjectType, ENGINE AS Engine, TABLE_ROWS AS EstimatedRows
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME LIKE ?
ORDER BY TABLE_TYPE, TABLE_NAME`

const describeObjectSQL = `SELECT ORDINAL_POSITION AS Position, COLUMN_NAME AS Field, COLUMN_TYPE AS Type, IS_NULLABLE AS Nullable,
       COLUMN_KEY AS KeyName, COLUMN_DEFAULT AS DefaultValue, EXTRA AS Extra
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?
ORDER BY ORDINAL_POSITION`

const tableListSQL = `SELECT TABLE_NAME AS TableName, ENGINE AS Engine, TABLE_ROWS AS EstimatedRows, DATA_LENGTH AS DataBytes
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_TYPE = 'BASE TABLE' AND TABLE_NAME LIKE ?
ORDER BY TABLE_NAME`

const viewListSQL = `SELECT TABLE_NAME AS ViewName, IS_UPDATABLE AS IsUpdatable, DEFINER AS Definer, SECURITY_TYPE AS SecurityType
FROM information_schema.VIEWS
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME LIKE ?
ORDER BY TABLE_NAME`

const indexListSQL = `SELECT TABLE_NAME AS TableName, INDEX_NAME AS IndexName, NON_UNIQUE AS NonUnique,
       SEQ_IN_INDEX AS SequenceNumber, COLUMN_NAME AS ColumnName, INDEX_TYPE AS IndexType
FROM information_schema.STATISTICS
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME LIKE ?
ORDER BY TABLE_NAME, INDEX_NAME, SEQ_IN_INDEX`

const foreignKeySQL = `SELECT k.CONSTRAINT_NAME AS ConstraintName,
       k.COLUMN_NAME AS ColumnName,
       k.REFERENCED_TABLE_SCHEMA AS ReferencedSchema,
       k.REFERENCED_TABLE_NAME AS ReferencedTable,
       k.REFERENCED_COLUMN_NAME AS ReferencedColumn,
       rc.UPDATE_RULE AS UpdateRule,
       rc.DELETE_RULE AS DeleteRule
FROM information_schema.KEY_COLUMN_USAGE AS k
LEFT JOIN information_schema.REFERENTIAL_CONSTRAINTS AS rc
  ON rc.CONSTRAINT_SCHEMA = k.CONSTRAINT_SCHEMA
 AND rc.CONSTRAINT_NAME = k.CONSTRAINT_NAME
WHERE k.TABLE_SCHEMA = DATABASE()
  AND k.TABLE_NAME = ?
  AND k.REFERENCED_TABLE_NAME IS NOT NULL
ORDER BY k.CONSTRAINT_NAME, k.ORDINAL_POSITION`

const tableSizeSQL = `SELECT TABLE_NAME AS TableName, ENGINE AS Engine,
       TABLE_ROWS AS EstimatedRows,
       ROUND(DATA_LENGTH / 1024 / 1024, 2) AS DataMB,
       ROUND(INDEX_LENGTH / 1024 / 1024, 2) AS IndexMB,
       ROUND((DATA_LENGTH + INDEX_LENGTH) / 1024 / 1024, 2) AS TotalMB,
       ROUND(DATA_FREE / 1024 / 1024, 2) AS FreeMB
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_TYPE = 'BASE TABLE' AND TABLE_NAME LIKE ?
ORDER BY (DATA_LENGTH + INDEX_LENGTH) DESC, TABLE_NAME`

const tableInfoSQL = `SELECT t.TABLE_SCHEMA AS ` + "`TableSchema`" + `,
       t.TABLE_NAME AS ` + "`TableName`" + `,
       t.ENGINE AS ` + "`Engine`" + `,
       t.ROW_FORMAT AS ` + "`RowFormat`" + `,
       t.TABLE_ROWS AS ` + "`EstimatedRows`" + `,
       ROUND(t.DATA_LENGTH / 1024 / 1024, 2) AS ` + "`DataMB`" + `,
       ROUND(t.INDEX_LENGTH / 1024 / 1024, 2) AS ` + "`IndexMB`" + `,
       ROUND((t.DATA_LENGTH + t.INDEX_LENGTH) / 1024 / 1024, 2) AS ` + "`TotalMB`" + `,
       ROUND(t.DATA_FREE / 1024 / 1024, 2) AS ` + "`FreeMB`" + `,
       t.AUTO_INCREMENT AS ` + "`AutoIncrement`" + `,
       t.TABLE_COLLATION AS ` + "`TableCollation`" + `,
       t.CREATE_TIME AS ` + "`CreateTime`" + `,
       t.UPDATE_TIME AS ` + "`UpdateTime`" + `,
       t.TABLE_COMMENT AS ` + "`TableComment`" + `
FROM information_schema.TABLES AS t
WHERE t.TABLE_SCHEMA = DATABASE() AND t.TABLE_NAME = ?`

const routineListSQL = `SELECT ROUTINE_NAME AS RoutineName, ROUTINE_TYPE AS RoutineType, DATA_TYPE AS ReturnType,
       DEFINER AS Definer, SECURITY_TYPE AS SecurityType
FROM information_schema.ROUTINES
WHERE ROUTINE_SCHEMA = DATABASE() AND ROUTINE_NAME LIKE ?
ORDER BY ROUTINE_TYPE, ROUTINE_NAME`

const triggerListSQL = `SELECT TRIGGER_NAME AS TriggerName, EVENT_MANIPULATION AS EventName, EVENT_OBJECT_TABLE AS TableName,
       ACTION_TIMING AS Timing, DEFINER AS Definer
FROM information_schema.TRIGGERS
WHERE TRIGGER_SCHEMA = DATABASE() AND TRIGGER_NAME LIKE ?
ORDER BY EVENT_OBJECT_TABLE, TRIGGER_NAME`

const eventListSQL = `SELECT EVENT_NAME AS EventName, STATUS AS Status, EVENT_TYPE AS EventType,
       EXECUTE_AT AS ExecuteAt, INTERVAL_VALUE AS IntervalValue, INTERVAL_FIELD AS IntervalField
FROM information_schema.EVENTS
WHERE EVENT_SCHEMA = DATABASE() AND EVENT_NAME LIKE ?
ORDER BY EVENT_NAME`

const sessionsSQL = `SELECT ID AS ConnectionId, USER AS UserName, HOST AS ClientHost, DB AS DatabaseName,
       COMMAND AS CommandName, TIME AS DurationSeconds, STATE AS StateName, LEFT(INFO, 200) AS SqlText
FROM information_schema.PROCESSLIST
WHERE ID <> CONNECTION_ID()
  AND (? OR COMMAND <> 'Sleep')
  AND TIME >= ?
  AND (? = '' OR USER = ?)
ORDER BY TIME DESC, ID`

const globalVariablesSQL = `SELECT VARIABLE_NAME AS VariableName, VARIABLE_VALUE AS VariableValue
FROM performance_schema.global_variables
WHERE VARIABLE_NAME LIKE ?
ORDER BY VARIABLE_NAME`

const sessionVariablesSQL = `SELECT VARIABLE_NAME AS VariableName, VARIABLE_VALUE AS VariableValue
FROM performance_schema.session_variables
WHERE VARIABLE_NAME LIKE ?
ORDER BY VARIABLE_NAME`

const statusSummarySQL = `SELECT
       MAX(CASE VARIABLE_NAME WHEN 'Uptime' THEN VARIABLE_VALUE END) AS UptimeSeconds,
       MAX(CASE VARIABLE_NAME WHEN 'Questions' THEN VARIABLE_VALUE END) AS Questions,
       MAX(CASE VARIABLE_NAME WHEN 'Slow_queries' THEN VARIABLE_VALUE END) AS SlowQueries,
       MAX(CASE VARIABLE_NAME WHEN 'Opened_tables' THEN VARIABLE_VALUE END) AS OpenedTables,
       MAX(CASE VARIABLE_NAME WHEN 'Open_tables' THEN VARIABLE_VALUE END) AS OpenTables,
       MAX(CASE VARIABLE_NAME WHEN 'Flush_commands' THEN VARIABLE_VALUE END) AS FlushCommands,
       MAX(CASE VARIABLE_NAME WHEN 'Threads_connected' THEN VARIABLE_VALUE END) AS ThreadsConnected,
       ROUND(
           CAST(MAX(CASE VARIABLE_NAME WHEN 'Questions' THEN VARIABLE_VALUE END) AS DECIMAL(20, 4)) /
           NULLIF(CAST(MAX(CASE VARIABLE_NAME WHEN 'Uptime' THEN VARIABLE_VALUE END) AS DECIMAL(20, 4)), 0),
           4
       ) AS QuestionsPerSecond
FROM performance_schema.global_status
WHERE VARIABLE_NAME IN ('Uptime', 'Questions', 'Slow_queries', 'Opened_tables', 'Open_tables', 'Flush_commands', 'Threads_connected')`

const charsetSQL = `SELECT VARIABLE_NAME AS VariableName, VARIABLE_VALUE AS VariableValue
FROM performance_schema.session_variables
WHERE VARIABLE_NAME IN (
    'character_set_client',
    'character_set_connection',
    'character_set_database',
    'character_set_results',
    'character_set_server',
    'collation_connection',
    'collation_database',
    'collation_server'
)
ORDER BY VARIABLE_NAME`

const slowLogVariablesSQL = `SELECT VARIABLE_NAME AS VariableName, VARIABLE_VALUE AS VariableValue
FROM performance_schema.global_variables
WHERE VARIABLE_NAME IN ('slow_query_log', 'slow_query_log_file', 'long_query_time', 'log_output')
ORDER BY VARIABLE_NAME`

const slowLogRecentSQL = `SELECT start_time AS StartTime,
       user_host AS UserHost,
       query_time AS QueryTime,
       lock_time AS LockTime,
       rows_sent AS RowsSent,
       rows_examined AS RowsExamined,
       LEFT(sql_text, 200) AS SqlText
FROM mysql.slow_log
ORDER BY start_time DESC
LIMIT ?`

const lockWaits8CapabilitySQL = `SELECT COUNT(*) AS TableCount
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = 'performance_schema'
  AND TABLE_NAME IN ('data_locks', 'data_lock_waits')`

const allLocks8CapabilitySQL = `SELECT COUNT(*) AS TableCount
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = 'performance_schema'
  AND TABLE_NAME = 'data_locks'`

const lockWaits57CapabilitySQL = `SELECT COUNT(*) AS TableCount
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = 'information_schema'
  AND TABLE_NAME IN ('INNODB_LOCK_WAITS', 'INNODB_TRX')`

const allLocks57CapabilitySQL = `SELECT COUNT(*) AS TableCount
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = 'information_schema'
  AND TABLE_NAME = 'INNODB_LOCKS'`

const locks8SQL = `SELECT waits.REQUESTING_ENGINE_TRANSACTION_ID AS WaitingTransaction,
       waiting.THREAD_ID AS WaitingThread, waiting.OBJECT_SCHEMA AS ObjectSchema,
       waiting.OBJECT_NAME AS ObjectName, waiting.LOCK_TYPE AS WaitingLockType,
       waiting.LOCK_MODE AS WaitingLockMode,
       blocking.ENGINE_TRANSACTION_ID AS BlockingTransaction,
       blocking.THREAD_ID AS BlockingThread, blocking.LOCK_MODE AS BlockingLockMode
FROM performance_schema.data_lock_waits waits
JOIN performance_schema.data_locks waiting ON waiting.ENGINE_LOCK_ID = waits.REQUESTING_ENGINE_LOCK_ID
JOIN performance_schema.data_locks blocking ON blocking.ENGINE_LOCK_ID = waits.BLOCKING_ENGINE_LOCK_ID`

const locks57SQL = `SELECT waits.requesting_trx_id AS WaitingTransaction,
       waiting.trx_mysql_thread_id AS WaitingConnection,
       waiting.trx_wait_started AS WaitStarted,
       waits.blocking_trx_id AS BlockingTransaction,
       blocking.trx_mysql_thread_id AS BlockingConnection,
       LEFT(waiting.trx_query, 200) AS WaitingSql,
       LEFT(blocking.trx_query, 200) AS BlockingSql
FROM information_schema.innodb_lock_waits waits
JOIN information_schema.innodb_trx waiting ON waiting.trx_id = waits.requesting_trx_id
JOIN information_schema.innodb_trx blocking ON blocking.trx_id = waits.blocking_trx_id`

const allLocks8SQL = `SELECT ENGINE_TRANSACTION_ID AS TransactionId, THREAD_ID AS ThreadId,
       OBJECT_SCHEMA AS ObjectSchema, OBJECT_NAME AS ObjectName,
       LOCK_TYPE AS LockType, LOCK_MODE AS LockMode, LOCK_STATUS AS LockStatus,
       LOCK_DATA AS LockData
FROM performance_schema.data_locks
ORDER BY OBJECT_SCHEMA, OBJECT_NAME, ENGINE_TRANSACTION_ID`

const allLocks57SQL = `SELECT lock_trx_id AS TransactionId, lock_mode AS LockMode,
       lock_type AS LockType, lock_table AS ObjectName, lock_index AS IndexName,
       lock_space AS SpaceId, lock_page AS PageId, lock_rec AS RecordId, lock_data AS LockData
FROM information_schema.innodb_locks
ORDER BY lock_table, lock_trx_id`

const lockTree8SQL = `SELECT CONCAT('BLOCKER ', waits.BLOCKING_THREAD_ID) AS BlockingNode,
       CONCAT('  -> WAITER ', waits.REQUESTING_THREAD_ID) AS WaitingNode,
       waiting.OBJECT_SCHEMA AS ObjectSchema, waiting.OBJECT_NAME AS ObjectName,
       blocking.LOCK_MODE AS BlockingMode, waiting.LOCK_MODE AS WaitingMode
FROM performance_schema.data_lock_waits waits
JOIN performance_schema.data_locks waiting ON waiting.ENGINE_LOCK_ID = waits.REQUESTING_ENGINE_LOCK_ID
JOIN performance_schema.data_locks blocking ON blocking.ENGINE_LOCK_ID = waits.BLOCKING_ENGINE_LOCK_ID
ORDER BY blocking.THREAD_ID, waiting.THREAD_ID`

const lockTree57SQL = `SELECT CONCAT('BLOCKER ', blocking.trx_mysql_thread_id) AS BlockingNode,
       CONCAT('  -> WAITER ', waiting.trx_mysql_thread_id) AS WaitingNode,
       waits.blocking_trx_id AS BlockingTransaction, waits.requesting_trx_id AS WaitingTransaction,
       LEFT(blocking.trx_query, 200) AS BlockingSql, LEFT(waiting.trx_query, 200) AS WaitingSql
FROM information_schema.innodb_lock_waits waits
JOIN information_schema.innodb_trx waiting ON waiting.trx_id = waits.requesting_trx_id
JOIN information_schema.innodb_trx blocking ON blocking.trx_id = waits.blocking_trx_id
ORDER BY blocking.trx_mysql_thread_id, waiting.trx_mysql_thread_id`

const userList8SQL = `SELECT User AS UserName, Host AS HostName, plugin AS AuthenticationPlugin,
       account_locked AS AccountLocked, password_expired AS PasswordExpired
FROM mysql.user
WHERE CONCAT(User, '@', Host) LIKE ?
ORDER BY User, Host`

const userList57SQL = `SELECT User AS UserName, Host AS HostName, plugin AS AuthenticationPlugin,
       password_expired AS PasswordExpired
FROM mysql.user
WHERE CONCAT(User, '@', Host) LIKE ?
ORDER BY User, Host`

const userDetail8SQL = `SELECT User AS UserName, Host AS HostName, plugin AS AuthenticationPlugin,
       account_locked AS AccountLocked, password_expired AS PasswordExpired,
       password_last_changed AS PasswordLastChanged
FROM mysql.user
WHERE User = ? AND Host = ?`

const userDetail57SQL = `SELECT User AS UserName, Host AS HostName, plugin AS AuthenticationPlugin,
       password_expired AS PasswordExpired
FROM mysql.user
WHERE User = ? AND Host = ?`

const currentRolesSQL = `SELECT CURRENT_ROLE() AS CurrentRoles`

const roleEdgesSQL = `SELECT FROM_USER AS RoleName, FROM_HOST AS RoleHost, TO_USER AS UserName, TO_HOST AS UserHost,
       WITH_ADMIN_OPTION AS AdminOption
FROM mysql.role_edges
WHERE TO_USER = ? AND TO_HOST = ?
ORDER BY FROM_USER, FROM_HOST`

const whoAmI8SQL = `SELECT USER() AS LoginIdentity, CURRENT_USER() AS PrivilegeIdentity, CURRENT_ROLE() AS CurrentRoles, DATABASE() AS CurrentDatabase`

const whoAmI57SQL = `SELECT USER() AS LoginIdentity, CURRENT_USER() AS PrivilegeIdentity, DATABASE() AS CurrentDatabase`

const replicationChannelsSQL = `SELECT CHANNEL_NAME AS ChannelName, HOST AS SourceHost, PORT AS SourcePort,
       USER AS ReplicationUser, NETWORK_INTERFACE AS NetworkInterface,
       AUTO_POSITION AS AutoPosition, SSL_ALLOWED AS SSLAllowed
FROM performance_schema.replication_connection_configuration
ORDER BY CHANNEL_NAME`
