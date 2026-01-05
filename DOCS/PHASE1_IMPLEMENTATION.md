# DB Provision Operator - Phase 1 Implementation Details

This document provides detailed documentation of the Phase 1 (Core MVP) implementation.

## Implemented Components

### 1. Custom Resource Definitions (CRDs)

#### 1.1 DatabaseInstance

**File**: `api/v1alpha1/databaseinstance_types.go`

**Purpose**: Represents a connection to a database server.

**Spec Fields**:
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `engine` | `EngineType` | Yes | Database engine (`postgres` or `mysql`) |
| `connection` | `ConnectionConfig` | Yes | Server connection details |
| `tls` | `*TLSConfig` | No | TLS configuration |
| `healthCheck` | `*HealthCheckConfig` | No | Health check settings |

**Status Fields**:
| Field | Type | Description |
|-------|------|-------------|
| `phase` | `Phase` | Current phase (Pending/Ready/Failed) |
| `message` | `string` | Human-readable status message |
| `version` | `string` | Database server version |
| `lastCheckedAt` | `*metav1.Time` | Last health check time |
| `conditions` | `[]metav1.Condition` | Detailed conditions |

**Example**:
```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: postgres-main
spec:
  engine: postgres
  connection:
    host: postgres.database.svc.cluster.local
    port: 5432
    database: postgres
    secretRef:
      name: postgres-admin-credentials
      keys:
        username: user
        password: pass
  tls:
    enabled: true
    mode: verify-full
    secretRef:
      name: postgres-tls
  healthCheck:
    intervalSeconds: 60
    timeoutSeconds: 5
```

#### 1.2 Database

**File**: `api/v1alpha1/database_types.go`

**Purpose**: Represents a database within a DatabaseInstance.

**Spec Fields**:
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | `string` | Yes | Database name |
| `instanceRef` | `InstanceReference` | Yes | Reference to DatabaseInstance |
| `deletionPolicy` | `DeletionPolicy` | No | `Retain` (default) or `Delete` |
| `deletionProtection` | `bool` | No | Block deletion |
| `postgres` | `*PostgresDatabaseConfig` | No | PostgreSQL-specific settings |
| `mysql` | `*MySQLDatabaseConfig` | No | MySQL-specific settings |

**PostgreSQL-Specific Fields** (`postgres`):
| Field | Type | Description |
|-------|------|-------------|
| `encoding` | `string` | Character encoding (e.g., UTF8) |
| `lcCollate` | `string` | Collation order |
| `lcCtype` | `string` | Character classification |
| `tablespace` | `string` | Default tablespace |
| `template` | `string` | Template database |
| `connectionLimit` | `*int32` | Max connections (-1 unlimited) |
| `isTemplate` | `bool` | Is template database |
| `allowConnections` | `bool` | Allow connections |
| `extensions` | `[]ExtensionConfig` | Extensions to install |
| `schemas` | `[]SchemaConfig` | Schemas to create |
| `defaultPrivileges` | `[]DefaultPrivilegeConfig` | Default privileges |

**MySQL-Specific Fields** (`mysql`):
| Field | Type | Description |
|-------|------|-------------|
| `charset` | `string` | Character set (e.g., utf8mb4) |
| `collation` | `string` | Collation |

**Status Fields**:
| Field | Type | Description |
|-------|------|-------------|
| `phase` | `Phase` | Current phase |
| `message` | `string` | Status message |
| `database` | `*DatabaseInfo` | Database details (name, owner, size) |
| `conditions` | `[]metav1.Condition` | Detailed conditions |

#### 1.3 DatabaseUser

**File**: `api/v1alpha1/databaseuser_types.go`

**Purpose**: Represents a database user/role.

**Spec Fields**:
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `username` | `string` | Yes | Username to create |
| `instanceRef` | `InstanceReference` | Yes | Reference to DatabaseInstance |
| `databaseRef` | `*DatabaseReference` | No | Reference to Database |
| `passwordSecret` | `*PasswordSecretConfig` | No | Password generation/storage config |
| `existingPasswordSecret` | `*ExistingPasswordSecret` | No | Use existing password |
| `postgres` | `*PostgresUserConfig` | No | PostgreSQL-specific settings |
| `mysql` | `*MySQLUserConfig` | No | MySQL-specific settings |

**PostgreSQL User Fields** (`postgres`):
| Field | Type | Description |
|-------|------|-------------|
| `superuser` | `bool` | SUPERUSER attribute |
| `createDB` | `bool` | CREATEDB attribute |
| `createRole` | `bool` | CREATEROLE attribute |
| `inherit` | `bool` | INHERIT attribute |
| `login` | `bool` | LOGIN attribute (default: true) |
| `replication` | `bool` | REPLICATION attribute |
| `bypassRLS` | `bool` | BYPASSRLS attribute |
| `connectionLimit` | `*int32` | Max connections |
| `validUntil` | `string` | Password expiration |
| `inRoles` | `[]string` | Role memberships |
| `configParameters` | `map[string]string` | Session config |

**MySQL User Fields** (`mysql`):
| Field | Type | Description |
|-------|------|-------------|
| `host` | `string` | Host pattern (default: %) |
| `authPlugin` | `MySQLAuthPlugin` | Authentication plugin |
| `resourceLimits` | `*MySQLResourceLimits` | Resource limits |
| `requireSSL` | `bool` | Require SSL |
| `accountLocked` | `bool` | Lock account |
| `passwordExpireInterval` | `*int32` | Password expiration days |

### 2. Controllers

#### 2.1 DatabaseInstance Controller

**File**: `internal/controller/databaseinstance_controller.go`

**RBAC Permissions**:
```go
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseinstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseinstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseinstances/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
```

**Reconciliation Logic**:
1. Fetch the DatabaseInstance resource
2. Check `skip-reconcile` annotation → skip if present
3. Check deletion timestamp → handle deletion
4. Add finalizer `dbops.dbprovision.io/database-instance`
5. Get credentials from referenced Secret
6. Get TLS credentials if TLS enabled
7. Build connection config using adapter helper
8. Create database adapter for engine type
9. Connect to database server
10. Ping to verify connection
11. Get server version
12. Update status:
    - Phase: `Ready`
    - Version: from database
    - LastCheckedAt: current time
    - Conditions: Connected=True, Healthy=True, Ready=True
13. Requeue after health check interval (default 60s)

**Error Handling**:
- Secret not found → Phase=Failed, requeue 30s
- Connection failed → Phase=Failed, requeue 30s
- Ping failed → Phase=Failed, requeue 30s

#### 2.2 Database Controller

**File**: `internal/controller/database_controller.go`

**RBAC Permissions**:
```go
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databases/finalizers,verbs=update
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseinstances,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
```

**Reconciliation Logic**:
1. Fetch the Database resource
2. Check `skip-reconcile` annotation
3. Check deletion timestamp → handle deletion
4. Add finalizer `dbops.dbprovision.io/database`
5. Fetch referenced DatabaseInstance
6. Wait if instance not Ready (requeue 10s)
7. Get credentials and TLS from instance's secrets
8. Create adapter and connect
9. Check if database exists
10. If not exists:
    - Set Phase=Creating
    - Build create options (engine-specific)
    - Create database
11. Update database settings (extensions, schemas, privileges)
12. Get database info (owner, size)
13. Update status:
    - Phase: `Ready`
    - Database info
    - Conditions: Synced=True, Ready=True
14. Requeue after 5 minutes

**Deletion Handling**:
1. Check deletion protection → block if enabled (unless force-delete)
2. Check deletion policy
3. If policy=Delete:
   - Connect to database
   - Drop database with force option if needed
4. Remove finalizer

#### 2.3 DatabaseUser Controller

**File**: `internal/controller/databaseuser_controller.go`

**RBAC Permissions**:
```go
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseusers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseusers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseusers/finalizers,verbs=update
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseinstances,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;delete
```

**Reconciliation Logic**:
1. Fetch the DatabaseUser resource
2. Check `skip-reconcile` annotation
3. Check deletion timestamp → handle deletion
4. Add finalizer `dbops.dbprovision.io/database-user`
5. Fetch referenced DatabaseInstance
6. Wait if instance not Ready
7. Determine password:
   - If `existingPasswordSecret` → retrieve from secret
   - If `passwordSecret.generate=true` → generate new password
   - Check if need to regenerate (secret missing)
8. Create adapter and connect
9. Check if user exists
10. If not exists:
    - Build create options (engine-specific)
    - Create user with password
11. If exists:
    - Update user attributes
12. Ensure credentials secret (with owner reference)
13. Update status:
    - Phase: `Ready`
    - Secret info (name, namespace)
    - Conditions: Synced=True, Ready=True
14. Requeue after 5 minutes

### 3. Database Adapters

#### 3.1 PostgreSQL Adapter

**Package**: `internal/adapter/postgres`

**Connection Configuration**:
```go
type ConnectionConfig struct {
    Host     string
    Port     int32
    Database string
    Username string
    Password string
    SSLMode  string    // disable, require, verify-ca, verify-full
    SSLCA    []byte
    SSLCert  []byte
    SSLKey   []byte
}
```

**Implemented Operations**:

| Operation | Method | Description |
|-----------|--------|-------------|
| Connect | `Connect(ctx)` | Establish connection pool |
| Close | `Close()` | Close all connections |
| Ping | `Ping(ctx)` | Verify connectivity |
| Version | `GetVersion(ctx)` | Get PostgreSQL version |
| Create DB | `CreateDatabase(ctx, opts)` | CREATE DATABASE with options |
| Drop DB | `DropDatabase(ctx, name, opts)` | DROP DATABASE with force |
| Update DB | `UpdateDatabase(ctx, name, opts)` | Update settings, extensions, schemas |
| DB Exists | `DatabaseExists(ctx, name)` | Check existence |
| DB Info | `GetDatabaseInfo(ctx, name)` | Get owner, size, etc. |
| Create User | `CreateUser(ctx, opts)` | CREATE ROLE with attributes |
| Drop User | `DropUser(ctx, name, opts)` | DROP ROLE |
| Update User | `UpdateUser(ctx, name, opts)` | ALTER ROLE |
| User Exists | `UserExists(ctx, name)` | Check existence |
| User Info | `GetUserInfo(ctx, name)` | Get role info |
| Grant | `Grant(ctx, opts)` | GRANT privileges |
| Revoke | `Revoke(ctx, opts)` | REVOKE privileges |
| Get Grants | `GetGrants(ctx, user, db)` | List grants |
| Create Schema | `CreateSchema(ctx, opts)` | CREATE SCHEMA |
| Drop Schema | `DropSchema(ctx, name, opts)` | DROP SCHEMA |
| Schema Exists | `SchemaExists(ctx, name)` | Check existence |
| Backup | `Backup(ctx, opts)` | pg_dump wrapper |
| Restore | `Restore(ctx, opts)` | pg_restore/psql wrapper |

**Key Implementation Details**:

1. **Connection Pool**: Uses pgxpool with configurable limits
2. **SQL Injection Prevention**: Uses `escapeIdentifier()` for identifiers
3. **Force Drop**: Terminates connections before drop
4. **Extensions**: CREATE EXTENSION IF NOT EXISTS
5. **Default Privileges**: ALTER DEFAULT PRIVILEGES

#### 3.2 MySQL Adapter

**Package**: `internal/adapter/mysql`

**Connection Configuration**:
```go
type ConnectionConfig struct {
    Host         string
    Port         int32
    Database     string
    Username     string
    Password     string
    TLSMode      string  // NONE, PREFERRED, REQUIRED, VERIFY_CA, VERIFY_IDENTITY
    TLSCA        []byte
    TLSCert      []byte
    TLSKey       []byte
}
```

**Implemented Operations**:
Same interface as PostgreSQL with MySQL-specific implementations.

**Key Implementation Details**:

1. **Connection**: Uses DSN format with go-sql-driver
2. **TLS Registration**: Custom TLS config registered with driver
3. **User Host**: CREATE USER 'name'@'host'
4. **Auth Plugins**: mysql_native_password, caching_sha2_password
5. **Resource Limits**: MAX_QUERIES_PER_HOUR, etc.

### 4. Secret Manager

**Package**: `internal/secret`

**Functions**:

| Function | Description |
|----------|-------------|
| `GetCredentials` | Retrieve username/password from Secret |
| `GetTLSCredentials` | Retrieve CA/cert/key from Secret |
| `GeneratePassword` | Secure random password generation |
| `CreateSecret` | Create Secret with owner reference |
| `CreateSecretWithOwner` | Create with runtime.Object owner |
| `UpdateSecret` | Update existing Secret |
| `EnsureSecret` | Create or update Secret |
| `EnsureSecretWithOwner` | Create or update with owner |
| `DeleteSecret` | Delete Secret |
| `SecretExists` | Check if Secret exists |
| `RenderSecretTemplate` | Render template with connection data |
| `GetPassword` | Get password from existing secret |

**Password Generation**:
```go
func GeneratePassword(opts *PasswordConfig) (string, error)
```
- Uses `crypto/rand` for secure randomness
- Configurable length (default 32)
- Optional special characters
- Character exclusion support

**Template Rendering**:
```go
type TemplateData struct {
    Username  string
    Password  string
    Host      string
    Port      int32
    Database  string
    SSLMode   string
    Namespace string
    Name      string
}
```

### 5. Utility Functions

**Package**: `internal/util`

#### Conditions

| Function | Condition Type |
|----------|---------------|
| `SetReadyCondition` | `Ready` |
| `SetConnectedCondition` | `Connected` |
| `SetHealthyCondition` | `Healthy` |
| `SetSyncedCondition` | `Synced` |
| `SetDegradedCondition` | `Degraded` |

#### Finalizers

| Constant | Value |
|----------|-------|
| `FinalizerDatabaseInstance` | `dbops.dbprovision.io/database-instance` |
| `FinalizerDatabase` | `dbops.dbprovision.io/database` |
| `FinalizerDatabaseUser` | `dbops.dbprovision.io/database-user` |

#### Annotations

| Function | Annotation |
|----------|------------|
| `ShouldSkipReconcile` | `dbops.dbprovision.io/skip-reconcile` |
| `HasForceDeleteAnnotation` | `dbops.dbprovision.io/force-delete` |
| `IsMarkedForDeletion` | Checks DeletionTimestamp |

## Testing

### Unit Tests

Tests should cover:
- Controller reconciliation logic
- Adapter operations (with mocks)
- Secret manager functions
- Utility functions

### Integration Tests

Tests require:
- Envtest for Kubernetes API
- PostgreSQL instance (Docker)
- MySQL instance (Docker)

### Running Tests

```bash
# Unit tests
make test

# Integration tests (requires envtest binaries)
make test-integration
```

## Build & Deploy

### Build

```bash
# Generate code
make generate

# Generate manifests (CRDs, RBAC)
make manifests

# Build binary
make build

# Build container
make docker-build IMG=<registry>/<image>:<tag>
```

### Deploy

```bash
# Install CRDs
make install

# Deploy operator
make deploy IMG=<registry>/<image>:<tag>

# Run locally (for development)
make run
```

## Git Commit

Phase 1 implementation was committed as:
```
feat: implement Phase 1 Core MVP - controllers, adapters, and secret manager
```

**Commit Contents**:
- API types for all CRDs
- Three controllers with full reconciliation
- PostgreSQL and MySQL adapters
- Secret manager
- Utility functions
- Generated manifests
