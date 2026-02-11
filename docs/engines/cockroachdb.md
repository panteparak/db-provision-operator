# CockroachDB

Complete guide for using DB Provision Operator with CockroachDB.

## Supported Versions

- CockroachDB 22.x
- CockroachDB 23.x
- CockroachDB 24.x

## PostgreSQL Wire Compatibility

CockroachDB is PostgreSQL wire-compatible, meaning it uses the same protocol and SQL syntax as PostgreSQL. The operator leverages the PostgreSQL driver (pgx) to communicate with CockroachDB.

However, CockroachDB has important differences from PostgreSQL:

| Feature | PostgreSQL | CockroachDB |
|---------|------------|-------------|
| SUPERUSER role | Yes | No |
| REPLICATION role | Yes | No |
| BYPASSRLS role | Yes | No |
| Extensions | Yes | No |
| Tablespaces | Yes | No |
| Row-Level Security | Yes | No |
| INHERIT/NOINHERIT | Yes | No |
| Backup method | pg_dump | Native BACKUP |
| Metadata schema | pg_catalog | crdb_internal |

!!! warning "PostgreSQL Options Ignored"
    PostgreSQL-specific options like `superuser`, `replication`, `bypassRLS`, `extensions`, and `schemas` are silently ignored when using CockroachDB. The operator will not raise an error, but these settings will have no effect.

## Admin Account Requirements

The operator requires a privileged account to manage databases, users, and roles in CockroachDB.

### Insecure Mode (Development Only)

!!! danger "Never Use Insecure Mode in Production"
    CockroachDB's `--insecure` flag disables TLS and password authentication. This is only for local development and testing.

In insecure mode, passwords are not supported. The operator will automatically detect this and create users without passwords.

```sql
-- Create the admin user (no password in insecure mode)
CREATE USER IF NOT EXISTS dbprovision_admin;

-- Grant role creation privilege
ALTER USER dbprovision_admin CREATEROLE;

-- Grant admin role for full database management
GRANT admin TO dbprovision_admin;
```

### Secure Mode (Production)

For production deployments, use CockroachDB with TLS certificates:

```sql
-- =============================================================
-- CockroachDB Secure Mode Admin Account Setup
-- =============================================================

-- 1. Create the operator admin role
CREATE USER dbprovision_admin WITH PASSWORD 'your-secure-password';

-- 2. Grant role creation and database creation privileges
ALTER USER dbprovision_admin WITH CREATEROLE CREATEDB;

-- 3. Grant admin role for full management capabilities
-- (alternative: grant only specific privileges you need)
GRANT admin TO dbprovision_admin;

-- 4. Verify the setup
SELECT username FROM system.users WHERE username = 'dbprovision_admin';
SHOW GRANTS FOR dbprovision_admin;
```

### Privilege Matrix

| Operation | Required Privileges | SQL to Grant |
|-----------|-------------------|--------------|
| **Create Database** | `CREATEDB` | `ALTER USER ... WITH CREATEDB` |
| **Drop Database** | `CREATEDB` | `ALTER USER ... WITH CREATEDB` |
| **Create User/Role** | `CREATEROLE` | `ALTER USER ... WITH CREATEROLE` |
| **Drop User/Role** | `CREATEROLE` | `ALTER USER ... WITH CREATEROLE` |
| **Alter User/Role** | `CREATEROLE` | `ALTER USER ... WITH CREATEROLE` |
| **Grant Privileges** | `admin` or owner | `GRANT admin TO ...` |
| **Backup Operations** | `admin` or `BACKUP` | `GRANT admin TO ...` |

!!! info "Complete Setup Guide"
    See [Admin Account Setup](../operations/admin-account-setup.md) for detailed instructions.

## DatabaseInstance

### Basic Configuration (Insecure Mode)

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: cockroach-cluster
  namespace: database
spec:
  engine: cockroachdb
  connection:
    host: cockroachdb.database.svc.cluster.local
    port: 26257
    database: defaultdb
    sslMode: disable  # Required for insecure mode
    secretRef:
      name: cockroach-admin-credentials
```

### Secure Mode with TLS

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: cockroach-cluster
  namespace: database
spec:
  engine: cockroachdb
  connection:
    host: cockroachdb.database.svc.cluster.local
    port: 26257
    database: defaultdb
    sslMode: verify-full
    secretRef:
      name: cockroach-admin-credentials
  tls:
    secretRef:
      name: cockroach-tls
      keys:
        ca: ca.crt
        cert: tls.crt
        key: tls.key
```

### SSL Modes

| Mode | Description |
|------|-------------|
| `disable` | No SSL (insecure mode only) |
| `require` | Require SSL, no certificate verification |
| `verify-ca` | Verify server certificate chain |
| `verify-full` | Verify server certificate and hostname |

### Credentials Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: cockroach-admin-credentials
  namespace: database
type: Opaque
stringData:
  username: dbprovision_admin
  password: ""  # Empty for insecure mode
```

For secure mode:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: cockroach-admin-credentials
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
  name: myapp-database
  namespace: database
spec:
  instanceRef:
    name: cockroach-cluster
  name: myapp
```

### Database Options

CockroachDB databases support fewer options than PostgreSQL:

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: myapp-database
spec:
  instanceRef:
    name: cockroach-cluster
  name: myapp
  deletionPolicy: Retain  # Retain, Delete
  deletionProtection: true
```

!!! note "No Extensions or Schemas"
    CockroachDB does not support PostgreSQL extensions or custom schemas. The `postgres.extensions` and `postgres.schemas` fields are ignored.

## DatabaseUser

### Basic User

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: myapp-user
  namespace: database
spec:
  instanceRef:
    name: cockroach-cluster
  username: myapp_user
  passwordSecret:
    generate: true
    length: 32
    secretName: myapp-user-credentials
```

### User with Role Membership

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: myapp-admin
spec:
  instanceRef:
    name: cockroach-cluster
  username: myapp_admin
  passwordSecret:
    generate: true
    secretName: myapp-admin-credentials
  postgres:
    createDB: true
    createRole: true
    connectionLimit: 20
    inRoles:
      - myapp_readers
```

### CockroachDB User Options

| Field | Type | Description |
|-------|------|-------------|
| `login` | bool | Can login (default: true for users) |
| `createDB` | bool | Can create databases |
| `createRole` | bool | Can create roles |
| `connectionLimit` | int | Max connections (-1 = unlimited) |
| `inRoles` | array | Roles to inherit from |
| `validUntil` | string | Password expiration |

!!! warning "Unsupported Options"
    The following PostgreSQL options are ignored for CockroachDB: `superuser`, `replication`, `bypassRLS`, `inherit`.

## DatabaseRole

### Read-Only Role

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseRole
metadata:
  name: readonly-role
spec:
  instanceRef:
    name: cockroach-cluster
  roleName: myapp_reader
  postgres:
    login: false
```

### Role with Grants

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseRole
metadata:
  name: writer-role
spec:
  instanceRef:
    name: cockroach-cluster
  roleName: myapp_writer
  postgres:
    login: false
    inRoles: [myapp_reader]
```

## DatabaseGrant

### Grant Database Privileges

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
        privileges: [CONNECT, CREATE]
```

### Grant Table Privileges

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseGrant
metadata:
  name: myapp-table-grants
spec:
  userRef:
    name: myapp-user
  postgres:
    grants:
      - database: myapp
        schema: public
        tables: ["*"]
        privileges: [SELECT, INSERT, UPDATE, DELETE]
```

### CockroachDB Privileges

| Object | Privileges |
|--------|-----------|
| Database | `ALL`, `CREATE`, `CONNECT` |
| Table | `ALL`, `SELECT`, `INSERT`, `UPDATE`, `DELETE`, `DROP` |
| Schema | `ALL`, `CREATE`, `USAGE` |
| Type | `ALL`, `USAGE` |

## Backup and Restore

CockroachDB uses its native `BACKUP` and `RESTORE` commands instead of pg_dump/pg_restore.

### Native Backup

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
      prefix: cockroach/
      secretRef:
        name: s3-credentials
```

### Backup Characteristics

| Feature | CockroachDB | PostgreSQL |
|---------|-------------|------------|
| Method | SQL `BACKUP` command | External pg_dump |
| Format | Native CockroachDB | custom/plain/tar |
| Parallelism | Built-in distributed | Jobs flag |
| Progress | SHOW JOBS | Byte counting |
| Storage | S3/GCS/Azure/nodelocal | S3/GCS/Azure/PVC |

### Restore Operation

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseRestore
metadata:
  name: myapp-restore
spec:
  backupRef:
    name: myapp-backup
  targetDatabaseRef:
    name: myapp-restored
```

## Multi-Region Considerations

CockroachDB excels in multi-region deployments. When using the operator:

1. **Single DatabaseInstance per region**: Create separate `DatabaseInstance` resources pointing to region-local SQL endpoints
2. **Locality-aware users**: CockroachDB handles user replication automatically
3. **Backup locality**: Backups can target region-specific storage

!!! tip "Enterprise Features"
    Some CockroachDB features like locality-aware queries and incremental backups require an Enterprise license.

## Best Practices

### Security

1. **Always use TLS** in production with `verify-full`
2. **Use certificate authentication** (mTLS) when possible
3. **Limit connection privileges** with `connectionLimit`
4. **Use roles for permission grouping**
5. **Enable audit logging** in CockroachDB

### Performance

1. **Use connection pooling** (CockroachDB supports PgBouncer)
2. **Set appropriate connection limits** per user
3. **Configure statement timeouts** for long-running queries

### Operations

1. **Schedule regular backups** with retention policies
2. **Monitor via CockroachDB Console** (port 8080)
3. **Use SHOW JOBS** to track backup/restore progress

## Troubleshooting

### Connection Issues

```bash
# Test connectivity to CockroachDB
kubectl run cockroach-test --rm -it --image=cockroachdb/cockroach:v24.1.0 -- \
  sql --insecure --host=cockroachdb:26257 -e "SELECT 1"

# For secure mode
kubectl run cockroach-test --rm -it --image=cockroachdb/cockroach:v24.1.0 -- \
  sql --url="postgresql://user:pass@cockroachdb:26257/defaultdb?sslmode=verify-full"
```

### Insecure Mode Password Error

If you see errors about passwords in insecure mode:

```
ERROR: password is not supported in insecure mode
```

This is expected. The operator will automatically retry user creation without a password.

### Permission Denied

```sql
-- Check user privileges
SHOW GRANTS FOR myuser;

-- Check role memberships
SHOW GRANTS ON ROLE FOR myuser;

-- Grant missing privileges
GRANT SELECT ON TABLE mydb.public.* TO myuser;
```

### Backup Job Status

```sql
-- Check backup job status
SELECT * FROM [SHOW JOBS] WHERE job_type = 'BACKUP' ORDER BY created DESC LIMIT 5;

-- Check for failed jobs
SELECT * FROM [SHOW JOBS] WHERE status = 'failed' AND job_type = 'BACKUP';
```

### Common Errors

| Error | Cause | Solution |
|-------|-------|----------|
| `insecure mode` | Using password with --insecure | Remove password or enable TLS |
| `CREATEDB required` | Missing CREATEDB privilege | `ALTER USER ... WITH CREATEDB` |
| `admin required` | Missing admin role for grants | `GRANT admin TO ...` |
| `node not healthy` | Cluster health issue | Check CockroachDB logs and status |
