# DB Provision Operator - Development Guidelines

## Resource Dependency Graph

```
DatabaseInstance / ClusterDatabaseInstance
  ├── Database (via instanceRef / clusterInstanceRef)
  ├── DatabaseUser (via instanceRef / clusterInstanceRef)
  └── DatabaseRole (via instanceRef / clusterInstanceRef)
        └── DatabaseGrant (via userRef / roleRef / databaseRef)
```

**Deletion order**: Delete leaf resources first (DatabaseGrant), then parents (Database, DatabaseUser, DatabaseRole), then root (DatabaseInstance). The operator enforces this via dependency checking in each controller's `handleDeletion()`.

## Key Architecture Patterns

### Controller Structure
- Controllers live in `internal/features/<name>/controller.go`
- Each controller has a matching Handler in `handler.go` and Repository in `repository.go`
- Controllers handle K8s reconciliation; Handlers encapsulate business logic; Repositories handle database operations

### Dependency-Aware Deletion
- Parent controllers check for child references before removing finalizers
- `hasChildDependencies()` / `hasGrantDependencies()` methods list children in the same namespace
- When children exist: set `Phase=Failed`, `Ready` condition to `DependenciesExist`, requeue after 10s
- Dependency check errors are logged but do not block deletion (fail-open for check errors)

**Force-delete with children:** When `force-delete` is set on a resource that has children, the controller enters `PhasePendingDeletion` and populates `status.deletionConfirmation` with the list of affected children and a confirmation hash. The user must set the annotation `dbops.dbprovision.io/confirm-force-delete` to the hash value to confirm. Once confirmed, children are cascade-deleted (each child's own controller handles cleanup per its `deletion-policy`). The parent tracks remaining children in `status.deletionConfirmation.remainingCount` and removes its own finalizer only after all children are gone. Resources with no children proceed immediately without confirmation.

### Condition Reasons
- `ReasonDependenciesExist` - deletion blocked by child resources
- `ReasonDeletionProtected` - deletion protection enabled
- `ReasonInstanceNotReady` - parent instance not in Ready phase
- `ReasonDatabaseNotReady` - parent database not in Ready phase
- `ReasonPendingDeletionConfirmation` - force-delete waiting for user confirmation hash
- `ReasonCascadeDeleting` - cascade-deleting children after force-delete confirmation

### Status Update Patterns
- Always set `Phase`, `Message`, and appropriate conditions before `Status().Update()`
- Use `util.SetReadyCondition()` for the Ready condition
- Set `ReconcileID` and `LastReconcileTime` for end-to-end tracing
- Set `ObservedGeneration` to track spec changes

## RBAC Marker Rules

**Critical**: `+kubebuilder:rbac` markers must START the comment block above `Reconcile()`. If placed after a doc comment line, they are silently ignored by controller-gen.

```go
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databases,verbs=list  // <-- markers FIRST
//
// Reconcile implements the reconciliation loop.                                   // <-- doc comment AFTER
func (c *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
```

After changing markers: `make manifests` to regenerate `config/rbac/role.yaml`, then verify Helm parity with `make test-templates`.

## Testing Requirements

### Unit Tests (controller_test.go)
Each controller needs these deletion tests:
1. `DeletionBlockedByChildDependencies` - verify requeue, finalizer retained, condition set
2. `DeletionSucceedsWhenNoChildren` - verify finalizer removed, clean deletion
3. `ForceDeleteBypassesChildCheck` - verify force-delete with children enters PendingDeletion with confirmation hash
4. `ForceDeleteConfirmedCascadesChildren` - verify confirmed cascade deletes children and parent
5. `ForceDeleteWrongHashBlocksDeletion` - verify wrong hash stays in PendingDeletion
6. `ForceDeleteNoChildrenSkipsConfirmation` - verify immediate deletion without confirmation
7. `ForceDeleteCascadeTracksRemainingChildren` - verify remaining count tracking during cascade

Use `fake.NewClientBuilder().WithScheme(scheme).WithObjects(...).WithStatusSubresource(...).Build()` for the test client.

### Verification Checklist
```bash
make test              # Unit tests
make test-envtest      # Integration tests with real CRDs
make manifests         # Regenerate RBAC
make test-templates    # Helm/Kustomize parity
golangci-lint run ./...# Lint
```

## Annotations

| Annotation | Value | Effect |
|---|---|---|
| `dbops.dbprovision.io/force-delete` | `"true"` | Bypass deletion protection; with children, triggers cascade confirmation flow |
| `dbops.dbprovision.io/confirm-force-delete` | `"<hash>"` | Confirm force-delete cascade; hash from `status.deletionConfirmation.hash` |
| `dbops.dbprovision.io/skip-reconcile` | `"true"` | Skip reconciliation entirely |
| `dbops.dbprovision.io/deletion-policy` | `"Delete"` / `"Retain"` | Control external resource cleanup on CR deletion |
| `dbops.dbprovision.io/deletion-protection` | `"true"` | Block deletion (used by User/Role controllers) |
| `dbops.dbprovision.io/allow-destructive-drift` | `"true"` | Allow destructive drift corrections |

## Pre-commit Hooks

The project uses extensive pre-commit hooks including golangci-lint, go test, envtest CRD validation, helm lint, and conventional commit message enforcement. All must pass before commit.
