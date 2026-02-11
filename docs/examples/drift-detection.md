# Drift Detection Examples

Examples demonstrating drift detection and correction configurations.

## Basic Drift Detection

### Detect-Only Mode

Monitor for configuration drift without automatic correction:

```yaml
---
# Instance with detect mode as default
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: postgres-primary
spec:
  engine: postgres
  connection:
    host: postgres.database.svc.cluster.local
    port: 5432
    database: postgres
    sslMode: require
    secretRef:
      name: postgres-admin-credentials
  driftPolicy:
    mode: detect
    interval: "5m"
---
# Database inherits detect mode from instance
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: production-db
spec:
  instanceRef:
    name: postgres-primary
  name: production
  deletionProtection: true
```

**Check drift status:**

```bash
# View drift detection results
kubectl get database production-db -o jsonpath='{.status.drift}'

# Check for DriftDetected events
kubectl get events --field-selector reason=DriftDetected
```

### Auto-Correct Mode

Automatically fix configuration drift:

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: dev-db
spec:
  instanceRef:
    name: postgres-primary
  name: development
  driftPolicy:
    mode: correct
    interval: "2m"
```

**Monitor corrections:**

```bash
# Watch for correction events
kubectl get events --field-selector reason=DriftCorrected -w
```

## Environment-Based Policies

### Multi-Environment Setup

Different drift policies per environment:

```yaml
---
# Development: Auto-correct everything
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: dev-db
  labels:
    environment: development
  annotations:
    # Allow destructive corrections in dev
    dbops.dbprovision.io/allow-destructive-drift: "true"
spec:
  instanceRef:
    name: postgres-primary
  name: development
  driftPolicy:
    mode: correct
    interval: "1m"
---
# Staging: Auto-correct non-destructive only
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: staging-db
  labels:
    environment: staging
spec:
  instanceRef:
    name: postgres-primary
  name: staging
  deletionProtection: true
  driftPolicy:
    mode: correct
    interval: "5m"
---
# Production: Detect only, alert humans
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: production-db
  labels:
    environment: production
spec:
  instanceRef:
    name: postgres-primary
  name: production
  deletionProtection: true
  deletionPolicy: Snapshot
  driftPolicy:
    mode: detect
    interval: "5m"
```

## User and Role Drift Detection

### User Configuration Drift

```yaml
---
# User with drift detection
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: app-user
spec:
  instanceRef:
    name: postgres-primary
  username: app_user
  passwordSecret:
    generate: true
    secretName: app-user-credentials
  postgres:
    connectionLimit: 100
  driftPolicy:
    mode: detect
    interval: "10m"
```

**Simulating drift (for testing):**

```sql
-- Manually change connection limit
ALTER ROLE app_user WITH CONNECTION LIMIT 50;
```

**Viewing drift:**

```bash
kubectl get databaseuser app-user -o jsonpath='{.status.drift}' | jq
```

**Expected output:**

```json
{
  "detected": true,
  "lastChecked": "2024-01-15T10:30:00Z",
  "diffs": [
    {
      "field": "connectionLimit",
      "expected": "100",
      "actual": "50",
      "destructive": false,
      "immutable": false
    }
  ]
}
```

### Role Drift Detection

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseRole
metadata:
  name: readonly-role
spec:
  instanceRef:
    name: postgres-primary
  roleName: app_readonly
  postgres:
    login: false
    inherit: true
    inRoles:
      - pg_read_all_data
  driftPolicy:
    mode: correct
    interval: "5m"
```

## Grant Drift Detection

### Database Grants

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseGrant
metadata:
  name: app-grants
spec:
  userRef:
    name: app-user
  postgres:
    grants:
      - database: production
        schema: public
        tables: ["*"]
        privileges: [SELECT, INSERT, UPDATE, DELETE]
  driftPolicy:
    mode: detect
    interval: "15m"
```

**Check for privilege drift:**

```sql
-- View current grants
SELECT grantee, privilege_type, table_name
FROM information_schema.table_privileges
WHERE grantee = 'app_user';
```

## Resource Discovery

### Enable Discovery on Instance

Find unmanaged database resources:

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
    database: postgres
    secretRef:
      name: postgres-admin-credentials
  driftPolicy:
    mode: detect
    interval: "5m"
  discovery:
    enabled: true
    interval: "30m"
```

**View discovered resources:**

```bash
kubectl get databaseinstance postgres-primary -o jsonpath='{.status.discoveredResources}' | jq
```

**Expected output:**

```json
{
  "lastScan": "2024-01-15T10:00:00Z",
  "databases": [
    {
      "name": "legacy_app",
      "discovered": "2024-01-15T10:00:00Z",
      "adopted": false
    }
  ],
  "users": [
    {
      "name": "old_admin",
      "discovered": "2024-01-15T10:00:00Z",
      "adopted": false
    }
  ]
}
```

### Adopt Discovered Resources

Bring discovered resources under management:

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: postgres-primary
  annotations:
    # Adopt these discovered databases
    dbops.dbprovision.io/adopt-databases: "legacy_app"
    # Adopt these discovered users
    dbops.dbprovision.io/adopt-users: "old_admin"
spec:
  engine: postgres
  connection:
    host: postgres.database.svc.cluster.local
    port: 5432
    database: postgres
    secretRef:
      name: postgres-admin-credentials
  discovery:
    enabled: true
    interval: "30m"
```

**Verify adoption:**

```bash
# Check for newly created CRs
kubectl get databases -l dbops.dbprovision.io/adopted=true
kubectl get databaseusers -l dbops.dbprovision.io/adopted=true
```

## Alerting on Drift

### Prometheus Rules

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: drift-detection-alerts
spec:
  groups:
    - name: drift-detection
      rules:
        # Alert on any drift detected in production
        - alert: ProductionDriftDetected
          expr: |
            dbprovision_drift_detected{
              namespace="production"
            } == 1
          for: 5m
          labels:
            severity: warning
          annotations:
            summary: "Configuration drift detected in production"
            description: "{{ $labels.kind }}/{{ $labels.name }} has configuration drift"

        # Alert on drift correction failures
        - alert: DriftCorrectionFailed
          expr: |
            increase(kubernetes_events_total{
              reason="DriftCorrectionFailed"
            }[15m]) > 0
          for: 0m
          labels:
            severity: critical
          annotations:
            summary: "Drift correction failed"
            description: "Failed to correct drift for {{ $labels.name }}"
```

### Event-Based Alerting with event-exporter

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: event-exporter-config
data:
  config.yaml: |
    route:
      routes:
        - match:
            - reason: "DriftDetected"
            - reason: "DriftCorrectionFailed"
          receiver: "slack-alerts"
    receivers:
      - name: "slack-alerts"
        slack:
          endpoint: "https://hooks.slack.com/services/..."
          channel: "#database-alerts"
          message: |
            :warning: *{{ .Reason }}*
            Resource: {{ .InvolvedObject.Kind }}/{{ .InvolvedObject.Name }}
            Namespace: {{ .InvolvedObject.Namespace }}
            Message: {{ .Message }}
```

## Complete Drift Detection Setup

Full example with all components:

```yaml
---
# Instance with detection and discovery
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: postgres-primary
spec:
  engine: postgres
  connection:
    host: postgres.database.svc.cluster.local
    port: 5432
    database: postgres
    sslMode: require
    secretRef:
      name: postgres-admin
  healthCheck:
    enabled: true
    intervalSeconds: 30
  driftPolicy:
    mode: detect
    interval: "5m"
  discovery:
    enabled: true
    interval: "30m"
---
# Production database: detect only
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: production-db
spec:
  instanceRef:
    name: postgres-primary
  name: production
  deletionProtection: true
  deletionPolicy: Snapshot
  # Inherits detect mode from instance
---
# Development database: auto-correct
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
---
# Production user: detect drift
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: prod-app-user
spec:
  instanceRef:
    name: postgres-primary
  username: prod_app
  passwordSecret:
    generate: true
    secretName: prod-app-credentials
  postgres:
    connectionLimit: 100
  driftPolicy:
    mode: detect
    interval: "10m"
---
# Production grants: detect drift
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseGrant
metadata:
  name: prod-app-grants
spec:
  userRef:
    name: prod-app-user
  postgres:
    grants:
      - database: production
        schema: public
        tables: ["*"]
        privileges: [SELECT, INSERT, UPDATE, DELETE]
  driftPolicy:
    mode: detect
    interval: "15m"
```

## Verify Drift Detection

```bash
# Check all resources for drift
for resource in database databaseuser databaserole databasegrant; do
  echo "=== $resource ==="
  kubectl get $resource -o custom-columns='NAME:.metadata.name,DRIFT:.status.drift.detected,LAST_CHECK:.status.drift.lastChecked'
done

# View drift details
kubectl get database production-db -o jsonpath='{.status.drift.diffs}' | jq

# Watch for drift events
kubectl get events --field-selector reason=DriftDetected -w
```
