# MySQL

Complete guide for using DB Provision Operator with MySQL.

## Supported Versions

- MySQL 5.7.x
- MySQL 8.0.x
- MySQL 8.4.x

## Admin Account Requirements

The operator requires a privileged database account to manage databases, users, and grants. For production security, create a dedicated **least-privilege admin account** instead of using `root`.

!!! warning "Never Use Root or ALL PRIVILEGES"
    The operator is designed to work **without SUPER privilege or ALL PRIVILEGES**. Using root or granting ALL PRIVILEGES violates the principle of least privilege and creates security risks.

### Privilege Matrix

This table shows the exact privileges required for each operator action:

| Operation | Required Privileges | SQL to Grant |
|-----------|-------------------|--------------|
| **Create Database** | `CREATE ON *.*` | `GRANT CREATE ON *.* TO ...` |
| **Drop Database** | `DROP ON *.*` | `GRANT DROP ON *.* TO ...` |
| **Alter Database** | `ALTER ON *.*` | `GRANT ALTER ON *.* TO ...` |
| **Force Drop Database** | `DROP ON *.*`, `CONNECTION_ADMIN ON *.*` | See setup below |
| **Create User** | `CREATE USER ON *.*` | `GRANT CREATE USER ON *.* TO ...` |
| **Drop User** | `CREATE USER ON *.*` | Same grant handles both operations |
| **Alter User** | `CREATE USER ON *.*` | Same grant handles all user operations |
| **Grant Privileges** | `GRANT OPTION ON *.*` | `GRANT GRANT OPTION ON *.* TO ...` |
| **Create Role** (8.0+) | `ROLE_ADMIN ON *.*` | `GRANT ROLE_ADMIN ON *.* TO ...` |
| **Grant Role** (8.0+) | `ROLE_ADMIN ON *.*` | Same grant handles role operations |
| **Query User Info** | `SELECT ON mysql.*` | `GRANT SELECT ON mysql.user TO ...` |
| **Apply Changes** | `RELOAD ON *.*` | `GRANT RELOAD ON *.* TO ...` |
| **Kill Connections** | `CONNECTION_ADMIN ON *.*` | `GRANT CONNECTION_ADMIN ON *.* TO ...` |
| **View Processes** | `PROCESS ON *.*` | `GRANT PROCESS ON *.* TO ...` |

### Understanding Each Privilege

| Privilege | What It Allows | What Happens Without It |
|-----------|----------------|------------------------|
| `CREATE` | Create databases and tables | `CREATE DATABASE` fails |
| `DROP` | Drop databases and tables | `DROP DATABASE` fails |
| `ALTER` | Modify database/table structure | `ALTER DATABASE` fails |
| `CREATE USER` | Create, drop, and alter users | `CREATE USER` fails |
| `GRANT OPTION` | Grant/revoke privileges to others | Cannot delegate privileges to app users |
| `SELECT ON mysql.*` | Read user/grant metadata | Cannot verify existing users/grants |
| `RELOAD` | Execute FLUSH statements | `FLUSH PRIVILEGES` fails |
| `CONNECTION_ADMIN` | Kill user connections | Force-drop fails with active connections |
| `ROLE_ADMIN` | Manage roles (MySQL 8.0+) | `CREATE ROLE` fails |
| `PROCESS` | View server process list | Cannot monitor connections |

### Complete Setup Script (Copy-Paste)

```sql
-- =============================================================
-- MySQL 8.0+ Least-Privilege Admin Account Setup
-- Compatible with: MySQL 8.0.x, 8.4.x
-- =============================================================

-- 1. Create the operator admin user
CREATE USER 'dbprovision_admin'@'%' IDENTIFIED BY 'your-secure-password';

-- 2. Database operations (create, drop, alter databases)
GRANT CREATE, DROP, ALTER ON *.* TO 'dbprovision_admin'@'%';

-- 3. User management (create, drop, alter users)
GRANT CREATE USER ON *.* TO 'dbprovision_admin'@'%';

-- 4. Privilege delegation (grant privileges to app users)
GRANT GRANT OPTION ON *.* TO 'dbprovision_admin'@'%';

-- 5. Query user and grant metadata from system tables
GRANT SELECT ON mysql.user TO 'dbprovision_admin'@'%';
GRANT SELECT ON mysql.db TO 'dbprovision_admin'@'%';
GRANT SELECT ON mysql.tables_priv TO 'dbprovision_admin'@'%';
GRANT SELECT ON mysql.columns_priv TO 'dbprovision_admin'@'%';
GRANT SELECT ON mysql.procs_priv TO 'dbprovision_admin'@'%';
GRANT SELECT ON mysql.global_grants TO 'dbprovision_admin'@'%';

-- 6. Administrative operations
GRANT RELOAD ON *.* TO 'dbprovision_admin'@'%';  -- FLUSH PRIVILEGES
GRANT PROCESS ON *.* TO 'dbprovision_admin'@'%'; -- View processes

-- 7. Connection management (required for force-drop)
GRANT CONNECTION_ADMIN ON *.* TO 'dbprovision_admin'@'%';

-- 8. Role management (MySQL 8.0+)
GRANT ROLE_ADMIN ON *.* TO 'dbprovision_admin'@'%';

-- 9. Data privileges (needed to grant these to app users)
--    The admin must have privileges it will grant to others
GRANT SELECT, INSERT, UPDATE, DELETE ON *.* TO 'dbprovision_admin'@'%';
GRANT SHOW VIEW, TRIGGER, LOCK TABLES ON *.* TO 'dbprovision_admin'@'%';

-- 10. Apply changes
FLUSH PRIVILEGES;

-- 11. Verify setup (should NOT show ALL PRIVILEGES or SUPER)
SHOW GRANTS FOR 'dbprovision_admin'@'%';

-- Expected output should list individual privileges, NOT:
--   GRANT ALL PRIVILEGES ON *.* TO 'dbprovision_admin'@'%'
```

### MySQL 5.7 Compatibility

For MySQL 5.7, some dynamic privileges don't exist. Use this alternative:

```sql
-- MySQL 5.7 setup (without dynamic privileges)
CREATE USER 'dbprovision_admin'@'%' IDENTIFIED BY 'your-secure-password';

-- Database operations
GRANT CREATE, DROP, ALTER ON *.* TO 'dbprovision_admin'@'%';

-- User management
GRANT CREATE USER ON *.* TO 'dbprovision_admin'@'%';
GRANT GRANT OPTION ON *.* TO 'dbprovision_admin'@'%';

-- Metadata access
GRANT SELECT ON mysql.* TO 'dbprovision_admin'@'%';

-- Administrative
GRANT RELOAD, PROCESS ON *.* TO 'dbprovision_admin'@'%';

-- Data privileges (for granting to app users)
GRANT SELECT, INSERT, UPDATE, DELETE ON *.* TO 'dbprovision_admin'@'%';
GRANT SHOW VIEW, TRIGGER, LOCK TABLES ON *.* TO 'dbprovision_admin'@'%';

-- Note: MySQL 5.7 doesn't have CONNECTION_ADMIN or ROLE_ADMIN
-- Force-drop requires SUPER or process termination from another tool

FLUSH PRIVILEGES;
```

!!! note "MySQL 5.7 Limitations"
    - No `CONNECTION_ADMIN`: Force-drop with active connections may require SUPER
    - No `ROLE_ADMIN`: Native roles require MySQL 8.0+
    - No `mysql.global_grants` table

!!! info "Complete Setup Guide"
    See [Admin Account Setup](../operations/admin-account-setup.md) for complete SQL scripts, verification steps, and security recommendations.

## DatabaseInstance

### Basic Configuration

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: mysql-primary
spec:
  engine: mysql
  connection:
    host: mysql.database.svc.cluster.local
    port: 3306
    database: mysql
    secretRef:
      name: mysql-admin-credentials
```

### TLS Configuration

```yaml
spec:
  connection:
    tls:
      enabled: true
      secretRef:
        name: mysql-tls
        keys:
          ca: ca.crt
          cert: tls.crt
          key: tls.key
```

## Database

### Basic Database

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: myapp-database
spec:
  instanceRef:
    name: mysql-primary
  name: myapp
  mysql:
    charset: utf8mb4
    collation: utf8mb4_unicode_ci
```

### Character Sets and Collations

| Charset | Description | Recommended Collation |
|---------|-------------|----------------------|
| `utf8mb4` | Full Unicode (recommended) | `utf8mb4_unicode_ci` |
| `utf8` | Basic Unicode (3 bytes) | `utf8_general_ci` |
| `latin1` | Western European | `latin1_swedish_ci` |

!!! warning "Use utf8mb4"
    Always use `utf8mb4` for full Unicode support including emojis. The `utf8` charset only supports 3-byte characters.

## DatabaseUser

### Basic User

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: myapp-user
spec:
  instanceRef:
    name: mysql-primary
  username: myapp_user
  passwordSecret:
    generate: true
    length: 32
    secretName: myapp-user-credentials
```

### User with Host Restrictions

```yaml
spec:
  username: myapp_user
  passwordSecret:
    generate: true
    secretName: myapp-user-credentials
  mysql:
    maxUserConnections: 50
    authPlugin: caching_sha2_password
    requireSSL: true
    allowedHosts:
      - "10.0.0.%"
      - "192.168.1.%"
      - "localhost"
```

### MySQL User Options

| Field | Type | Description |
|-------|------|-------------|
| `maxQueriesPerHour` | int | Query limit (0 = unlimited) |
| `maxUpdatesPerHour` | int | Update limit |
| `maxConnectionsPerHour` | int | Connection limit per hour |
| `maxUserConnections` | int | Max concurrent connections |
| `authPlugin` | string | Authentication plugin |
| `requireSSL` | bool | Require SSL connections |
| `allowedHosts` | array | Allowed connection hosts |
| `accountLocked` | bool | Lock the account |

### Authentication Plugins

| Plugin | MySQL Version | Description |
|--------|---------------|-------------|
| `mysql_native_password` | 5.7+ | Traditional password |
| `caching_sha2_password` | 8.0+ | Default in 8.0 (recommended) |
| `sha256_password` | 5.7+ | SHA-256 |

## DatabaseRole (MySQL 8.0+)

MySQL 8.0 introduced native role support.

### Create Role

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseRole
metadata:
  name: readonly-role
spec:
  instanceRef:
    name: mysql-primary
  roleName: readonly
  mysql:
    grants:
      - level: database
        database: myapp
        privileges: [SELECT]
```

### Role Hierarchy

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseRole
metadata:
  name: readwrite-role
spec:
  instanceRef:
    name: mysql-primary
  roleName: readwrite
  mysql:
    grants:
      - level: database
        database: myapp
        privileges: [SELECT, INSERT, UPDATE, DELETE]
```

## DatabaseGrant

### Database-Level Grant

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseGrant
metadata:
  name: myapp-grants
spec:
  userRef:
    name: myapp-user
  mysql:
    grants:
      - level: database
        database: myapp
        privileges: [SELECT, INSERT, UPDATE, DELETE]
```

### Table-Level Grant

```yaml
spec:
  userRef:
    name: myapp-user
  mysql:
    grants:
      - level: table
        database: myapp
        table: users
        privileges: [SELECT, INSERT, UPDATE]
      - level: table
        database: myapp
        table: audit_log
        privileges: [SELECT]
```

### Global Grant

```yaml
spec:
  userRef:
    name: admin-user
  mysql:
    grants:
      - level: global
        privileges: [PROCESS, REPLICATION CLIENT]
```

### Grant Levels

| Level | Scope | Example |
|-------|-------|---------|
| `global` | All databases | `PROCESS`, `REPLICATION CLIENT` |
| `database` | Single database | `SELECT`, `INSERT` on `myapp.*` |
| `table` | Single table | `SELECT` on `myapp.users` |

### Common Privileges

**Data Privileges:**

| Privilege | Description |
|-----------|-------------|
| `SELECT` | Read data |
| `INSERT` | Insert data |
| `UPDATE` | Update data |
| `DELETE` | Delete data |

**Structure Privileges:**

| Privilege | Description |
|-----------|-------------|
| `CREATE` | Create tables/databases |
| `ALTER` | Modify tables |
| `DROP` | Drop tables/databases |
| `INDEX` | Create/drop indexes |

**Administrative Privileges:**

| Privilege | Description |
|-----------|-------------|
| `PROCESS` | View processes |
| `RELOAD` | Flush operations |
| `REPLICATION CLIENT` | View replication status |
| `REPLICATION SLAVE` | Read binary logs |

## Backup and Restore

### mysqldump Backup

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseBackup
metadata:
  name: myapp-backup
spec:
  databaseRef:
    name: myapp-database
  storage:
    type: s3
    s3:
      bucket: my-backups
      secretRef:
        name: s3-credentials
  mysql:
    method: mysqldump
    singleTransaction: true
    routines: true
    triggers: true
    events: false
```

### Backup Options

| Option | Description |
|--------|-------------|
| `singleTransaction` | Consistent snapshot without locking |
| `routines` | Include stored procedures/functions |
| `triggers` | Include triggers |
| `events` | Include scheduled events |

## Complete Example

### Application Database Setup

```yaml
# Admin credentials
apiVersion: v1
kind: Secret
metadata:
  name: mysql-admin-credentials
type: Opaque
stringData:
  username: root
  password: your-root-password
---
# Database instance
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: mysql-primary
spec:
  engine: mysql
  connection:
    host: mysql.database.svc.cluster.local
    port: 3306
    database: mysql
    secretRef:
      name: mysql-admin-credentials
---
# Application database
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: myapp-database
spec:
  instanceRef:
    name: mysql-primary
  name: myapp
  mysql:
    charset: utf8mb4
    collation: utf8mb4_unicode_ci
---
# Application user
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: myapp-user
spec:
  instanceRef:
    name: mysql-primary
  username: myapp_user
  passwordSecret:
    generate: true
    secretName: myapp-user-credentials
    secretTemplate:
      data:
        DATABASE_URL: "mysql://{{ .Username }}:{{ .Password }}@{{ .Host }}:{{ .Port }}/myapp"
  mysql:
    maxUserConnections: 100
---
# User grants
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseGrant
metadata:
  name: myapp-user-grants
spec:
  userRef:
    name: myapp-user
  mysql:
    grants:
      - level: database
        database: myapp
        privileges: [SELECT, INSERT, UPDATE, DELETE]
```

## Best Practices

### Security

1. **Use caching_sha2_password** for MySQL 8.0+
2. **Restrict allowed hosts** to known IP ranges
3. **Enable SSL** for remote connections
4. **Use separate users** per application
5. **Avoid global privileges** unless necessary

### Performance

1. **Set appropriate connection limits**
2. **Use connection pooling**
3. **Use utf8mb4 consistently**

### Maintenance

1. **Use singleTransaction** for consistent backups
2. **Include routines and triggers** in backups
3. **Test restores regularly**

## Troubleshooting

### Connection Issues

```bash
# Test connectivity
kubectl run mysql-test --rm -it --image=mysql:8.0 -- \
  mysql -h host -u user -p -e "SELECT 1"
```

### Access Denied

- Check user exists: `SELECT user, host FROM mysql.user`
- Verify host restrictions match client IP
- Check grants: `SHOW GRANTS FOR 'user'@'host'`

### Authentication Plugin Mismatch

```sql
-- Update user authentication plugin
ALTER USER 'user'@'host' IDENTIFIED WITH caching_sha2_password BY 'password';
```
