# CI/CD

Continuous integration and deployment pipeline for the DB Provision Operator.

## Overview

The CI pipeline runs on every push to `main` and every pull request. It uses a 6-stage architecture where each stage gates the next:

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           CI Pipeline                                   │
│                                                                         │
│  Stage 0    Stage 1          Stage 2    Stage 3       Stage 4   Stage 5 │
│  ┌──────┐  ┌──────────────┐  ┌──────┐  ┌──────────┐  ┌─────┐  ┌─────┐ │
│  │Setup ├─►│ 8 Parallel   ├─►│ Gate ├─►│Integration├─►│ E2E ├─►│ CI  │ │
│  │      │  │ Jobs         │  │      │  │ Tests     │  │     │  │Done │ │
│  └──────┘  └──────────────┘  └──────┘  └──────────┘  └─────┘  └─────┘ │
│                                                                         │
│  On tag: Stage 6 ──► Release Artifacts + Security Scan                  │
└─────────────────────────────────────────────────────────────────────────┘
```

### Triggers

| Event | Branches/Tags | What Runs |
|-------|---------------|-----------|
| Pull request | `main` | Stages 0–5 (Docker build without push) |
| Push | `main` | Stages 0–5 + Docker push + Security scan |
| Tag | `v*` | Stages 0–5 + Stage 6 (Release) |

## Stage 1: Parallel Jobs

Eight jobs run concurrently after setup:

### 1. Lint

Runs `golangci-lint` (v2.1.0) via the official GitHub Action.

### 2. Unit Tests

```bash
make test  # go test (excludes /e2e), outputs cover.out
```

Uploads coverage to Codecov.

### 3. Controller Tests (envtest)

```bash
make test-envtest  # Runs controller + CRD validation tests with envtest
```

Uses `KUBEBUILDER_ASSETS` from `setup-envtest`. Envtest binaries (~100MB) are cached by Kubernetes version.

### 4. Template Comparison

```bash
make test-templates  # Verifies Helm ≡ Kustomize template equivalence
```

Ensures the Helm chart and Kustomize overlays produce equivalent manifests.

### 5. Docker Build

Builds the operator image per platform (currently `linux/amd64`). Uses a multi-layer cache strategy:

- **GHA cache** (fast, per-platform, workflow-local)
- **Registry cache** (persistent fallback when GHA cache is evicted)

On **push events**: Pushes by digest, uploads digest artifact for manifest merging.
On **PRs**: Builds locally, saves as tarball artifact for E2E tests.

### 6. Verify Manifests

```bash
make manifests  # Generates CRDs, RBAC, webhook configs
```

On `main` push: auto-commits if manifests are outdated.
On PRs: fails the check so the developer runs `make manifests` locally.

Also runs Helm chart linting, template rendering, and strict YAML validation (duplicate key detection).

### 7. Verify Generated Code

```bash
make generate  # Generates DeepCopy methods
```

Fails if generated code is outdated.

### 8. Docs Build

```bash
mkdocs build --strict  # Builds documentation, fails on warnings
```

## Stage 2: Gate

Waits for all Stage 1 jobs to succeed before allowing integration tests. Checks each job's result and fails with a clear error message identifying which job(s) failed.

For PRs, `docker-merge` is skipped (no push), so the gate only checks `docker-build`.

## Stage 3: Integration Tests

Runs after the gate passes. Uses a matrix strategy across all four database engines:

```yaml
matrix:
  database: [postgresql, mysql, mariadb, cockroachdb]
```

Each job uses **testcontainers-go** to manage Docker containers programmatically — no manual container setup needed. The tests run against real database instances with envtest providing the Kubernetes API.

```bash
make test-integration INTEGRATION_TEST_DATABASE=${{ matrix.database }}
```

## Stage 4: E2E Tests

Runs after integration tests pass. Full end-to-end validation using Docker Compose + k3d + Helm.

### Pipeline Per Database (16 Steps)

Each database engine runs in parallel with this sequence:

| # | Step | Description |
|---|------|-------------|
| 1 | Checkout | Clone repository |
| 2 | Setup Go | Install Go from `go.mod` version |
| 3 | Get SHA | Short commit SHA for image tagging |
| 4 | Install k3d | Install k3d CLI |
| 5 | Start database | `make e2e-db-up E2E_DATABASE=<engine>` |
| 6 | Create cluster | `make e2e-cluster-create` |
| 7 | Get operator image | Pull from registry (push) or load from artifact (PR) |
| 8 | Load into k3d | `make e2e-image-load E2E_IMG=<image>` |
| 9 | Install CRDs | `make e2e-install-crds` |
| 10 | Install Helm | Setup Helm v3.14.0 |
| 11 | Deploy operator | `helm install` with the chart (not Kustomize) |
| 12 | Wait for ready | `kubectl wait --for=condition=Available` |
| 13 | Create instance | `make e2e-create-db-instance E2E_DATABASE=<engine>` |
| 14 | Run tests | `make e2e-local-run-tests E2E_DATABASE=<engine>` |
| 15 | Collect logs | On failure: operator logs, pod descriptions, Docker Compose logs, CR YAML, events |
| 16 | Cleanup | `make e2e-local-cleanup` (always runs) |

### Helm Deployment (Not Kustomize)

CI intentionally deploys the operator via the **Helm chart** (`charts/db-provision-operator/`) rather than Kustomize. This validates the Helm chart in a real cluster as part of every CI run. Local development uses `make e2e-deploy-operator` (Kustomize) for faster iteration.

### Log Collection on Failure

When any E2E step fails, CI collects:

- Operator pod logs (last 500 lines)
- Operator pod description
- Docker Compose database logs (last 100 lines)
- All database-related CR YAML dumps
- Kubernetes events (last 100)
- Pod list across all namespaces

## Stage 5: CI Success

A summary job that runs after all E2E tests pass. Confirms the full pipeline succeeded.

## Security Scan (Push Events Only)

After `docker-merge`, Trivy scans each platform image:

1. **Generate CycloneDX SBOM** from the container image
2. **Scan for vulnerabilities** — outputs JSON (all severities) + table (MEDIUM+ with unfixed ignored)
3. **Generate SARIF report** — uploaded to GitHub Security tab via CodeQL
4. **Upload artifacts** — SBOM, vulnerability reports retained for 30 days

## Docker Image Build

### Multi-Stage Dockerfile

The operator image uses a multi-stage build:

1. **Builder stage**: Compiles the Go binary
2. **Runtime stage**: Alpine-based minimal image with database client tools

### Image Tags

| Tag Pattern | When | Example |
|-------------|------|---------|
| `sha-<short>` | Every build | `sha-abc1234` |
| `latest` | Push to main | `latest` |
| `main` | Push to main | `main` |
| `<version>` | Semver tag | `1.2.3` |
| `<major>.<minor>` | Semver tag | `1.2` |

### Cache Strategy

```
GHA cache (fast, per-platform)
  └── fallback: Registry cache (persistent, cross-workflow)
```

PRs share cache from the `main` branch via registry cache, enabling fast builds even on new branches.

## Release Pipeline (Stage 6)

Triggered only on tags matching `v*`. Runs after `ci-success`.

### Steps

1. **Package Helm chart** — `helm package charts/db-provision-operator`
2. **Build Kustomize manifest** — `kustomize build config/default > install.yaml`
3. **Build CRDs manifest** — `kustomize build config/crd > crds.yaml`
4. **Create GitHub Release** — with release notes, attaches all artifacts

### Release Assets

Each release includes:

| Asset | Description |
|-------|-------------|
| `db-provision-operator-*.tgz` | Helm chart package |
| `install.yaml` | Full installation manifest (CRDs + operator) |
| `crds.yaml` | CRD definitions only |

## Build Artifacts

### Container Images

Published to: `ghcr.io/<owner>/db-provision-operator`

### SBOM

Generated in two formats:

- **SPDX** (via `anchore/sbom-action`) — uploaded as artifact
- **CycloneDX** (via Trivy) — used for vulnerability scanning

## Environment Configuration

### Secrets

| Secret | Purpose |
|--------|---------|
| `GITHUB_TOKEN` | Auto-provided — registry login, release creation |
| `CODECOV_TOKEN` | Coverage reporting |

### Global Environment

| Variable | Value | Description |
|----------|-------|-------------|
| `GO_VERSION_FILE` | `go.mod` | Go version source of truth |
| `REGISTRY` | `ghcr.io` | Container registry |
| `IMAGE_NAME` | `${{ github.repository }}` | Image name |

## Local CI Simulation

Reproduce each CI stage locally:

```bash
# Stage 1: Lint
make lint

# Stage 1: Unit tests
make test

# Stage 1: Controller tests (envtest)
make test-envtest

# Stage 1: Template comparison
make test-templates

# Stage 1: Docker build
make docker-build

# Stage 1: Manifests
make manifests && git diff --exit-code

# Stage 1: Generated code
make generate && git diff --exit-code

# Stage 1: Docs
mkdocs build --strict

# Stage 3: Integration tests (single engine)
make test-integration INTEGRATION_TEST_DATABASE=postgresql

# Stage 4: E2E tests (single engine)
make e2e-local-postgresql

# Full E2E (all engines)
make e2e-local-all
```
