# CI/CD

Continuous integration and deployment pipeline documentation.

## Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        CI/CD Pipeline                           │
│                                                                 │
│  ┌─────────┐    ┌─────────┐    ┌─────────┐    ┌─────────────┐  │
│  │  Push   │───▶│  Test   │───▶│  Build  │───▶│   Deploy    │  │
│  │         │    │  & Lint │    │  Image  │    │   (Release) │  │
│  └─────────┘    └─────────┘    └─────────┘    └─────────────┘  │
│       │              │              │               │           │
│       │              │              │               │           │
│  ┌────┴────┐    ┌────┴────┐   ┌────┴────┐    ┌────┴────────┐  │
│  │  PR     │    │  Unit   │   │  Docker │    │  GitHub     │  │
│  │  Main   │    │  E2E    │   │  Multi- │    │  Release    │  │
│  │  Tags   │    │  Lint   │   │  arch   │    │  Helm Chart │  │
│  └─────────┘    └─────────┘   └─────────┘    │  Docs       │  │
│                                              └─────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

## Workflows

### CI Workflow (`.github/workflows/ci.yml`)

Runs on:
- Pull requests to main
- Push to main

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'

      - name: Run tests
        run: make test

      - name: Upload coverage
        uses: codecov/codecov-action@v3

  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: golangci/golangci-lint-action@v3
        with:
          version: latest

  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Build
        run: make build

      - name: Build Docker image
        run: make docker-build

  e2e:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Create kind cluster
        uses: helm/kind-action@v1

      - name: Run E2E tests
        run: make test-e2e
```

### Release Workflow (`.github/workflows/release.yml`)

Runs on:
- Git tags matching `v*`

```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

jobs:
  release:
    runs-on: ubuntu-latest
    permissions:
      contents: write
      packages: write

    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'

      - name: Run tests
        run: make test

      - name: Login to GHCR
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Build and push image
        run: |
          make docker-build docker-push \
            IMG=ghcr.io/${{ github.repository }}:${{ github.ref_name }}

      - name: Create GitHub Release
        uses: softprops/action-gh-release@v1
        with:
          generate_release_notes: true
          files: |
            dist/*.yaml

  helm-release:
    needs: [release]
    runs-on: ubuntu-latest
    permissions:
      contents: write

    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Configure Git
        run: |
          git config user.name "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"

      - name: Package Helm chart
        run: |
          helm package charts/db-provision-operator \
            --version ${{ github.ref_name }} \
            --app-version ${{ github.ref_name }}

      - name: Update Helm repo
        run: |
          git checkout gh-pages
          mv *.tgz charts/
          helm repo index charts/ --url https://panteparak.github.io/db-provision-operator/charts
          git add charts/
          git commit -m "Release Helm chart ${{ github.ref_name }}"
          git push

  docs:
    needs: [release]
    runs-on: ubuntu-latest
    permissions:
      contents: write

    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-python@v5
        with:
          python-version: '3.11'

      - name: Install dependencies
        run: pip install -r docs/requirements.txt

      - name: Deploy docs
        run: mkdocs gh-deploy --force
```

### Documentation Workflow (`.github/workflows/docs.yml`)

Runs on:
- Push to main (docs changes)
- Pull requests (docs changes)

```yaml
name: Documentation

on:
  push:
    branches: [main]
    paths:
      - 'docs/**'
      - 'mkdocs.yml'
  pull_request:
    paths:
      - 'docs/**'
      - 'mkdocs.yml'

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-python@v5
        with:
          python-version: '3.11'

      - run: pip install -r docs/requirements.txt

      - name: Build docs
        run: mkdocs build --strict

  deploy:
    needs: [build]
    runs-on: ubuntu-latest
    if: github.event_name == 'push' && github.ref == 'refs/heads/main'
    permissions:
      contents: write

    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-python@v5
        with:
          python-version: '3.11'

      - run: pip install -r docs/requirements.txt

      - name: Deploy
        run: mkdocs gh-deploy --force
```

## Build Artifacts

### Container Images

| Tag | Description |
|-----|-------------|
| `v1.2.3` | Release version |
| `latest` | Latest release |
| `main` | Latest main branch |
| `sha-abc123` | Specific commit |

### Helm Charts

Published to: `https://panteparak.github.io/db-provision-operator/charts`

### Release Assets

Each release includes:
- `install.yaml` - Full installation manifest
- `crds.yaml` - CRD definitions only
- `db-provision-operator-*.tgz` - Helm chart package

## Environment Configuration

### Secrets

| Secret | Purpose |
|--------|---------|
| `GITHUB_TOKEN` | Auto-provided, GitHub API access |
| `CODECOV_TOKEN` | Code coverage reporting |

### Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `GO_VERSION` | `1.21` | Go version for builds |
| `REGISTRY` | `ghcr.io` | Container registry |

## Local CI Simulation

### Run CI Locally

```bash
# Lint
make lint

# Test
make test

# Build
make build

# Docker build
make docker-build

# E2E (requires cluster)
make test-e2e
```

### Act (GitHub Actions Locally)

```bash
# Install act
brew install act

# Run CI workflow
act -j test

# Run with secrets
act -j release --secret-file .secrets
```

## Pipeline Stages

### Test Stage

1. **Unit tests** - Fast, isolated tests
2. **Integration tests** - Tests with envtest
3. **E2E tests** - Full cluster tests
4. **Lint** - Code quality checks

### Build Stage

1. **Go build** - Compile binary
2. **Docker build** - Create image
3. **Multi-arch** - AMD64 and ARM64

### Release Stage

1. **Tag validation** - Verify semver
2. **Image push** - Push to registry
3. **GitHub release** - Create release
4. **Helm publish** - Update chart repo
5. **Docs deploy** - Update documentation

## Quality Gates

### Required Checks

- All tests pass
- Lint passes
- Build succeeds
- Coverage meets threshold

### Branch Protection

```yaml
# .github/settings.yml
branches:
  - name: main
    protection:
      required_status_checks:
        strict: true
        contexts:
          - test
          - lint
          - build
      required_pull_request_reviews:
        required_approving_review_count: 1
```

## Troubleshooting

### Failed Tests

```bash
# View test logs in Actions
# Click on failed job > View raw logs

# Reproduce locally
make test
```

### Failed Docker Build

```bash
# Check Dockerfile syntax
docker build --no-cache .

# Check multi-arch
docker buildx build --platform linux/amd64,linux/arm64 .
```

### Failed Release

```bash
# Verify tag format
git tag -l 'v*'

# Ensure tag is pushed
git push origin v1.2.3
```

## Metrics

### Pipeline Duration

Target times:
- CI: < 10 minutes
- Release: < 15 minutes
- Docs: < 5 minutes

### Success Rate

Monitor in GitHub Actions insights:
- Workflow runs
- Success/failure rate
- Duration trends
