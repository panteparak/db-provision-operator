# Running E2E Tests Locally

This guide explains how to run end-to-end tests locally using the same infrastructure as CI.

## Prerequisites

- Docker and Docker Compose
- k3d (`curl -s https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | bash`)
- kubectl
- Go 1.24+

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                     Local Machine                             │
│                                                              │
│  ┌─────────────────┐     ┌─────────────────────────────────┐ │
│  │  Docker Compose │     │         k3d Cluster             │ │
│  │                 │     │                                 │ │
│  │  ┌───────────┐  │     │  ┌─────────────────────────┐   │ │
│  │  │ PostgreSQL│◄─┼─────┼──│  Operator Pod           │   │ │
│  │  │ :5432     │  │     │  │  (db-provision-operator)│   │ │
│  │  └───────────┘  │     │  └─────────────────────────┘   │ │
│  │  ┌───────────┐  │     │                                 │ │
│  │  │ MySQL     │◄─┼─────┼── host.k3d.internal             │ │
│  │  │ :3306     │  │     │                                 │ │
│  │  └───────────┘  │     │  ┌─────────────────────────┐   │ │
│  │  ┌───────────┐  │     │  │  DatabaseInstance CRs   │   │ │
│  │  │ MariaDB   │  │     │  │  → host.k3d.internal    │   │ │
│  │  │ :3307     │  │     │  └─────────────────────────┘   │ │
│  │  └───────────┘  │     │                                 │ │
│  └─────────────────┘     └─────────────────────────────────┘ │
│          │                                                    │
│          ▼                                                    │
│  ┌─────────────────┐                                         │
│  │  E2E Tests      │  (go test connects to 127.0.0.1)        │
│  └─────────────────┘                                         │
└──────────────────────────────────────────────────────────────┘
```

**Key Design Points:**

- **Databases run on Docker Compose** (outside k3d) for faster iteration and easier debugging
- **Operator runs inside k3d** cluster
- **k3d uses `host.k3d.internal`** to reach databases on Docker host
- **Same Makefile targets** are used by both local and CI for consistency

## Quick Start

### One Command (Recommended)

```bash
# Run PostgreSQL E2E tests (sets up everything)
make e2e-local-postgresql

# Run MySQL E2E tests
make e2e-local-mysql

# Run MariaDB E2E tests
make e2e-local-mariadb

# Run all databases
make e2e-local-all
```

### Step-by-Step (For Debugging)

Each step is a separate Makefile target—the same targets CI uses:

```bash
# 1. Start databases (Docker Compose with health checks)
make e2e-db-up

# 2. Create k3d cluster
make e2e-cluster-create

# 3. Build operator Docker image
make e2e-docker-build

# 4. Load image into k3d
make e2e-image-load

# 5. Install CRDs
make e2e-install-crds

# 6. Deploy operator
make e2e-deploy-operator

# 7. Create DatabaseInstance (points to host.k3d.internal)
make e2e-create-db-instance E2E_DATABASE=postgresql

# 8. Run tests
make e2e-local-run-tests E2E_DATABASE=postgresql
```

## Configuration

### Port Configuration

Override ports via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `E2E_POSTGRES_PORT` | 5432 | PostgreSQL host port |
| `E2E_MYSQL_PORT` | 3306 | MySQL host port |
| `E2E_MARIADB_PORT` | 3307 | MariaDB host port |

```bash
# Use custom ports (e.g., if default ports are in use)
E2E_POSTGRES_PORT=15432 make e2e-local-postgresql
```

### Operator Image

```bash
# Use different image name/tag
E2E_IMG=my-operator:dev make e2e-local-postgresql
```

### k3d Cluster Name

```bash
# Use different cluster name
E2E_K3D_CLUSTER=my-cluster make e2e-local-postgresql
```

## Available Makefile Targets

### Micro-Steps (called by CI)

| Target | Description |
|--------|-------------|
| `e2e-db-up` | Start databases via Docker Compose (with health checks, 180s timeout) |
| `e2e-db-down` | Stop databases and remove volumes |
| `e2e-cluster-create` | Create k3d cluster (idempotent) |
| `e2e-cluster-delete` | Delete k3d cluster |
| `e2e-docker-build` | Build operator Docker image |
| `e2e-image-load` | Load image into k3d |
| `e2e-install-crds` | Install CRDs |
| `e2e-deploy-operator` | Deploy operator |
| `e2e-create-db-instance` | Create DatabaseInstance CR |
| `e2e-local-run-tests` | Run E2E tests |

### Unified Targets

| Target | Description |
|--------|-------------|
| `e2e-local-setup` | Full setup (DBs + k3d + operator) |
| `e2e-local-postgresql` | Setup + PostgreSQL tests |
| `e2e-local-mysql` | Setup + MySQL tests |
| `e2e-local-mariadb` | Setup + MariaDB tests |
| `e2e-local-all` | Setup + all database tests |
| `e2e-local-cleanup` | Clean up everything |

## Debugging

### View Operator Logs

```bash
kubectl logs -n db-provision-operator-system -l control-plane=controller-manager -f
```

### Check Database Health

```bash
# View Docker Compose service status
docker compose -f docker-compose.e2e.yml ps

# View logs for a specific database
docker compose -f docker-compose.e2e.yml logs postgres
docker compose -f docker-compose.e2e.yml logs mysql
docker compose -f docker-compose.e2e.yml logs mariadb
```

### Connect to Database Directly

```bash
# PostgreSQL
psql -h localhost -p ${E2E_POSTGRES_PORT:-5432} -U dbprovision_admin -d postgres

# MySQL
mysql -h 127.0.0.1 -P ${E2E_MYSQL_PORT:-3306} -u root -prootpassword123

# MariaDB
mysql -h 127.0.0.1 -P ${E2E_MARIADB_PORT:-3307} -u root -prootpassword123
```

### Inspect CRs

```bash
# List all database-related CRs
kubectl get databaseinstances,databases,databaseusers -A

# Describe a specific DatabaseInstance
kubectl describe databaseinstance postgres-instance -n postgresql

# Check events
kubectl get events -n postgresql --sort-by='.lastTimestamp'
```

### Verify k3d to Docker Host Connectivity

```bash
# Run a test pod to verify host.k3d.internal resolves correctly
kubectl run test --rm -it --image=busybox -- nslookup host.k3d.internal

# Test TCP connectivity to PostgreSQL
kubectl run test --rm -it --image=busybox -- nc -zv host.k3d.internal 5432
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

Local and CI use the **exact same Makefile targets**. This ensures what works locally will work in CI.

```yaml
# CI workflow calls the same targets:
- run: make e2e-db-up
- run: make e2e-cluster-create
- run: make e2e-docker-build
- run: make e2e-image-load
- run: make e2e-install-crds
- run: make e2e-deploy-operator
- run: make e2e-create-db-instance E2E_DATABASE=${{ matrix.database }}
- run: make e2e-local-run-tests E2E_DATABASE=${{ matrix.database }}
```

## Troubleshooting

### "host.k3d.internal" not resolving

This hostname is automatically configured by k3d. If it's not resolving:

```bash
# Verify the cluster was created correctly
k3d cluster list

# Check k3d version (should be recent)
k3d version

# Recreate the cluster
make e2e-cluster-delete
make e2e-cluster-create
```

### Database connection refused

```bash
# Check if containers are running
docker compose -f docker-compose.e2e.yml ps

# Check container logs
docker compose -f docker-compose.e2e.yml logs postgres

# Verify health check passed
docker inspect --format='{{.State.Health.Status}}' db-provision-operator-postgres-1
```

### Operator not starting

```bash
# Check deployment status
kubectl describe deployment -n db-provision-operator-system db-provision-operator-controller-manager

# Check pod logs
kubectl logs -n db-provision-operator-system -l control-plane=controller-manager --tail=100

# Check if image was loaded into k3d
docker exec k3d-dbprov-e2e-server-0 crictl images | grep db-provision-operator
```

### Tests timing out

```bash
# Increase test timeout
go test ./test/e2e/... -v -tags=e2e -timeout=20m

# Run a specific test
go test ./test/e2e/... -v -tags=e2e -ginkgo.focus="should create database" -timeout=10m
```

### Port already in use

```bash
# Check what's using the port
lsof -i :5432

# Use different ports
E2E_POSTGRES_PORT=15432 E2E_MYSQL_PORT=13306 E2E_MARIADB_PORT=13307 make e2e-local-postgresql
```

## File Structure

```
db-provision-operator/
├── docker-compose.e2e.yml           # Database services for E2E
├── test/e2e/
│   ├── fixtures/
│   │   ├── postgres-init/           # PostgreSQL init scripts
│   │   │   └── 01-create-admin.sql
│   │   ├── mysql-init/              # MySQL init scripts
│   │   │   └── 01-create-admin.sql
│   │   ├── mariadb-init/            # MariaDB init scripts
│   │   │   └── 01-create-admin.sql
│   │   ├── postgresql-local.yaml    # DatabaseInstance for local testing
│   │   ├── mysql-local.yaml
│   │   └── mariadb-local.yaml
│   └── *.go                         # E2E test files
└── Makefile                         # E2E targets
```
