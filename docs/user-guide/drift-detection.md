# Drift Detection

Drift detection monitors the actual state of database resources and compares them to the desired state defined in your Custom Resources. When differences (drift) are found, the operator can report or automatically correct them.

## Overview

**Configuration drift** occurs when the actual database state no longer matches what's defined in Kubernetes. This can happen due to:

- Manual changes via SQL clients
- Scripts or applications modifying database objects
- Other operators or tools managing the same resources
- Database migrations that modify schema

Without drift detection, these changes can lead to:

- Security misconfigurations (e.g., excessive privileges)
- Inconsistent environments between staging and production
- Deployment failures when expected state differs from actual state
- Audit compliance issues

## Drift Modes

The operator supports three drift detection modes:

| Mode | Behavior |
|------|----------|
| `ignore` | No drift detection. Changes outside Kubernetes are not tracked. |
| `detect` | Detects drift and reports it in status/events. Does not auto-correct. |
| `correct` | Detects drift and automatically corrects it to match the CR spec. |

## Configuration

### Instance-Level Policy (Default)

Set a default drift policy on `DatabaseInstance` that applies to all child resources:

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: postgres-primary
spec:
  engine: postgres
  connection:
    host: postgres.database.svc.cluster.local
    port: 5432
    secretRef:
      name: postgres-credentials
  driftPolicy:
    mode: detect  # Default for all databases/users/roles
    interval: "5m"
```

### Resource-Level Override

Individual resources can override the instance's default policy:

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: production-db
spec:
  instanceRef:
    name: postgres-primary
  name: production
  driftPolicy:
    mode: correct  # Override: auto-correct for this database
    interval: "2m"
```

### Policy Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `mode` | string | `detect` | `ignore`, `detect`, or `correct` |
| `interval` | string | `5m` | How often to check for drift (Go duration) |

## Status Fields

When drift is detected, the status shows details:

```yaml
status:
  phase: Ready
  drift:
    detected: true
    lastChecked: "2024-01-15T10:30:00Z"
    diffs:
      - field: "encoding"
        expected: "UTF8"
        actual: "LATIN1"
        destructive: true
        immutable: false
      - field: "connectionLimit"
        expected: "100"
        actual: "50"
        destructive: false
        immutable: false
```

### Diff Fields

| Field | Description |
|-------|-------------|
| `field` | The name of the field that differs |
| `expected` | The value defined in the CR spec |
| `actual` | The current value in the database |
| `destructive` | If correcting this would cause data loss |
| `immutable` | If this field cannot be changed after creation |

## Events

The operator emits events for drift-related activities:

| Event | Type | Description |
|-------|------|-------------|
| `DriftDetected` | Warning | Drift was detected in one or more fields |
| `DriftCorrected` | Normal | Drift was successfully corrected |
| `DriftCorrectionFailed` | Warning | Attempted to correct drift but failed |

View events with:

```bash
kubectl describe database production-db
# or
kubectl get events --field-selector involvedObject.name=production-db
```

## Destructive Drift Corrections

Some drift corrections are potentially destructive (e.g., changing encoding requires recreating the database). By default, the operator **skips destructive corrections** even in `correct` mode.

### Allowing Destructive Corrections

To allow destructive corrections, add an annotation:

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: dev-db
  annotations:
    dbops.dbprovision.io/allow-destructive-drift: "true"
spec:
  instanceRef:
    name: postgres-primary
  driftPolicy:
    mode: correct
```

!!! danger "Use with Caution"
    Destructive corrections may cause **data loss**. Only enable this for:

    - Development/test environments
    - Resources where data loss is acceptable
    - After verifying the current state and having backups

### Destructive vs Non-Destructive Changes

| Resource | Non-Destructive | Destructive |
|----------|-----------------|-------------|
| Database | Owner, connection limit | Encoding, collation, template |
| User | Password, connection limit, roles | Username |
| Role | Privileges, role membership | Role name |
| Grant | Adding privileges | Revoking privileges |

## Immutable Fields

Some fields cannot be changed after resource creation:

| Resource | Immutable Fields |
|----------|-----------------|
| Database | Template (PostgreSQL) |
| User | Username (in most cases) |
| DatabaseInstance | Engine type |

When drift is detected in immutable fields, the operator reports it but cannot correct it automatically.

## Resource Discovery

The operator can discover database resources that exist but are not managed by Kubernetes CRs.

### Enabling Discovery

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: postgres-primary
spec:
  engine: postgres
  discovery:
    enabled: true
    interval: "30m"
```

### Discovered Resources Status

```yaml
status:
  discoveredResources:
    lastScan: "2024-01-15T10:00:00Z"
    databases:
      - name: legacy_app
        discovered: "2024-01-15T10:00:00Z"
        adopted: false
      - name: temp_data
        discovered: "2024-01-15T10:00:00Z"
        adopted: false
    users:
      - name: old_admin
        discovered: "2024-01-15T10:00:00Z"
        adopted: false
```

### Adopting Discovered Resources

To bring a discovered resource under management, use adoption annotations:

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: postgres-primary
  annotations:
    dbops.dbprovision.io/adopt-databases: "legacy_app,temp_data"
    dbops.dbprovision.io/adopt-users: "old_admin"
```

When adopted:

1. The operator creates a CR for the discovered resource
2. The CR is labeled with `dbops.dbprovision.io/adopted: "true"`
3. Future changes are managed through the CR

## Metrics

The operator exposes drift-related metrics:

| Metric | Description |
|--------|-------------|
| `dbprovision_drift_detected` | Gauge: 1 if drift detected, 0 otherwise |
| `dbprovision_drift_corrections_total` | Counter of drift corrections |

Example Prometheus query to alert on drift:

```promql
dbprovision_drift_detected{resource_type="database"} == 1
```

## Best Practices

### Development Environments

Use `correct` mode to keep environments consistent:

```yaml
driftPolicy:
  mode: correct
  interval: "1m"
```

### Production Environments

Use `detect` mode with alerts:

```yaml
driftPolicy:
  mode: detect
  interval: "5m"
```

Set up alerts for `DriftDetected` events:

```yaml
# Example Prometheus alert rule
- alert: DatabaseDriftDetected
  expr: dbprovision_drift_detected == 1
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "Configuration drift detected"
    description: "{{ $labels.name }} has configuration drift"
```

### Audit Trails

For compliance, combine with event logging:

```bash
# Export events to external logging system
kubectl get events -o json | jq '.items[] | select(.reason | contains("Drift"))'
```

## Supported Resources

| Resource | Drift Detection | Auto-Correction |
|----------|----------------|-----------------|
| Database | Yes | Yes |
| DatabaseUser | Yes | Yes |
| DatabaseRole | Yes | Yes |
| DatabaseGrant | Yes | Yes |
| DatabaseBackup | No | No |
| DatabaseRestore | No | No |
| DatabaseBackupSchedule | No | No |

## Example: Full Drift Detection Setup

```yaml
---
# Instance with detect-only default
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: postgres-primary
spec:
  engine: postgres
  connection:
    host: postgres.database.svc.cluster.local
    port: 5432
    secretRef:
      name: postgres-credentials
  driftPolicy:
    mode: detect
    interval: "5m"
  discovery:
    enabled: true
    interval: "30m"
---
# Production database: detect only, alert on drift
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: production-db
spec:
  instanceRef:
    name: postgres-primary
  name: production
  # Uses instance default (detect mode)
---
# Dev database: auto-correct drift
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: dev-db
  annotations:
    dbops.dbprovision.io/allow-destructive-drift: "true"
spec:
  instanceRef:
    name: postgres-primary
  name: development
  driftPolicy:
    mode: correct
    interval: "1m"
```

## Troubleshooting

### Drift Not Detected

1. Check drift mode is not `ignore`
2. Verify the interval has passed since last check
3. Check controller logs for errors

```bash
kubectl logs -l app.kubernetes.io/name=db-provision-operator -n db-provision-operator-system | grep -i drift
```

### Drift Correction Failing

1. Check if the field is marked as `immutable`
2. Check if destructive corrections need the annotation
3. Verify database permissions

### Drift Detected but Expected

For known differences that should be ignored, consider:

1. Updating the CR spec to match actual state
2. Setting `mode: ignore` for that resource
3. Using the skip-reconcile annotation temporarily
