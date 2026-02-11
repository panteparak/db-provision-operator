# Events Reference

DB Provision Operator emits Kubernetes events to provide visibility into resource lifecycle and operational status. Events are useful for monitoring, alerting, and debugging.

## Viewing Events

### Using kubectl describe

```bash
# View events for a specific resource
kubectl describe database my-database

# View events for all resources of a type
kubectl describe databases
```

### Using kubectl get events

```bash
# All events in a namespace
kubectl get events -n my-namespace

# Filter by resource
kubectl get events --field-selector involvedObject.name=my-database

# Filter by reason
kubectl get events --field-selector reason=DriftDetected

# Sort by timestamp
kubectl get events --sort-by='.lastTimestamp'

# Watch events in real-time
kubectl get events -w
```

### Using JSON output for automation

```bash
# Export events as JSON
kubectl get events -o json | jq '.items[] | select(.reason | contains("Drift"))'

# Get events for a specific resource type
kubectl get events -o json | jq '.items[] | select(.involvedObject.kind == "Database")'
```

## Events by Resource Type

### DatabaseInstance

| Event | Type | Description |
|-------|------|-------------|
| `Connected` | Normal | Successfully connected to the database server |
| `ConnectionFailed` | Warning | Failed to establish connection to the database server |
| `HealthCheckPassed` | Normal | Health check succeeded |
| `HealthCheckFailed` | Warning | Health check failed |
| `DeletionBlocked` | Warning | Deletion blocked by deletion protection |
| `ReconcileFailed` | Warning | Reconciliation loop encountered an error |

### Database

| Event | Type | Description |
|-------|------|-------------|
| `Created` | Normal | Database was successfully created |
| `Deleted` | Normal | Database was successfully deleted |
| `CreateFailed` | Warning | Failed to create the database |
| `DeleteFailed` | Warning | Failed to delete the database |
| `DeletionBlocked` | Warning | Deletion blocked by deletion protection |
| `DriftDetected` | Warning | Configuration drift was detected |
| `DriftCorrected` | Normal | Drift was automatically corrected |
| `DriftCorrectionFailed` | Warning | Failed to correct detected drift |
| `ReconcileFailed` | Warning | Reconciliation loop encountered an error |

### DatabaseUser

| Event | Type | Description |
|-------|------|-------------|
| `Created` | Normal | User was successfully created |
| `Deleted` | Normal | User was successfully deleted |
| `CreateFailed` | Warning | Failed to create the user |
| `DeleteFailed` | Warning | Failed to delete the user |
| `DeletionBlocked` | Warning | Deletion blocked by deletion protection |
| `PasswordRotated` | Normal | User password was rotated |
| `SecretCreated` | Normal | Credentials secret was created |
| `SecretUpdated` | Normal | Credentials secret was updated |
| `DriftDetected` | Warning | Configuration drift was detected |
| `DriftCorrected` | Normal | Drift was automatically corrected |
| `DriftCorrectionFailed` | Warning | Failed to correct detected drift |
| `ReconcileFailed` | Warning | Reconciliation loop encountered an error |

### DatabaseRole

| Event | Type | Description |
|-------|------|-------------|
| `Created` | Normal | Role was successfully created |
| `Deleted` | Normal | Role was successfully deleted |
| `CreateFailed` | Warning | Failed to create the role |
| `DeleteFailed` | Warning | Failed to delete the role |
| `DeletionBlocked` | Warning | Deletion blocked by deletion protection |
| `DriftDetected` | Warning | Configuration drift was detected |
| `DriftCorrected` | Normal | Drift was automatically corrected |
| `DriftCorrectionFailed` | Warning | Failed to correct detected drift |
| `ReconcileFailed` | Warning | Reconciliation loop encountered an error |

### DatabaseGrant

| Event | Type | Description |
|-------|------|-------------|
| `Applied` | Normal | Grants were successfully applied |
| `Revoked` | Normal | Grants were successfully revoked |
| `ApplyFailed` | Warning | Failed to apply grants |
| `RevokeFailed` | Warning | Failed to revoke grants |
| `DeletionBlocked` | Warning | Deletion blocked by deletion protection |
| `DriftDetected` | Warning | Configuration drift was detected |
| `DriftCorrected` | Normal | Drift was automatically corrected |
| `DriftCorrectionFailed` | Warning | Failed to correct detected drift |
| `ReconcileFailed` | Warning | Reconciliation loop encountered an error |

### DatabaseBackup

| Event | Type | Description |
|-------|------|-------------|
| `Started` | Normal | Backup operation started |
| `Completed` | Normal | Backup completed successfully |
| `Failed` | Warning | Backup operation failed |
| `ProgressUpdated` | Normal | Backup progress was updated |
| `ReconcileFailed` | Warning | Reconciliation loop encountered an error |

### DatabaseRestore

| Event | Type | Description |
|-------|------|-------------|
| `Started` | Normal | Restore operation started |
| `Completed` | Normal | Restore completed successfully |
| `Failed` | Warning | Restore operation failed |
| `DeadlineExceeded` | Warning | Restore exceeded the configured timeout |
| `ValidationFailed` | Warning | Restore validation failed |
| `ProgressUpdated` | Normal | Restore progress was updated |
| `ReconcileFailed` | Warning | Reconciliation loop encountered an error |

### DatabaseBackupSchedule

| Event | Type | Description |
|-------|------|-------------|
| `BackupTriggered` | Normal | Scheduled backup was triggered |
| `Paused` | Normal | Backup schedule was paused |
| `Resumed` | Normal | Backup schedule was resumed |
| `DeletionBlocked` | Warning | Deletion blocked by deletion protection |
| `RetentionApplied` | Normal | Retention policy was applied, old backups deleted |
| `ReconcileFailed` | Warning | Reconciliation loop encountered an error |

## Event Categories

### Lifecycle Events

Events related to resource creation and deletion:

- `Created`, `Deleted` - Successful operations
- `CreateFailed`, `DeleteFailed` - Failed operations
- `DeletionBlocked` - Deletion protection active

### Drift Events

Events related to configuration drift detection and correction:

- `DriftDetected` - Drift found between spec and actual state
- `DriftCorrected` - Drift was automatically fixed
- `DriftCorrectionFailed` - Auto-correction failed

### Backup/Restore Events

Events related to backup and restore operations:

- `Started`, `Completed`, `Failed` - Operation lifecycle
- `ProgressUpdated` - Progress tracking
- `DeadlineExceeded` - Timeout exceeded
- `ValidationFailed` - Pre-restore validation failed

### Connection Events

Events related to database connectivity:

- `Connected`, `ConnectionFailed` - Connection status
- `HealthCheckPassed`, `HealthCheckFailed` - Health monitoring

## Alerting Integration

### Prometheus AlertManager

Use kube-state-metrics to convert events to metrics for alerting:

```yaml
# PrometheusRule for critical events
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: db-provision-alerts
spec:
  groups:
    - name: db-provision-operator
      rules:
        - alert: DatabaseConnectionFailed
          expr: |
            increase(kubernetes_events_total{
              reason="ConnectionFailed",
              involved_object_kind="DatabaseInstance"
            }[5m]) > 0
          for: 1m
          labels:
            severity: critical
          annotations:
            summary: "Database connection failed"
            description: "DatabaseInstance {{ $labels.involved_object_name }} failed to connect"

        - alert: DatabaseDriftDetected
          expr: |
            increase(kubernetes_events_total{
              reason="DriftDetected"
            }[15m]) > 0
          for: 5m
          labels:
            severity: warning
          annotations:
            summary: "Configuration drift detected"
            description: "{{ $labels.involved_object_kind }}/{{ $labels.involved_object_name }} has configuration drift"

        - alert: BackupFailed
          expr: |
            increase(kubernetes_events_total{
              reason="Failed",
              involved_object_kind="DatabaseBackup"
            }[1h]) > 0
          for: 0m
          labels:
            severity: critical
          annotations:
            summary: "Database backup failed"
            description: "Backup {{ $labels.involved_object_name }} failed"
```

### Event Exporter

Use tools like [kubernetes-event-exporter](https://github.com/resmoio/kubernetes-event-exporter) to forward events to external systems:

```yaml
# Event exporter configuration
apiVersion: v1
kind: ConfigMap
metadata:
  name: event-exporter-config
data:
  config.yaml: |
    route:
      routes:
        - match:
            - receiver: "slack"
          drop:
            - type: "Normal"  # Only alert on Warning events
    receivers:
      - name: "slack"
        slack:
          endpoint: "https://hooks.slack.com/services/..."
          channel: "#db-alerts"
          message: |
            {{ .InvolvedObject.Kind }}/{{ .InvolvedObject.Name }}: {{ .Reason }}
            {{ .Message }}
```

### Loki/Grafana

Query events in Grafana using Loki:

```logql
# All warning events from db-provision-operator
{namespace="db-provision-operator-system"} |= "Warning"

# Drift detection events
{namespace="default"} | json | reason="DriftDetected"

# Failed backup events
{namespace="default"} | json | involvedObject_kind="DatabaseBackup" | reason="Failed"
```

## Best Practices

### Event Retention

Kubernetes events have a default TTL of 1 hour. For longer retention:

1. **Increase cluster event TTL** (kube-apiserver flag):
   ```
   --event-ttl=24h
   ```

2. **Export events to external storage** using event exporters

3. **Use the operator's metrics** for long-term monitoring:
   ```promql
   dbprovision_reconcile_errors_total
   dbprovision_drift_detected
   ```

### Monitoring Strategy

| Use Case | Approach |
|----------|----------|
| Real-time debugging | `kubectl get events -w` |
| Historical analysis | Event exporter + log storage |
| Alerting | Prometheus rules on event metrics |
| Dashboards | Grafana with Loki or event-derived metrics |

### Event Filtering

Focus on actionable events:

```bash
# Only warning events (exclude Normal)
kubectl get events --field-selector type=Warning

# Specific reasons to watch
kubectl get events --field-selector reason=ConnectionFailed
kubectl get events --field-selector reason=DriftDetected
kubectl get events --field-selector reason=BackupFailed
```

## Troubleshooting with Events

### Database Not Ready

```bash
# Check for connection issues
kubectl get events --field-selector involvedObject.name=my-instance,reason=ConnectionFailed

# Check for creation failures
kubectl get events --field-selector involvedObject.name=my-database,reason=CreateFailed
```

### Drift Not Being Corrected

```bash
# Check for drift correction failures
kubectl get events --field-selector reason=DriftCorrectionFailed

# Verify drift is being detected
kubectl get events --field-selector reason=DriftDetected
```

### Backup Issues

```bash
# Check backup lifecycle
kubectl get events --field-selector involvedObject.kind=DatabaseBackup --sort-by='.lastTimestamp'

# Check for failures
kubectl get events --field-selector involvedObject.kind=DatabaseBackup,reason=Failed
```

### Deletion Blocked

```bash
# Find resources with deletion protection blocking
kubectl get events --field-selector reason=DeletionBlocked
```
