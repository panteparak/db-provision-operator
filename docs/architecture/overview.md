# System Overview

## Introduction

The **db-provision-operator** is a Kubernetes operator designed to provide unified database management across multiple database engines. It enables declarative management of database servers, databases, users, and access controls through Kubernetes Custom Resource Definitions (CRDs).

## Project Goals

1. **Unified Database Management**: Provide a consistent API for managing databases across different engines (PostgreSQL, MySQL, and future engines)
2. **Kubernetes-Native**: Leverage Kubernetes patterns (controllers, CRDs, finalizers) for lifecycle management
3. **Security-First**: Secure credential management with automatic password generation and secret templating
4. **Production-Ready**: Support for TLS/mTLS, health checks, deletion protection, and graceful cleanup
5. **Extensibility**: Adapter-based architecture for easy addition of new database engines

## High-Level Architecture

```mermaid
graph TD
    subgraph K8s["Kubernetes Cluster"]
        subgraph Operator["db-provision-operator"]
            subgraph CoreCtrl["Core Controllers"]
                DIC[Instance Controller]
                DBC[Database Controller]
                DUC[User Controller]
            end
            subgraph AccessCtrl["Access Controllers"]
                DRC[Role Controller]
                DGC[Grant Controller]
            end
            subgraph BackupCtrl["Backup Controllers"]
                DBKC[Backup Controller]
                DBSC[Schedule Controller]
                DRSC[Restore Controller]
            end
            subgraph ClusterCtrl["Cluster-Scoped Controllers"]
                CDIC[ClusterInstance Controller]
                CDRC[ClusterRole Controller]
                CDGC[ClusterGrant Controller]
            end
            CoreCtrl & AccessCtrl & BackupCtrl & ClusterCtrl --> AL
            subgraph AL["Adapter Layer"]
                PGA[PostgreSQL Adapter]
                MYA[MySQL/MariaDB Adapter]
                CRA[CockroachDB Adapter]
                CHA[ClickHouse Adapter]
            end
            SM[Secret Manager] <--> AL
            SB[SQL Builder] <--> AL
        end
    end
    AL --> ExtDB
    subgraph ExtDB["External Databases"]
        PGS[PostgreSQL]
        MYS[MySQL/MariaDB]
        CRS[CockroachDB]
        CHS[ClickHouse]
    end
```

## Technology Stack

| Component | Technology | Version |
|-----------|------------|---------|
| Operator Framework | Operator SDK | v1.42.0 |
| Kubernetes Client | controller-runtime | Latest |
| PostgreSQL Driver | pgx/v5 | v5.x |
| MySQL Driver | go-sql-driver/mysql | v1.x |
| ClickHouse Driver | clickhouse-go/v2 | v2.x |
| Go Version | Go | 1.24+ |

## Custom Resource Definitions (CRDs)

### Primary Resources

| CRD | API Group | Description |
|-----|-----------|-------------|
| `DatabaseInstance` | `dbops.dbprovision.io/v1alpha1` | Represents a database server connection |
| `Database` | `dbops.dbprovision.io/v1alpha1` | Represents a database within an instance |
| `DatabaseUser` | `dbops.dbprovision.io/v1alpha1` | Represents a database user/role |
| `DatabaseRole` | `dbops.dbprovision.io/v1alpha1` | Represents a permission group |
| `DatabaseGrant` | `dbops.dbprovision.io/v1alpha1` | Represents fine-grained access control |
| `DatabaseBackup` | `dbops.dbprovision.io/v1alpha1` | Represents a backup operation |
| `DatabaseBackupSchedule` | `dbops.dbprovision.io/v1alpha1` | Represents scheduled backups |
| `DatabaseRestore` | `dbops.dbprovision.io/v1alpha1` | Represents a restore operation |
| `ClusterDatabaseInstance` | `dbops.dbprovision.io/v1alpha1` | Cluster-scoped database server connection |
| `ClusterDatabaseRole` | `dbops.dbprovision.io/v1alpha1` | Cluster-scoped permission group |
| `ClusterDatabaseGrant` | `dbops.dbprovision.io/v1alpha1` | Cluster-scoped fine-grained access control |

### Resource Hierarchy

```mermaid
graph TD
    DI[DatabaseInstance] --> DB[Database]
    DI --> DU[DatabaseUser]
    DI --> DR[DatabaseRole]
    DB --> DBK[DatabaseBackup]
    DB --> DBS[DatabaseBackupSchedule]
    DU --> DG1[DatabaseGrant]
    DR --> DG2[DatabaseGrant]
    DBK --> DRE[DatabaseRestore]

    CDI[ClusterDatabaseInstance] --> CDR[ClusterDatabaseRole]
    CDI -.->|"referenced via clusterInstanceRef"| DB
    CDI -.->|"referenced via clusterInstanceRef"| DU
    CDR --> CDG[ClusterDatabaseGrant]
```

## Component Architecture

### Controllers Layer

Controllers implement the reconciliation loop pattern, watching CRD resources and ensuring the actual state matches the desired state.

#### DatabaseInstance Controller
```
Responsibilities:
├── Validate database server connectivity
├── Perform periodic health checks
├── Maintain connection status in status field
└── Handle graceful deletion (finalizer cleanup)
```

#### Database Controller
```
Responsibilities:
├── Create databases on target instance
├── Configure database settings (encoding, collation, extensions)
├── Manage database lifecycle
└── Drop database on deletion (if policy allows)
```

#### DatabaseUser Controller
```
Responsibilities:
├── Create database users/roles
├── Manage passwords (generate or use existing)
├── Create credentials secrets
├── Configure user attributes and grants
└── Drop user on deletion (if policy allows)
```

### Adapter Layer

The adapter layer provides a unified interface for database operations across different engines.

#### Interface Hierarchy
```mermaid
classDiagram
    class DatabaseAdapter {
        +Connect(ctx) error
        +Close() error
        +Ping(ctx) error
        +GetVersion(ctx) (string, error)
    }
    class DatabaseOperations {
        +CreateDatabase(ctx, opts) error
        +DropDatabase(ctx, name, opts) error
        +UpdateDatabase(ctx, name, opts) error
        +DatabaseExists(ctx, name) (bool, error)
        +GetDatabaseInfo(ctx, name) (*DatabaseInfo, error)
    }
    class UserOperations {
        +CreateUser(ctx, opts) error
        +DropUser(ctx, name, opts) error
        +UpdateUser(ctx, name, opts) error
        +UserExists(ctx, name) (bool, error)
        +GetUserInfo(ctx, name) (*UserInfo, error)
    }
    class GrantOperations {
        +Grant(ctx, opts) error
        +Revoke(ctx, opts) error
        +GetGrants(ctx, user, db) ([]GrantInfo, error)
    }
    class BackupOperations {
        +Backup(ctx, opts) (*BackupResult, error)
        +GetBackupProgress(ctx, id) (int, error)
        +CancelBackup(ctx, id) error
    }
    class RestoreOperations {
        +Restore(ctx, opts) (*RestoreResult, error)
        +GetRestoreProgress(ctx, id) (int, error)
        +CancelRestore(ctx, id) error
    }
    class SchemaOperations {
        +CreateSchema(ctx, opts) error
        +DropSchema(ctx, name, opts) error
        +SchemaExists(ctx, name) (bool, error)
    }
    DatabaseAdapter <|-- DatabaseOperations
    DatabaseAdapter <|-- UserOperations
    DatabaseAdapter <|-- GrantOperations
    DatabaseAdapter <|-- BackupOperations
    DatabaseAdapter <|-- RestoreOperations
    DatabaseAdapter <|-- SchemaOperations
```

### Secret Manager

```
Package: internal/secret

Responsibilities:
├── Retrieve credentials from Kubernetes Secrets
├── Retrieve TLS certificates from Secrets
├── Generate cryptographically secure passwords
├── Create/update/delete Secrets with owner references
├── Render secret templates with database connection info
└── Support configurable key names for credentials
```

## Data Flow

### Resource Creation Flow
```mermaid
graph TD
    A[User Creates CRD] --> B[Controller Receives Event]
    B --> C[Fetch Resource from API Server]
    C --> D[Add Finalizer]
    D --> E[Get Referenced Resources]
    E --> F[Get Credentials from Secret]
    F --> G[Create Database Adapter]
    G --> H[Connect to Database Server]
    H --> I[Execute Database Operations]
    I --> J[Create Credentials Secret]
    J --> K[Update Resource Status]
    K --> L[Requeue for Periodic Reconciliation]
```

### Resource Deletion Flow
```mermaid
graph TD
    A[User Deletes CRD] --> B[Controller Receives Delete Event]
    B --> C{Check Deletion Protection}
    C -->|Protected + No Force| D[Return Error, Keep Resource]
    C -->|Not Protected or Force| E{Has Child Dependencies?}
    E -->|Children Exist + No Force| F[Phase=Failed, DependenciesExist, Requeue 10s]
    E -->|No Children| G{Check Deletion Policy}
    E -->|Children Exist + Force| H[Enter PhasePendingDeletion]
    H --> I{confirm-force-delete Hash Matches?}
    I -->|No| J[Wait for Confirmation]
    I -->|Yes| K[Cascade Delete Children]
    K --> L{All Children Gone?}
    L -->|No| K
    L -->|Yes| G
    G -->|Retain| M[Remove Finalizer Only]
    G -->|Delete| N[Connect to Database]
    N --> O["Drop Resource (DB/User/Grant)"]
    O -->|Success| P[Delete Credentials Secret]
    O -->|Failure + Force| P
    O -->|Failure + No Force| Q[Return Error]
    P --> R[Remove Finalizer]
    R --> S[Resource Removed from Cluster]
```

## Project Structure

```
db-provision-operator/
├── api/v1alpha1/           # CRD type definitions
├── cmd/main.go             # Operator entrypoint
├── config/                 # Kubernetes manifests
│   ├── crd/               # Generated CRD manifests
│   ├── rbac/              # RBAC configuration
│   └── samples/           # Example resources
├── internal/
│   ├── adapter/           # Database adapters
│   │   ├── postgres/      # PostgreSQL implementation
│   │   ├── mysql/         # MySQL/MariaDB implementation
│   │   ├── cockroachdb/   # CockroachDB implementation
│   │   ├── clickhouse/    # ClickHouse implementation
│   │   ├── sqlbuilder/    # Centralized SQL builder with dialect escaping
│   │   └── types/         # Shared adapter types
│   ├── features/          # Kubernetes controllers (handler + repository per feature)
│   ├── secret/            # Secret management
│   └── util/              # Utility functions
└── docs/                  # Project documentation
```

## Supported Database Engines

### PostgreSQL
- Full support for database operations (create, drop, update)
- User/role management with all PostgreSQL attributes
- Schema-level and default privileges
- Extensions management
- Backup/restore using pg_dump and pg_restore

### MySQL
- Database operations with charset/collation support
- User management with resource limits and authentication plugins
- Grant management at multiple levels (global, database, table, column, routine)
- Backup/restore using mysqldump

### CockroachDB
- PostgreSQL wire-compatible via pgx driver
- Database CRUD operations
- User/role management (no SUPERUSER, REPLICATION, or BYPASSRLS)
- Grant management (database, table, schema levels)
- Native BACKUP/RESTORE commands
- Multi-region deployment support

### ClickHouse
- Native protocol via clickhouse-go/v2 with LZ4 compression
- Database operations with engine selection (Atomic, Lazy, Replicated)
- User management with allowed hosts and default database
- Role management with settings
- Grant management at global, database, and table levels
- Privilege allowlist validation via SQL builder
- Backup/restore not yet supported
