# PostgreSQL

Complete guide for using DB Provision Operator with PostgreSQL.

## Supported Versions

- PostgreSQL 12.x
- PostgreSQL 13.x
- PostgreSQL 14.x
- PostgreSQL 15.x
- PostgreSQL 16.x

## DatabaseInstance

### Basic Configuration

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: postgres-primary
spec:
  engine: postgres
  connection:
    host: postgres.database.svc.cluster.local
    port: 5432
    database: postgres
    sslMode: require
    secretRef:
      name: postgres-admin-credentials
```

### SSL Modes

| Mode | Description |
|------|-------------|
| `disable` | No SSL |
| `allow` | Try SSL, allow non-SSL |
| `prefer` | Try SSL first (default) |
| `require` | Require SSL |
| `verify-ca` | Verify server certificate |
| `verify-full` | Verify server certificate and hostname |

### TLS Configuration

```yaml
spec:
  connection:
    sslMode: verify-full
    tls:
      secretRef:
        name: postgres-tls
        keys:
          ca: ca.crt
          cert: tls.crt
          key: tls.key
```

## Database

### Extensions

PostgreSQL supports extensions for additional functionality:

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: myapp-database
spec:
  instanceRef:
    name: postgres-primary
  name: myapp
  postgres:
    encoding: UTF8
    lcCollate: en_US.UTF-8
    lcCtype: en_US.UTF-8
    extensions:
      - name: uuid-ossp
        schema: public
      - name: pgcrypto
        schema: public
      - name: pg_stat_statements
        schema: public
      - name: hstore
        schema: extensions
```

### Common Extensions

| Extension | Purpose |
|-----------|---------|
| `uuid-ossp` | UUID generation |
| `pgcrypto` | Cryptographic functions |
| `pg_stat_statements` | Query statistics |
| `hstore` | Key-value storage |
| `postgis` | Geographic objects |
| `pg_trgm` | Trigram similarity |
| `btree_gin` | GIN index for B-tree types |

### Schemas

Organize objects with schemas:

```yaml
spec:
  postgres:
    schemas:
      - name: app
        owner: myapp_admin
      - name: audit
        owner: myapp_admin
      - name: analytics
        owner: analytics_role
```

## DatabaseUser

### Basic User

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: myapp-user
spec:
  instanceRef:
    name: postgres-primary
  username: myapp_user
  passwordSecret:
    generate: true
    length: 32
    secretName: myapp-user-credentials
```

### User with Role Membership

```yaml
spec:
  username: myapp_user
  passwordSecret:
    generate: true
    secretName: myapp-user-credentials
  postgres:
    connectionLimit: 20
    inherit: true
    inRoles:
      - readonly_role
      - analytics_role
    configParameters:
      search_path: "app,public"
      statement_timeout: "30000"
```

### PostgreSQL User Options

| Field | Type | Description |
|-------|------|-------------|
| `login` | bool | Can login (default: true) |
| `inherit` | bool | Inherit role privileges (default: true) |
| `createDB` | bool | Can create databases |
| `createRole` | bool | Can create roles |
| `superuser` | bool | Is superuser |
| `replication` | bool | Can replicate |
| `bypassRLS` | bool | Bypass row-level security |
| `connectionLimit` | int | Max connections (-1 = unlimited) |
| `inRoles` | array | Roles to inherit from |
| `configParameters` | map | Session parameters |

## DatabaseRole

### Read-Only Role

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseRole
metadata:
  name: readonly-role
spec:
  instanceRef:
    name: postgres-primary
  roleName: readonly
  postgres:
    login: false
    grants:
      - database: myapp
        schema: public
        tables: ["*"]
        privileges: [SELECT]
      - database: myapp
        schema: public
        sequences: ["*"]
        privileges: [USAGE, SELECT]
```

### Role Hierarchy

```yaml
# Base reader role
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseRole
metadata:
  name: base-reader
spec:
  instanceRef:
    name: postgres-primary
  roleName: base_reader
  postgres:
    login: false
    grants:
      - database: myapp
        schema: public
        tables: ["*"]
        privileges: [SELECT]
---
# Writer role inherits from reader
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseRole
metadata:
  name: base-writer
spec:
  instanceRef:
    name: postgres-primary
  roleName: base_writer
  postgres:
    login: false
    inRoles: [base_reader]
    grants:
      - database: myapp
        schema: public
        tables: ["*"]
        privileges: [INSERT, UPDATE, DELETE]
```

## DatabaseGrant

### Direct Table Grants

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseGrant
metadata:
  name: myapp-user-grants
spec:
  userRef:
    name: myapp-user
  postgres:
    grants:
      - database: myapp
        schema: public
        tables: [users, orders, products]
        privileges: [SELECT, INSERT, UPDATE]
      - database: myapp
        schema: public
        sequences: ["*"]
        privileges: [USAGE, SELECT]
```

### Default Privileges

Apply privileges to future objects:

```yaml
spec:
  userRef:
    name: myapp-user
  postgres:
    defaultPrivileges:
      - database: myapp
        schema: public
        grantedBy: myapp_admin
        objectType: tables
        privileges: [SELECT, INSERT, UPDATE, DELETE]
      - database: myapp
        schema: public
        grantedBy: myapp_admin
        objectType: sequences
        privileges: [USAGE, SELECT]
```

### Privileges Reference

**Table Privileges:**

| Privilege | Description |
|-----------|-------------|
| `SELECT` | Read rows |
| `INSERT` | Insert rows |
| `UPDATE` | Update rows |
| `DELETE` | Delete rows |
| `TRUNCATE` | Truncate table |
| `REFERENCES` | Create foreign keys |
| `TRIGGER` | Create triggers |

**Sequence Privileges:**

| Privilege | Description |
|-----------|-------------|
| `USAGE` | Use currval/nextval |
| `SELECT` | Use currval |
| `UPDATE` | Use setval |

**Function Privileges:**

| Privilege | Description |
|-----------|-------------|
| `EXECUTE` | Execute function |

## Backup and Restore

### pg_dump Backup

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
  postgres:
    method: pg_dump
    format: custom
    jobs: 4
    blobs: true
    noOwner: false
    noPrivileges: false
```

### Backup Formats

| Format | Description | Parallel Restore |
|--------|-------------|------------------|
| `plain` | SQL script | No |
| `custom` | Compressed archive | Yes |
| `directory` | Directory with files | Yes |
| `tar` | Tar archive | No |

### Restore Options

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseRestore
metadata:
  name: myapp-restore
spec:
  backupRef:
    name: myapp-backup
  targetDatabaseRef:
    name: myapp-database-restored
  postgres:
    dropExisting: false
    dataOnly: false
    schemaOnly: false
    noOwner: true
    jobs: 4
    disableTriggers: true
    analyze: true
```

## Best Practices

### Security

1. **Use verify-full SSL mode** in production
2. **Limit connection privileges** with `connectionLimit`
3. **Use roles for permission grouping**
4. **Enable row-level security** where appropriate
5. **Use separate users** for different applications

### Performance

1. **Install pg_stat_statements** for query analysis
2. **Set appropriate connection limits**
3. **Use connection pooling** (PgBouncer)
4. **Configure statement_timeout** to prevent runaway queries

### Maintenance

1. **Schedule regular backups** with retention
2. **Use ANALYZE after restores**
3. **Monitor with pg_stat_statements**
4. **Plan for extension upgrades**

## Troubleshooting

### Connection Issues

```bash
# Test connectivity
kubectl run psql-test --rm -it --image=postgres:15 -- \
  psql "postgresql://user:pass@host:5432/dbname?sslmode=require"
```

### Permission Denied

- Verify role membership with `\du` in psql
- Check grants with `\dp tablename`
- Ensure schema USAGE granted

### Extension Installation Failed

- Check PostgreSQL has the extension installed
- Verify user has CREATE EXTENSION privilege
- Some extensions require superuser
