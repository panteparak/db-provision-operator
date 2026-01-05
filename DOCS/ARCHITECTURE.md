# DB Provision Operator - Architecture

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                          Kubernetes Cluster                              │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌────────────────────────────────────────────────────────────────────┐ │
│  │                    db-provision-operator                            │ │
│  │                                                                      │ │
│  │  ┌──────────────────┐ ┌──────────────────┐ ┌──────────────────┐   │ │
│  │  │ DatabaseInstance │ │    Database      │ │  DatabaseUser    │   │ │
│  │  │   Controller     │ │   Controller     │ │   Controller     │   │ │
│  │  └────────┬─────────┘ └────────┬─────────┘ └────────┬─────────┘   │ │
│  │           │                    │                    │              │ │
│  │           └────────────────────┴────────────────────┘              │ │
│  │                              │                                      │ │
│  │                    ┌─────────┴─────────┐                           │ │
│  │                    │   Adapter Layer   │                           │ │
│  │                    ├───────────────────┤                           │ │
│  │                    │ ┌───────────────┐ │                           │ │
│  │                    │ │   PostgreSQL  │ │                           │ │
│  │                    │ │    Adapter    │ │                           │ │
│  │                    │ └───────────────┘ │                           │ │
│  │                    │ ┌───────────────┐ │                           │ │
│  │                    │ │     MySQL     │ │                           │ │
│  │                    │ │    Adapter    │ │                           │ │
│  │                    │ └───────────────┘ │                           │ │
│  │                    └───────────────────┘                           │ │
│  │                              │                                      │ │
│  │  ┌───────────────────┐      │      ┌───────────────────┐          │ │
│  │  │  Secret Manager   │◄─────┴─────►│   Utility Layer   │          │ │
│  │  └───────────────────┘             └───────────────────┘          │ │
│  │                                                                      │ │
│  └────────────────────────────────────────────────────────────────────┘ │
│                                                                          │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐             │
│  │DatabaseInst. │    │   Database   │    │DatabaseUser  │             │
│  │    CRD       │    │     CRD      │    │    CRD       │             │
│  └──────────────┘    └──────────────┘    └──────────────┘             │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
                    ┌───────────────────────────────┐
                    │     External Databases        │
                    ├───────────────────────────────┤
                    │  ┌─────────┐    ┌─────────┐  │
                    │  │PostgreSQL│   │  MySQL  │  │
                    │  │ Server   │   │ Server  │  │
                    │  └─────────┘    └─────────┘  │
                    └───────────────────────────────┘
```

## Component Architecture

### 1. Controllers Layer

Controllers implement the reconciliation loop pattern, watching CRD resources and ensuring the actual state matches the desired state.

#### DatabaseInstance Controller
```
Responsibilities:
├── Validate database server connectivity
├── Perform periodic health checks
├── Maintain connection status in status field
└── Handle graceful deletion (finalizer cleanup)

Reconciliation Flow:
1. Fetch DatabaseInstance resource
2. Check skip-reconcile annotation
3. Handle deletion if marked
4. Add finalizer if missing
5. Get credentials from Secret
6. Get TLS credentials if enabled
7. Create database adapter
8. Connect and ping
9. Get server version
10. Update status (Ready/Failed)
11. Requeue for health check interval
```

#### Database Controller
```
Responsibilities:
├── Create databases on target instance
├── Configure database settings (encoding, collation, extensions)
├── Manage database lifecycle
└── Drop database on deletion (if policy allows)

Reconciliation Flow:
1. Fetch Database resource
2. Check skip-reconcile annotation
3. Handle deletion if marked
4. Add finalizer if missing
5. Fetch referenced DatabaseInstance
6. Wait if instance not ready
7. Create adapter and connect
8. Check if database exists
9. Create database if missing
10. Update database settings
11. Get database info
12. Update status (Ready/Failed)
```

#### DatabaseUser Controller
```
Responsibilities:
├── Create database users/roles
├── Manage passwords (generate or use existing)
├── Create credentials secrets
├── Configure user attributes and grants
└── Drop user on deletion (if policy allows)

Reconciliation Flow:
1. Fetch DatabaseUser resource
2. Check skip-reconcile annotation
3. Handle deletion if marked
4. Add finalizer if missing
5. Fetch referenced DatabaseInstance
6. Wait if instance not ready
7. Determine password (generate or existing)
8. Create adapter and connect
9. Check if user exists
10. Create or update user
11. Ensure credentials secret
12. Update status (Ready/Failed)
```

### 2. Adapter Layer

The adapter layer provides a unified interface for database operations across different engines.

#### Interface Hierarchy
```
DatabaseAdapter (main interface)
├── Connect(ctx) error
├── Close() error
├── Ping(ctx) error
├── GetVersion(ctx) (string, error)
│
├── DatabaseOperations
│   ├── CreateDatabase(ctx, opts) error
│   ├── DropDatabase(ctx, name, opts) error
│   ├── UpdateDatabase(ctx, name, opts) error
│   ├── DatabaseExists(ctx, name) (bool, error)
│   └── GetDatabaseInfo(ctx, name) (*DatabaseInfo, error)
│
├── UserOperations
│   ├── CreateUser(ctx, opts) error
│   ├── DropUser(ctx, name, opts) error
│   ├── UpdateUser(ctx, name, opts) error
│   ├── UserExists(ctx, name) (bool, error)
│   └── GetUserInfo(ctx, name) (*UserInfo, error)
│
├── GrantOperations
│   ├── Grant(ctx, opts) error
│   ├── Revoke(ctx, opts) error
│   └── GetGrants(ctx, user, db) ([]GrantInfo, error)
│
├── BackupOperations
│   ├── Backup(ctx, opts) (*BackupResult, error)
│   ├── GetBackupProgress(ctx, id) (int, error)
│   └── CancelBackup(ctx, id) error
│
├── RestoreOperations
│   ├── Restore(ctx, opts) (*RestoreResult, error)
│   ├── GetRestoreProgress(ctx, id) (int, error)
│   └── CancelRestore(ctx, id) error
│
└── SchemaOperations (PostgreSQL only)
    ├── CreateSchema(ctx, opts) error
    ├── DropSchema(ctx, name, opts) error
    └── SchemaExists(ctx, name) (bool, error)
```

#### PostgreSQL Adapter
```
Package: internal/adapter/postgres

Files:
├── adapter.go       # Core adapter, connection management
├── database.go      # Database operations
├── user.go          # User/role operations
├── grants.go        # Grant/revoke operations
├── schema.go        # Schema operations
├── backup.go        # pg_dump integration
└── restore.go       # pg_restore/psql integration

Connection:
└── Uses pgx/v5 connection pool with TLS support

Features:
├── Connection pooling with configurable limits
├── TLS/mTLS with custom CA
├── All PostgreSQL role attributes
├── Schema-level privileges
├── Default privileges (ALTER DEFAULT PRIVILEGES)
├── Extension management
└── Async backup/restore with progress tracking
```

#### MySQL Adapter
```
Package: internal/adapter/mysql

Files:
├── adapter.go       # Core adapter, connection management
├── database.go      # Database operations
├── user.go          # User operations
├── grants.go        # Grant/revoke operations
├── backup.go        # mysqldump integration
└── restore.go       # mysql restore integration

Connection:
└── Uses go-sql-driver/mysql with TLS support

Features:
├── Charset/collation configuration
├── Authentication plugins (mysql_native_password, caching_sha2_password)
├── Resource limits (MAX_QUERIES_PER_HOUR, etc.)
├── Multi-level grants (global, database, table, column, routine)
├── TLS modes (NONE, PREFERRED, REQUIRED, VERIFY_CA, VERIFY_IDENTITY)
└── Async backup/restore with progress tracking
```

### 3. Secret Manager

```
Package: internal/secret

Responsibilities:
├── Retrieve credentials from Kubernetes Secrets
├── Retrieve TLS certificates from Secrets
├── Generate cryptographically secure passwords
├── Create/update/delete Secrets with owner references
├── Render secret templates with database connection info
└── Support configurable key names for credentials

Key Functions:
├── GetCredentials(ctx, namespace, ref) - Get username/password
├── GetTLSCredentials(ctx, namespace, tlsConfig) - Get TLS certs
├── GeneratePassword(opts) - Secure password generation
├── EnsureSecretWithOwner(ctx, ...) - Create or update secret
├── RenderSecretTemplate(tmpl, data) - Template rendering
└── GetPassword(ctx, namespace, ref) - Get existing password
```

### 4. Utility Layer

```
Package: internal/util

Components:

Conditions (conditions.go):
├── SetReadyCondition - Resource readiness
├── SetConnectedCondition - Connection status
├── SetHealthyCondition - Health check status
├── SetSyncedCondition - Sync status
└── SetDegradedCondition - Degraded status

Finalizers (finalizers.go):
├── FinalizerDatabaseInstance - "dbops.dbprovision.io/database-instance"
├── FinalizerDatabase - "dbops.dbprovision.io/database"
└── FinalizerDatabaseUser - "dbops.dbprovision.io/database-user"

Annotations (annotations.go):
├── ShouldSkipReconcile - Check skip-reconcile annotation
├── HasForceDeleteAnnotation - Check force-delete annotation
└── IsMarkedForDeletion - Check deletion timestamp
```

## Data Flow

### Resource Creation Flow
```
User Creates CRD
       │
       ▼
Controller Receives Event
       │
       ▼
Fetch Resource from API Server
       │
       ▼
Add Finalizer (if missing)
       │
       ▼
Get Referenced Resources
       │
       ▼
Get Credentials from Secret
       │
       ▼
Create Database Adapter
       │
       ▼
Connect to Database Server
       │
       ▼
Execute Database Operations
       │
       ▼
Create Credentials Secret (if needed)
       │
       ▼
Update Resource Status
       │
       ▼
Requeue for Periodic Reconciliation
```

### Resource Deletion Flow
```
User Deletes CRD
       │
       ▼
Controller Receives Delete Event
       │
       ▼
Check Deletion Protection
       │
       ├── Protected + No Force Delete
       │   └── Return Error, Keep Resource
       │
       └── Not Protected or Force Delete
           │
           ▼
    Check Deletion Policy
           │
           ├── Retain
           │   └── Remove Finalizer Only
           │
           └── Delete
               │
               ▼
        Connect to Database
               │
               ▼
        Drop Resource (DB/User)
               │
               ▼
        Delete Credentials Secret
               │
               ▼
        Remove Finalizer
               │
               ▼
        Resource Removed from Cluster
```

## Security Architecture

### Credential Management
```
┌─────────────────────────────────────────────────────┐
│                  Secret Sources                      │
├─────────────────────────────────────────────────────┤
│  ┌─────────────────┐    ┌─────────────────┐        │
│  │ Admin Creds     │    │ TLS Certs       │        │
│  │ (user-provided) │    │ (user-provided) │        │
│  └────────┬────────┘    └────────┬────────┘        │
│           │                      │                  │
│           └──────────┬───────────┘                  │
│                      ▼                              │
│            ┌─────────────────┐                     │
│            │  Secret Manager │                     │
│            └────────┬────────┘                     │
│                     │                              │
│    ┌────────────────┼────────────────┐            │
│    ▼                ▼                ▼            │
│ ┌──────┐      ┌──────────┐    ┌───────────┐      │
│ │ Read │      │ Generate │    │  Create   │      │
│ │Creds │      │ Password │    │  Secret   │      │
│ └──────┘      └──────────┘    └───────────┘      │
└─────────────────────────────────────────────────────┘
```

### TLS Configuration
```
TLSConfig:
├── Enabled: bool
├── Mode: disable|require|verify-ca|verify-full (PostgreSQL)
├── Mode: NONE|PREFERRED|REQUIRED|VERIFY_CA|VERIFY_IDENTITY (MySQL)
├── SecretRef:
│   ├── Name: secret name
│   └── Keys:
│       ├── CA: ca.crt (default)
│       ├── Cert: tls.crt (default)
│       └── Key: tls.key (default)
└── InsecureSkipVerify: bool
```

## Error Handling

### Retry Strategy
```
Error Type              │ Behavior
────────────────────────┼──────────────────────────
Resource Not Found      │ Don't requeue
Connection Failed       │ Requeue after 30s
Secret Not Found        │ Requeue after 30s
Instance Not Ready      │ Requeue after 10s
Database Operation Fail │ Requeue after 30s
Deletion Protected      │ Return error, don't delete
```

### Status Conditions
```
Condition   │ Meaning
────────────┼─────────────────────────────────
Ready       │ Resource is fully reconciled
Connected   │ Can connect to database server
Healthy     │ Health check passed
Synced      │ Resource matches desired state
Degraded    │ Resource partially functional
```
