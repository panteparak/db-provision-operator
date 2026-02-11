# Deletion Protection

Deletion protection prevents accidental deletion of critical database resources. When enabled, attempting to delete the Kubernetes resource will fail until protection is explicitly disabled or a force-delete annotation is added.

## Overview

Production databases and their associated users, roles, and grants are critical infrastructure. Accidental deletion can cause:

- Application downtime
- Data loss (depending on deletion policy)
- Service disruptions
- Compliance violations

Deletion protection provides a safety net against:

- Accidental `kubectl delete` commands
- GitOps automation errors
- Misconfigured cleanup jobs
- Namespace deletion cascades

## Enabling Deletion Protection

Add `deletionProtection: true` to any supported resource:

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: production-db
spec:
  instanceRef:
    name: postgres-primary
  name: production
  deletionProtection: true  # Prevents accidental deletion
```

## Supported Resources

| Resource | deletionProtection | Notes |
|----------|-------------------|-------|
| DatabaseInstance | Yes | Blocks deletion of the instance connection |
| Database | Yes | Blocks deletion of the logical database |
| DatabaseUser | Yes | Blocks deletion of the database user |
| DatabaseRole | Yes | Blocks deletion of the database role |
| DatabaseGrant | Yes | Blocks deletion of permission grants |
| DatabaseBackupSchedule | Yes | Blocks deletion of the backup schedule |
| DatabaseBackup | No | Backups are typically transient |
| DatabaseRestore | No | Restores are one-time operations |

## Behavior When Protected

When you attempt to delete a protected resource:

1. The Kubernetes API accepts the delete request
2. The resource is marked for deletion (finalizer prevents immediate removal)
3. The operator detects deletion and checks for protection
4. Deletion is blocked with a `DeletionBlocked` event
5. Resource remains in `Failed` phase with protection message

```bash
$ kubectl delete database production-db
database.dbops.dbprovision.io "production-db" deleted

$ kubectl get database production-db
NAME            PHASE   MESSAGE
production-db   Failed  Deletion blocked by deletion protection
```

## Viewing Protected Resources

Check which resources have deletion protection:

```bash
# All databases with deletion protection
kubectl get databases -o jsonpath='{range .items[?(@.spec.deletionProtection==true)]}{.metadata.name}{"\n"}{end}'

# Using labels (if you label protected resources)
kubectl get databases -l protected=true
```

## Disabling Deletion Protection

### Method 1: Update the Spec

Remove or disable the protection:

```yaml
spec:
  deletionProtection: false
```

Then delete normally:

```bash
kubectl apply -f database.yaml
kubectl delete database production-db
```

### Method 2: Force Delete Annotation

For emergency situations, add the force-delete annotation:

```bash
kubectl annotate database production-db \
  dbops.dbprovision.io/force-delete="true"
```

The resource will be deleted on the next reconciliation loop.

!!! danger "Force Delete is Immediate"
    The force-delete annotation bypasses all safety checks. Use only when you're certain the deletion is intentional.

### Method 3: Edit and Delete

Quick one-liner to disable and delete:

```bash
kubectl patch database production-db -p '{"spec":{"deletionProtection":false}}' && \
kubectl delete database production-db
```

## Deletion Policies

Deletion protection is separate from deletion policy. The deletion policy controls **what happens** when a resource is deleted; deletion protection controls **whether** it can be deleted.

| Setting | Behavior |
|---------|----------|
| `deletionProtection: true` | Cannot delete the CR |
| `deletionPolicy: Retain` | CR deleted, database object kept |
| `deletionPolicy: Delete` | CR deleted, database object deleted |
| `deletionPolicy: Snapshot` | CR deleted after backup created |

### Combined Example

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: production-db
spec:
  instanceRef:
    name: postgres-primary
  name: production
  deletionProtection: true   # Can't delete CR accidentally
  deletionPolicy: Retain     # Even if deleted, keep the actual database
```

## Events

| Event | Type | Description |
|-------|------|-------------|
| `DeletionBlocked` | Warning | Deletion was attempted but blocked by protection |

View events:

```bash
kubectl describe database production-db
# Look for Events section

kubectl get events --field-selector reason=DeletionBlocked
```

## Best Practices

### Production Resources

Always enable deletion protection for production databases:

```yaml
spec:
  deletionProtection: true
  deletionPolicy: Snapshot  # Additional safety: backup before delete
```

### GitOps Workflows

In GitOps (ArgoCD, Flux), deletion protection prevents drift corrections from accidentally deleting resources:

```yaml
# argocd Application
spec:
  syncPolicy:
    automated:
      prune: true  # Would delete resources not in Git
    # But deletion protection prevents actual deletion
```

### Multi-Environment Strategy

| Environment | deletionProtection | deletionPolicy |
|-------------|-------------------|----------------|
| Development | `false` | `Delete` |
| Staging | `true` | `Delete` |
| Production | `true` | `Retain` or `Snapshot` |

### Namespace Deletion

When a namespace is deleted, all resources in it are deleted. Deletion protection **still applies**:

```bash
# This will hang waiting for protected resources
kubectl delete namespace production

# Check which resources are blocking
kubectl get databases,users,roles -n production \
  -o jsonpath='{range .items[?(@.spec.deletionProtection==true)]}{.kind}/{.metadata.name}{"\n"}{end}'
```

To delete the namespace, first remove protection or force-delete each resource.

## Force Delete Script

For emergency cleanup of multiple protected resources:

```bash
#!/bin/bash
# force-delete-all.sh - USE WITH EXTREME CAUTION

NAMESPACE=${1:-default}

for kind in database databaseuser databaserole databasegrant databasebackupschedule; do
  for name in $(kubectl get $kind -n $NAMESPACE -o name); do
    echo "Force deleting $name..."
    kubectl annotate $name -n $NAMESPACE \
      dbops.dbprovision.io/force-delete="true" --overwrite
  done
done

echo "Resources will be deleted on next reconciliation"
```

!!! warning "Audit Force Deletes"
    Always document why force-delete was used. Consider alerting on force-delete annotations:

    ```promql
    # Alert when force-delete annotation is added
    kube_resource_annotations{annotation_dbops_dbprovision_io_force_delete="true"}
    ```

## Troubleshooting

### Resource Stuck in Terminating

If a protected resource shows `Terminating`:

```bash
# Check events for DeletionBlocked
kubectl describe database my-db

# Check if finalizer is still present
kubectl get database my-db -o jsonpath='{.metadata.finalizers}'

# Option 1: Disable protection via spec
kubectl patch database my-db -p '{"spec":{"deletionProtection":false}}'

# Option 2: Force delete
kubectl annotate database my-db dbops.dbprovision.io/force-delete="true"
```

### Protection Not Working

If resources are deleted despite protection:

1. Verify `deletionProtection: true` is in the spec
2. Check operator logs for errors
3. Ensure the operator has proper RBAC permissions
4. Verify the finalizer is being added

```bash
# Check finalizer
kubectl get database my-db -o jsonpath='{.metadata.finalizers}'
# Should include: dbops.dbprovision.io/database
```

### Operator Not Running

If the operator is down, protected resources cannot be deleted (finalizers block deletion). To recover:

```bash
# Option 1: Restart the operator
kubectl rollout restart deployment db-provision-operator-controller-manager \
  -n db-provision-operator-system

# Option 2: Emergency - remove finalizer directly (DANGEROUS)
kubectl patch database my-db -p '{"metadata":{"finalizers":null}}' --type=merge
```

!!! danger "Removing Finalizers"
    Removing finalizers bypasses all cleanup logic. The database object in the actual database will NOT be deleted, potentially leaving orphaned resources.
