# Engine Guides

DB Provision Operator supports multiple database engines with a unified API.

## Supported Engines

| Engine | Status | Versions |
|--------|--------|----------|
| [PostgreSQL](postgresql.md) | ✅ Stable | 12, 13, 14, 15, 16 |
| [MySQL](mysql.md) | ✅ Stable | 5.7, 8.0, 8.4 |
| [MariaDB](mariadb.md) | ✅ Stable | 10.5, 10.6, 10.11, 11.x |
| [CockroachDB](cockroachdb.md) | ✅ Stable | 22.x, 23.x, 24.x |

## Engine Comparison

| Feature | PostgreSQL | MySQL | MariaDB | CockroachDB |
|---------|------------|-------|---------|-------------|
| Roles | ✅ Native | ✅ 8.0+ | ✅ 10.0.5+ | ✅ Native |
| Row-Level Security | ✅ | ❌ | ❌ | ❌ |
| Extensions | ✅ | ❌ | ❌ | ❌ |
| Schemas | ✅ | ❌ | ❌ | ❌ |
| Default Privileges | ✅ | ❌ | ❌ | ❌ |
| Backup | pg_dump | mysqldump | mariadb-dump | Native BACKUP |
| Multi-Region | ❌ | ❌ | ❌ | ✅ Built-in |
| Distributed | ❌ | ❌ | ❌ | ✅ Built-in |

## Choosing an Engine

### PostgreSQL

Best for:

- Complex queries and analytics
- JSON/JSONB document storage
- Full-text search
- Geographic data (PostGIS)
- Strict data integrity requirements

### MySQL

Best for:

- Web applications
- High-volume OLTP
- Wide ecosystem support
- Replication simplicity

### MariaDB

Best for:

- MySQL compatibility with enhancements
- Open-source focus
- Additional storage engines
- Galera cluster support

### CockroachDB

Best for:

- Distributed SQL requirements
- Multi-region deployments
- Horizontal scalability
- PostgreSQL wire compatibility
- Strong consistency guarantees

## Engine-Specific Features

### PostgreSQL Only

```yaml
spec:
  postgres:
    extensions:
      - name: uuid-ossp
      - name: pg_stat_statements
    schemas:
      - name: app
        owner: myapp_admin
    defaultPrivileges:
      - objectType: tables
        privileges: [SELECT]
```

### MySQL/MariaDB

```yaml
spec:
  mysql:
    charset: utf8mb4
    collation: utf8mb4_unicode_ci
    authPlugin: caching_sha2_password
    allowedHosts:
      - "10.0.0.%"
```

### CockroachDB

CockroachDB uses PostgreSQL wire protocol but has a simpler configuration:

```yaml
spec:
  engine: cockroachdb
  connection:
    host: cockroachdb.example.com
    port: 26257
    database: defaultdb
    sslMode: verify-full  # or disable for insecure mode
```

!!! note "No Extensions or Schemas"
    CockroachDB does not support PostgreSQL extensions or custom schemas. Use it for distributed SQL without PostgreSQL-specific features.

## Connection Configuration

All engines use the same `DatabaseInstance` structure:

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: my-database
spec:
  engine: postgres  # or mysql, mariadb, cockroachdb
  connection:
    host: database.example.com
    port: 5432  # 3306 for MySQL/MariaDB, 26257 for CockroachDB
    database: postgres  # mysql for MySQL/MariaDB, defaultdb for CockroachDB
    secretRef:
      name: admin-credentials
```

## Next Steps

- [PostgreSQL Guide](postgresql.md) - PostgreSQL-specific features
- [MySQL Guide](mysql.md) - MySQL-specific features
- [MariaDB Guide](mariadb.md) - MariaDB-specific features
- [CockroachDB Guide](cockroachdb.md) - CockroachDB-specific features
