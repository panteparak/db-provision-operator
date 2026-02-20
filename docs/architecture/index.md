# Architecture

Overview of DB Provision Operator's architecture and design.

## Overview

DB Provision Operator follows the Kubernetes Operator pattern, using Custom Resource Definitions (CRDs) to extend the Kubernetes API with database-specific resources.

```mermaid
graph TD
    subgraph K8s["Kubernetes Cluster"]
        subgraph CP["Control Plane"]
            API[API Server]
            ETCD[etcd]
            CM[Controller Manager]
        end
        subgraph OP["DB Provision Operator"]
            RL["Reconciler Loop"]
            IC[Instance Ctrl]
            DC[Database Ctrl]
            UC[User Ctrl]
        end
        API <--> RL
        CM --> RL
        subgraph DP["Data Plane"]
            PG[PostgreSQL Instance]
            MY[MySQL Instance]
            MR[MariaDB Instance]
        end
    end
    OP --> DP
```

## Core Components

### Controllers

Each CRD has a dedicated controller:

| Controller | Responsibility |
|------------|----------------|
| DatabaseInstance | Connection management, health checks |
| Database | Database lifecycle, extensions, schemas |
| DatabaseUser | User management, credential generation |
| DatabaseRole | Role management, permission grouping |
| DatabaseGrant | Permission management |
| DatabaseBackup | Backup execution |
| DatabaseBackupSchedule | Scheduled backup management |
| DatabaseRestore | Restore execution |

### Reconciliation Loop

```mermaid
graph LR
    A[Watch Event] --> B[Compare State]
    B --> C[Execute Actions]
    C --> D[Update Status]
    D -->|Requeue| A
```

## Resource Hierarchy

```mermaid
graph TD
    DI[DatabaseInstance] --> DB[Database]
    DI --> DU[DatabaseUser]
    DI --> DR[DatabaseRole]
    DB --> BK[DatabaseBackup]
    DB --> BS[DatabaseBackupSchedule]
    DU --> DG1[DatabaseGrant]
    DR --> DG2[DatabaseGrant]
```

### Dependencies

| Resource | Depends On |
|----------|------------|
| Database | DatabaseInstance |
| DatabaseUser | DatabaseInstance |
| DatabaseRole | DatabaseInstance |
| DatabaseGrant | DatabaseUser OR DatabaseRole |
| DatabaseBackup | Database |
| DatabaseBackupSchedule | Database |
| DatabaseRestore | DatabaseBackup, Database |

## Design Principles

### 1. Declarative Management

Resources are declared in YAML; the operator reconciles actual state to match desired state.

### 2. Idempotency

Reconciliation can run multiple times safely without side effects.

### 3. Eventually Consistent

The operator continuously reconciles until desired state is achieved.

### 4. Fail-Safe

Errors are logged and retried; resources maintain last known good state.

### 5. Secure by Default

- Credentials are stored in Kubernetes Secrets
- TLS connections supported
- Minimum required permissions

## Next Steps

- [Design Decisions](design.md) - Detailed design rationale
- [Security](security.md) - Security architecture
