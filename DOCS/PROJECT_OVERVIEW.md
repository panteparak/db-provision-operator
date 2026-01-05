# DB Provision Operator - Project Overview

## Introduction

The **db-provision-operator** is a Kubernetes operator designed to provide unified database management across multiple database engines. It enables declarative management of database servers, databases, users, and access controls through Kubernetes Custom Resource Definitions (CRDs).

## Project Goals

1. **Unified Database Management**: Provide a consistent API for managing databases across different engines (PostgreSQL, MySQL, and future engines)
2. **Kubernetes-Native**: Leverage Kubernetes patterns (controllers, CRDs, finalizers) for lifecycle management
3. **Security-First**: Secure credential management with automatic password generation and secret templating
4. **Production-Ready**: Support for TLS/mTLS, health checks, deletion protection, and graceful cleanup
5. **Extensibility**: Adapter-based architecture for easy addition of new database engines

## Technology Stack

| Component | Technology | Version |
|-----------|------------|---------|
| Operator Framework | Operator SDK | v1.42.0 |
| Kubernetes Client | controller-runtime | Latest |
| PostgreSQL Driver | pgx/v5 | v5.x |
| MySQL Driver | go-sql-driver/mysql | v1.x |
| Go Version | Go | 1.21+ |

## Custom Resource Definitions (CRDs)

### Primary Resources

| CRD | API Group | Description |
|-----|-----------|-------------|
| `DatabaseInstance` | `dbops.dbprovision.io/v1alpha1` | Represents a database server connection |
| `Database` | `dbops.dbprovision.io/v1alpha1` | Represents a database within an instance |
| `DatabaseUser` | `dbops.dbprovision.io/v1alpha1` | Represents a database user/role |

### Resource Hierarchy

```
DatabaseInstance (server connection)
├── Database (logical database)
│   └── DatabaseUser (user with access to database)
└── DatabaseUser (instance-level user/role)
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

## Implementation Phases

### Phase 1: Core MVP (COMPLETED)
- DatabaseInstance, Database, DatabaseUser controllers
- PostgreSQL and MySQL adapters
- Secret management with password generation
- TLS/mTLS support
- Deletion protection

### Phase 2: Advanced Features (PLANNED)
- DatabaseGrant CRD for fine-grained access control
- DatabaseBackup CRD for backup management
- Cross-namespace references
- Metrics and monitoring

### Phase 3: Enterprise Features (PLANNED)
- Multi-cluster support
- Database migration tools
- Audit logging
- Policy enforcement

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
│   │   ├── mysql/         # MySQL implementation
│   │   └── types/         # Shared adapter types
│   ├── controller/        # Kubernetes controllers
│   ├── secret/            # Secret management
│   └── util/              # Utility functions
└── DOCS/                  # Project documentation
```

## Quick Start

### Prerequisites
- Kubernetes cluster (1.26+)
- kubectl configured
- Database server (PostgreSQL or MySQL)

### Installation

```bash
# Clone the repository
git clone <repository-url>
cd db-provision-operator

# Install CRDs
make install

# Run the operator
make run
```

### Example: Create a PostgreSQL Database

```yaml
# 1. Create credentials secret
apiVersion: v1
kind: Secret
metadata:
  name: postgres-admin-creds
type: Opaque
stringData:
  username: postgres
  password: supersecret

---
# 2. Create DatabaseInstance
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: postgres-server
spec:
  engine: postgres
  connection:
    host: postgres.default.svc.cluster.local
    port: 5432
    secretRef:
      name: postgres-admin-creds

---
# 3. Create Database
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: myapp-db
spec:
  name: myapp
  instanceRef:
    name: postgres-server
  postgres:
    encoding: UTF8

---
# 4. Create DatabaseUser
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: myapp-user
spec:
  username: myapp
  instanceRef:
    name: postgres-server
  databaseRef:
    name: myapp-db
  passwordSecret:
    generate: true
    secretName: myapp-db-credentials
```

## Development

```bash
# Generate code and manifests
make generate
make manifests

# Build
make build

# Run tests
make test

# Run locally
make run
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make changes following Go conventions
4. Run tests and linting
5. Submit a pull request

## License

Apache License 2.0
