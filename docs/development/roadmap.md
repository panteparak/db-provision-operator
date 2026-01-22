# Development Roadmap

This document provides the development roadmap and context for the db-provision-operator.

## Current State Summary

### Completed Phases

| Phase | Component | Status | Notes |
|-------|-----------|--------|-------|
| **Phase 1** | DatabaseInstance CRD | Complete | Connection management, health checks |
| | Database CRD | Complete | CRUD operations, extensions, schemas |
| | DatabaseUser CRD | Complete | User management, password generation |
| | PostgreSQL Adapter | Complete | Full CRUD, grants, backup/restore |
| | MySQL Adapter | Complete | Full CRUD, grants, backup/restore |
| | Secret Manager | Complete | Credentials, TLS, templates |
| | Controllers | Complete | Reconciliation, finalizers, status |
| **Phase 2** | DatabaseRole CRD | Complete | Role management for PostgreSQL/MySQL |
| | DatabaseGrant CRD | Complete | Fine-grained grant management |
| | DatabaseBackup CRD | Complete | Backup lifecycle management |
| | DatabaseRestore CRD | Complete | Restore from backup or path |
| | DatabaseBackupSchedule CRD | Complete | Cron scheduling, retention policies |
| | Retry Utility | Complete | Exponential backoff with jitter |
| **Phase 2.5** | S3 Storage Backend | Complete | AWS S3 compatible with custom endpoints |
| | GCS Storage Backend | Complete | Google Cloud Storage |
| | Azure Blob Backend | Complete | Azure Blob Storage |
| | PVC Storage Backend | Complete | Kubernetes PVC storage |
| | Compression | Complete | gzip, lz4, zstd algorithms |
| | Encryption | Complete | AES-256-GCM encryption |
| | Prometheus Metrics | Complete | 25 metrics, 100% coverage |
| | ServiceMonitor | Complete | Auto-discovery for Prometheus |
| | PrometheusRules | Complete | 10+ alerting rules |

## Phase 3 Planned Work

### 3.1 Multi-Cluster Support

**Concept**: Manage databases across multiple clusters

**Approach Options**:
1. Remote cluster kubeconfig references
2. Submariner/Skupper for cross-cluster networking
3. Hub-spoke model with agent per cluster

### 3.2 Database Migration CRD

**Purpose**: Manage database schema migrations

**Proposed Integration**:
- Flyway integration
- Liquibase integration
- Or simple SQL migration files

### 3.3 Audit Logging

**Requirements**:
- Log all database operations
- Include user, operation, timestamp
- Support external audit sinks (Elasticsearch, Loki)

### 3.4 Policy Enforcement

**Concept**: OPA/Gatekeeper integration for policies

**Example Policies**:
- Password complexity requirements
- Naming conventions
- Required labels/annotations
- Deletion protection rules

## Development Guidelines

### Adding a New Database Engine

1. Create new package: `internal/adapter/<engine>/`
2. Implement `DatabaseAdapter` interface from `internal/adapter/types/`
3. Add engine type to `EngineType` enum in `api/v1alpha1/common_types.go`
4. Add engine-specific config types in `api/v1alpha1/<engine>_types.go`
5. Update `NewAdapter()` factory in `internal/adapter/adapter.go`
6. Add engine-specific option handling in controllers

### Adding a New CRD

1. Create type file: `api/v1alpha1/<resource>_types.go`
2. Run `make generate` to generate DeepCopy methods
3. Run `make manifests` to generate CRD YAML
4. Create controller: `internal/controller/<resource>_controller.go`
5. Register controller in `cmd/main.go`
6. Add RBAC markers to controller
7. Run `make manifests` again to update RBAC

### Code Style

- Follow standard Go conventions
- Use `logf.FromContext(ctx)` for logging
- Set status conditions for all outcomes
- Always handle errors and update status
- Use finalizers for cleanup operations
- Add kubebuilder markers for RBAC and validation

## File Locations Reference

```
db-provision-operator/
├── api/v1alpha1/
│   ├── common_types.go          # Shared types (EngineType, Phase, etc.)
│   ├── databaseinstance_types.go # DatabaseInstance CRD
│   ├── database_types.go         # Database CRD
│   ├── databaseuser_types.go     # DatabaseUser CRD
│   ├── postgres_types.go         # PostgreSQL-specific types
│   ├── mysql_types.go            # MySQL-specific types
│   └── groupversion_info.go      # API group registration
│
├── internal/
│   ├── adapter/
│   │   ├── types/types.go        # Adapter interfaces and option types
│   │   ├── adapter.go            # Factory and helpers
│   │   ├── postgres/             # PostgreSQL implementation
│   │   └── mysql/                # MySQL implementation
│   │
│   ├── controller/
│   │   ├── databaseinstance_controller.go
│   │   ├── database_controller.go
│   │   ├── databaseuser_controller.go
│   │   ├── databaserole_controller.go
│   │   ├── databasegrant_controller.go
│   │   ├── databasebackup_controller.go
│   │   ├── databaserestore_controller.go
│   │   └── databasebackupschedule_controller.go
│   │
│   ├── metrics/
│   │   ├── metrics.go            # Prometheus metrics definitions
│   │   └── metrics_test.go       # Metrics tests
│   │
│   ├── storage/
│   │   ├── storage.go            # Storage interface
│   │   ├── s3.go                 # AWS S3 backend
│   │   ├── gcs.go                # Google Cloud Storage backend
│   │   ├── azure.go              # Azure Blob Storage backend
│   │   ├── pvc.go                # PVC storage backend
│   │   ├── compression.go        # gzip, lz4, zstd compression
│   │   └── encryption.go         # AES-256-GCM encryption
│   │
│   ├── secret/
│   │   └── manager.go            # Secret management
│   │
│   └── util/
│       ├── conditions.go         # Status condition helpers
│       ├── finalizers.go         # Finalizer constants
│       ├── annotations.go        # Annotation helpers
│       └── retry.go              # Exponential backoff retry
│
├── config/
│   ├── crd/bases/                # Generated CRD manifests
│   ├── rbac/                     # Generated RBAC manifests
│   ├── prometheus/               # Prometheus monitoring
│   └── samples/                  # Example resources
│
├── cmd/
│   └── main.go                   # Operator entrypoint
│
└── docs/                         # Project documentation
```

## Quick Commands

```bash
# Development cycle
make generate           # Generate DeepCopy, etc.
make manifests          # Generate CRDs, RBAC
make build              # Build binary
make run                # Run locally

# Testing
make test               # Run tests (needs envtest)
go build ./...          # Verify compilation
go vet ./...            # Static analysis

# Deployment
make install            # Install CRDs to cluster
make deploy IMG=<img>   # Deploy operator
make undeploy           # Remove operator

# Cleanup
make uninstall          # Remove CRDs
```

## Handoff Checklist

Before starting next phase:

- [ ] Review architecture documentation
- [ ] Run `go build ./...` to verify clean build
- [ ] Run `make generate && make manifests` to ensure generated files are current
- [ ] Review open issues/TODOs in code
- [ ] Set up test database instances (PostgreSQL, MySQL)
- [ ] Understand the adapter interface pattern
- [ ] Review secret manager for credential handling
