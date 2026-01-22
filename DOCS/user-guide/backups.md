# Backups and Restores

DB Provision Operator supports automated backups with multiple storage backends.

## Resources

| Resource | Description |
|----------|-------------|
| `DatabaseBackup` | One-time backup operation |
| `DatabaseBackupSchedule` | Scheduled backups with retention |
| `DatabaseRestore` | Restore from a backup |

## DatabaseBackup

### Overview

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseBackup
metadata:
  name: myapp-backup
spec:
  databaseRef:
    name: myapp-database
  storage:
    type: s3
    s3:
      bucket: my-backups
      region: us-east-1
      secretRef:
        name: s3-credentials
```

### Storage Types

#### S3 (AWS/MinIO)

```yaml
storage:
  type: s3
  s3:
    bucket: my-backups
    region: us-east-1
    prefix: db-backups/postgres
    endpoint: ""  # Optional: for MinIO
    secretRef:
      name: s3-credentials
      keys:
        accessKey: AWS_ACCESS_KEY_ID
        secretKey: AWS_SECRET_ACCESS_KEY
```

#### Google Cloud Storage (GCS)

```yaml
storage:
  type: gcs
  gcs:
    bucket: my-backups
    prefix: db-backups/postgres
    secretRef:
      name: gcs-credentials
      key: credentials.json
```

#### Azure Blob Storage

```yaml
storage:
  type: azure
  azure:
    container: my-backups
    prefix: db-backups/postgres
    secretRef:
      name: azure-credentials
      keys:
        accountName: AZURE_STORAGE_ACCOUNT
        accountKey: AZURE_STORAGE_KEY
```

#### PersistentVolumeClaim (PVC)

```yaml
storage:
  type: pvc
  pvc:
    claimName: backup-storage
    subPath: postgres-backups
```

### Compression

```yaml
compression:
  enabled: true
  algorithm: gzip  # gzip, lz4, zstd
  level: 6         # 1-9 for gzip
```

### Encryption

```yaml
encryption:
  enabled: true
  algorithm: aes-256-gcm
  secretRef:
    name: backup-encryption-key
    key: encryption-key
```

### PostgreSQL Options

```yaml
postgres:
  method: pg_dump
  format: custom  # plain, custom, directory, tar
  jobs: 4         # Parallel jobs (directory format)
  blobs: true     # Include large objects
  noOwner: false
  noPrivileges: false
  schemas: []     # Specific schemas (empty = all)
  excludeSchemas: [pg_catalog, information_schema]
```

### MySQL Options

```yaml
mysql:
  method: mysqldump
  singleTransaction: true
  routines: true
  triggers: true
  events: false
```

## DatabaseBackupSchedule

### Overview

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseBackupSchedule
metadata:
  name: daily-backup
spec:
  databaseRef:
    name: myapp-database
  schedule: "0 2 * * *"  # Daily at 2 AM
  retention:
    keepLast: 7
  backupTemplate:
    storage:
      type: s3
      s3:
        bucket: my-backups
        secretRef:
          name: s3-credentials
```

### Schedule Format

Standard cron format: `minute hour day-of-month month day-of-week`

| Example | Description |
|---------|-------------|
| `0 2 * * *` | Daily at 2:00 AM |
| `0 */6 * * *` | Every 6 hours |
| `0 2 * * 0` | Weekly on Sunday at 2:00 AM |
| `0 2 1 * *` | Monthly on the 1st at 2:00 AM |

### Retention Policy

```yaml
retention:
  keepLast: 7        # Keep last 7 backups
  keepDaily: 7       # Keep daily backups for 7 days
  keepWeekly: 4      # Keep weekly backups for 4 weeks
  keepMonthly: 3     # Keep monthly backups for 3 months
```

### Concurrency Policy

```yaml
concurrencyPolicy: Forbid  # Allow, Forbid, Replace
```

| Policy | Description |
|--------|-------------|
| `Allow` | Run concurrent backups |
| `Forbid` | Skip if previous backup is running |
| `Replace` | Cancel running backup and start new |

## DatabaseRestore

### Overview

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseRestore
metadata:
  name: myapp-restore
spec:
  backupRef:
    name: myapp-backup
  targetDatabaseRef:
    name: myapp-database-restored
```

### Restore Options

```yaml
spec:
  backupRef:
    name: myapp-backup
  targetDatabaseRef:
    name: myapp-database-restored
  createTarget: true  # Create database if not exists
  postgres:
    dropExisting: false
    dataOnly: false
    schemaOnly: false
    noOwner: true
    jobs: 4
    disableTriggers: true
    analyze: true  # Run ANALYZE after restore
```

## Complete Example

### Daily Backup to S3 with Encryption

```yaml
# Storage credentials
apiVersion: v1
kind: Secret
metadata:
  name: s3-backup-credentials
type: Opaque
stringData:
  AWS_ACCESS_KEY_ID: your-access-key
  AWS_SECRET_ACCESS_KEY: your-secret-key
---
# Encryption key
apiVersion: v1
kind: Secret
metadata:
  name: backup-encryption-key
type: Opaque
stringData:
  encryption-key: your-32-character-encryption-key
---
# Backup schedule
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseBackupSchedule
metadata:
  name: myapp-daily-backup
spec:
  databaseRef:
    name: myapp-database
  schedule: "0 2 * * *"
  timezone: "UTC"
  retention:
    keepLast: 7
    keepDaily: 7
    keepWeekly: 4
  backupTemplate:
    storage:
      type: s3
      s3:
        bucket: my-backups
        region: us-east-1
        prefix: myapp/postgres
        secretRef:
          name: s3-backup-credentials
    compression:
      enabled: true
      algorithm: gzip
    encryption:
      enabled: true
      secretRef:
        name: backup-encryption-key
    postgres:
      format: custom
      jobs: 4
```

### Manual Backup and Restore

```yaml
# Create manual backup
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseBackup
metadata:
  name: myapp-manual-backup
spec:
  databaseRef:
    name: myapp-database
  storage:
    type: s3
    s3:
      bucket: my-backups
      secretRef:
        name: s3-credentials
  ttl: "168h"  # 7 days
---
# Restore to new database
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseRestore
metadata:
  name: myapp-restore
spec:
  backupRef:
    name: myapp-manual-backup
  targetDatabaseRef:
    name: myapp-database-copy
  createTarget: true
```

## Status Monitoring

### Backup Status

```bash
kubectl get databasebackup myapp-backup -o yaml
```

```yaml
status:
  phase: Completed
  startTime: "2024-01-01T02:00:00Z"
  completionTime: "2024-01-01T02:05:00Z"
  backupSize: "1.2GB"
  backupPath: "s3://my-backups/myapp/postgres/myapp-backup-20240101020000.dump"
```

### Schedule Status

```bash
kubectl get databasebackupschedule daily-backup -o yaml
```

```yaml
status:
  lastBackupTime: "2024-01-01T02:00:00Z"
  lastBackupName: "daily-backup-20240101020000"
  nextBackupTime: "2024-01-02T02:00:00Z"
  activeBackups: 7
```

## Troubleshooting

### Backup failed

- Check storage credentials are correct
- Verify bucket/container exists and is accessible
- Review backup pod logs
- Check database connectivity

### Restore failed

- Verify backup exists and is accessible
- Check target database doesn't exist (or `dropExisting: true`)
- Review restore pod logs
- Ensure sufficient disk space
