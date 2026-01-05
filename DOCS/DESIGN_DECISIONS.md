# DB Provision Operator - Design Decisions

This document captures the key design decisions made during the development of the db-provision-operator, including the rationale behind each choice.

## 1. Framework Selection

### Decision: Operator SDK over Kubebuilder

**Choice**: Operator SDK v1.42.0

**Rationale**:
- Operator SDK is built on top of Kubebuilder but provides additional features
- Better support for Helm and Ansible-based operators if needed in future
- OLM (Operator Lifecycle Manager) integration for production deployment
- More opinionated project structure reduces decision fatigue
- Active community and Red Hat backing

**Alternatives Considered**:
- Kubebuilder: Lower level, more flexibility but more boilerplate
- KUDO: Too opinionated, less Go-native
- Metacontroller: Too simple for our requirements

## 2. API Design

### Decision: Separate CRDs for Instance, Database, and User

**Choice**: Three distinct CRDs with references between them

```yaml
DatabaseInstance → connection to server
Database → references DatabaseInstance
DatabaseUser → references DatabaseInstance + optional DatabaseRef
```

**Rationale**:
- Clear separation of concerns
- Independent lifecycle management
- Reusability (multiple databases per instance)
- Kubernetes-native reference pattern
- Easier RBAC configuration

**Alternatives Considered**:
- Single CRD with nested resources: Would be too complex
- Tightly coupled CRDs: Would limit flexibility

### Decision: Use Spec-based Configuration Over Annotations

**Choice**: All configuration in `.spec` fields

**Rationale**:
- Better schema validation via CRD schemas
- IDE autocompletion support
- Clear documentation in CRD
- Type safety

**Exceptions**:
- `dbops.dbprovision.io/skip-reconcile`: Operational annotation
- `dbops.dbprovision.io/force-delete`: Override deletion protection

## 3. Database Adapter Architecture

### Decision: Interface-Based Adapter Pattern

**Choice**: Single `DatabaseAdapter` interface with engine-specific implementations

```go
type DatabaseAdapter interface {
    Connect(ctx context.Context) error
    Close() error
    Ping(ctx context.Context) error
    // ... operation methods
}
```

**Rationale**:
- Clean abstraction for different database engines
- Easy to add new engines without changing controllers
- Testable with mock implementations
- Follows Go idioms

### Decision: Separate Types Package to Avoid Import Cycles

**Choice**: `internal/adapter/types/types.go` for shared types

**Rationale**:
- Controllers need adapter types for options structs
- Adapters need to implement common interfaces
- Circular import between controller → adapter and adapter → types
- Separate package breaks the cycle cleanly

**Package Structure**:
```
internal/adapter/
├── types/          # Shared types (ConnectionConfig, Options, etc.)
├── adapter.go      # Factory and common functions
├── postgres/       # PostgreSQL implementation
└── mysql/          # MySQL implementation
```

## 4. Driver Selection

### Decision: pgx/v5 for PostgreSQL

**Choice**: `github.com/jackc/pgx/v5`

**Rationale**:
- Native PostgreSQL protocol support
- Connection pooling built-in (pgxpool)
- Excellent performance
- Active development and community
- Full support for PostgreSQL-specific features
- TLS configuration flexibility

**Alternatives Considered**:
- lib/pq: Deprecated, no longer actively maintained
- database/sql: Generic interface, loses PostgreSQL-specific features

### Decision: go-sql-driver/mysql for MySQL

**Choice**: `github.com/go-sql-driver/mysql`

**Rationale**:
- De facto standard Go MySQL driver
- Excellent stability and performance
- Well-documented TLS support
- Active maintenance
- Widely used in production

## 5. Authentication & Security

### Decision: External Secret References

**Choice**: Credentials stored in Kubernetes Secrets, referenced by CRDs

**Rationale**:
- Follows Kubernetes security best practices
- Secrets can be managed by external systems (Vault, External Secrets Operator)
- RBAC controls access to credentials
- No credentials in CRD specs

### Decision: Configurable Secret Key Names

**Choice**: Allow customization of key names in secrets

```yaml
secretRef:
  name: my-secret
  keys:
    username: db-user    # instead of default "username"
    password: db-pass    # instead of default "password"
```

**Rationale**:
- Compatibility with existing secrets
- Different naming conventions in different organizations
- Integration with secret generators that use different keys

### Decision: Automatic Password Generation

**Choice**: Operator can generate secure passwords with configurable options

```yaml
passwordSecret:
  generate: true
  length: 32
  includeSpecialChars: true
  excludeChars: "'\"`"
```

**Rationale**:
- Reduces manual secret management
- Cryptographically secure generation using `crypto/rand`
- Configurable to meet password policies
- Option to exclude problematic characters for different contexts

## 6. Secret Template System

### Decision: Go Text Templates for Secret Generation

**Choice**: Use Go `text/template` for rendering credential secrets

```yaml
secretTemplate:
  data:
    DATABASE_URL: "postgresql://{{.Username}}:{{.Password}}@{{.Host}}:{{.Port}}/{{.Database}}"
    JDBC_URL: "jdbc:postgresql://{{.Host}}:{{.Port}}/{{.Database}}"
```

**Rationale**:
- Flexible format for different application needs
- Standard Go templating (familiar to operators)
- Can generate multiple keys in one secret
- Supports various connection string formats

## 7. Deletion Handling

### Decision: Finalizers for Cleanup

**Choice**: Use Kubernetes finalizers to ensure proper cleanup

**Finalizers Used**:
- `dbops.dbprovision.io/database-instance`
- `dbops.dbprovision.io/database`
- `dbops.dbprovision.io/database-user`

**Rationale**:
- Kubernetes pattern for pre-deletion hooks
- Ensures resources are cleaned up before removal
- Prevents orphaned database objects

### Decision: Configurable Deletion Policy

**Choice**: Two policies: `Retain` (default) and `Delete`

**Rationale**:
- Safe default (retain) prevents accidental data loss
- Explicit delete policy for development/testing
- Consistent with StatefulSet/PVC patterns

### Decision: Deletion Protection with Force Override

**Choice**: `deletionProtection: true` blocks deletion, `force-delete` annotation overrides

**Rationale**:
- Production databases should be protected by default
- Deliberate override mechanism for planned deletions
- Two-step process prevents accidents
- Audit trail (annotation visible in resource history)

## 8. Status Management

### Decision: Conditions-Based Status

**Choice**: Use Kubernetes conditions pattern for status

```go
Conditions:
- Type: Ready
  Status: "True"
  Reason: ReconcileSuccess
- Type: Connected
  Status: "True"
  Reason: ConnectionSuccess
```

**Rationale**:
- Standard Kubernetes pattern
- Multiple independent status indicators
- Historical tracking with LastTransitionTime
- Works with kubectl wait conditions

### Decision: Phase Field for Simple Status

**Choice**: High-level phase alongside conditions

Phases: `Pending`, `Creating`, `Ready`, `Failed`, `Deleting`

**Rationale**:
- Simple status for quick checks
- Easy to understand for operators
- Complements detailed conditions

## 9. Health Checks

### Decision: Configurable Health Check Intervals

**Choice**: Requeue reconciliation at configurable intervals

```yaml
healthCheck:
  intervalSeconds: 60
  timeoutSeconds: 5
```

**Rationale**:
- Continuous verification of connectivity
- Early detection of database issues
- Status reflects actual state
- Configurable per-instance for different SLAs

## 10. Error Handling Strategy

### Decision: Requeue with Backoff for Transient Errors

**Choice**: Return `ctrl.Result{RequeueAfter: duration}` for recoverable errors

| Error Type | Requeue Interval |
|------------|-----------------|
| Connection failed | 30 seconds |
| Secret not found | 30 seconds |
| Instance not ready | 10 seconds |
| Permanent error | Don't requeue |

**Rationale**:
- Automatic recovery from transient failures
- Prevents tight error loops
- Gives external systems time to recover
- Clear distinction between recoverable and permanent errors

## 11. Backup/Restore Implementation

### Decision: External Tool Integration (pg_dump, mysqldump)

**Choice**: Shell out to native database tools for backup/restore

**Rationale**:
- Proven, reliable tools
- Handle all edge cases
- Format compatibility guaranteed
- Full feature support (parallel, compression)
- No need to reimplement complex logic

**Trade-offs**:
- Requires tools installed in operator container
- Process management complexity
- Progress tracking is approximate

### Decision: Async Operations with Progress Tracking

**Choice**: Background operations with progress polling

**Rationale**:
- Backups can be long-running
- Non-blocking for controller
- Cancelable operations
- Progress visibility for users

## 12. Cross-Namespace References

### Decision: Support Optional Namespace in References

**Choice**: References default to same namespace, optionally specify different

```yaml
instanceRef:
  name: shared-postgres
  namespace: database-infra  # optional
```

**Rationale**:
- Simple default case (same namespace)
- Flexibility for shared database instances
- RBAC can control cross-namespace access

## 13. Engine-Specific Options

### Decision: Nested Engine-Specific Structs

**Choice**: Separate `postgres:` and `mysql:` sections in specs

```yaml
spec:
  postgres:
    encoding: UTF8
    extensions:
      - name: uuid-ossp
  # OR
  mysql:
    charset: utf8mb4
    collation: utf8mb4_unicode_ci
```

**Rationale**:
- Clear separation of engine-specific options
- No naming conflicts
- Type safety per engine
- Only relevant section used based on engine type

## 14. RBAC Design

### Decision: Minimal Required Permissions

**Choice**: Operator requests only necessary permissions

```yaml
# DatabaseInstance controller:
- dbops.dbprovision.io/databaseinstances: full CRUD + status
- secrets: get, list, watch (read-only)

# Database controller:
- dbops.dbprovision.io/databases: full CRUD + status
- dbops.dbprovision.io/databaseinstances: get, list, watch
- secrets: get, list, watch

# DatabaseUser controller:
- dbops.dbprovision.io/databaseusers: full CRUD + status
- dbops.dbprovision.io/databaseinstances: get, list, watch
- secrets: get, list, watch, create, update, delete
```

**Rationale**:
- Principle of least privilege
- Each controller only accesses what it needs
- Secrets write only for user controller (credential generation)
- Easy to audit permissions

## Summary of Key Patterns

| Pattern | Implementation |
|---------|---------------|
| Reconciliation | Controller-runtime with requeue |
| Resource References | Namespaced name references |
| Error Handling | Status update + requeue with backoff |
| Cleanup | Finalizers with deletion policy |
| Configuration | CRD spec with engine-specific sections |
| Credentials | External secrets with configurable keys |
| Status | Conditions + phase + engine-specific info |
| Database Operations | Interface-based adapters |
