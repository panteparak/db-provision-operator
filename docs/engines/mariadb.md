# MariaDB

Complete guide for using DB Provision Operator with MariaDB.

## Supported Versions

- MariaDB 10.5.x
- MariaDB 10.6.x (LTS)
- MariaDB 10.11.x (LTS)
- MariaDB 11.x

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
