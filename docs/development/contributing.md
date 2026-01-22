# Contributing

Guidelines for contributing to DB Provision Operator.

## Getting Started

### Prerequisites

- Go 1.21+
- Docker
- kubectl
- kind or minikube
- Make

### Setup

```bash
# Fork and clone
git clone https://github.com/YOUR_USERNAME/db-provision-operator.git
cd db-provision-operator

# Add upstream remote
git remote add upstream https://github.com/panteparak/db-provision-operator.git

# Install tools
make install-tools

# Verify setup
make test
```

## Development Workflow

### 1. Create Branch

```bash
# Update main
git checkout main
git pull upstream main

# Create feature branch
git checkout -b feature/my-feature
```

### 2. Make Changes

Follow these guidelines:

- **Code style:** Follow Go conventions
- **Comments:** Document exported functions
- **Tests:** Add tests for new code
- **Commits:** Use conventional commits

### 3. Test Changes

```bash
# Run all tests
make test

# Run linters
make lint

# Run E2E tests (requires cluster)
make test-e2e
```

### 4. Submit PR

```bash
# Push branch
git push origin feature/my-feature
```

Open PR against `main` branch.

## Code Style

### Go Guidelines

- Follow [Effective Go](https://go.dev/doc/effective_go)
- Run `gofmt` and `goimports`
- Use meaningful variable names
- Keep functions focused and small

### Naming Conventions

| Type | Convention | Example |
|------|------------|---------|
| Files | `snake_case.go` | `database_controller.go` |
| Packages | `lowercase` | `database` |
| Types | `PascalCase` | `DatabaseInstance` |
| Functions | `PascalCase` (exported) | `CreateDatabase` |
| Variables | `camelCase` | `dbInstance` |
| Constants | `PascalCase` or `UPPER_SNAKE` | `DefaultPort` |

### Error Handling

```go
// Good
if err != nil {
    return fmt.Errorf("failed to create database: %w", err)
}

// Bad
if err != nil {
    return err  // No context
}
```

### Logging

```go
// Use structured logging
log.Info("Reconciling database",
    "namespace", db.Namespace,
    "name", db.Name)

// Error logging
log.Error(err, "Failed to create database",
    "namespace", db.Namespace,
    "name", db.Name)
```

## Commit Messages

Use [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description>

[optional body]

[optional footer]
```

### Types

| Type | Description |
|------|-------------|
| `feat` | New feature |
| `fix` | Bug fix |
| `docs` | Documentation |
| `style` | Formatting (no code change) |
| `refactor` | Code refactoring |
| `test` | Tests |
| `chore` | Maintenance |

### Examples

```
feat(database): add support for PostgreSQL extensions

fix(user): handle special characters in password generation

docs: update installation guide

test(e2e): add backup/restore tests
```

## Pull Requests

### PR Title

Follow conventional commit format:
```
feat(database): add support for schemas
```

### PR Description

Use the template:

```markdown
## Description
Brief description of changes.

## Type of Change
- [ ] Bug fix
- [ ] New feature
- [ ] Breaking change
- [ ] Documentation update

## Testing
- [ ] Unit tests added/updated
- [ ] E2E tests added/updated
- [ ] Manual testing performed

## Checklist
- [ ] Code follows style guidelines
- [ ] Self-review completed
- [ ] Documentation updated
- [ ] Tests pass
```

### Review Process

1. Automated checks must pass
2. At least one maintainer approval
3. All conversations resolved
4. Squash and merge

## Adding New Features

### Adding a New CRD

1. Define API in `api/v1alpha1/`:
   ```bash
   kubebuilder create api --group dbops --version v1alpha1 --kind NewResource
   ```

2. Implement types in `*_types.go`

3. Generate manifests:
   ```bash
   make generate
   make manifests
   ```

4. Implement controller in `internal/controller/`

5. Add tests

6. Update documentation

### Adding Database Engine Support

1. Implement `DatabaseEngine` interface in `internal/database/`

2. Add engine-specific options to API types

3. Register engine in factory

4. Add tests for new engine

5. Add documentation

## Testing Guidelines

### Unit Tests

```go
func TestCreateDatabase(t *testing.T) {
    // Setup
    db := &Database{...}

    // Execute
    err := createDatabase(db)

    // Assert
    assert.NoError(t, err)
}
```

### Integration Tests

```go
var _ = Describe("Database Controller", func() {
    Context("When creating Database", func() {
        It("Should create database in instance", func() {
            // Test code
        })
    })
})
```

### E2E Tests

```go
var _ = Describe("Database E2E", func() {
    It("Should create and delete database", func() {
        // Create instance
        // Create database
        // Verify database exists
        // Delete database
        // Verify database removed
    })
})
```

## Documentation

### Code Documentation

```go
// CreateDatabase creates a new database in the instance.
// It returns an error if the database already exists or
// if the connection to the instance fails.
func CreateDatabase(ctx context.Context, db *Database) error {
    // ...
}
```

### User Documentation

Update docs in `docs/` directory when:
- Adding new features
- Changing API
- Adding new CRDs
- Changing behavior

## Release Process

Releases are automated via GitHub Actions:

1. Create tag: `git tag v1.2.3`
2. Push tag: `git push origin v1.2.3`
3. GitHub Actions builds and releases

## Getting Help

- **Questions:** Open a GitHub Discussion
- **Bugs:** Open a GitHub Issue
- **Security:** Email security@example.com

## Code of Conduct

Be respectful and inclusive. See [CODE_OF_CONDUCT.md](https://github.com/panteparak/db-provision-operator/blob/main/CODE_OF_CONDUCT.md).
