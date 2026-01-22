# Engine Guides

DB Provision Operator supports multiple database engines with a unified API.

## Supported Engines

| Engine | Status | Versions |
|--------|--------|----------|
| [PostgreSQL](postgresql.md) | ✅ Stable | 12, 13, 14, 15, 16 |
| [MySQL](mysql.md) | ✅ Stable | 5.7, 8.0, 8.4 |
| [MariaDB](mariadb.md) | ✅ Stable | 10.5, 10.6, 10.11, 11.x |

## Engine Comparison

| Feature | PostgreSQL | MySQL | MariaDB |
|---------|------------|-------|---------|
| Roles | ✅ Native | ✅ 8.0+ | ✅ 10.0.5+ |
| Row-Level Security | ✅ | ❌ | ❌ |
| Extensions | ✅ | ❌ | ❌ |
| Schemas | ✅ | ❌ | ❌ |
| Default Privileges | ✅ | ❌ | ❌ |
| Backup | pg_dump | mysqldump | mariadb-dump |

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

## Connection Configuration

All engines use the same `DatabaseInstance` structure:

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: my-database
spec:
  engine: postgres  # or mysql, mariadb
  connection:
    host: database.example.com
    port: 5432  # 3306 for MySQL/MariaDB
    database: postgres  # mysql for MySQL/MariaDB
    secretRef:
      name: admin-credentials
```

## Next Steps

- [PostgreSQL Guide](postgresql.md) - PostgreSQL-specific features
- [MySQL Guide](mysql.md) - MySQL-specific features
- [MariaDB Guide](mariadb.md) - MariaDB-specific features
