# DatabaseInstance

A `DatabaseInstance` represents a connection to a database server. It's the foundational resource that all other resources depend on.

## Overview

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
    secretRef:
      name: postgres-admin-credentials
```

## Spec Fields

### engine (required)

The database engine type. Immutable after creation.

| Value | Description |
|-------|-------------|
| `postgres` | PostgreSQL database |
| `mysql` | MySQL or MariaDB database |

### connection (required)

Connection configuration for the database server.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `host` | string | Yes | Database server hostname or IP |
| `port` | int | Yes | Database server port |
| `database` | string | No | Initial database to connect to |
| `secretRef` | object | Yes | Reference to credentials Secret |

#### secretRef

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Name of the Secret |
| `namespace` | string | No | Namespace of the Secret (default: same as resource) |
| `keys.username` | string | No | Key containing username (default: `username`) |
| `keys.password` | string | No | Key containing password (default: `password`) |

### tls (optional)

TLS configuration for secure connections.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `enabled` | bool | No | Enable TLS (default: false) |
| `mode` | string | No | TLS mode (see below) |
| `secretRef` | object | No | Reference to TLS certificates Secret |

**TLS Modes:**

| Mode | Description |
|------|-------------|
| `disable` | No TLS |
| `require` | TLS required, no certificate verification |
| `verify-ca` | Verify server certificate against CA |
| `verify-full` | Verify server certificate and hostname |

### healthCheck (optional)

Health check configuration.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `enabled` | bool | No | Enable health checks (default: true) |
| `intervalSeconds` | int | No | Check interval (default: 30) |
| `timeoutSeconds` | int | No | Check timeout (default: 5) |

### postgres (optional)

PostgreSQL-specific configuration.

| Field | Type | Description |
|-------|------|-------------|
| `sslMode` | string | SSL mode (disable, require, verify-ca, verify-full) |
| `connectTimeout` | int | Connection timeout in seconds |
| `applicationName` | string | Application name for connection identification |

### mysql (optional)

MySQL-specific configuration.

| Field | Type | Description |
|-------|------|-------------|
| `charset` | string | Default charset (e.g., utf8mb4) |
| `collation` | string | Default collation |
| `timeout` | string | Connection timeout (e.g., "10s") |
| `tls` | string | TLS mode (disabled, preferred, required) |

## Status

| Field | Description |
|-------|-------------|
| `phase` | Current phase (Pending, Ready, Failed) |
| `conditions` | Detailed conditions |
| `observedGeneration` | Last observed generation |
| `lastHealthCheck` | Last health check timestamp |

## Examples

### Basic PostgreSQL Instance

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
    secretRef:
      name: postgres-admin-credentials
```

### PostgreSQL with TLS

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: postgres-secure
spec:
  engine: postgres
  connection:
    host: postgres.database.svc.cluster.local
    port: 5432
    secretRef:
      name: postgres-admin-credentials
  tls:
    enabled: true
    mode: verify-ca
    secretRef:
      name: postgres-tls-certs
      keys:
        ca: ca.crt
        cert: tls.crt
        key: tls.key
  postgres:
    sslMode: verify-ca
```

### MySQL Instance

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
    secretRef:
      name: mysql-admin-credentials
  mysql:
    charset: utf8mb4
    collation: utf8mb4_unicode_ci
```

### Cross-Namespace Secret Reference

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: postgres-with-shared-creds
  namespace: app-namespace
spec:
  engine: postgres
  connection:
    host: postgres.database.svc.cluster.local
    port: 5432
    secretRef:
      name: shared-postgres-credentials
      namespace: secrets-namespace
```

!!! note "RBAC for Cross-Namespace"
    Cross-namespace secret access requires additional RBAC configuration.
    See [Security - Cross-Namespace Mode](../architecture/security.md#cross-namespace-mode).

## Troubleshooting

### Instance stuck in Pending

- Verify the credentials Secret exists and has correct keys
- Check that the database server is reachable from the cluster
- Review operator logs for connection errors

### Connection refused

- Verify the host and port are correct
- Check network policies allow traffic to the database
- Ensure the database server is accepting connections

### Authentication failed

- Verify the credentials in the Secret are correct
- Check the user has appropriate permissions
- For PostgreSQL, ensure `pg_hba.conf` allows the connection
