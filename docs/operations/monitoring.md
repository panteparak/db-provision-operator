# Monitoring

Set up monitoring, metrics, and alerting for DB Provision Operator.

## Prometheus Metrics

### Enable Metrics

Metrics are enabled by default. Configure via Helm:

```yaml
# values.yaml
metrics:
  enabled: true
  port: 8080
  serviceMonitor:
    enabled: true
    interval: 30s
```

### Available Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `db_provision_reconcile_total` | Counter | Total reconciliations |
| `db_provision_reconcile_errors_total` | Counter | Failed reconciliations |
| `db_provision_reconcile_duration_seconds` | Histogram | Reconciliation duration |
| `db_provision_resource_status` | Gauge | Resource status (1=Ready, 0=NotReady) |
| `db_provision_database_connection_status` | Gauge | Instance connection status |
| `db_provision_backup_last_success_timestamp` | Gauge | Last successful backup time |
| `db_provision_backup_size_bytes` | Gauge | Backup size |

### ServiceMonitor

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: db-provision-operator
  namespace: db-provision-operator-system
  labels:
    app: db-provision-operator
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: db-provision-operator
  endpoints:
    - port: metrics
      interval: 30s
      path: /metrics
```

## Grafana Dashboards

### Import Dashboard

Import the provided dashboard JSON:

```bash
# Download dashboard
curl -o db-provision-dashboard.json \
  https://raw.githubusercontent.com/panteparak/db-provision-operator/main/dashboards/db-provision-operator.json

# Import via Grafana API
curl -X POST http://admin:admin@localhost:3000/api/dashboards/db/import \
  -H "Content-Type: application/json" \
  -d @db-provision-dashboard.json
```

### Dashboard Panels

The dashboard includes:

1. **Overview**
   - Total resources by type
   - Resource status distribution
   - Reconciliation rate

2. **Instance Health**
   - Connection status per instance
   - Health check results
   - Connection latency

3. **Reconciliation**
   - Reconciliation rate
   - Error rate
   - Duration percentiles

4. **Backups**
   - Backup success/failure rate
   - Backup sizes over time
   - Next scheduled backups

### Custom Queries

**Resources by status:**
```promql
sum by (kind, status) (db_provision_resource_status)
```

**Reconciliation error rate:**
```promql
rate(db_provision_reconcile_errors_total[5m])
/ rate(db_provision_reconcile_total[5m])
```

**Backup age:**
```promql
time() - db_provision_backup_last_success_timestamp
```

## Alerting

### PrometheusRule

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: db-provision-alerts
  namespace: db-provision-operator-system
spec:
  groups:
    - name: db-provision-operator
      rules:
        # Instance connectivity
        - alert: DatabaseInstanceUnhealthy
          expr: db_provision_database_connection_status == 0
          for: 5m
          labels:
            severity: critical
          annotations:
            summary: "Database instance {{ $labels.name }} is unhealthy"
            description: "Instance has been unhealthy for more than 5 minutes"

        # Reconciliation errors
        - alert: HighReconcileErrorRate
          expr: |
            rate(db_provision_reconcile_errors_total[5m])
            / rate(db_provision_reconcile_total[5m]) > 0.1
          for: 10m
          labels:
            severity: warning
          annotations:
            summary: "High reconciliation error rate"
            description: "More than 10% of reconciliations are failing"

        # Backup alerts
        - alert: BackupFailed
          expr: |
            db_provision_backup_status{phase="Failed"} == 1
          for: 1m
          labels:
            severity: warning
          annotations:
            summary: "Backup {{ $labels.name }} failed"
            description: "Database backup has failed"

        - alert: BackupOverdue
          expr: |
            time() - db_provision_backup_last_success_timestamp > 86400
          for: 1h
          labels:
            severity: warning
          annotations:
            summary: "Backup overdue for {{ $labels.database }}"
            description: "No successful backup in the last 24 hours"

        # Resource stuck
        - alert: ResourceStuckPending
          expr: |
            db_provision_resource_status{phase="Pending"} == 1
          for: 30m
          labels:
            severity: warning
          annotations:
            summary: "Resource {{ $labels.kind }}/{{ $labels.name }} stuck in Pending"
            description: "Resource has been Pending for more than 30 minutes"
```

### Alert Severity Guidelines

| Severity | Response | Examples |
|----------|----------|----------|
| `critical` | Immediate | Instance unhealthy, operator down |
| `warning` | Within hours | Backup failed, high error rate |
| `info` | Next business day | Resource stuck, minor issues |

## Logging

### Log Levels

Configure log level via Helm:

```yaml
logging:
  level: info  # debug, info, warn, error
  format: json  # json, text
```

### Log Format

JSON logs for aggregation:

```json
{
  "level": "info",
  "ts": "2024-01-01T00:00:00.000Z",
  "logger": "controller.database",
  "msg": "Reconciling Database",
  "namespace": "default",
  "name": "myapp-database",
  "generation": 1
}
```

### Log Queries

**Loki/Grafana:**
```logql
{namespace="db-provision-operator-system"} |= "error"
```

**Elasticsearch:**
```json
{
  "query": {
    "bool": {
      "must": [
        {"match": {"kubernetes.namespace": "db-provision-operator-system"}},
        {"match": {"level": "error"}}
      ]
    }
  }
}
```

## Health Endpoints

### Operator Health

```bash
# Readiness probe
curl http://operator:8081/readyz

# Liveness probe
curl http://operator:8081/healthz

# Metrics endpoint
curl http://operator:8080/metrics
```

### Database Instance Health

Check via status:

```bash
kubectl get databaseinstance postgres-primary -o jsonpath='{.status.conditions[?(@.type=="Healthy")].status}'
```

## Tracing (Optional)

Enable OpenTelemetry tracing:

```yaml
# values.yaml
tracing:
  enabled: true
  endpoint: "http://jaeger-collector:14268/api/traces"
  samplingRate: 0.1
```

## Resource Monitoring

### Watch Resource Changes

```bash
# Watch all DB Provision resources
kubectl get databaseinstances,databases,databaseusers -A -w
```

### Status Summary Script

```bash
#!/bin/bash
echo "=== DB Provision Operator Status ==="
echo ""
echo "Instances:"
kubectl get databaseinstances -A -o custom-columns=\
'NAMESPACE:.metadata.namespace,NAME:.metadata.name,ENGINE:.spec.engine,PHASE:.status.phase'
echo ""
echo "Databases:"
kubectl get databases -A -o custom-columns=\
'NAMESPACE:.metadata.namespace,NAME:.metadata.name,INSTANCE:.spec.instanceRef.name,PHASE:.status.phase'
echo ""
echo "Users:"
kubectl get databaseusers -A -o custom-columns=\
'NAMESPACE:.metadata.namespace,NAME:.metadata.name,USERNAME:.spec.username,PHASE:.status.phase'
echo ""
echo "Backups:"
kubectl get databasebackups -A -o custom-columns=\
'NAMESPACE:.metadata.namespace,NAME:.metadata.name,PHASE:.status.phase,SIZE:.status.backupSize'
```

## Best Practices

1. **Set up alerts** before going to production
2. **Monitor backup success** - set up alerts for backup failures
3. **Track reconciliation errors** - investigate spikes in error rate
4. **Dashboard review** - regularly review dashboards for anomalies
5. **Log retention** - keep logs for debugging production issues
