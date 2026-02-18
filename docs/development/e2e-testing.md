# E2E Testing

End-to-end tests validate the full operator lifecycle against real databases and a real Kubernetes cluster.

## Overview

| Aspect | Detail |
|--------|--------|
| **Framework** | Ginkgo v2 + Gomega |
| **Build tag** | `e2e` |
| **Engines** | PostgreSQL, MySQL, MariaDB, CockroachDB |
| **Cluster** | k3d (lightweight k3s in Docker) |
| **Databases** | Docker Compose on the host |
| **Operator deployment** | Helm chart in k3d (CI) / Kustomize (local) |

## Architecture

Databases run in Docker Compose on the host machine. The operator runs inside a k3d cluster and reaches databases via `host.k3d.internal` — a DNS name that k3d automatically resolves to the Docker host IP.

```
┌──────────────────────────────────────────────────────────────────┐
│                         Local Machine                            │
│                                                                  │
│  ┌─────────────────────┐      ┌────────────────────────────────┐ │
│  │   Docker Compose    │      │         k3d Cluster            │ │
│  │                     │      │                                │ │
│  │  ┌──────────────┐   │      │  ┌──────────────────────────┐  │ │
│  │  │ PostgreSQL   │◄──┼──────┼──│  Operator Pod            │  │ │
│  │  │ :15432       │   │      │  │  (db-provision-operator)  │  │ │
│  │  └──────────────┘   │      │  └──────────────────────────┘  │ │
│  │  ┌──────────────┐   │      │              │                 │ │
│  │  │ MySQL        │◄──┼──────┼── host.k3d.internal            │ │
│  │  │ :13306       │   │      │                                │ │
│  │  └──────────────┘   │      │  ┌──────────────────────────┐  │ │
│  │  ┌──────────────┐   │      │  │  DatabaseInstance CRs    │  │ │
│  │  │ MariaDB      │   │      │  │  → host.k3d.internal     │  │ │
│  │  │ :13307       │◄──┼──────┼──│                          │  │ │
│  │  └──────────────┘   │      │  └──────────────────────────┘  │ │
│  │  ┌──────────────┐   │      │                                │ │
│  │  │ CockroachDB  │◄──┼──────┤                                │ │
│  │  │ :26257       │   │      │                                │ │
│  │  └──────────────┘   │      └────────────────────────────────┘ │
│  └─────────────────────┘                                         │
│            │                                                     │
│            ▼                                                     │
│  ┌─────────────────────┐                                         │
│  │  E2E Tests (go test)│  connects to 127.0.0.1                  │
│  └─────────────────────┘                                         │
└──────────────────────────────────────────────────────────────────┘
```

### Dual-Connection Model

Each test connects to the database via **two paths**:

```
┌─────────────┐  E2E_DATABASE_HOST (127.0.0.1)    ┌────────────┐
│  Test Runner ├──────────────────────────────────►│  Database   │
│  (go test)   │                                    │  (Docker    │
└─────────────┘                                    │   Compose)  │
                                                   │             │
┌─────────────┐  E2E_INSTANCE_HOST                 │             │
│  Operator    │  (host.k3d.internal) ────────────►│             │
│  (k3d pod)   │                                    └────────────┘
└─────────────┘
```

1. **Test verifier** (`E2E_DATABASE_HOST`): The test process connects from the host to the database on `127.0.0.1` to verify that the operator's SQL operations (CREATE DATABASE, CREATE USER, etc.) took effect.
2. **Operator / CR** (`E2E_INSTANCE_HOST`): The DatabaseInstance CR uses `host.k3d.internal` so the operator pod inside k3d can reach the database on the Docker host.

### Key Assumptions

- **No database pods in k3d.** All databases run in Docker Compose outside the cluster.
- **Tests that need to interact with database pods (e.g., restart, kill) use `Skip()`.** Since pods don't exist in this mode, pod-dependent tests are skipped.
- **`host.k3d.internal`** is provided automatically by k3d. No manual configuration needed.
- **Health checks** in `docker-compose.e2e.yml` ensure databases are ready before tests start.
- **Init scripts** in `test/e2e/fixtures/*-init/` create the admin credentials and initial setup.

## Test Suite Architecture

### Suite Bootstrap (`e2e_suite_test.go`)

The suite creates two clients:

- `k8sClient` (`kubernetes.Interface`) — for core resources (Secrets, Namespaces)
- `dynamicClient` (`dynamic.Interface`) — for CRD resources (DatabaseInstance, Database, etc.)

`BeforeSuite` verifies cluster connectivity and that the operator pod is running.

### GVR Registry (11 CRDs)

| GVR Variable | Resource |
|-------------|----------|
| `databaseInstanceGVR` | `databaseinstances` |
| `databaseGVR` | `databases` |
| `databaseUserGVR` | `databaseusers` |
| `databaseRoleGVR` | `databaseroles` |
| `databaseGrantGVR` | `databasegrants` |
| `databaseBackupGVR` | `databasebackups` |
| `databaseRestoreGVR` | `databaserestores` |
| `databaseBackupScheduleGVR` | `databasebackupschedules` |
| `clusterDatabaseInstanceGVR` | `clusterdatabaseinstances` |
| `clusterDatabaseRoleGVR` | `clusterdatabaseroles` |
| `clusterDatabaseGrantGVR` | `clusterdatabasegrants` |

### Engine-Specific Test Files

| File | Engine | Label |
|------|--------|-------|
| `postgresql_test.go` | PostgreSQL | `postgresql` |
| `mysql_test.go` | MySQL | `mysql` |
| `mariadb_test.go` | MariaDB | `mariadb` |
| `cockroachdb_test.go` | CockroachDB | `cockroachdb` |
| `clusterdatabaseinstance_test.go` | PostgreSQL | `clusterdatabaseinstance` |
| `clusterdatabaserole_test.go` | PostgreSQL | `clusterdatabaserole` |
| `clusterdatabasegrant_test.go` | PostgreSQL | `clusterdatabasegrant` |

### Test Phases (Ordered)

Each engine test file uses `Ordered` containers to run phases sequentially:

1. **Setup** (`BeforeAll`): Create verifier, read admin credentials from Secret
2. **DatabaseInstance**: Create instance CR, verify `Ready` phase
3. **Database**: Create database CR, verify it exists in the real DB
4. **DatabaseUser**: Create user CR, verify login works
5. **DatabaseRole / DatabaseGrant**: Create role/grant CRs, verify permissions
6. **Cleanup** (`AfterAll`): Delete all CRs, verify finalizers complete

### DatabaseVerifier Interface

Each engine has a verifier in `test/e2e/testutil/` that validates actual database state:

| Verifier | Engine | Key Methods |
|----------|--------|-------------|
| `PostgresVerifier` | PostgreSQL | `DatabaseExists()`, `UserExists()`, `UserCanLogin()` |
| `MySQLVerifier` | MySQL / MariaDB | `DatabaseExists()`, `UserExists()`, `UserCanLogin()` |
| `CockroachDBVerifier` | CockroachDB | `DatabaseExists()`, `UserExists()`, `UserCanLogin()` |

### Timeout Constants

| Constant | Value | Usage |
|----------|-------|-------|
| `reconcileTimeout` | 30s | CR phase transitions, resource creation/deletion |
| `pollingInterval` | 2s | `Eventually` / `Consistently` polling frequency |
| `podRestartTimeout` | 3m | Infrastructure recovery (skipped in Docker Compose mode) |
| `driftTimeout` | 60s | Drift detection and correction cycles |
| `timeout` (MySQL/MariaDB) | 2m | General operations |

### Skip Patterns

Tests that require in-cluster database pods use `Skip()`:

```go
Skip("Pod restart tests require in-cluster database pods")
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `E2E_DATABASE_ENGINE` | _(none)_ | Which engine suite to run (set by Makefile) |
| `E2E_DATABASE_HOST` | `host.k3d.internal` | Host for test verifier connections |
| `E2E_DATABASE_PORT` | Engine default | Port for test verifier connections |
| `E2E_INSTANCE_HOST` | `host.k3d.internal` | Host for the DatabaseInstance CR (what operator uses) |
| `E2E_INSTANCE_PORT` | Engine default | Port for the DatabaseInstance CR |
| `E2E_POSTGRES_PORT` | `15432` | PostgreSQL Docker Compose host port |
| `E2E_MYSQL_PORT` | `13306` | MySQL Docker Compose host port |
| `E2E_MARIADB_PORT` | `13307` | MariaDB Docker Compose host port |
| `E2E_COCKROACHDB_PORT` | `26257` | CockroachDB Docker Compose host port |
| `E2E_K3D_CLUSTER` | `dbprov-e2e` | k3d cluster name |
| `E2E_IMG` | `db-provision-operator:e2e` | Operator Docker image name |

## Quick Start

### One Command

```bash
# PostgreSQL
make e2e-local-postgresql

# MySQL
make e2e-local-mysql

# MariaDB
make e2e-local-mariadb

# CockroachDB
make e2e-local-cockroachdb

# All engines sequentially
make e2e-local-all
```

### Step-by-Step Setup

Each step is a separate Makefile target — the same targets CI uses:

```bash
# 1. Start database(s) via Docker Compose (with health checks)
make e2e-db-up E2E_DATABASE=postgresql

# 2. Create k3d cluster (idempotent — skips if exists)
make e2e-cluster-create

# 3. Build operator Docker image
make e2e-docker-build

# 4. Load image into k3d
make e2e-image-load

# 5. Install CRDs into the cluster
make e2e-install-crds

# 6. Deploy operator (via Kustomize)
make e2e-deploy-operator

# 7. Create DatabaseInstance CR (points to host.k3d.internal)
make e2e-create-db-instance E2E_DATABASE=postgresql

# 8. Run E2E tests
make e2e-local-run-tests E2E_DATABASE=postgresql
```

## Makefile Targets Reference

### Micro-Step Targets

These targets are called individually by CI and can be used for debugging:

| Target | Description |
|--------|-------------|
| `e2e-db-up` | Start database(s) via Docker Compose (with health checks, 180s timeout) |
| `e2e-db-down` | Stop database(s) and remove volumes |
| `e2e-cluster-create` | Create k3d cluster (idempotent) |
| `e2e-cluster-delete` | Delete k3d cluster |
| `e2e-docker-build` | Build operator Docker image |
| `e2e-image-load` | Load image into k3d |
| `e2e-install-crds` | Install CRDs into cluster |
| `e2e-deploy-operator` | Deploy operator and wait for readiness |
| `e2e-create-db-instance` | Create DatabaseInstance CR pointing to Docker host |
| `e2e-local-run-tests` | Run E2E tests for a specific engine |

### Unified Targets

These combine micro-steps into single commands:

| Target | Description |
|--------|-------------|
| `e2e-local-setup` | Full setup (DB + k3d + operator). Use `E2E_DATABASE` for single engine |
| `e2e-local-postgresql` | Setup + PostgreSQL tests |
| `e2e-local-mysql` | Setup + MySQL tests |
| `e2e-local-mariadb` | Setup + MariaDB tests |
| `e2e-local-cockroachdb` | Setup + CockroachDB tests |
| `e2e-local-all` | Setup + all engine tests (sequential) |
| `e2e-local-cleanup` | Delete k3d cluster + stop databases |

### Utility Targets

| Target | Description |
|--------|-------------|
| `e2e-logs` | Tail operator logs from the cluster |
| `e2e-debug` | Dump cluster state + Docker Compose logs to file |

## Fixtures Reference

### Init Scripts

Each engine has an init directory mounted into the Docker Compose container:

| Directory | Purpose |
|-----------|---------|
| `test/e2e/fixtures/postgres-init/` | Creates admin user, sets up `pg_hba.conf` |
| `test/e2e/fixtures/mysql-init/` | Creates admin user with required privileges |
| `test/e2e/fixtures/mariadb-init/` | Creates admin user with required privileges |
| `test/e2e/fixtures/cockroachdb-init/` | Creates admin user (runs via init container) |

### DatabaseInstance Fixtures

These YAML files define the `DatabaseInstance` CR pointing to `host.k3d.internal`:

| File | Engine |
|------|--------|
| `test/e2e/fixtures/postgresql-local.yaml` | PostgreSQL |
| `test/e2e/fixtures/mysql-local.yaml` | MySQL |
| `test/e2e/fixtures/mariadb-local.yaml` | MariaDB |
| `test/e2e/fixtures/cockroachdb-local.yaml` | CockroachDB |

### Test Data (`test/e2e/fixtures/testdata/`)

Contains additional fixture files used by specific test cases.

## Test Utilities Reference

### Verifiers (`test/e2e/testutil/`)

Connect directly to the database to verify operator actions:

```go
verifier := testutil.NewPostgresVerifier(host, port, adminUser, adminPassword)
exists, err := verifier.DatabaseExists("testdb")
canLogin, err := verifier.UserCanLogin("testuser", "password123")
```

### Wait Functions (`e2e_suite_test.go`)

| Function | Description |
|----------|-------------|
| `waitForPhase()` | Wait for a CR to reach a specific status phase |
| `getResourcePhase()` | Get current phase of a CR |
| `getSecretValue()` | Read a value from a Kubernetes Secret |

### Common Helpers (`common_test.go`)

| Function | Description |
|----------|-------------|
| `PostgreSQLTestConfig()` | Default test configuration for PostgreSQL |
| `MySQLTestConfig()` | Default test configuration for MySQL |
| `MariaDBTestConfig()` | Default test configuration for MariaDB |

## Debugging

### Operator Logs

```bash
# Stream operator logs
make e2e-logs

# Or directly
kubectl logs -n db-provision-operator-system -l control-plane=controller-manager -f
```

### Database Health

```bash
# Docker Compose service status
docker compose -f docker-compose.e2e.yml ps

# Individual database logs
docker compose -f docker-compose.e2e.yml logs postgres
docker compose -f docker-compose.e2e.yml logs mysql
docker compose -f docker-compose.e2e.yml logs mariadb
docker compose -f docker-compose.e2e.yml logs cockroachdb
```

### Direct Database Connection

```bash
# PostgreSQL
psql -h localhost -p ${E2E_POSTGRES_PORT:-15432} -U dbprovision_admin -d postgres

# MySQL
mysql -h 127.0.0.1 -P ${E2E_MYSQL_PORT:-13306} -u root -prootpassword123

# MariaDB
mysql -h 127.0.0.1 -P ${E2E_MARIADB_PORT:-13307} -u root -prootpassword123

# CockroachDB
cockroach sql --insecure --host=localhost --port=${E2E_COCKROACHDB_PORT:-26257}
```

### CR Inspection

```bash
# List all database-related CRs
kubectl get databaseinstances,databases,databaseusers,databaseroles,databasegrants -A

# Describe a specific DatabaseInstance
kubectl describe databaseinstance postgres-e2e-instance -n default

# Check recent events
kubectl get events --sort-by='.lastTimestamp' | tail -20
```

### k3d-to-Docker Connectivity

```bash
# Verify host.k3d.internal resolves inside the cluster
kubectl run test --rm -it --image=busybox -- nslookup host.k3d.internal

# Test TCP connectivity to PostgreSQL
kubectl run test --rm -it --image=busybox -- nc -zv host.k3d.internal 15432
```

### Full Debug Dump

```bash
# Saves cluster state + Docker Compose logs to e2e-debug-postgresql.log
make e2e-debug E2E_DATABASE=postgresql
```

## Troubleshooting

### `host.k3d.internal` Not Resolving

This hostname is automatically configured by k3d. If it's not resolving:

```bash
# Verify the cluster was created correctly
k3d cluster list

# Check k3d version (should be v5+)
k3d version

# Recreate the cluster
make e2e-cluster-delete
make e2e-cluster-create
```

### Database Connection Refused

```bash
# Check if containers are running and healthy
docker compose -f docker-compose.e2e.yml ps

# Check container logs for errors
docker compose -f docker-compose.e2e.yml logs postgres

# Verify the port is correct
docker compose -f docker-compose.e2e.yml port postgres 5432
```

### Operator Not Starting

```bash
# Check deployment status
kubectl describe deployment -n db-provision-operator-system db-provision-operator-controller-manager

# Check pod logs
kubectl logs -n db-provision-operator-system -l control-plane=controller-manager --tail=100

# Check if image was loaded into k3d
docker exec k3d-dbprov-e2e-server-0 crictl images | grep db-provision-operator
```

### Tests Timing Out

```bash
# Increase test timeout
go test ./test/e2e/... -v -tags=e2e -timeout=20m

# Run a specific test
go test ./test/e2e/... -v -tags=e2e -ginkgo.focus="should create database" -timeout=10m
```

### Port Conflicts

The default ports use high numbers to avoid conflicts with local database instances:

```bash
# Check what's using a port
lsof -i :15432

# Use different ports
E2E_POSTGRES_PORT=25432 E2E_MYSQL_PORT=23306 make e2e-local-postgresql
```

## Cleanup

```bash
# Full cleanup (cluster + databases)
make e2e-local-cleanup

# Or individually
make e2e-cluster-delete  # Delete k3d cluster only
make e2e-db-down         # Stop databases only
```

## CI Consistency

Local and CI use the **exact same Makefile micro-step targets**. The only difference is that CI deploys the operator via **Helm** (to validate the Helm chart), while local development uses **Kustomize** (`make e2e-deploy-operator`).

```yaml
# CI workflow calls the same targets:
- run: make e2e-db-up E2E_DATABASE=${{ matrix.database }}
- run: make e2e-cluster-create
# CI uses a pre-built image instead of e2e-docker-build
- run: make e2e-image-load E2E_IMG=<registry-image>
- run: make e2e-install-crds
# CI deploys via Helm (not Kustomize)
- run: helm install db-provision-operator ./charts/db-provision-operator ...
- run: make e2e-create-db-instance E2E_DATABASE=${{ matrix.database }}
- run: make e2e-local-run-tests E2E_DATABASE=${{ matrix.database }}
```

## File Structure

```
db-provision-operator/
├── docker-compose.e2e.yml           # Database services (PG, MySQL, MariaDB, CockroachDB)
├── test/e2e/
│   ├── e2e_suite_test.go            # Suite bootstrap, GVRs, helper functions
│   ├── common_test.go               # Shared test configs, verifier helpers
│   ├── postgresql_test.go           # PostgreSQL engine tests
│   ├── mysql_test.go                # MySQL engine tests
│   ├── mariadb_test.go              # MariaDB engine tests
│   ├── cockroachdb_test.go          # CockroachDB engine tests
│   ├── clusterdatabaseinstance_test.go  # ClusterDatabaseInstance tests
│   ├── clusterdatabaserole_test.go      # ClusterDatabaseRole tests
│   ├── clusterdatabasegrant_test.go     # ClusterDatabaseGrant tests
│   ├── testutil/                    # Database verifiers (Postgres, MySQL, CockroachDB)
│   └── fixtures/
│       ├── postgres-init/           # PostgreSQL init scripts
│       ├── mysql-init/              # MySQL init scripts
│       ├── mariadb-init/            # MariaDB init scripts
│       ├── cockroachdb-init/        # CockroachDB init scripts
│       ├── postgresql-local.yaml    # DatabaseInstance CR for PostgreSQL
│       ├── mysql-local.yaml         # DatabaseInstance CR for MySQL
│       ├── mariadb-local.yaml       # DatabaseInstance CR for MariaDB
│       ├── cockroachdb-local.yaml   # DatabaseInstance CR for CockroachDB
│       └── testdata/                # Additional test fixture data
└── Makefile                         # E2E targets (micro-steps + unified)
```
