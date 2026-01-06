# E2E Testing Guide

This guide explains how to run end-to-end tests for the db-provision-operator.

## Overview

The E2E tests validate the operator's functionality against real Kubernetes clusters with real database instances. Tests are organized by database engine (PostgreSQL, MySQL) and run via a CI matrix or locally.

## Prerequisites

- Docker (running)
- k3d v5.x+ (`curl -s https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | bash`)
- kubectl
- Go 1.24+
- make

## Quick Start

### PostgreSQL Tests

```bash
# Run complete E2E tests with PostgreSQL
make test-e2e-postgresql
```

### MySQL Tests

```bash
# Run complete E2E tests with MySQL
make test-e2e-mysql
```

## Manual Setup (Step by Step)

### 1. Create k3d Cluster

```bash
k3d cluster create e2e-test --agents 1 --wait
```

### 2. Deploy Database

Choose one database engine:

**PostgreSQL:**
```bash
kubectl apply -f test/e2e/fixtures/postgresql.yaml
kubectl wait --for=condition=Ready pod/postgres-0 -n postgres --timeout=120s
```

**MySQL:**
```bash
kubectl apply -f test/e2e/fixtures/mysql.yaml
kubectl wait --for=condition=Ready pod/mysql-0 -n mysql --timeout=120s
```

### 3. Install CRDs

```bash
make install
```

### 4. Build and Deploy Operator

```bash
# Build the operator image
make docker-build IMG=db-provision-operator:e2e

# Load image into k3d cluster
k3d image import db-provision-operator:e2e -c e2e-test

# Deploy operator
make deploy IMG=db-provision-operator:e2e

# Wait for operator to be ready
kubectl wait --for=condition=Available deployment/db-provision-operator-controller-manager \
  -n db-provision-operator-system --timeout=120s
```

### 5. Run Tests

```bash
# Run all tests
go test ./test/e2e/... -v -tags=e2e -ginkgo.v

# Run only PostgreSQL tests
go test ./test/e2e/... -v -tags=e2e -ginkgo.focus="postgresql"

# Run only MySQL tests
go test ./test/e2e/... -v -tags=e2e -ginkgo.focus="mysql"
```

### 6. Cleanup

```bash
k3d cluster delete e2e-test
```

## CI/CD Pipeline

The GitHub Actions workflow (`.github/workflows/test-e2e.yml`) runs E2E tests automatically:

- **Matrix Strategy**: Each database engine runs in a separate parallel job
- **Isolation**: Each job creates its own k3d cluster
- **Failure Handling**: Logs are collected on failure for debugging

### CI Workflow Steps

1. Checkout code
2. Setup Go
3. Install k3d
4. Create k3d cluster
5. Deploy database (PostgreSQL or MySQL)
6. Install CRDs
7. Build and load operator image
8. Deploy operator
9. Run E2E tests with `-ginkgo.focus` for the specific database
10. Cleanup cluster

## Test Structure

### Test Files

| File | Purpose |
|------|---------|
| `e2e_suite_test.go` | Test suite setup, clients, helpers |
| `e2e_test.go` | Controller and metrics tests |
| `postgresql_test.go` | PostgreSQL-specific E2E tests |
| `mysql_test.go` | MySQL-specific E2E tests |

### Test Fixtures

| File | Purpose |
|------|---------|
| `fixtures/postgresql.yaml` | PostgreSQL StatefulSet + Secret + Service |
| `fixtures/mysql.yaml` | MySQL StatefulSet + Secret + Service |

### Test Cases

Each database engine tests:

1. **DatabaseInstance lifecycle**
   - Create DatabaseInstance CR
   - Wait for Ready phase
   - Verify Connected condition

2. **Database lifecycle**
   - Create Database CR referencing the instance
   - Wait for Ready phase

3. **DatabaseUser lifecycle**
   - Create DatabaseUser CR
   - Wait for Ready phase
   - Verify credentials Secret is created

4. **Cross-namespace Secret reference**
   - Verify operator can access Secrets from different namespaces
   - Tests ClusterRoleBinding RBAC configuration

## Troubleshooting

### Check Operator Logs

```bash
kubectl logs -n db-provision-operator-system -l control-plane=controller-manager -f
```

### Check Database Pod Logs

```bash
# PostgreSQL
kubectl logs postgres-0 -n postgres

# MySQL
kubectl logs mysql-0 -n mysql
```

### Check CR Status

```bash
# List all CRs
kubectl get databaseinstances,databases,databaseusers -A

# Describe specific CR
kubectl describe databaseinstance <name>
```

### Check Events

```bash
kubectl get events -A --sort-by='.lastTimestamp' | tail -20
```

### Common Issues

1. **Database pod not ready**
   - Check pod logs for startup errors
   - Ensure PVC is bound (if using persistent storage)

2. **Operator not connecting to database**
   - Verify database service is accessible
   - Check Secret credentials are correct
   - Verify network policies (if any)

3. **Cross-namespace Secret access denied**
   - Verify ClusterRoleBinding is installed
   - Check RBAC configuration in `config/rbac/`

## Makefile Targets

| Target | Description |
|--------|-------------|
| `test-e2e-postgresql` | Run full E2E with PostgreSQL |
| `test-e2e-mysql` | Run full E2E with MySQL |
| `e2e-setup-cluster` | Setup k3d cluster with database |
| `e2e-run-tests` | Run E2E tests |
| `e2e-cleanup` | Delete k3d cluster |
| `e2e-logs` | Stream operator logs |

## Environment Variables

| Variable | Description |
|----------|-------------|
| `E2E_DATABASE_ENGINE` | Database engine to test (postgresql/mysql) |
| `E2E_K3D_CLUSTER` | k3d cluster name prefix |
| `E2E_IMG` | Operator image to use for E2E tests |
