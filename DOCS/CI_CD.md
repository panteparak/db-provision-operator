# CI/CD Documentation

This document describes the CI/CD pipelines and release process for the db-provision-operator.

## GitHub Workflows

### 1. CI Workflow (`.github/workflows/ci.yml`)

Runs on every push and pull request to main branch.

**Jobs:**

| Job | Description |
|-----|-------------|
| `lint` | Runs golangci-lint to check code quality |
| `unit-test` | Runs unit tests with envtest |
| `build` | Builds binary and Docker image |
| `manifests` | Verifies generated manifests are up-to-date |
| `generate` | Verifies generated code is up-to-date |

### 2. Docker Build Workflow (`.github/workflows/docker.yml`)

Builds and pushes Docker images to GitHub Container Registry (ghcr.io).

**Triggers:**
- Push to `main` branch
- Push of version tags (`v*`)
- Pull requests (build only, no push)

**Image Tags:**

| Trigger | Tags |
|---------|------|
| Push to main | `latest`, `sha-<commit>` |
| Version tag (v1.2.3) | `1.2.3`, `1.2`, `1`, `latest` |
| Pull request | None (build only) |

**Features:**
- Multi-architecture builds (linux/amd64, linux/arm64)
- Build caching with GitHub Actions cache
- SBOM (Software Bill of Materials) generation

### 3. E2E Test Workflow (`.github/workflows/test-e2e.yml`)

Runs end-to-end tests using k3d with real databases.

**Matrix Strategy:**
- PostgreSQL tests
- MySQL tests

See [E2E Testing Guide](./E2E_TESTING.md) for details.

### 4. Release Workflow (`.github/workflows/release.yml`)

Triggered when a version tag is pushed (e.g., `v1.0.0`).

**Jobs:**

1. **test** - Run tests before release
2. **docker** - Build and push multi-arch Docker image
3. **artifacts** - Build release artifacts:
   - Kustomize installer YAML
   - CRDs tarball
   - Helm chart package
4. **release** - Create GitHub Release with artifacts
5. **helm-repo** - Update Helm repository on gh-pages

## Docker Images

Images are published to GitHub Container Registry:

```
ghcr.io/panteparak/db-provision-operator:<tag>
```

### Pulling Images

```bash
# Latest from main branch
docker pull ghcr.io/panteparak/db-provision-operator:latest

# Specific version
docker pull ghcr.io/panteparak/db-provision-operator:1.0.0

# Specific commit
docker pull ghcr.io/panteparak/db-provision-operator:sha-abc1234
```

## Release Process

### Creating a Release

1. **Update version** in `Makefile`:
   ```makefile
   VERSION ?= 1.0.0
   ```

2. **Update Helm chart version** (automated in CI):
   ```yaml
   # charts/db-provision-operator/Chart.yaml
   version: 1.0.0
   appVersion: "1.0.0"
   ```

3. **Create and push tag**:
   ```bash
   git tag v1.0.0
   git push origin v1.0.0
   ```

4. **CI will automatically**:
   - Run tests
   - Build multi-arch Docker image
   - Create GitHub Release with:
     - `db-provision-operator-1.0.0.yaml` (Kustomize installer)
     - `crds-1.0.0.tar.gz` (CRD definitions)
     - `db-provision-operator-1.0.0.tgz` (Helm chart)
     - `checksums.txt` (SHA256 checksums)

### Version Tags

| Tag Format | Description | Example |
|------------|-------------|---------|
| `v1.0.0` | Stable release | `v1.0.0` |
| `v1.0.0-rc1` | Release candidate | `v1.0.0-rc1` |
| `v1.0.0-alpha1` | Alpha release | `v1.0.0-alpha1` |
| `v1.0.0-beta1` | Beta release | `v1.0.0-beta1` |

Pre-release tags are marked as pre-releases on GitHub.

## Installation Methods

### Method 1: Kustomize (from Release)

```bash
# Install specific version
kubectl apply -f https://github.com/panteparak/db-provision-operator/releases/download/v1.0.0/db-provision-operator-1.0.0.yaml

# Or download and apply
curl -LO https://github.com/panteparak/db-provision-operator/releases/download/v1.0.0/db-provision-operator-1.0.0.yaml
kubectl apply -f db-provision-operator-1.0.0.yaml
```

### Method 2: Helm Chart

```bash
# Add Helm repository (if using gh-pages)
helm repo add db-provision-operator https://panteparak.github.io/db-provision-operator/charts
helm repo update

# Install
helm install db-provision-operator db-provision-operator/db-provision-operator \
  --namespace db-provision-operator-system \
  --create-namespace

# Or install from release tarball
curl -LO https://github.com/panteparak/db-provision-operator/releases/download/v1.0.0/db-provision-operator-1.0.0.tgz
helm install db-provision-operator db-provision-operator-1.0.0.tgz \
  --namespace db-provision-operator-system \
  --create-namespace
```

### Method 3: Kustomize (from Source)

```bash
# Clone repository
git clone https://github.com/panteparak/db-provision-operator.git
cd db-provision-operator

# Install CRDs
make install

# Deploy operator
make deploy IMG=ghcr.io/panteparak/db-provision-operator:1.0.0
```

## Makefile Targets

### Helm Targets

| Target | Description |
|--------|-------------|
| `helm-lint` | Lint Helm chart |
| `helm-template` | Render templates locally |
| `helm-package` | Package chart to dist/ |
| `helm-install` | Install chart to cluster |
| `helm-upgrade` | Upgrade existing release |
| `helm-uninstall` | Uninstall release |
| `helm-update-crds` | Copy CRDs to chart |

### Release Targets

| Target | Description |
|--------|-------------|
| `release-manifests` | Generate CRDs and installer YAML |
| `release` | Build all release artifacts |
| `build-installer` | Generate consolidated install.yaml |

## Helm Chart

The Helm chart is located at `charts/db-provision-operator/`.

### Chart Structure

```
charts/db-provision-operator/
├── Chart.yaml           # Chart metadata
├── values.yaml          # Default values
├── .helmignore          # Files to ignore
├── crds/                # CRD definitions (auto-installed)
│   ├── dbops.dbprovision.io_databases.yaml
│   ├── dbops.dbprovision.io_databaseinstances.yaml
│   └── ...
└── templates/
    ├── _helpers.tpl              # Template helpers
    ├── deployment.yaml           # Controller deployment
    ├── serviceaccount.yaml       # ServiceAccount
    ├── clusterrole.yaml          # ClusterRole + metrics-reader
    ├── clusterrolebinding.yaml   # ClusterRoleBinding
    ├── leader-election-rbac.yaml # Leader election Role/RoleBinding
    ├── metrics-service.yaml      # Metrics Service
    ├── servicemonitor.yaml       # Prometheus ServiceMonitor
    └── NOTES.txt                 # Post-install notes
```

### Key Configuration Options

```yaml
# values.yaml highlights

image:
  repository: ghcr.io/db-provision-operator/db-provision-operator
  tag: ""  # Defaults to Chart.appVersion

replicaCount: 1

resources:
  limits:
    cpu: 500m
    memory: 128Mi
  requests:
    cpu: 10m
    memory: 64Mi

metrics:
  enabled: true
  serviceMonitor:
    enabled: false  # Enable for Prometheus Operator

rbac:
  create: true

crds:
  install: true  # CRDs auto-installed from crds/ directory
```

## Secrets Required

### GitHub Secrets

| Secret | Required | Description |
|--------|----------|-------------|
| `GITHUB_TOKEN` | Auto | Provided by GitHub, used for packages |
| `CODECOV_TOKEN` | Optional | For code coverage uploads |

## Troubleshooting

### Image Pull Errors

If you see `ImagePullBackOff`:

```bash
# Check if image exists
docker pull ghcr.io/panteparak/db-provision-operator:1.0.0

# For private repos, create image pull secret
kubectl create secret docker-registry ghcr-secret \
  --docker-server=ghcr.io \
  --docker-username=<github-username> \
  --docker-password=<github-pat> \
  -n db-provision-operator-system
```

### Failed Release

If a release fails:

1. Check GitHub Actions logs
2. Delete the failed release/tag if needed:
   ```bash
   git tag -d v1.0.0
   git push origin :refs/tags/v1.0.0
   ```
3. Fix the issue and re-tag

### Helm Chart Issues

```bash
# Validate chart
make helm-lint

# Render templates to debug
make helm-template > rendered.yaml

# Dry-run install
helm install db-provision-operator charts/db-provision-operator \
  --namespace db-provision-operator-system \
  --dry-run
```
