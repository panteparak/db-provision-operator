# MariaDB

Complete guide for using DB Provision Operator with MariaDB.

## Supported Versions

- MariaDB 10.5.x
- MariaDB 10.6.x (LTS)
- MariaDB 10.11.x (LTS)
- MariaDB 11.x

## Admin Account Requirements

The operator requires a privileged database account to manage databases, users, and grants. For production security, create a dedicated **least-privilege admin account** instead of using `root`.

!!! warning "Never Use Root or ALL PRIVILEGES"
    The operator is designed to work **without SUPER privilege or ALL PRIVILEGES**. Using root or granting ALL PRIVILEGES violates the principle of least privilege and creates security risks.

### Key Differences from MySQL

| Feature | MariaDB | MySQL 8.0+ |
|---------|---------|------------|
| Kill connections privilege | `CONNECTION ADMIN` (with space) | `CONNECTION_ADMIN` (underscore) |
| Role management | Roles are users with `CREATE USER` | `ROLE_ADMIN` privilege |
| Global grants table | Not available | `mysql.global_grants` |

### Privilege Matrix

This table shows the exact privileges required for each operator action:

| Operation | Required Privileges | SQL to Grant |
|-----------|-------------------|--------------|
| **Create Database** | `CREATE ON *.*` | `GRANT CREATE ON *.* TO ...` |
| **Drop Database** | `DROP ON *.*` | `GRANT DROP ON *.* TO ...` |
| **Alter Database** | `ALTER ON *.*` | `GRANT ALTER ON *.* TO ...` |
| **Force Drop Database** | `DROP ON *.*`, `CONNECTION ADMIN ON *.*` | See setup below |
| **Create User** | `CREATE USER ON *.*` | `GRANT CREATE USER ON *.* TO ...` |
| **Drop User** | `CREATE USER ON *.*` | Same grant handles both operations |
| **Alter User** | `CREATE USER ON *.*` | Same grant handles all user operations |
| **Grant Privileges** | `WITH GRANT OPTION` | `GRANT ... WITH GRANT OPTION` |
| **Create Role** | `CREATE USER ON *.*` | MariaDB roles are users |
| **Grant Role** | `WITH GRANT OPTION` | Same as grant privileges |
| **Query User Info** | `SELECT ON mysql.*` | `GRANT SELECT ON mysql.user TO ...` |
| **Apply Changes** | `RELOAD ON *.*` | `GRANT RELOAD ON *.* TO ...` |
| **Kill Connections** | `CONNECTION ADMIN ON *.*` | `GRANT CONNECTION ADMIN ON *.* TO ...` |
| **View Processes** | `PROCESS ON *.*` | `GRANT PROCESS ON *.* TO ...` |

### Understanding Each Privilege

| Privilege | What It Allows | What Happens Without It |
|-----------|----------------|------------------------|
| `CREATE` | Create databases and tables | `CREATE DATABASE` fails |
| `DROP` | Drop databases and tables | `DROP DATABASE` fails |
| `ALTER` | Modify database/table structure | `ALTER DATABASE` fails |
| `CREATE USER` | Create, drop, and alter users/roles | `CREATE USER` fails |
| `WITH GRANT OPTION` | Grant/revoke privileges to others | Cannot delegate privileges |
| `SELECT ON mysql.*` | Read user/grant metadata | Cannot verify existing users |
| `RELOAD` | Execute FLUSH statements | `FLUSH PRIVILEGES` fails |
| `CONNECTION ADMIN` | Kill user connections (10.5.2+) | Force-drop fails with active connections |
| `PROCESS` | View server process list | Cannot monitor connections |

### Complete Setup Script (Copy-Paste)

```sql
-- =============================================================
-- MariaDB 10.5+ Least-Privilege Admin Account Setup
-- Compatible with: MariaDB 10.5.x, 10.6.x (LTS), 10.11.x (LTS), 11.x
-- =============================================================

-- 1. Create the operator admin user
CREATE USER 'dbprovision_admin'@'%' IDENTIFIED BY 'your-secure-password';

-- 2. Database operations (create, drop, alter databases)
GRANT CREATE, DROP, ALTER ON *.* TO 'dbprovision_admin'@'%';

-- 3. User management with grant delegation
--    Note: WITH GRANT OPTION is critical for role/privilege delegation
GRANT CREATE USER ON *.* TO 'dbprovision_admin'@'%' WITH GRANT OPTION;

-- 4. Query user and grant metadata from system tables
--    Note: MariaDB doesn't have mysql.global_grants
GRANT SELECT ON mysql.user TO 'dbprovision_admin'@'%';
GRANT SELECT ON mysql.db TO 'dbprovision_admin'@'%';
GRANT SELECT ON mysql.tables_priv TO 'dbprovision_admin'@'%';
GRANT SELECT ON mysql.columns_priv TO 'dbprovision_admin'@'%';
GRANT SELECT ON mysql.procs_priv TO 'dbprovision_admin'@'%';
GRANT SELECT ON mysql.roles_mapping TO 'dbprovision_admin'@'%';

-- 5. Administrative operations
GRANT RELOAD ON *.* TO 'dbprovision_admin'@'%';  -- FLUSH PRIVILEGES
GRANT PROCESS ON *.* TO 'dbprovision_admin'@'%'; -- View processes

-- 6. Connection management (required for force-drop)
--    IMPORTANT: Note the space in "CONNECTION ADMIN" (not underscore!)
GRANT CONNECTION ADMIN ON *.* TO 'dbprovision_admin'@'%';

-- 7. Data privileges (needed to grant these to app users)
GRANT SELECT, INSERT, UPDATE, DELETE ON *.* TO 'dbprovision_admin'@'%';
GRANT SHOW VIEW, TRIGGER, LOCK TABLES ON *.* TO 'dbprovision_admin'@'%';

-- 8. Apply changes
FLUSH PRIVILEGES;

-- 9. Verify setup (should NOT show ALL PRIVILEGES or SUPER)
SHOW GRANTS FOR 'dbprovision_admin'@'%';

-- Expected output should list individual privileges, NOT:
--   GRANT ALL PRIVILEGES ON *.* TO 'dbprovision_admin'@'%'
```

### MariaDB 10.4 and Earlier

For MariaDB versions before 10.5, `CONNECTION ADMIN` doesn't exist:

```sql
-- MariaDB 10.4 and earlier setup
CREATE USER 'dbprovision_admin'@'%' IDENTIFIED BY 'your-secure-password';

-- Database operations
GRANT CREATE, DROP, ALTER ON *.* TO 'dbprovision_admin'@'%';

-- User management
GRANT CREATE USER ON *.* TO 'dbprovision_admin'@'%' WITH GRANT OPTION;

-- Metadata access
GRANT SELECT ON mysql.user TO 'dbprovision_admin'@'%';
GRANT SELECT ON mysql.db TO 'dbprovision_admin'@'%';
GRANT SELECT ON mysql.tables_priv TO 'dbprovision_admin'@'%';
GRANT SELECT ON mysql.columns_priv TO 'dbprovision_admin'@'%';
GRANT SELECT ON mysql.procs_priv TO 'dbprovision_admin'@'%';

-- Administrative
GRANT RELOAD, PROCESS ON *.* TO 'dbprovision_admin'@'%';

-- Data privileges
GRANT SELECT, INSERT, UPDATE, DELETE ON *.* TO 'dbprovision_admin'@'%';
GRANT SHOW VIEW, TRIGGER, LOCK TABLES ON *.* TO 'dbprovision_admin'@'%';

-- Note: MariaDB < 10.5 doesn't have CONNECTION ADMIN
-- Force-drop requires SUPER or process termination from another tool

FLUSH PRIVILEGES;
```

!!! warning "MariaDB 10.4 Limitations"
    - No `CONNECTION ADMIN`: Force-drop with active connections may require SUPER
    - Consider upgrading to MariaDB 10.5+ for full operator support

!!! note "MariaDB vs MySQL Differences"
    - MariaDB uses `CONNECTION ADMIN` (with a space) instead of MySQL's `CONNECTION_ADMIN`
    - MariaDB roles are implemented as users, not requiring a separate `ROLE_ADMIN` privilege
    - MariaDB doesn't have `mysql.global_grants` table

!!! info "Complete Setup Guide"
    See [Admin Account Setup](../operations/admin-account-setup.md) for complete SQL scripts, verification steps, and security recommendations.

## MariaDB vs MySQL

MariaDB is a MySQL-compatible fork with additional features:

| Feature | MariaDB | MySQL |
|---------|---------|-------|
| Storage Engines | More options (Aria, ColumnStore) | InnoDB, MyISAM |
| Replication | Galera Cluster built-in | Group Replication |
| JSON | JSON type (10.2+) | Native JSON |
| Window Functions | 10.2+ | 8.0+ |
| Roles | 10.0.5+ | 8.0+ |

## DatabaseInstance

### Basic Configuration

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: mariadb-primary
spec:
  engine: mariadb
  connection:
    host: mariadb.database.svc.cluster.local
    port: 3306
    database: mysql
    secretRef:
      name: mariadb-admin-credentials
```

### TLS Configuration

```yaml
spec:
  connection:
    tls:
      enabled: true
      secretRef:
        name: mariadb-tls
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
    name: mariadb-primary
  name: myapp
  mysql:
    charset: utf8mb4
    collation: utf8mb4_unicode_ci
```

!!! note "MySQL Configuration Block"
    MariaDB uses the `mysql` configuration block due to its MySQL compatibility.

### Character Sets

| Charset | Description | Recommended For |
|---------|-------------|-----------------|
| `utf8mb4` | Full Unicode | Most applications |
| `utf8mb3` | 3-byte Unicode | Legacy compatibility |
| `latin1` | Western European | Legacy systems |

## DatabaseUser

### Basic User

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: myapp-user
spec:
  instanceRef:
    name: mariadb-primary
  username: myapp_user
  passwordSecret:
    generate: true
    length: 32
    secretName: myapp-user-credentials
```

### User with Options

```yaml
spec:
  username: myapp_user
  passwordSecret:
    generate: true
    secretName: myapp-user-credentials
  mysql:
    maxUserConnections: 50
    requireSSL: true
    allowedHosts:
      - "10.0.0.%"
      - "192.168.1.%"
```

### Authentication Plugins

MariaDB supports multiple authentication plugins:

| Plugin | Description |
|--------|-------------|
| `mysql_native_password` | Traditional (default) |
| `ed25519` | EdDSA (more secure) |
| `gssapi` | Kerberos/GSSAPI |
| `pam` | PAM authentication |

## DatabaseRole

MariaDB supports roles since version 10.0.5.

### Create Role

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseRole
metadata:
  name: readonly-role
spec:
  instanceRef:
    name: mariadb-primary
  roleName: readonly
  mysql:
    grants:
      - level: database
        database: myapp
        privileges: [SELECT]
```

### Role with Multiple Privileges

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseRole
metadata:
  name: developer-role
spec:
  instanceRef:
    name: mariadb-primary
  roleName: developer
  mysql:
    grants:
      - level: database
        database: myapp
        privileges: [SELECT, INSERT, UPDATE, DELETE, CREATE, ALTER, INDEX]
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
```

### Assign Roles

```yaml
spec:
  userRef:
    name: myapp-user
  mysql:
    roles:
      - readonly
      - developer
```

## Backup and Restore

### mariadb-dump Backup

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
    method: mysqldump  # Uses mariadb-dump internally
    singleTransaction: true
    routines: true
    triggers: true
```

## Complete Example

### Application Setup

```yaml
# Admin credentials
apiVersion: v1
kind: Secret
metadata:
  name: mariadb-admin-credentials
type: Opaque
stringData:
  username: root
  password: your-root-password
---
# Database instance
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: mariadb-primary
spec:
  engine: mariadb
  connection:
    host: mariadb.database.svc.cluster.local
    port: 3306
    database: mysql
    secretRef:
      name: mariadb-admin-credentials
  healthCheck:
    enabled: true
    intervalSeconds: 30
---
# Application database
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: myapp-database
spec:
  instanceRef:
    name: mariadb-primary
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
    name: mariadb-primary
  username: myapp_user
  passwordSecret:
    generate: true
    secretName: myapp-user-credentials
    secretTemplate:
      data:
        DATABASE_URL: "mysql://{{ .Username }}:{{ .Password }}@{{ .Host }}:{{ .Port }}/myapp"
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

## Galera Cluster

When using MariaDB Galera Cluster:

### Considerations

1. **Use any node** for the connection - writes are synchronous
2. **Configure `wsrep_sync_wait`** for read-after-write consistency
3. **Monitor cluster status** before operations

### Cluster-Aware Configuration

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: mariadb-galera
spec:
  engine: mariadb
  connection:
    host: mariadb-galera.database.svc.cluster.local
    port: 3306
    database: mysql
    secretRef:
      name: mariadb-admin-credentials
  healthCheck:
    enabled: true
    intervalSeconds: 15  # More frequent for cluster
```

## Best Practices

### Security

1. **Use ed25519 authentication** for better security
2. **Restrict host access** to known IP ranges
3. **Enable TLS** for remote connections
4. **Use roles** for permission management

### Performance

1. **Use InnoDB** for transactional workloads
2. **Configure connection pools**
3. **Use utf8mb4 consistently**

### Maintenance

1. **Use singleTransaction** for backups
2. **Monitor Galera cluster** health
3. **Test failover procedures**

## Troubleshooting

### Connection Issues

```bash
# Test connectivity
kubectl run mariadb-test --rm -it --image=mariadb:10.11 -- \
  mariadb -h host -u user -p -e "SELECT 1"
```

### Galera Issues

```sql
-- Check cluster status
SHOW STATUS LIKE 'wsrep_%';

-- Check if node is synced
SHOW STATUS LIKE 'wsrep_local_state_comment';
-- Should return 'Synced'
```

### Access Denied

- Check user exists: `SELECT user, host FROM mysql.user`
- Verify host restrictions
- Check grants: `SHOW GRANTS FOR 'user'@'host'`
