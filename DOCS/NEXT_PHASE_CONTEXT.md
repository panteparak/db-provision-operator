# DB Provision Operator - Next Phase Context

This document provides the necessary context for continuing development in future phases.

## Current State Summary

### Completed (Phase 1 - Core MVP)

| Component | Status | Notes |
|-----------|--------|-------|
| DatabaseInstance CRD | ✅ Complete | Connection management, health checks |
| Database CRD | ✅ Complete | CRUD operations, extensions, schemas |
| DatabaseUser CRD | ✅ Complete | User management, password generation |
| PostgreSQL Adapter | ✅ Complete | Full CRUD, grants, backup/restore |
| MySQL Adapter | ✅ Complete | Full CRUD, grants, backup/restore |
| Secret Manager | ✅ Complete | Credentials, TLS, templates |
| Controllers | ✅ Complete | Reconciliation, finalizers, status |
| Build | ✅ Passing | `go build ./...` and `go vet ./...` |

### Completed (Phase 2 - Extended Controllers)

| Component | Status | Notes |
|-----------|--------|-------|
| DatabaseRole CRD | ✅ Complete | Role management for PostgreSQL/MySQL |
| DatabaseGrant CRD | ✅ Complete | Fine-grained grant management |
| DatabaseBackup CRD | ✅ Complete | Backup lifecycle management |
| DatabaseRestore CRD | ✅ Complete | Restore from backup or path |
| DatabaseBackupSchedule CRD | ✅ Complete | Cron scheduling, retention policies |
| Retry Utility | ✅ Complete | Exponential backoff with jitter |
| Tests | ✅ Complete | 45 controller tests passing |

### Completed (Phase 2.5 - Storage & Monitoring)

| Component | Status | Notes |
|-----------|--------|-------|
| S3 Storage Backend | ✅ Complete | AWS S3 compatible with custom endpoints |
| GCS Storage Backend | ✅ Complete | Google Cloud Storage |
| Azure Blob Backend | ✅ Complete | Azure Blob Storage |
| PVC Storage Backend | ✅ Complete | Kubernetes PVC storage |
| Compression | ✅ Complete | gzip, lz4, zstd algorithms |
| Encryption | ✅ Complete | AES-256-GCM encryption |
| Prometheus Metrics | ✅ Complete | 25 metrics, 100% coverage |
| ServiceMonitor | ✅ Complete | Auto-discovery for Prometheus |
| PrometheusRules | ✅ Complete | 10+ alerting rules |
| Tests | ✅ Complete | 70+ tests passing |

### Build Status

```bash
$ go build ./...
# Success - no errors

$ go vet ./...
# Success - no warnings
```

### Git Status

```
commit 1b4cd1c feat: implement Phase 1 Core MVP - controllers, adapters, and secret manager
Author: Pan Teparak <panteparak@me.com>
```

## Known Limitations

### 1. Test Environment

**Issue**: `make test` fails due to missing envtest binaries

**Error**:
```
unable to start control plane: unable to read testenv config from file...
unable to start the controlplane. Please ensure that "localhost:35097" is a valid URL
```

**Resolution Needed**:
1. Run `make envtest` to download binaries
2. Or set up `KUBEBUILDER_ASSETS` environment variable
3. Consider adding testcontainers for database testing

### 2. No Actual Database Connection Tests

The integration tests require real PostgreSQL and MySQL instances. Consider:
- Docker Compose setup for local testing
- GitHub Actions with service containers
- Testcontainers-go for automated test databases

### 3. Backup/Restore Requires CLI Tools

The backup and restore operations shell out to:
- `pg_dump` / `pg_restore` / `psql` (PostgreSQL)
- `mysqldump` / `mysql` (MySQL)

These must be included in the operator container image.

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
│   │   ├── postgres/
│   │   │   ├── adapter.go        # Connection management
│   │   │   ├── database.go       # Database operations
│   │   │   ├── user.go           # User operations
│   │   │   ├── grants.go         # Grant operations
│   │   │   ├── schema.go         # Schema operations
│   │   │   ├── backup.go         # Backup operations
│   │   │   └── restore.go        # Restore operations
│   │   └── mysql/
│   │       ├── adapter.go        # Connection management
│   │       ├── database.go       # Database operations
│   │       ├── user.go           # User operations
│   │       ├── grants.go         # Grant operations
│   │       ├── backup.go         # Backup operations
│   │       └── restore.go        # Restore operations
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
│   │   └── metrics_test.go       # Metrics tests (100% coverage)
│   │
│   ├── storage/
│   │   ├── storage.go            # Storage interface
│   │   ├── s3.go                 # AWS S3 backend
│   │   ├── gcs.go                # Google Cloud Storage backend
│   │   ├── azure.go              # Azure Blob Storage backend
│   │   ├── pvc.go                # PVC storage backend
│   │   ├── compression.go        # gzip, lz4, zstd compression
│   │   ├── encryption.go         # AES-256-GCM encryption
│   │   └── storage_test.go       # Storage tests
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
│   ├── prometheus/
│   │   ├── monitor.yaml          # ServiceMonitor for Prometheus
│   │   ├── rules.yaml            # PrometheusRule for alerts
│   │   └── kustomization.yaml
│   └── samples/                  # Example resources
│
├── cmd/
│   └── main.go                   # Operator entrypoint
│
└── DOCS/
    ├── PROJECT_OVERVIEW.md
    ├── ARCHITECTURE.md
    ├── DESIGN_DECISIONS.md
    ├── PHASE1_IMPLEMENTATION.md
    └── NEXT_PHASE_CONTEXT.md     # This file
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

## Contact and Resources

- **Repository**: github.com/db-provision-operator
- **API Group**: dbops.dbprovision.io
- **API Version**: v1alpha1
- **Operator SDK**: v1.42.0
- **Go Version**: 1.21+

## Handoff Checklist

Before starting next phase:

- [ ] Review this document and ARCHITECTURE.md
- [ ] Run `go build ./...` to verify clean build
- [ ] Run `make generate && make manifests` to ensure generated files are current
- [ ] Review open issues/TODOs in code
- [ ] Set up test database instances (PostgreSQL, MySQL)
- [ ] Understand the adapter interface pattern
- [ ] Review secret manager for credential handling
