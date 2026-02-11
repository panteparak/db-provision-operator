# CockroachDB Examples

Complete examples for CockroachDB database provisioning.

## Development Setup (Insecure Mode)

### Step 1: Admin Credentials (Empty Password)

In insecure mode, CockroachDB doesn't support password authentication:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: cockroach-admin-credentials
type: Opaque
stringData:
  username: dbprovision_admin
  password: ""  # Must be empty for insecure mode
```

### Step 2: Database Instance (Insecure)

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: cockroach-cluster
spec:
  engine: cockroachdb
  connection:
    host: cockroachdb.database.svc.cluster.local
    port: 26257
    database: defaultdb
    sslMode: disable  # Required for insecure mode
    secretRef:
      name: cockroach-admin-credentials
  healthCheck:
    enabled: true
    intervalSeconds: 30
    timeoutSeconds: 5
```

### Step 3: Database

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: myapp-database
spec:
  instanceRef:
    name: cockroach-cluster
  name: myapp
  deletionPolicy: Retain
```

### Step 4: Application User (No Password in Insecure Mode)

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: myapp-user
spec:
  instanceRef:
    name: cockroach-cluster
  username: myapp_user
  # In insecure mode, user is created without password
  # Password secret will contain empty password
  passwordSecret:
    generate: false  # Cannot generate in insecure mode
    secretName: myapp-user-credentials
  postgres:
    connectionLimit: 50
```

## Production Setup (Secure Mode)

### Step 1: TLS Certificates

Create a secret with TLS certificates:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: cockroach-tls
type: kubernetes.io/tls
data:
  ca.crt: <base64-encoded-ca-certificate>
  tls.crt: <base64-encoded-client-certificate>
  tls.key: <base64-encoded-client-key>
```

### Step 2: Admin Credentials

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: cockroach-admin-credentials
type: Opaque
stringData:
  username: dbprovision_admin
  password: your-secure-admin-password
```

### Step 3: Database Instance (Secure)

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: cockroach-cluster
spec:
  engine: cockroachdb
  connection:
    host: cockroachdb.database.svc.cluster.local
    port: 26257
    database: defaultdb
    sslMode: verify-full
    secretRef:
      name: cockroach-admin-credentials
  tls:
    secretRef:
      name: cockroach-tls
      keys:
        ca: ca.crt
        cert: tls.crt
        key: tls.key
  healthCheck:
    enabled: true
    intervalSeconds: 30
```

### Step 4: Production Database

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: production-db
spec:
  instanceRef:
    name: cockroach-cluster
  name: production
  deletionProtection: true
  deletionPolicy: Retain
  driftPolicy:
    mode: detect
    interval: "5m"
```

### Step 5: Application User

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: app-user
spec:
  instanceRef:
    name: cockroach-cluster
  username: app_user
  passwordSecret:
    generate: true
    length: 32
    secretName: app-user-credentials
    secretTemplate:
      labels:
        app: myapp
      data:
        DATABASE_URL: "postgresql://{{ .Username }}:{{ .Password }}@{{ .Host }}:{{ .Port }}/production?sslmode=verify-full"
  postgres:
    connectionLimit: 100
```

## Role-Based Access

### Read-Only Role

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseRole
metadata:
  name: myapp-readonly
spec:
  instanceRef:
    name: cockroach-cluster
  roleName: myapp_readonly
  postgres:
    login: false
```

### Read-Write Role

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseRole
metadata:
  name: myapp-readwrite
spec:
  instanceRef:
    name: cockroach-cluster
  roleName: myapp_readwrite
  postgres:
    login: false
    inRoles: [myapp_readonly]
```

### Grants

```yaml
---
# Read-only grants
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseGrant
metadata:
  name: readonly-grants
spec:
  roleRef:
    name: myapp-readonly
  postgres:
    grants:
      - database: production
        privileges: [CONNECT]
      - database: production
        schema: public
        tables: ["*"]
        privileges: [SELECT]
---
# Read-write grants
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseGrant
metadata:
  name: readwrite-grants
spec:
  roleRef:
    name: myapp-readwrite
  postgres:
    grants:
      - database: production
        schema: public
        tables: ["*"]
        privileges: [INSERT, UPDATE, DELETE]
---
# Assign role to user
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseGrant
metadata:
  name: user-role-assignment
spec:
  userRef:
    name: app-user
  postgres:
    roles:
      - myapp_readwrite
```

## Backup Configuration

CockroachDB uses native `BACKUP` command instead of pg_dump.

### One-Time Backup to S3

```yaml
# S3 credentials
apiVersion: v1
kind: Secret
metadata:
  name: s3-backup-credentials
type: Opaque
stringData:
  AWS_ACCESS_KEY_ID: your-access-key
  AWS_SECRET_ACCESS_KEY: your-secret-key
---
# Backup
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseBackup
metadata:
  name: production-backup
spec:
  databaseRef:
    name: production-db
  storage:
    type: s3
    s3:
      bucket: my-cockroach-backups
      region: us-east-1
      prefix: cockroach/production
      secretRef:
        name: s3-backup-credentials
  ttl: "720h"  # 30 days
```

### Scheduled Backups

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseBackupSchedule
metadata:
  name: production-daily
spec:
  databaseRef:
    name: production-db
  schedule: "0 2 * * *"  # Daily at 2 AM UTC
  timezone: "UTC"
  retention:
    keepLast: 7
    keepDaily: 7
    keepWeekly: 4
  concurrencyPolicy: Forbid
  deletionProtection: true
  backupTemplate:
    storage:
      type: s3
      s3:
        bucket: my-cockroach-backups
        region: us-east-1
        prefix: cockroach/production/scheduled
        secretRef:
          name: s3-backup-credentials
```

### Restore from Backup

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseRestore
metadata:
  name: production-restore
spec:
  backupRef:
    name: production-backup
  targetDatabaseRef:
    name: production-restored
  createTarget: true
```

## Complete Application Stack

Full production-ready example:

```yaml
---
# TLS certificates (create separately)
apiVersion: v1
kind: Secret
metadata:
  name: cockroach-tls
type: kubernetes.io/tls
data:
  ca.crt: <base64-ca-cert>
  tls.crt: <base64-client-cert>
  tls.key: <base64-client-key>
---
# Admin credentials
apiVersion: v1
kind: Secret
metadata:
  name: cockroach-admin
type: Opaque
stringData:
  username: dbprovision_admin
  password: super-secret-admin-password
---
# S3 backup credentials
apiVersion: v1
kind: Secret
metadata:
  name: s3-credentials
type: Opaque
stringData:
  AWS_ACCESS_KEY_ID: your-access-key
  AWS_SECRET_ACCESS_KEY: your-secret-key
---
# Database instance
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: cockroach-cluster
spec:
  engine: cockroachdb
  connection:
    host: cockroachdb.database.svc.cluster.local
    port: 26257
    database: defaultdb
    sslMode: verify-full
    secretRef:
      name: cockroach-admin
  tls:
    secretRef:
      name: cockroach-tls
      keys:
        ca: ca.crt
        cert: tls.crt
        key: tls.key
  healthCheck:
    enabled: true
    intervalSeconds: 30
  driftPolicy:
    mode: detect
    interval: "5m"
---
# Application database
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: myapp-db
spec:
  instanceRef:
    name: cockroach-cluster
  name: myapp
  deletionProtection: true
  deletionPolicy: Retain
---
# Read-only role
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseRole
metadata:
  name: myapp-readonly
spec:
  instanceRef:
    name: cockroach-cluster
  roleName: myapp_readonly
  postgres:
    login: false
---
# Read-write role
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseRole
metadata:
  name: myapp-readwrite
spec:
  instanceRef:
    name: cockroach-cluster
  roleName: myapp_readwrite
  postgres:
    login: false
    inRoles: [myapp_readonly]
---
# Role grants
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseGrant
metadata:
  name: readonly-grants
spec:
  roleRef:
    name: myapp-readonly
  postgres:
    grants:
      - database: myapp
        privileges: [CONNECT]
      - database: myapp
        schema: public
        tables: ["*"]
        privileges: [SELECT]
---
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseGrant
metadata:
  name: readwrite-grants
spec:
  roleRef:
    name: myapp-readwrite
  postgres:
    grants:
      - database: myapp
        schema: public
        tables: ["*"]
        privileges: [INSERT, UPDATE, DELETE]
---
# Application user
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: myapp-user
spec:
  instanceRef:
    name: cockroach-cluster
  username: myapp_user
  passwordSecret:
    generate: true
    length: 32
    secretName: myapp-credentials
    secretTemplate:
      data:
        DATABASE_URL: "postgresql://{{ .Username }}:{{ .Password }}@{{ .Host }}:{{ .Port }}/myapp?sslmode=verify-full"
  postgres:
    connectionLimit: 100
---
# Assign roles to user
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseGrant
metadata:
  name: myapp-user-roles
spec:
  userRef:
    name: myapp-user
  postgres:
    roles:
      - myapp_readwrite
---
# Scheduled backups
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseBackupSchedule
metadata:
  name: myapp-daily-backup
spec:
  databaseRef:
    name: myapp-db
  schedule: "0 2 * * *"
  timezone: "UTC"
  retention:
    keepLast: 7
    keepDaily: 7
    keepWeekly: 4
    keepMonthly: 3
  deletionProtection: true
  backupTemplate:
    storage:
      type: s3
      s3:
        bucket: myapp-backups
        region: us-east-1
        prefix: cockroach/myapp
        secretRef:
          name: s3-credentials
```

## Verify Setup

```bash
# Check instance status
kubectl get databaseinstance cockroach-cluster

# Check all resources
kubectl get databases,databaseusers,databaseroles,databasegrants

# Get connection string
kubectl get secret myapp-credentials -o jsonpath='{.data.DATABASE_URL}' | base64 -d

# Test connection (using cockroach client)
kubectl run cockroach-test --rm -it --image=cockroachdb/cockroach:v24.1.0 -- \
  sql --url="$(kubectl get secret myapp-credentials -o jsonpath='{.data.DATABASE_URL}' | base64 -d)"

# Check CockroachDB jobs (for backups)
kubectl exec -it cockroachdb-0 -- cockroach sql --insecure -e "SHOW JOBS;"

# View backup status
kubectl get databasebackup,databasebackupschedule
```

## Troubleshooting

### Insecure Mode Connection Issues

```bash
# Verify instance uses sslMode: disable
kubectl get databaseinstance cockroach-cluster -o jsonpath='{.spec.connection.sslMode}'

# Verify empty password in secret
kubectl get secret cockroach-admin-credentials -o jsonpath='{.data.password}' | base64 -d
# Should return empty string

# Test connection manually
kubectl run cockroach-test --rm -it --image=cockroachdb/cockroach:v24.1.0 -- \
  sql --insecure --host=cockroachdb:26257 -e "SELECT 1;"
```

### Permission Issues

```bash
# Check if admin has required privileges
kubectl exec -it cockroachdb-0 -- cockroach sql --insecure -e "SHOW GRANTS FOR dbprovision_admin;"

# Grant missing privileges
kubectl exec -it cockroachdb-0 -- cockroach sql --insecure -e "
  ALTER USER dbprovision_admin WITH CREATEDB CREATEROLE;
  GRANT admin TO dbprovision_admin;
"
```

### Backup Issues

```bash
# Check backup job status in CockroachDB
kubectl exec -it cockroachdb-0 -- cockroach sql --insecure -e "
  SELECT * FROM [SHOW JOBS] WHERE job_type = 'BACKUP' ORDER BY created DESC LIMIT 5;
"

# Check for failed jobs
kubectl exec -it cockroachdb-0 -- cockroach sql --insecure -e "
  SELECT * FROM [SHOW JOBS] WHERE status = 'failed' AND job_type = 'BACKUP';
"
```
