# Advanced Examples

Production patterns and advanced configurations.

## Cross-Namespace Access

### Scenario

Database instance in `database` namespace, applications in `app-team-a` and `app-team-b` namespaces.

### Database Namespace Setup

```yaml
# database/postgres-instance.yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: shared-postgres
  namespace: database
spec:
  engine: postgres
  connection:
    host: postgres.database.svc.cluster.local
    port: 5432
    database: postgres
    sslMode: require
    secretRef:
      name: postgres-admin-credentials
```

### Application Namespace Resources

```yaml
# app-team-a/database.yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: team-a-db
  namespace: app-team-a
spec:
  instanceRef:
    name: shared-postgres
    namespace: database  # Cross-namespace reference
  name: team_a
  postgres:
    encoding: UTF8
---
# app-team-a/user.yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: team-a-user
  namespace: app-team-a
spec:
  instanceRef:
    name: shared-postgres
    namespace: database
  username: team_a_user
  passwordSecret:
    generate: true
    secretName: team-a-db-credentials
```

### RBAC Configuration

```yaml
# Allow app-team-a to reference database namespace resources
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: database-reader
  namespace: database
rules:
  - apiGroups: ["dbops.dbprovision.io"]
    resources: ["databaseinstances"]
    verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: app-team-a-database-reader
  namespace: database
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: database-reader
subjects:
  - kind: ServiceAccount
    name: default
    namespace: app-team-a
```

## TLS Connections

### PostgreSQL with TLS

```yaml
# TLS certificates
apiVersion: v1
kind: Secret
metadata:
  name: postgres-tls
type: kubernetes.io/tls
data:
  ca.crt: <base64-encoded-ca-cert>
  tls.crt: <base64-encoded-client-cert>
  tls.key: <base64-encoded-client-key>
---
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: postgres-tls
spec:
  engine: postgres
  connection:
    host: postgres.database.svc.cluster.local
    port: 5432
    database: postgres
    sslMode: verify-full
    secretRef:
      name: postgres-admin-credentials
    tls:
      secretRef:
        name: postgres-tls
        keys:
          ca: ca.crt
          cert: tls.crt
          key: tls.key
```

### MySQL with TLS

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: mysql-tls
type: kubernetes.io/tls
data:
  ca.crt: <base64-encoded-ca-cert>
  tls.crt: <base64-encoded-client-cert>
  tls.key: <base64-encoded-client-key>
---
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: mysql-tls
spec:
  engine: mysql
  connection:
    host: mysql.database.svc.cluster.local
    port: 3306
    database: mysql
    secretRef:
      name: mysql-admin-credentials
    tls:
      enabled: true
      secretRef:
        name: mysql-tls
        keys:
          ca: ca.crt
          cert: tls.crt
          key: tls.key
```

## Secret Template Functions

Real-world examples of `secretTemplate.data` with custom template functions.

### PostgreSQL Connection String

```yaml
secretTemplate:
  data:
    DATABASE_URL: "postgresql://{{ urlEncode .Username }}:{{ urlEncode .Password }}@{{ .Host }}:{{ .Port }}/{{ .Database }}?sslmode={{ default \"prefer\" .SSLMode }}"
```

### MySQL DSN for Go

```yaml
secretTemplate:
  data:
    DSN: "{{ urlEncode .Username }}:{{ urlEncode .Password }}@tcp({{ .Host }}:{{ .Port }})/{{ .Database }}?tls=required&parseTime=true"
```

### Spring Boot

```yaml
secretTemplate:
  data:
    SPRING_DATASOURCE_URL: "jdbc:postgresql://{{ .Host }}:{{ .Port }}/{{ .Database }}?ssl=true&sslmode={{ .SSLMode }}"
    SPRING_DATASOURCE_USERNAME: "{{ .Username }}"
    SPRING_DATASOURCE_PASSWORD: "{{ .Password }}"
```

### .env File Format

```yaml
secretTemplate:
  data:
    .env: |
      DB_HOST={{ .Host }}
      DB_PORT={{ .Port }}
      DB_USER={{ .Username }}
      DB_PASS={{ .Password }}
      DB_NAME={{ .Database }}
      DB_SSLMODE={{ default "disable" .SSLMode }}
```

### ClickHouse DSN

```yaml
secretTemplate:
  data:
    DSN: "clickhouse://{{ urlEncode .Username }}:{{ urlEncode .Password }}@{{ .Host }}:{{ .Port }}/{{ .Database }}?secure=true"
```

## mTLS with cert-manager

End-to-end example: cert-manager issues certificates, operator distributes them to app secrets.

```yaml
# Step 0: Bootstrap a self-signed CA (dev/staging — use corporate PKI in production)
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: selfsigned
spec:
  selfSigned: {}
---
# Step 0b: Create the CA certificate
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: database-ca
spec:
  isCA: true
  secretName: database-ca-keypair
  commonName: "Database CA"
  duration: 87600h      # 10 years
  privateKey:
    algorithm: ECDSA
    size: 256
  issuerRef:
    name: selfsigned
    kind: ClusterIssuer
---
# Step 1: CA Issuer that signs client/server certs
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: database-ca
spec:
  ca:
    secretName: database-ca-keypair
---
# Step 2: Client certificate for operator
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: postgres-client-cert
spec:
  secretName: postgres-tls-certs
  issuerRef:
    name: database-ca
    kind: Issuer
  commonName: db-provision-operator
  usages:
    - client auth
  duration: 8760h
  renewBefore: 720h
---
# Step 3: DatabaseInstance references cert-manager's output secret
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: postgres-mtls
spec:
  engine: postgres
  connection:
    host: postgres.database.svc.cluster.local
    port: 5432
    secretRef:
      name: postgres-admin-credentials
  tls:
    enabled: true
    mode: verify-full
    secretRef:
      name: postgres-tls-certs
---
# Step 4: DatabaseUser distributes certs via template
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: app-user
spec:
  instanceRef:
    name: postgres-mtls
  username: app_user
  passwordSecret:
    generate: true
    secretName: app-db-credentials
    secretTemplate:
      data:
        DATABASE_URL: "postgresql://{{ urlEncode .Username }}:{{ urlEncode .Password }}@{{ .Host }}:{{ .Port }}/{{ .Database }}?sslmode=verify-full"
        ca.crt: "{{ .CA }}"
        tls.crt: "{{ .TLSCert }}"
        tls.key: "{{ .TLSKey }}"
---
# Step 5: App Deployment mounting the credential secret
apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
spec:
  template:
    spec:
      containers:
        - name: app
          env:
            - name: DATABASE_URL
              valueFrom:
                secretKeyRef:
                  name: app-db-credentials
                  key: DATABASE_URL
          volumeMounts:
            - name: db-certs
              mountPath: /certs
              readOnly: true
      volumes:
        - name: db-certs
          secret:
            secretName: app-db-credentials
            items:
              - key: ca.crt
                path: ca.crt
              - key: tls.crt
                path: tls.crt
              - key: tls.key
                path: tls.key
```

## Cloud SQL with Pre-Provisioned Certs

```yaml
# Step 1: Cloud SQL CA cert in a K8s secret
apiVersion: v1
kind: Secret
metadata:
  name: cloudsql-ca
type: Opaque
stringData:
  ca.crt: |
    -----BEGIN CERTIFICATE-----
    ... Cloud SQL server CA cert from GCP console ...
    -----END CERTIFICATE-----
---
# Step 2: DatabaseInstance with TLS (server verification only)
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: cloudsql-postgres
spec:
  engine: postgres
  connection:
    host: 10.0.0.5
    port: 5432
    secretRef:
      name: cloudsql-admin-credentials
  tls:
    enabled: true
    mode: verify-ca
    secretRef:
      name: cloudsql-ca
      keys:
        ca: ca.crt
---
# Step 3: DatabaseUser with CA in template
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: api-user
spec:
  instanceRef:
    name: cloudsql-postgres
  username: api_service
  passwordSecret:
    generate: true
    secretName: api-db-credentials
    secretTemplate:
      data:
        DATABASE_URL: "postgresql://{{ urlEncode .Username }}:{{ urlEncode .Password }}@{{ .Host }}:{{ .Port }}/{{ .Database }}?sslmode=verify-ca&sslrootcert=/certs/ca.crt"
        ca.crt: "{{ .CA }}"
```

## Cloud Backups

### AWS S3

```yaml
# S3 credentials
apiVersion: v1
kind: Secret
metadata:
  name: s3-credentials
type: Opaque
stringData:
  AWS_ACCESS_KEY_ID: AKIAIOSFODNN7EXAMPLE
  AWS_SECRET_ACCESS_KEY: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
---
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseBackupSchedule
metadata:
  name: production-backup
spec:
  databaseRef:
    name: production-db
  schedule: "0 */6 * * *"  # Every 6 hours
  timezone: "UTC"
  retention:
    keepLast: 10
    keepDaily: 7
    keepWeekly: 4
    keepMonthly: 6
  backupTemplate:
    storage:
      type: s3
      s3:
        bucket: my-company-db-backups
        region: us-east-1
        prefix: production/postgres
        secretRef:
          name: s3-credentials
    compression:
      enabled: true
      algorithm: zstd
    encryption:
      enabled: true
      algorithm: aes-256-gcm
      secretRef:
        name: backup-encryption-key
```

### Google Cloud Storage

```yaml
# GCS credentials
apiVersion: v1
kind: Secret
metadata:
  name: gcs-credentials
type: Opaque
stringData:
  credentials.json: |
    {
      "type": "service_account",
      "project_id": "my-project",
      ...
    }
---
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseBackupSchedule
metadata:
  name: production-backup-gcs
spec:
  databaseRef:
    name: production-db
  schedule: "0 2 * * *"
  retention:
    keepLast: 7
  backupTemplate:
    storage:
      type: gcs
      gcs:
        bucket: my-company-db-backups
        prefix: production/postgres
        secretRef:
          name: gcs-credentials
          key: credentials.json
    compression:
      enabled: true
```

### Azure Blob Storage

```yaml
# Azure credentials
apiVersion: v1
kind: Secret
metadata:
  name: azure-credentials
type: Opaque
stringData:
  AZURE_STORAGE_ACCOUNT: mystorageaccount
  AZURE_STORAGE_KEY: <storage-account-key>
---
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseBackupSchedule
metadata:
  name: production-backup-azure
spec:
  databaseRef:
    name: production-db
  schedule: "0 2 * * *"
  retention:
    keepLast: 7
  backupTemplate:
    storage:
      type: azure
      azure:
        container: db-backups
        prefix: production/postgres
        secretRef:
          name: azure-credentials
          keys:
            accountName: AZURE_STORAGE_ACCOUNT
            accountKey: AZURE_STORAGE_KEY
    compression:
      enabled: true
```

## Force Delete with Cascade Confirmation

End-to-end example of force-deleting a DatabaseInstance that has child resources.

### Setup: Instance with Children

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: staging-postgres
spec:
  engine: postgres
  connection:
    host: postgres.staging.svc.cluster.local
    port: 5432
    secretRef:
      name: staging-admin-credentials
  deletionProtection: true
---
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: staging-app-db
spec:
  instanceRef:
    name: staging-postgres
  name: staging_app
  deletionPolicy: Delete
---
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: staging-app-user
spec:
  instanceRef:
    name: staging-postgres
  username: staging_app_user
  passwordSecret:
    generate: true
    secretName: staging-app-credentials
```

### Force Delete the Instance

```bash
# 1. Trigger force-delete (bypasses deletionProtection)
kubectl annotate databaseinstance staging-postgres \
  dbops.dbprovision.io/force-delete="true"

# 2. Read the confirmation hash from status
HASH=$(kubectl get databaseinstance staging-postgres \
  -o jsonpath='{.status.deletionConfirmation.hash}')
echo "Confirmation hash: $HASH"

# 3. Review what will be deleted
kubectl get databaseinstance staging-postgres \
  -o jsonpath='{.status.deletionConfirmation.children}' | jq .
# ["Database/staging-app-db", "DatabaseUser/staging-app-user"]

# 4. Confirm the cascade
kubectl annotate databaseinstance staging-postgres \
  dbops.dbprovision.io/confirm-force-delete="$HASH"

# 5. Watch cascade progress
kubectl get databaseinstance staging-postgres -w
# PHASE              REMAINING
# PendingDeletion    2
# PendingDeletion    1
# PendingDeletion    0
# (resource deleted)
```

Each child is deleted according to its own `deletionPolicy`:
- `staging-app-db` has `deletionPolicy: Delete` — the actual database is dropped
- `staging-app-user` uses the default `Retain` — the database user is kept

See [Deletion Protection: Cascade Confirmation](../user-guide/deletion-protection.md#force-delete-with-children-cascade-confirmation) for the full reference.

## Multi-Tenant Setup

### Tenant Isolation Pattern

```yaml
# Shared database instance
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: multi-tenant-postgres
  namespace: database
spec:
  engine: postgres
  connection:
    host: postgres.database.svc.cluster.local
    port: 5432
    secretRef:
      name: postgres-admin
---
# Per-tenant database
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: tenant-acme
  namespace: database
spec:
  instanceRef:
    name: multi-tenant-postgres
  name: tenant_acme
  deletionProtection: true
  postgres:
    encoding: UTF8
    schemas:
      - name: app
---
# Per-tenant user
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: tenant-acme-user
  namespace: database
spec:
  instanceRef:
    name: multi-tenant-postgres
  username: tenant_acme_user
  passwordSecret:
    generate: true
    secretName: tenant-acme-credentials
  postgres:
    connectionLimit: 20
---
# Per-tenant grants (isolated to their database)
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseGrant
metadata:
  name: tenant-acme-grants
  namespace: database
spec:
  userRef:
    name: tenant-acme-user
  postgres:
    grants:
      - database: tenant_acme
        schema: app
        tables: ["*"]
        privileges: [SELECT, INSERT, UPDATE, DELETE]
      - database: tenant_acme
        schema: app
        sequences: ["*"]
        privileges: [USAGE, SELECT, UPDATE]
```

## High Availability

### Read Replica Configuration

```yaml
# Primary instance
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: postgres-primary
spec:
  engine: postgres
  connection:
    host: postgres-primary.database.svc.cluster.local
    port: 5432
    secretRef:
      name: postgres-primary-credentials
---
# Read replica instance
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: postgres-replica
spec:
  engine: postgres
  connection:
    host: postgres-replica.database.svc.cluster.local
    port: 5432
    secretRef:
      name: postgres-replica-credentials
---
# Read-only user on replica
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: reporting-user
spec:
  instanceRef:
    name: postgres-replica  # Points to replica
  username: reporting
  passwordSecret:
    generate: true
    secretName: reporting-credentials
    secretTemplate:
      data:
        DATABASE_URL: "postgresql://{{ .Username }}:{{ .Password }}@{{ .Host }}:{{ .Port }}/myapp?sslmode=require"
```

## Disaster Recovery

### Point-in-Time Recovery Setup

```yaml
# Frequent backups for minimal data loss
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseBackupSchedule
metadata:
  name: pitr-backup
spec:
  databaseRef:
    name: production-db
  schedule: "*/15 * * * *"  # Every 15 minutes
  retention:
    keepLast: 96  # 24 hours of 15-minute backups
    keepDaily: 7
  backupTemplate:
    storage:
      type: s3
      s3:
        bucket: pitr-backups
        prefix: production
        secretRef:
          name: s3-credentials
    postgres:
      format: custom
      jobs: 4
---
# Daily full backups
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseBackupSchedule
metadata:
  name: daily-full-backup
spec:
  databaseRef:
    name: production-db
  schedule: "0 0 * * *"  # Midnight
  retention:
    keepLast: 30
    keepMonthly: 12
  backupTemplate:
    storage:
      type: s3
      s3:
        bucket: full-backups
        prefix: production
        secretRef:
          name: s3-credentials
    compression:
      enabled: true
      algorithm: zstd
    encryption:
      enabled: true
      secretRef:
        name: backup-encryption-key
```

## Migration Patterns

### Database Migration from External Source

```yaml
# Step 1: Create instance pointing to external database
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: external-postgres
spec:
  engine: postgres
  connection:
    host: external-db.example.com
    port: 5432
    secretRef:
      name: external-db-credentials
---
# Step 2: Backup from external
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseBackup
metadata:
  name: migration-backup
spec:
  databaseRef:
    name: external-database  # References external
  storage:
    type: s3
    s3:
      bucket: migration-backups
      secretRef:
        name: s3-credentials
---
# Step 3: Restore to internal
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseRestore
metadata:
  name: migration-restore
spec:
  backupRef:
    name: migration-backup
  targetDatabaseRef:
    name: internal-database  # New internal database
  createTarget: true
  postgres:
    noOwner: true
    analyze: true
```

## Monitoring Integration

### Prometheus ServiceMonitor

```yaml
# Enable metrics in operator deployment
# Then create ServiceMonitor
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: db-provision-operator
  labels:
    app: db-provision-operator
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: db-provision-operator
  endpoints:
    - port: metrics
      interval: 30s
```

### Custom Metrics User

```yaml
# User for Prometheus postgres_exporter
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: prometheus-exporter
spec:
  instanceRef:
    name: postgres-primary
  username: prometheus
  passwordSecret:
    generate: true
    secretName: prometheus-exporter-credentials
---
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseGrant
metadata:
  name: prometheus-grants
spec:
  userRef:
    name: prometheus-exporter
  postgres:
    grants:
      - database: postgres
        schema: pg_catalog
        functions: [pg_stat_statements]
        privileges: [EXECUTE]
```
