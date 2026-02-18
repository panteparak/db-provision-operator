# Testing

Test strategy and commands for the DB Provision Operator.

## Test Pyramid

| Level | Tools | Location | Command |
|-------|-------|----------|---------|
| **Unit** | Go `testing`, table-driven | `internal/**/*_test.go` | `make test` |
| **envtest** | controller-runtime envtest, Ginkgo | `internal/controller/*_test.go`, `api/v1alpha1/*_test.go` | `make test-envtest` |
| **Integration** | testcontainers-go, envtest | `internal/controller/*_test.go`, `internal/adapter/*_test.go` | `make test-integration` |
| **E2E** | Ginkgo v2, k3d, Docker Compose | `test/e2e/` | `make e2e-local-postgresql` |
| **CRD Validation** | envtest, Ginkgo | `api/v1alpha1/*_test.go` | `make test-envtest` |
| **Template** | Go `testing`, Helm, Kustomize | `test/template/` | `make test-templates` |
| **Benchmark** | Go `testing -bench` | `internal/controller/` | `make test-benchmark` |

## Unit Tests

Fast, isolated tests for individual functions and business logic. No external dependencies.

```bash
make test
```

Runs `go test` on all packages except `e2e`, with coverage output to `cover.out`.

## envtest (Controller + CRD Validation)

Tests controllers and CRD validation webhooks against a real Kubernetes API server (provided by envtest) without a full cluster.

```bash
make test-envtest
```

This runs two suites:

1. **Controller tests** (`internal/controller/`) — reconciliation logic with a fake K8s API
2. **CRD validation tests** (`api/v1alpha1/`) — webhook admission validation

Uses `KUBEBUILDER_ASSETS` binaries. First run downloads ~100MB of binaries (cached thereafter).

## Integration Tests

Tests the full adapter layer (SQL execution) against real database containers managed by testcontainers-go.

```bash
# All engines
make test-integration

# Single engine
make test-integration INTEGRATION_TEST_DATABASE=postgresql
make test-integration INTEGRATION_TEST_DATABASE=mysql
make test-integration INTEGRATION_TEST_DATABASE=cockroachdb
```

Uses `-tags=integration`. testcontainers-go starts and stops Docker containers automatically.

## E2E Tests

Full end-to-end tests against a real k3d cluster with databases running in Docker Compose. Validates the complete operator lifecycle: DatabaseInstance → Database → DatabaseUser → DatabaseRole → DatabaseGrant.

See **[E2E Testing](e2e-testing.md)** for the comprehensive guide covering architecture, setup, debugging, and troubleshooting.

```bash
# Quick start
make e2e-local-postgresql
make e2e-local-mysql
make e2e-local-mariadb
make e2e-local-cockroachdb
make e2e-local-all
```

## Template Comparison

Verifies that Helm chart and Kustomize overlays produce equivalent manifests.

```bash
make test-templates
```

## Running Tests

### All Levels

```bash
# Unit + envtest + integration
make test && make test-envtest && make test-integration

# E2E (all engines)
make e2e-local-all
```

### Coverage

```bash
# Unit test coverage
make test
go tool cover -html=cover.out -o coverage.html

# envtest coverage
make test-envtest
go tool cover -html=cover-envtest.out -o coverage-envtest.html

# Integration coverage
make test-integration
go tool cover -html=cover-integration.out -o coverage-integration.html
```

## Debugging Tests

### Verbose Output

```bash
# Unit / integration
go test -v ./internal/controller/...

# E2E with Ginkgo verbose
go test ./test/e2e/... -v -tags=e2e -ginkgo.v -timeout=10m
```

### Focus a Single Test

```bash
# By test name (Go)
go test -v ./internal/controller/... -run TestDatabaseController

# By Ginkgo label (E2E)
go test ./test/e2e/... -v -tags=e2e -ginkgo.focus="should create database"
```

### Ginkgo Focus in Code

```go
FDescribe("Database Controller", func() {
    // Only this describe block runs
})

FIt("Should create database", func() {
    // Only this test runs
})
```

!!! warning
    Remove `F` prefixes before committing — they skip all other tests.

### envtest Debugging

```bash
# Show envtest K8s version
go list -m -f "{{ .Version }}" k8s.io/api

# Manually install envtest binaries
make setup-envtest
```

### E2E Debugging

See [E2E Testing — Debugging](e2e-testing.md#debugging) for operator logs, database health, direct DB connection, and CR inspection.
