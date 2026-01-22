# Development

Guides for contributing to and developing DB Provision Operator.

## Overview

- [Contributing](contributing.md) - How to contribute
- [Testing](testing.md) - Testing strategies
- [CI/CD](ci-cd.md) - Continuous integration and deployment

## Quick Start

### Prerequisites

- Go 1.21+
- Docker
- kubectl
- kind or minikube
- Make

### Clone and Build

```bash
# Clone repository
git clone https://github.com/panteparak/db-provision-operator.git
cd db-provision-operator

# Install dependencies
make install-tools

# Build
make build

# Run tests
make test
```

### Local Development

```bash
# Start local cluster
make kind-create

# Install CRDs
make install

# Run operator locally
make run
```

### Development Workflow

1. Create feature branch
2. Make changes
3. Write/update tests
4. Run tests locally
5. Submit pull request

## Project Structure

```
db-provision-operator/
├── api/                    # CRD API definitions
│   └── v1alpha1/          # API version
├── cmd/                    # Main entrypoints
│   └── main.go            # Operator entrypoint
├── config/                 # Kubernetes manifests
│   ├── crd/               # CRD definitions
│   ├── manager/           # Operator deployment
│   ├── rbac/              # RBAC rules
│   └── samples/           # Sample resources
├── internal/               # Internal packages
│   ├── controller/        # Controllers
│   ├── database/          # Database adapters
│   └── utils/             # Utilities
├── test/                   # Tests
│   ├── e2e/               # End-to-end tests
│   └── fixtures/          # Test fixtures
├── charts/                 # Helm charts
│   └── db-provision-operator/
├── docs/                   # Documentation
├── DOCS/                   # Additional docs and examples
├── Dockerfile             # Container image
├── Makefile               # Build automation
└── go.mod                 # Go modules
```

## Key Technologies

| Technology | Purpose |
|------------|---------|
| [Kubebuilder](https://kubebuilder.io) | Operator framework |
| [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime) | Controller library |
| [Ginkgo](https://onsi.github.io/ginkgo/) | Testing framework |
| [Gomega](https://onsi.github.io/gomega/) | Assertion library |
| [envtest](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/envtest) | Integration testing |

## Makefile Targets

```bash
# Development
make build          # Build binary
make run            # Run locally
make install        # Install CRDs
make uninstall      # Uninstall CRDs

# Testing
make test           # Unit tests
make test-e2e       # E2E tests
make lint           # Run linters

# Docker
make docker-build   # Build image
make docker-push    # Push image

# Cluster
make kind-create    # Create kind cluster
make kind-delete    # Delete kind cluster
make deploy         # Deploy to cluster

# Documentation
make docs-serve     # Serve docs locally
make docs-build     # Build docs
```

## Code Generation

After modifying API types:

```bash
# Regenerate manifests and code
make generate
make manifests
```

## Debugging

### VS Code Launch Config

```json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Debug Operator",
      "type": "go",
      "request": "launch",
      "mode": "auto",
      "program": "${workspaceFolder}/cmd/main.go",
      "env": {
        "KUBECONFIG": "${env:HOME}/.kube/config"
      }
    }
  ]
}
```

### GoLand/IntelliJ

1. Run > Edit Configurations
2. Add Go Build
3. Package path: `./cmd`
4. Environment: `KUBECONFIG=$HOME/.kube/config`

## Next Steps

- [Contributing](contributing.md) - Contribution guidelines
- [Testing](testing.md) - Write and run tests
- [CI/CD](ci-cd.md) - Understanding the pipeline
