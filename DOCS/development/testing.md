# Testing

Testing strategies and guides for DB Provision Operator.

## Test Types

| Type | Purpose | Tools | Location |
|------|---------|-------|----------|
| Unit | Test individual functions | Go testing | `*_test.go` |
| Integration | Test with K8s API | envtest | `internal/controller/*_test.go` |
| E2E | Full system tests | Ginkgo, real cluster | `test/e2e/` |

## Running Tests

### All Tests

```bash
# Unit and integration tests
make test

# With coverage
make test-coverage
```

### Unit Tests Only

```bash
go test ./internal/... -short
```

### Integration Tests

```bash
# Requires envtest binaries
make envtest
go test ./internal/controller/...
```

### E2E Tests

```bash
# Create test cluster
make kind-create

# Deploy operator
make deploy

# Run E2E tests
make test-e2e

# Cleanup
make kind-delete
```

## Writing Tests

### Unit Tests

```go
package database

import (
    "testing"
    "github.com/stretchr/testify/assert"
)

func TestGeneratePassword(t *testing.T) {
    tests := []struct {
        name   string
        length int
        want   int
    }{
        {"default length", 24, 24},
        {"custom length", 32, 32},
        {"minimum length", 8, 8},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := GeneratePassword(tt.length)
            assert.Equal(t, tt.want, len(got))
        })
    }
}
```

### Integration Tests with envtest

```go
package controller

import (
    "context"
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/types"

    dbopsv1alpha1 "github.com/panteparak/db-provision-operator/api/v1alpha1"
)

var _ = Describe("Database Controller", func() {
    const (
        timeout  = time.Second * 10
        interval = time.Millisecond * 250
    )

    Context("When creating Database", func() {
        It("Should update status to Ready", func() {
            ctx := context.Background()

            // Create Database
            database := &dbopsv1alpha1.Database{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-database",
                    Namespace: "default",
                },
                Spec: dbopsv1alpha1.DatabaseSpec{
                    InstanceRef: dbopsv1alpha1.ObjectReference{
                        Name: "test-instance",
                    },
                    Name: "testdb",
                },
            }
            Expect(k8sClient.Create(ctx, database)).Should(Succeed())

            // Verify status
            Eventually(func() string {
                db := &dbopsv1alpha1.Database{}
                err := k8sClient.Get(ctx, types.NamespacedName{
                    Name:      "test-database",
                    Namespace: "default",
                }, db)
                if err != nil {
                    return ""
                }
                return db.Status.Phase
            }, timeout, interval).Should(Equal("Ready"))
        })
    })
})
```

### E2E Tests

```go
package e2e

import (
    "context"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("PostgreSQL E2E", func() {
    var namespace string

    BeforeEach(func() {
        namespace = createTestNamespace()
    })

    AfterEach(func() {
        deleteTestNamespace(namespace)
    })

    It("Should provision complete database stack", func() {
        ctx := context.Background()

        // Create admin credentials
        secret := &corev1.Secret{
            ObjectMeta: metav1.ObjectMeta{
                Name:      "postgres-admin",
                Namespace: namespace,
            },
            StringData: map[string]string{
                "username": "postgres",
                "password": "testpassword",
            },
        }
        Expect(k8sClient.Create(ctx, secret)).Should(Succeed())

        // Create DatabaseInstance
        instance := createDatabaseInstance(namespace, "postgres-test")
        Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

        // Wait for instance to be ready
        Eventually(func() string {
            return getInstancePhase(namespace, "postgres-test")
        }, 2*time.Minute, 5*time.Second).Should(Equal("Ready"))

        // Create Database
        database := createDatabase(namespace, "test-db", "postgres-test")
        Expect(k8sClient.Create(ctx, database)).Should(Succeed())

        // Wait for database to be ready
        Eventually(func() string {
            return getDatabasePhase(namespace, "test-db")
        }, time.Minute, 5*time.Second).Should(Equal("Ready"))

        // Verify database exists in PostgreSQL
        Expect(verifyDatabaseExists("testdb")).Should(BeTrue())
    })
})
```

## Test Environment Setup

### envtest Suite Setup

```go
// suite_test.go
package controller

import (
    "path/filepath"
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "k8s.io/client-go/kubernetes/scheme"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/envtest"

    dbopsv1alpha1 "github.com/panteparak/db-provision-operator/api/v1alpha1"
)

var (
    k8sClient client.Client
    testEnv   *envtest.Environment
)

func TestControllers(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
    testEnv = &envtest.Environment{
        CRDDirectoryPaths: []string{
            filepath.Join("..", "..", "config", "crd", "bases"),
        },
    }

    cfg, err := testEnv.Start()
    Expect(err).NotTo(HaveOccurred())

    err = dbopsv1alpha1.AddToScheme(scheme.Scheme)
    Expect(err).NotTo(HaveOccurred())

    k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
    Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
    Expect(testEnv.Stop()).To(Succeed())
})
```

### E2E Suite Setup

```go
// suite_test.go
package e2e

import (
    "os"
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "k8s.io/client-go/kubernetes/scheme"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/client/config"
)

var k8sClient client.Client

func TestE2E(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "E2E Suite")
}

var _ = BeforeSuite(func() {
    cfg, err := config.GetConfig()
    Expect(err).NotTo(HaveOccurred())

    k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
    Expect(err).NotTo(HaveOccurred())
})
```

## Test Fixtures

### Sample Resources

```yaml
# test/e2e/fixtures/postgres-instance.yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: test-postgres
spec:
  engine: postgres
  connection:
    host: postgres.test.svc.cluster.local
    port: 5432
    secretRef:
      name: postgres-admin
```

### Test Databases

Deploy test databases with:

```bash
# PostgreSQL
kubectl apply -f test/e2e/fixtures/postgres-deployment.yaml

# MySQL
kubectl apply -f test/e2e/fixtures/mysql-deployment.yaml
```

## Mocking

### Mock Database Engine

```go
type MockDatabaseEngine struct {
    CreateDatabaseFunc func(ctx context.Context, db *Database) error
    // ...
}

func (m *MockDatabaseEngine) CreateDatabase(ctx context.Context, db *Database) error {
    if m.CreateDatabaseFunc != nil {
        return m.CreateDatabaseFunc(ctx, db)
    }
    return nil
}
```

### Using Mocks in Tests

```go
func TestDatabaseController(t *testing.T) {
    mock := &MockDatabaseEngine{
        CreateDatabaseFunc: func(ctx context.Context, db *Database) error {
            return nil
        },
    }

    controller := &DatabaseReconciler{
        Engine: mock,
    }

    // Test controller with mock
}
```

## Coverage

### Generate Coverage Report

```bash
# Run tests with coverage
go test ./... -coverprofile=coverage.out

# View coverage
go tool cover -html=coverage.out -o coverage.html
```

### Coverage Requirements

- Minimum 70% coverage for new code
- Critical paths should have 90%+ coverage

## CI Integration

Tests run automatically on:

- Pull requests
- Push to main
- Release tags

See [CI/CD](ci-cd.md) for pipeline details.

## Debugging Tests

### Verbose Output

```bash
go test -v ./internal/controller/...
```

### Single Test

```bash
go test -v ./internal/controller/... -run TestDatabaseController
```

### Ginkgo Focus

```go
FDescribe("Database Controller", func() {
    // Only this describe block runs
})

FIt("Should create database", func() {
    // Only this test runs
})
```

### Debug E2E in Cluster

```bash
# Keep cluster after test failure
SKIP_CLEANUP=true make test-e2e

# Inspect cluster
kubectl get all -A
kubectl logs deployment/db-provision-operator -n db-provision-operator-system
```

## Best Practices

1. **Test in isolation** - Each test should be independent
2. **Clean up resources** - Use AfterEach to clean up
3. **Use meaningful names** - Describe what's being tested
4. **Test edge cases** - Include error conditions
5. **Keep tests fast** - Mock external dependencies
6. **Test at appropriate level** - Unit for logic, E2E for integration
