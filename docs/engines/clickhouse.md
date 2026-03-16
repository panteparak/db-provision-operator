# ClickHouse

Complete guide for using DB Provision Operator with ClickHouse.

## Supported Versions

- ClickHouse 23.x
- ClickHouse 24.x

## ClickHouse Protocol

ClickHouse uses its own native protocol (default port 9000) for high-performance communication. The operator uses the `clickhouse-go/v2` driver with LZ4 compression and connection pooling.

ClickHouse has important differences from traditional RDBMS:

| Feature | PostgreSQL/MySQL | ClickHouse |
|---------|-----------------|------------|
| Storage Model | Row-oriented | Column-oriented |
| Primary Use | OLTP | OLAP/Analytics |
| Transactions | Full ACID | Limited |
| Object Ownership | Yes | No |
| Default Privileges | Yes | No |
| Extensions | Yes (PG) | No |
| Schemas | Yes (PG) | No |
| ALTER DATABASE | Yes | No |
| Backup | pg_dump/mysqldump | SQL-based (limited) |

!!! warning "Unsupported Features"
    ClickHouse does not support ALTER DATABASE, object ownership, default privileges, extensions, or schemas. The operator will ignore any PostgreSQL/MySQL-specific options when using ClickHouse.

## Admin Account Requirements

The operator requires a privileged account to manage databases, users, roles, and grants in ClickHouse.

```sql
-- Create the admin user
CREATE USER IF NOT EXISTS dbprovision_admin
  IDENTIFIED WITH sha256_password BY 'your-secure-password';

-- Grant all necessary privileges
GRANT CREATE USER, ALTER USER, DROP USER ON *.* TO dbprovision_admin WITH GRANT OPTION;
GRANT CREATE ROLE, ALTER ROLE, DROP ROLE ON *.* TO dbprovision_admin WITH GRANT OPTION;
GRANT CREATE DATABASE, DROP DATABASE ON *.* TO dbprovision_admin;
GRANT ALL ON *.* TO dbprovision_admin WITH GRANT OPTION;

-- Verify the setup
SHOW GRANTS FOR dbprovision_admin;
```

### Privilege Matrix

| Operation | Required Privileges | SQL to Grant |
|-----------|-------------------|--------------|
| **Create Database** | `CREATE DATABASE` | `GRANT CREATE DATABASE ON *.* TO ...` |
| **Drop Database** | `DROP DATABASE` | `GRANT DROP DATABASE ON *.* TO ...` |
| **Create User** | `CREATE USER` | `GRANT CREATE USER ON *.* TO ...` |
| **Drop User** | `DROP USER` | `GRANT DROP USER ON *.* TO ...` |
| **Create Role** | `CREATE ROLE` | `GRANT CREATE ROLE ON *.* TO ...` |
| **Drop Role** | `DROP ROLE` | `GRANT DROP ROLE ON *.* TO ...` |
| **Grant Privileges** | `WITH GRANT OPTION` | `GRANT ... WITH GRANT OPTION` |

## DatabaseInstance

### Basic Configuration

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: clickhouse-cluster
  namespace: database
spec:
  engine: clickhouse
  connection:
    host: clickhouse.database.svc.cluster.local
    port: 9000
    database: default
    secretRef:
      name: clickhouse-admin-credentials
```

### Secure Mode with TLS

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: clickhouse-secure
  namespace: database
spec:
  engine: clickhouse
  connection:
    host: clickhouse.database.svc.cluster.local
    port: 9440  # TLS native port
    database: default
    secretRef:
      name: clickhouse-admin-credentials
  clickhouse:
    secure: true
  tls:
    secretRef:
      name: clickhouse-tls
      keys:
        ca: ca.crt
        cert: tls.crt
        key: tls.key
```

### ClickHouse Instance Options

| Field | Type | Description |
|-------|------|-------------|
| `clickhouse.httpPort` | int | HTTP interface port (optional, for HTTP alongside native) |
| `clickhouse.secure` | bool | Enable TLS for native protocol connections |

### Credentials Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: clickhouse-admin-credentials
  namespace: database
type: Opaque
stringData:
  username: dbprovision_admin
  password: your-secure-password
```

## Database

### Basic Configuration

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: analytics-db
  namespace: database
spec:
  instanceRef:
    name: clickhouse-cluster
  name: analytics
```

### Database with Engine

ClickHouse supports multiple database engines:

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: analytics-db
spec:
  instanceRef:
    name: clickhouse-cluster
  name: analytics
  clickhouse:
    engine: Replicated  # Atomic (default), Lazy, Replicated
  deletionPolicy: Retain
  deletionProtection: true
```

### Database Engine Options

| Engine | Description |
|--------|-------------|
| `Atomic` | Default engine. Supports non-blocking DROP/RENAME and atomic table swaps. |
| `Lazy` | Keeps tables in memory only for `expiration_time_in_seconds` after last access. Intended for `*Log` tables only. |
| `Replicated` | Based on Atomic but with DDL replication across replicas via ZooKeeper/ClickHouse Keeper. |

!!! note "No ALTER DATABASE"
    ClickHouse does not support ALTER DATABASE. The database engine cannot be changed after creation. To change the engine, delete and recreate the database.

## DatabaseUser

### Basic User

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: analytics-user
  namespace: database
spec:
  instanceRef:
    name: clickhouse-cluster
  username: analytics_user
  passwordSecret:
    generate: true
    length: 32
    secretName: analytics-user-credentials
```

### User with Allowed Hosts

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: restricted-user
spec:
  instanceRef:
    name: clickhouse-cluster
  username: restricted_user
  passwordSecret:
    generate: true
    secretName: restricted-user-credentials
  clickhouse:
    allowedHosts:
      - "10.0.0.%"
      - "192.168.1.0/24"
    defaultDatabase: analytics
```

### ClickHouse User Options

| Field | Type | Description |
|-------|------|-------------|
| `clickhouse.allowedHosts` | array | IP addresses, hostnames, or LIKE patterns. Empty = anywhere. |
| `clickhouse.defaultDatabase` | string | Default database for the user |

!!! warning "No User Settings via CRD"
    ClickHouse user settings (max_memory_usage, max_execution_time, etc.) are managed via ClickHouse configuration files, not through the operator. The operator manages user creation, passwords, and host restrictions.

## DatabaseRole

### Basic Role

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseRole
metadata:
  name: analytics-reader
spec:
  instanceRef:
    name: clickhouse-cluster
  roleName: analytics_reader
```

### Role with Settings

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseRole
metadata:
  name: analytics-writer
spec:
  instanceRef:
    name: clickhouse-cluster
  roleName: analytics_writer
  clickhouse:
    settings:
      max_memory_usage: "10000000000"
      max_execution_time: "60"
```

### ClickHouse Role Options

| Field | Type | Description |
|-------|------|-------------|
| `clickhouse.settings` | map[string]string | ClickHouse settings to apply to the role |

## DatabaseGrant

### Grant with Roles

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseGrant
metadata:
  name: analytics-user-roles
spec:
  userRef:
    name: analytics-user
  clickhouse:
    roles:
      - analytics_reader
      - analytics_writer
```

### Grant with Direct Privileges

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseGrant
metadata:
  name: analytics-user-grants
spec:
  userRef:
    name: analytics-user
  clickhouse:
    grants:
      - level: database
        database: analytics
        privileges: [SELECT, INSERT]
      - level: table
        database: analytics
        table: events
        privileges: [SELECT, INSERT, ALTER]
        withGrantOption: true
```

### Grant Levels

| Level | Target | Example |
|-------|--------|---------|
| `global` | All databases and tables (`*.*`) | Server-wide admin access |
| `database` | All tables in a database (`db.*`) | Database-level read/write |
| `table` | Specific table (`db.table`) | Table-level access |

### Grant Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `level` | string | Yes | `global`, `database`, or `table` |
| `database` | string | For database/table | Target database name |
| `table` | string | For table | Target table name |
| `privileges` | array | Yes | Privileges to grant (1-20 items) |
| `withGrantOption` | bool | No | Allow grantee to grant to others |

## ClickHouse Privileges

The operator validates privileges against an allowlist:

| Category | Privileges |
|----------|-----------|
| **Data Access** | `SELECT`, `INSERT` |
| **DDL** | `ALTER`, `CREATE`, `DROP`, `TRUNCATE` |
| **Show** | `SHOW`, `SHOW DATABASES`, `SHOW TABLES`, `SHOW DICTIONARIES`, `SHOW COLUMNS` |
| **Optimize** | `OPTIMIZE` |
| **Database** | `CREATE DATABASE`, `DROP DATABASE` |
| **Table** | `CREATE TABLE`, `DROP TABLE`, `ALTER TABLE` |
| **View** | `CREATE VIEW`, `DROP VIEW`, `ALTER VIEW` |
| **Dictionary** | `CREATE DICTIONARY`, `DROP DICTIONARY` |
| **Function** | `CREATE FUNCTION`, `DROP FUNCTION` |
| **Other** | `CREATE TEMPORARY TABLE`, `KILL QUERY`, `SYSTEM` |
| **Wildcard** | `ALL` |

!!! tip "Privilege Naming"
    Privilege names are case-insensitive in the CRD but must match the pattern `^[A-Z][A-Z ]*$`. Multi-word privileges use spaces (e.g., `CREATE TABLE`).

## Backup and Restore

!!! warning "Limited Support"
    ClickHouse backup and restore through the operator is not yet fully supported. The adapter returns "not yet supported" for backup/restore operations.

For ClickHouse backups, consider using:

- [clickhouse-backup](https://github.com/Altinity/clickhouse-backup) — dedicated ClickHouse backup tool
- `BACKUP` / `RESTORE` SQL commands (ClickHouse 23.3+)
- File-level backups of ClickHouse data directories

## Limitations

| Limitation | Description |
|------------|-------------|
| No ALTER DATABASE | Database engine cannot be changed after creation |
| No Object Ownership | ClickHouse does not have PostgreSQL-style object ownership |
| No Default Privileges | Cannot set privileges for future objects automatically |
| No Extensions | ClickHouse has no extension system |
| No Schemas | ClickHouse does not support schemas within databases |
| No User Settings via CRD | User settings are managed via ClickHouse config files |
| Limited Backup | Backup/restore not yet implemented in the adapter |
| UpdateUser No-op | User attribute updates (except password) are no-ops |
| UpdateDatabase No-op | Database updates after creation are no-ops |

## Best Practices

### Security

1. **Always use TLS** in production with `clickhouse.secure: true`
2. **Restrict allowed hosts** for users with `clickhouse.allowedHosts`
3. **Use roles for permission grouping** rather than direct user grants
4. **Set a default database** to limit scope with `clickhouse.defaultDatabase`

### Performance

1. **Use connection pooling** — the adapter configures 25 max connections, 5 idle
2. **LZ4 compression** is enabled by default for native protocol connections
3. **Use the native protocol** (port 9000) for best performance, not HTTP

### Operations

1. **Use Replicated engine** for databases that need DDL replication
2. **Monitor via ClickHouse system tables** (`system.query_log`, `system.processes`)
3. **Use external backup tools** until operator backup support is complete

## Troubleshooting

### Connection Issues

```bash
# Test connectivity to ClickHouse native port
kubectl run ch-test --rm -it --image=clickhouse/clickhouse-client -- \
  --host=clickhouse --port=9000 --user=default --query="SELECT 1"

# For TLS connections
kubectl run ch-test --rm -it --image=clickhouse/clickhouse-client -- \
  --host=clickhouse --port=9440 --secure --user=admin --password=pass \
  --query="SELECT 1"
```

### Permission Denied

```sql
-- Check user privileges
SHOW GRANTS FOR analytics_user;

-- Check role memberships
SELECT * FROM system.role_grants WHERE user_name = 'analytics_user';

-- Grant missing privileges
GRANT SELECT ON analytics.* TO analytics_user;
```

### User Cannot Connect

```sql
-- Check allowed hosts for a user
SELECT name, host_ip, host_names, host_names_like
FROM system.users
WHERE name = 'restricted_user';
```

### Database Creation Fails

```sql
-- Check existing databases
SHOW DATABASES;

-- Verify admin privileges
SHOW GRANTS FOR dbprovision_admin;
```

### Common Errors

| Error | Cause | Solution |
|-------|-------|----------|
| `Authentication failed` | Wrong credentials | Verify Secret username/password |
| `Connection refused` | Wrong port or host | Check port 9000 (native) or 9440 (TLS) |
| `Code: 497` | Insufficient privileges | Grant required privileges to admin user |
| `Code: 81` | Database already exists | Expected on re-reconciliation, handled by operator |
| `Code: 60` | Table/database not found | Verify the target exists |
| `UNKNOWN_USER` | User does not exist | Check user was created successfully |
