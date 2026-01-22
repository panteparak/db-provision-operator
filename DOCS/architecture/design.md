# Design Decisions

Key architectural decisions and rationale.

## CRD Design

### Unified API

**Decision:** Single API group (`dbops.dbprovision.io`) for all database engines.

**Rationale:**
- Consistent user experience across engines
- Easier learning curve
- Simplified RBAC policies
- Engine-specific options in dedicated spec fields

**Alternative considered:** Per-engine CRDs (PostgresDatabase, MySQLDatabase)
- Rejected due to API fragmentation and maintenance burden

### Immutable Fields

**Decision:** Certain fields (e.g., `username`, `name`) are immutable after creation.

**Rationale:**
- Prevents accidental data loss
- Simplifies reconciliation logic
- Aligns with database behavior (renaming is risky)

**Immutable fields:**
- `DatabaseInstance.spec.engine`
- `Database.spec.name`
- `DatabaseUser.spec.username`
- `DatabaseRole.spec.roleName`

### Resource References

**Decision:** Use cross-resource references (`instanceRef`, `databaseRef`).

**Rationale:**
- Enables resource reuse
- Supports cross-namespace scenarios
- Clear ownership model

## Credential Management

### Generated Secrets

**Decision:** Auto-generate passwords and store in Kubernetes Secrets.

**Rationale:**
- Secure credential storage
- Integration with Secret management tools
- Automatic rotation support

### Secret Templates

**Decision:** Support custom Secret templates with variables.

**Rationale:**
- Application-specific formats (DATABASE_URL, JDBC_URL)
- Reduces manual Secret creation
- Single source of truth

**Template variables:**
```
{{ .Username }}
{{ .Password }}
{{ .Host }}
{{ .Port }}
{{ .Database }}
{{ .SSLMode }}
```

## Reconciliation Strategy

### Controller Architecture

**Decision:** One controller per CRD type.

**Rationale:**
- Clear separation of concerns
- Independent scaling
- Simpler testing

### Requeue Strategy

**Decision:** Exponential backoff with configurable limits.

**Rationale:**
- Prevents API overload
- Handles transient failures
- Configurable for different environments

```go
// Default requeue intervals
const (
    defaultRequeueAfter = 30 * time.Second
    maxRequeueAfter     = 5 * time.Minute
)
```

### Dependency Resolution

**Decision:** Wait for dependencies before proceeding.

**Rationale:**
- Ensures prerequisites exist
- Clear error messages
- Predictable behavior

**Example:** Database waits for DatabaseInstance to be Ready.

## Health Checks

### Connection Verification

**Decision:** Periodic health checks for DatabaseInstance.

**Rationale:**
- Detect connection issues early
- Update status for monitoring
- Enable alerting

**Implementation:**
```yaml
healthCheck:
  enabled: true
  intervalSeconds: 30
  timeoutSeconds: 5
  failureThreshold: 3
```

## Deletion Policies

### Retain by Default

**Decision:** Default deletion policy is `Retain`.

**Rationale:**
- Prevent accidental data loss
- Explicit deletion requires `Delete` policy
- Safe default for production

### Deletion Protection

**Decision:** Optional `deletionProtection` field.

**Rationale:**
- Additional safety layer
- Requires explicit removal before deletion
- Production-grade protection

## Multi-Engine Support

### Engine Abstraction

**Decision:** Abstract database operations behind interfaces.

```go
type DatabaseEngine interface {
    CreateDatabase(ctx context.Context, db *Database) error
    DeleteDatabase(ctx context.Context, db *Database) error
    GetDatabase(ctx context.Context, name string) (*DatabaseInfo, error)
}
```

**Rationale:**
- Consistent internal API
- Easy to add new engines
- Testable with mocks

### Engine Detection

**Decision:** Explicit engine specification in DatabaseInstance.

**Rationale:**
- No ambiguity
- Clear user intent
- Avoid auto-detection errors

## Status Management

### Conditions

**Decision:** Use Kubernetes-style conditions.

```yaml
status:
  conditions:
    - type: Ready
      status: "True"
      reason: DatabaseCreated
      message: Database myapp created successfully
      lastTransitionTime: "2024-01-01T00:00:00Z"
```

**Rationale:**
- Standard Kubernetes pattern
- Machine-readable status
- Detailed history

### Phases

**Decision:** Simple phase enum for quick status check.

**Rationale:**
- Human-readable summary
- Easy filtering/sorting
- Dashboard-friendly

## Backup Design

### Backup Job Execution

**Decision:** Execute backups as Kubernetes Jobs.

**Rationale:**
- Native Kubernetes scheduling
- Resource limits/requests
- Automatic retry on failure
- Clean separation from operator

### Storage Abstraction

**Decision:** Support multiple storage backends.

```yaml
storage:
  type: s3|gcs|azure|pvc
```

**Rationale:**
- Cloud-agnostic
- On-premises support (PVC)
- Consistent interface

## Security Decisions

### RBAC Scope

**Decision:** Minimum required permissions per resource type.

**Rationale:**
- Principle of least privilege
- Namespace isolation
- Audit-friendly

### Cross-Namespace References

**Decision:** Allow cross-namespace references with RBAC checks.

**Rationale:**
- Shared database instances
- Team isolation
- Controlled access

See [Security](security.md) for detailed security architecture.

## Performance Considerations

### Concurrent Reconciliation

**Decision:** Configurable concurrent reconciles per controller.

```go
ctrl.Options{
    MaxConcurrentReconciles: 10,
}
```

**Rationale:**
- Scale to large clusters
- Configurable per deployment
- Prevent resource exhaustion

### Caching

**Decision:** Use controller-runtime's built-in caching.

**Rationale:**
- Reduced API server load
- Faster reconciliation
- Memory-efficient

### Connection Pooling

**Decision:** Pool database connections per instance.

**Rationale:**
- Reduce connection overhead
- Respect database limits
- Efficient resource use

## Future Considerations

### Webhook Validation

Planned: Admission webhooks for:
- Immutable field enforcement
- Cross-reference validation
- Quota enforcement

### Metrics

Current: Basic reconciliation metrics
Planned: Per-resource and per-engine metrics

### API Versioning

Current: `v1alpha1`
Path to GA: `v1alpha1` → `v1beta1` → `v1`

Conversion webhooks will handle version migrations.
