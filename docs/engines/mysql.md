# MySQL

Complete guide for using DB Provision Operator with MySQL.

## Supported Versions

- MySQL 5.7.x
- MySQL 8.0.x
- MySQL 8.4.x

## Admin Account Requirements

The operator requires a privileged database account to manage databases, users, and grants. For production security, create a dedicated least-privilege admin account instead of using `root`.

**Required privileges:**

| Privilege | Purpose |
|-----------|---------|
| `CREATE, DROP, ALTER ON *.*` | Database operations |
| `CREATE USER ON *.*` | User management |
| `GRANT OPTION ON *.*` | Delegate privileges |
| `SELECT ON mysql.*` | Query user metadata |
| `RELOAD ON *.*` | FLUSH PRIVILEGES |
| `CONNECTION_ADMIN ON *.*` | Kill connections (force-drop) |
| `ROLE_ADMIN ON *.*` | Role management (MySQL 8.0+) |

**Quick setup:**

```sql
CREATE USER 'dbprovision_admin'@'%' IDENTIFIED BY 'your-secure-password';
GRANT CREATE, DROP, ALTER ON *.* TO 'dbprovision_admin'@'%';
GRANT CREATE USER ON *.* TO 'dbprovision_admin'@'%';
GRANT GRANT OPTION ON *.* TO 'dbprovision_admin'@'%';
GRANT SELECT ON mysql.* TO 'dbprovision_admin'@'%';
GRANT RELOAD, CONNECTION_ADMIN, ROLE_ADMIN ON *.* TO 'dbprovision_admin'@'%';
FLUSH PRIVILEGES;
```

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
