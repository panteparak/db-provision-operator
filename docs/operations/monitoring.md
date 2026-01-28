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

### Metrics Namespace

!!! warning "Correct Metric Prefix"
    All metrics use the `dbops_` namespace prefix. Do not use `db_provision_` - that prefix is incorrect.

### Available Metrics (27 total)

All metrics use the `dbops_` namespace prefix.

#### Connection Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `dbops_connection_attempts_total` | Counter | instance, engine, status, namespace | Total connection attempts (status: success/failure) |
| `dbops_connection_latency_seconds` | Histogram | instance, engine, namespace | Connection latency distribution (buckets: 1ms to 10s) |

#### Instance Health Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `dbops_instance_healthy` | Gauge | instance, engine, namespace | Instance health (1=healthy, 0=unhealthy) |
| `dbops_instance_last_health_check_timestamp_seconds` | Gauge | instance, engine, namespace | Unix timestamp of last health check |

#### Database Operation Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `dbops_database_operations_total` | Counter | operation, engine, status, namespace | Total database operations (create/update/delete) |
| `dbops_database_operation_duration_seconds` | Histogram | operation, engine, namespace | Operation duration distribution |
| `dbops_database_size_bytes` | Gauge | database, instance, engine, namespace | Database size in bytes |

#### User/Role/Grant Operation Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `dbops_user_operations_total` | Counter | operation, engine, status, namespace | Total user operations |
| `dbops_role_operations_total` | Counter | operation, engine, status, namespace | Total role operations |
| `dbops_grant_operations_total` | Counter | operation, engine, status, namespace | Total grant operations |

#### Backup Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `dbops_backup_operations_total` | Counter | engine, status, namespace | Total backup operations |
| `dbops_backup_duration_seconds` | Histogram | engine, namespace | Backup duration (buckets: 1s to 1h) |
| `dbops_backup_size_bytes` | Gauge | database, engine, namespace | Backup size in bytes |
| `dbops_backup_last_success_timestamp_seconds` | Gauge | database, engine, namespace | Last successful backup Unix timestamp |

#### Restore Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `dbops_restore_operations_total` | Counter | engine, status, namespace | Total restore operations |
| `dbops_restore_duration_seconds` | Histogram | engine, namespace | Restore duration distribution |

#### Schedule Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `dbops_scheduled_backups_total` | Counter | status, namespace | Total scheduled backup triggers |
| `dbops_schedule_next_backup_timestamp_seconds` | Gauge | database, namespace | Next scheduled backup Unix timestamp |

#### Resource Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `dbops_resource_count` | Gauge | resource_type, phase, namespace | Count of resources by type and phase |

#### Info Metrics (for Grafana Tables)

Info metrics expose resource metadata as labels. The value is always `1`, following the kube-state-metrics pattern.

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `dbops_instance_info` | Gauge | instance, namespace, engine, version, host, port, phase | Instance metadata |
| `dbops_database_info` | Gauge | database, namespace, instance_ref, db_name, phase | Database metadata |
| `dbops_user_info` | Gauge | user, namespace, instance_ref, username, phase | User metadata |
| `dbops_role_info` | Gauge | role, namespace, instance_ref, role_name, phase | Role metadata |
| `dbops_grant_info` | Gauge | grant, namespace, user_ref, database_ref, privileges, phase | Grant metadata |
| `dbops_backup_info` | Gauge | backup, namespace, database_ref, storage_type, phase | Backup metadata |
| `dbops_schedule_info` | Gauge | schedule, namespace, database_ref, cron, paused | Schedule metadata |
| `dbops_restore_info` | Gauge | restore, namespace, backup_ref, target_instance, phase | Restore metadata |

### Label Values

| Label | Possible Values |
|-------|-----------------|
| `status` | `success`, `failure` |
| `operation` | `create`, `update`, `delete`, `connect`, `backup`, `restore` |
| `engine` | `postgres`, `mysql`, `mariadb` |
| `phase` | `Pending`, `Ready`, `Failed`, `Deleting` |

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

The operator provides **9 pre-built Grafana dashboards** in the `dashboards/` directory.

### Dashboard Overview

| Dashboard | File | Purpose |
|-----------|------|---------|
| Overview | `overview.json` | High-level summary of all resources |
| Instances | `instances.json` | Database instance health and connections |
| Databases | `databases.json` | Database operations and sizes |
| Users | `users.json` | User management operations |
| Roles | `roles.json` | Role management operations |
| Grants | `grants.json` | Grant operations and permissions |
| Backups | `backups.json` | Backup operations, sizes, and success rates |
| Schedules | `schedules.json` | Backup schedule status and timing |
| Restores | `restores.json` | Restore operations and durations |

### Dashboard Details

#### 1. Overview Dashboard (`overview.json`)

The main entry point showing:

- **Resource Summary**: Count of instances, databases, users, roles, grants
- **Health Status**: Healthy vs unhealthy instances
- **Operation Rates**: Create/update/delete operations per minute
- **Error Rates**: Failed operations across all resource types
- **Recent Activity**: Timeline of resource changes

**Key metrics used:**
- `dbops_resource_count`
- `dbops_instance_healthy`
- `dbops_*_operations_total`

#### 2. Instances Dashboard (`instances.json`)

Detailed instance monitoring:

- **Instance Table**: List all instances with engine, host, port, phase
- **Connection Status**: Health status per instance
- **Connection Latency**: P50, P90, P99 latency percentiles
- **Connection Attempts**: Success/failure rates over time
- **Health Check History**: Timeline of health check results

**Key metrics used:**
- `dbops_instance_info`
- `dbops_instance_healthy`
- `dbops_connection_latency_seconds`
- `dbops_connection_attempts_total`

#### 3. Databases Dashboard (`databases.json`)

Database operations and sizes:

- **Database Table**: List databases with instance, name, phase
- **Database Sizes**: Size per database over time
- **Operation Counts**: Create/update/delete operations
- **Operation Duration**: P50, P90, P99 duration percentiles

**Key metrics used:**
- `dbops_database_info`
- `dbops_database_size_bytes`
- `dbops_database_operations_total`
- `dbops_database_operation_duration_seconds`

#### 4. Users Dashboard (`users.json`)

User management:

- **User Table**: List users with instance, username, phase
- **Operation Counts**: User create/update/delete operations
- **Operation Status**: Success vs failure rates

**Key metrics used:**
- `dbops_user_info`
- `dbops_user_operations_total`

#### 5. Roles Dashboard (`roles.json`)

Role management:

- **Role Table**: List roles with instance, role name, phase
- **Operation Counts**: Role operations over time

**Key metrics used:**
- `dbops_role_info`
- `dbops_role_operations_total`

#### 6. Grants Dashboard (`grants.json`)

Permission grants:

- **Grant Table**: List grants with user, database, privileges, phase
- **Operation Counts**: Grant operations over time

**Key metrics used:**
- `dbops_grant_info`
- `dbops_grant_operations_total`

#### 7. Backups Dashboard (`backups.json`)

Backup monitoring:

- **Backup Table**: List backups with database, storage type, phase
- **Backup Success Rate**: Success vs failure over time
- **Backup Sizes**: Size per backup
- **Backup Duration**: Duration histogram
- **Last Success Age**: Time since last successful backup

**Key metrics used:**
- `dbops_backup_info`
- `dbops_backup_operations_total`
- `dbops_backup_size_bytes`
- `dbops_backup_duration_seconds`
- `dbops_backup_last_success_timestamp_seconds`

#### 8. Schedules Dashboard (`schedules.json`)

Backup scheduling:

- **Schedule Table**: List schedules with database, cron, paused status
- **Scheduled Backup Counts**: Triggered backups over time
- **Next Backup Times**: Upcoming scheduled backups
- **Schedule Status**: Active vs paused schedules

**Key metrics used:**
- `dbops_schedule_info`
- `dbops_scheduled_backups_total`
- `dbops_schedule_next_backup_timestamp_seconds`

#### 9. Restores Dashboard (`restores.json`)

Restore operations:

- **Restore Table**: List restores with backup, target instance, phase
- **Restore Counts**: Operations over time
- **Restore Duration**: Duration histogram

**Key metrics used:**
- `dbops_restore_info`
- `dbops_restore_operations_total`
- `dbops_restore_duration_seconds`

### Installing Dashboards

#### Via Helm (Recommended)

```yaml
# values.yaml
grafana:
  dashboards:
    enabled: true
    provider:
      name: db-provision-operator
      folder: DB Provision
```

#### Manual Import (All Dashboards)

```bash
# Import all dashboards
for dashboard in dashboards/*.json; do
  curl -X POST http://admin:admin@localhost:3000/api/dashboards/db/import \
    -H "Content-Type: application/json" \
    -d "{\"dashboard\": $(cat $dashboard), \"overwrite\": true}"
done
```

#### Via ConfigMap (Kubernetes)

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: grafana-dashboards-dbops
  labels:
    grafana_dashboard: "1"
data:
  overview.json: |
    <dashboard JSON content>
```

### Custom Queries

**Instance health status:**
```promql
dbops_instance_healthy{engine="postgres"}
```

**Connection error rate:**
```promql
rate(dbops_connection_attempts_total{status="failure"}[5m])
/ rate(dbops_connection_attempts_total[5m])
```

**Database operations by type:**
```promql
sum by (operation, status) (rate(dbops_database_operations_total[5m]))
```

**Backup age (seconds since last success):**
```promql
time() - dbops_backup_last_success_timestamp_seconds
```

**Resource counts by phase:**
```promql
sum by (resource_type, phase) (dbops_resource_count)
```

**Connection latency P99:**
```promql
histogram_quantile(0.99, rate(dbops_connection_latency_seconds_bucket[5m]))
```

**Backup duration P95:**
```promql
histogram_quantile(0.95, rate(dbops_backup_duration_seconds_bucket[5m]))
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
          expr: dbops_instance_healthy == 0
          for: 5m
          labels:
            severity: critical
          annotations:
            summary: "Database instance {{ $labels.instance }} is unhealthy"
            description: "Instance {{ $labels.instance }} ({{ $labels.engine }}) has been unhealthy for more than 5 minutes"

        # High connection error rate
        - alert: HighConnectionErrorRate
          expr: |
            rate(dbops_connection_attempts_total{status="failure"}[5m])
            / rate(dbops_connection_attempts_total[5m]) > 0.1
          for: 10m
          labels:
            severity: warning
          annotations:
            summary: "High connection error rate for {{ $labels.instance }}"
            description: "More than 10% of connection attempts are failing"

        # High operation error rate
        - alert: HighDatabaseOperationErrorRate
          expr: |
            rate(dbops_database_operations_total{status="failure"}[5m])
            / rate(dbops_database_operations_total[5m]) > 0.1
          for: 10m
          labels:
            severity: warning
          annotations:
            summary: "High database operation error rate"
            description: "More than 10% of database operations are failing"

        # Backup alerts
        - alert: BackupFailed
          expr: |
            increase(dbops_backup_operations_total{status="failure"}[1h]) > 0
          for: 1m
          labels:
            severity: warning
          annotations:
            summary: "Backup failed for {{ $labels.engine }}"
            description: "A backup operation failed in the last hour"

        - alert: BackupOverdue
          expr: |
            time() - dbops_backup_last_success_timestamp_seconds > 86400
          for: 1h
          labels:
            severity: warning
          annotations:
            summary: "Backup overdue for {{ $labels.database }}"
            description: "No successful backup in the last 24 hours"

        # Resource stuck in pending
        - alert: ResourceStuckPending
          expr: |
            dbops_resource_count{phase="Pending"} > 0
          for: 30m
          labels:
            severity: warning
          annotations:
            summary: "Resources stuck in Pending state"
            description: "{{ $value }} {{ $labels.resource_type }} resources have been Pending for more than 30 minutes"

        # Connection latency
        - alert: HighConnectionLatency
          expr: |
            histogram_quantile(0.95, rate(dbops_connection_latency_seconds_bucket[5m])) > 1
          for: 10m
          labels:
            severity: warning
          annotations:
            summary: "High connection latency for {{ $labels.instance }}"
            description: "P95 connection latency is above 1 second"

        # Restore failures
        - alert: RestoreFailed
          expr: |
            increase(dbops_restore_operations_total{status="failure"}[1h]) > 0
          for: 1m
          labels:
            severity: critical
          annotations:
            summary: "Database restore failed"
            description: "A restore operation failed in the last hour"
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
