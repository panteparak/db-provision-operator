# Operations

Operational guides for running DB Provision Operator in production.

## Overview

This section covers:

- [Monitoring](monitoring.md) - Prometheus metrics, Grafana dashboards, alerting
- [Troubleshooting](troubleshooting.md) - Common issues and solutions

## Operational Tasks

### Checking Status

```bash
# All resources across namespaces
kubectl get databaseinstances,databases,databaseusers,databaseroles,databasegrants -A

# Specific namespace
kubectl get all -l app.kubernetes.io/managed-by=db-provision-operator -n myapp

# Detailed status
kubectl describe databaseinstance postgres-primary
```

### Health Verification

```bash
# Check operator health
kubectl get pods -n db-provision-operator-system

# Check operator logs
kubectl logs -n db-provision-operator-system deployment/db-provision-operator -f

# Check specific resource events
kubectl get events --field-selector involvedObject.name=myapp-database
```

### Credential Management

```bash
# Get generated credentials
kubectl get secret myapp-user-credentials -o jsonpath='{.data.password}' | base64 -d

# Rotate password (delete secret, operator regenerates)
kubectl delete secret myapp-user-credentials

# Or use annotation
kubectl annotate databaseuser myapp-user dbops.dbprovision.io/rotate-password=true
```

### Backup Operations

```bash
# List backups
kubectl get databasebackups

# Check backup status
kubectl describe databasebackup myapp-backup

# Trigger manual backup from schedule
kubectl create job --from=cronjob/myapp-backup-schedule manual-backup-$(date +%s)

# List scheduled backups
kubectl get databasebackupschedules
```

### Restore Operations

```bash
# Create restore
kubectl apply -f restore.yaml

# Monitor restore progress
kubectl get databaserestore myapp-restore -w

# Check restore logs
kubectl logs job/myapp-restore-job
```

## Resource Lifecycle

### Creation Order

For new deployments, create resources in this order:

1. Secrets (admin credentials)
2. DatabaseInstance
3. Database
4. DatabaseRole (optional)
5. DatabaseUser
6. DatabaseGrant
7. DatabaseBackupSchedule (optional)

### Deletion Order

For cleanup, delete in reverse order:

1. DatabaseBackupSchedule
2. DatabaseGrant
3. DatabaseUser
4. DatabaseRole
5. Database
6. DatabaseInstance
7. Secrets

### Finalizers

Resources use finalizers to ensure cleanup:

```bash
# Check finalizers
kubectl get database myapp -o jsonpath='{.metadata.finalizers}'

# Force delete (use with caution!)
kubectl patch database myapp -p '{"metadata":{"finalizers":null}}' --type=merge
```

## Scaling

### Operator Scaling

```yaml
# Increase replicas for HA
apiVersion: apps/v1
kind: Deployment
metadata:
  name: db-provision-operator
spec:
  replicas: 3
```

### Concurrent Reconciliation

Configure via Helm values:

```yaml
controller:
  maxConcurrentReconciles: 20
```

## Upgrades

### Operator Upgrade

```bash
# Helm upgrade
helm upgrade db-provision-operator db-provision/db-provision-operator \
  --namespace db-provision-operator-system \
  --version X.Y.Z

# Verify upgrade
kubectl rollout status deployment/db-provision-operator -n db-provision-operator-system
```

### CRD Upgrade

CRDs are upgraded automatically with Helm. For manual upgrade:

```bash
kubectl apply -f https://github.com/panteparak/db-provision-operator/releases/latest/download/crds.yaml
```

## Maintenance Windows

### Planned Maintenance

1. Scale down operator:
   ```bash
   kubectl scale deployment/db-provision-operator --replicas=0 -n db-provision-operator-system
   ```

2. Perform maintenance

3. Scale up operator:
   ```bash
   kubectl scale deployment/db-provision-operator --replicas=1 -n db-provision-operator-system
   ```

### Emergency Procedures

**Pause all reconciliation:**
```bash
kubectl scale deployment/db-provision-operator --replicas=0 -n db-provision-operator-system
```

**Resume:**
```bash
kubectl scale deployment/db-provision-operator --replicas=1 -n db-provision-operator-system
```

## Disaster Recovery

### Backup Strategy

1. **Database backups** - Managed by DatabaseBackupSchedule
2. **Kubernetes resources** - Export CRDs for recovery
3. **Secrets** - Backup encryption keys separately

### Export Resources

```bash
# Export all DB Provision resources
kubectl get databaseinstances,databases,databaseusers,databaseroles,databasegrants,databasebackups,databasebackupschedules \
  -A -o yaml > db-provision-backup.yaml
```

### Recovery Steps

1. Deploy operator
2. Apply Secrets
3. Apply DatabaseInstance (wait for Ready)
4. Apply remaining resources
5. Restore from DatabaseBackup if needed

## Next Steps

- [Monitoring](monitoring.md) - Set up monitoring and alerting
- [Troubleshooting](troubleshooting.md) - Solve common problems
