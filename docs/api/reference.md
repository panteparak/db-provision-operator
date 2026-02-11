# API Reference

## Packages
- [dbops.dbprovision.io/v1alpha1](#dbopsdbprovisioniov1alpha1)


## dbops.dbprovision.io/v1alpha1

Package v1alpha1 contains API Schema definitions for the dbops v1alpha1 API group.

### Resource Types
- [Database](#database)
- [DatabaseBackup](#databasebackup)
- [DatabaseBackupSchedule](#databasebackupschedule)
- [DatabaseGrant](#databasegrant)
- [DatabaseInstance](#databaseinstance)
- [DatabaseRestore](#databaserestore)
- [DatabaseRole](#databaserole)
- [DatabaseUser](#databaseuser)



#### AppliedGrantsInfo



AppliedGrantsInfo contains information about applied grants



_Appears in:_
- [DatabaseGrantStatus](#databasegrantstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `roles` _string array_ | Roles lists assigned roles |  |  |
| `directGrants` _integer_ | DirectGrants is the count of direct grants applied |  |  |
| `defaultPrivileges` _integer_ | DefaultPrivileges is the count of default privileges applied |  |  |


#### AzureStorageConfig



AzureStorageConfig defines Azure Blob Storage configuration



_Appears in:_
- [StorageConfig](#storageconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `container` _string_ | Container name |  | Required: \{\} <br /> |
| `storageAccount` _string_ | StorageAccount name |  | Required: \{\} <br /> |
| `prefix` _string_ | Prefix (path prefix within the container) |  | Optional: \{\} <br /> |
| `secretRef` _[SecretReference](#secretreference)_ | SecretRef references a secret containing Azure credentials |  | Required: \{\} <br /> |


#### BackupInfo



BackupInfo contains backup file information



_Appears in:_
- [DatabaseBackupStatus](#databasebackupstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `path` _string_ | Path is the full path to the backup file |  |  |
| `sizeBytes` _integer_ | SizeBytes is the backup size in bytes (uncompressed) |  |  |
| `compressedSizeBytes` _integer_ | CompressedSizeBytes is the backup size in bytes (compressed) |  |  |
| `checksum` _string_ | Checksum is the backup file checksum |  |  |
| `format` _string_ | Format is the backup format (e.g., custom, plain, directory) |  |  |


#### BackupReference



BackupReference references a DatabaseBackup



_Appears in:_
- [DatabaseRestoreSpec](#databaserestorespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the DatabaseBackup resource |  | Required: \{\} <br /> |
| `namespace` _string_ | Namespace of the DatabaseBackup (defaults to the resource namespace) |  | Optional: \{\} <br /> |


#### BackupSourceInfo



BackupSourceInfo contains information about the backup source



_Appears in:_
- [DatabaseBackupStatus](#databasebackupstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `instance` _string_ | Instance is the DatabaseInstance name |  |  |
| `database` _string_ | Database is the database name |  |  |
| `engine` _string_ | Engine is the database engine type |  |  |
| `version` _string_ | Version is the database server version |  |  |
| `timestamp` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#time-v1-meta)_ | Timestamp is the point-in-time of the backup |  |  |


#### BackupStatistics



BackupStatistics contains backup statistics



_Appears in:_
- [DatabaseBackupScheduleStatus](#databasebackupschedulestatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `totalBackups` _integer_ | TotalBackups is the total number of backups created |  |  |
| `successfulBackups` _integer_ | SuccessfulBackups is the number of successful backups |  |  |
| `failedBackups` _integer_ | FailedBackups is the number of failed backups |  |  |
| `averageDurationSeconds` _integer_ | AverageDurationSeconds is the average backup duration |  |  |
| `averageSizeBytes` _integer_ | AverageSizeBytes is the average backup size |  |  |
| `totalStorageBytes` _integer_ | TotalStorageBytes is the total storage used by all backups |  |  |


#### BackupTemplateMeta



BackupTemplateMeta defines metadata for created backups



_Appears in:_
- [BackupTemplateSpec](#backuptemplatespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `labels` _object (keys:string, values:string)_ | Labels to add to created backups |  | Optional: \{\} <br /> |
| `annotations` _object (keys:string, values:string)_ | Annotations to add to created backups |  | Optional: \{\} <br /> |


#### BackupTemplateSpec



BackupTemplateSpec defines the template for created backups



_Appears in:_
- [DatabaseBackupScheduleSpec](#databasebackupschedulespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `metadata` _[BackupTemplateMeta](#backuptemplatemeta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[DatabaseBackupSpec](#databasebackupspec)_ | Spec for the created backup |  | Required: \{\} <br /> |


#### CompressionAlgorithm

_Underlying type:_ _string_

CompressionAlgorithm defines the compression algorithm

_Validation:_
- Enum: [gzip lz4 zstd none]

_Appears in:_
- [CompressionConfig](#compressionconfig)

| Field | Description |
| --- | --- |
| `gzip` |  |
| `lz4` |  |
| `zstd` |  |
| `none` |  |


#### CompressionConfig



CompressionConfig defines backup compression settings



_Appears in:_
- [DatabaseBackupSpec](#databasebackupspec)
- [RestoreFromPath](#restorefrompath)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled enables compression | true |  |
| `algorithm` _[CompressionAlgorithm](#compressionalgorithm)_ | Algorithm specifies the compression algorithm | gzip | Enum: [gzip lz4 zstd] <br /> |
| `level` _integer_ | Level specifies the compression level (1-9) | 6 | Maximum: 9 <br />Minimum: 1 <br /> |


#### ConcurrencyPolicy

_Underlying type:_ _string_

ConcurrencyPolicy defines how to handle concurrent backups

_Validation:_
- Enum: [Allow Forbid Replace]

_Appears in:_
- [DatabaseBackupScheduleSpec](#databasebackupschedulespec)

| Field | Description |
| --- | --- |
| `Allow` |  |
| `Forbid` |  |
| `Replace` |  |




#### ConnectionConfig



ConnectionConfig defines the database connection settings



_Appears in:_
- [DatabaseInstanceSpec](#databaseinstancespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `host` _string_ | Host is the database server hostname or IP |  | MinLength: 1 <br />Required: \{\} <br /> |
| `port` _integer_ | Port is the database server port |  | Maximum: 65535 <br />Minimum: 1 <br />Required: \{\} <br /> |
| `database` _string_ | Database is the admin database name for initial connection |  | Required: \{\} <br /> |
| `secretRef` _[CredentialSecretRef](#credentialsecretref)_ | SecretRef references a secret containing credentials (mutually exclusive with ExistingSecret) |  | Optional: \{\} <br /> |
| `existingSecret` _[CredentialSecretRef](#credentialsecretref)_ | ExistingSecret references an existing secret with custom keys (mutually exclusive with SecretRef) |  | Optional: \{\} <br /> |


#### CredentialKeys



CredentialKeys defines the key names within a credential secret



_Appears in:_
- [CredentialSecretRef](#credentialsecretref)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `username` _string_ | Username key in the secret (default: "username") | username |  |
| `password` _string_ | Password key in the secret (default: "password") | password |  |


#### CredentialSecretRef



CredentialSecretRef references credentials in a secret



_Appears in:_
- [ConnectionConfig](#connectionconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the secret containing credentials |  | Required: \{\} <br /> |
| `namespace` _string_ | Namespace of the secret (defaults to the resource's namespace if not specified) |  | Optional: \{\} <br /> |
| `keys` _[CredentialKeys](#credentialkeys)_ | Keys defines the key names for username and password |  | Optional: \{\} <br /> |


#### Database



Database is the Schema for the databases API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `dbops.dbprovision.io/v1alpha1` | | |
| `kind` _string_ | `Database` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[DatabaseSpec](#databasespec)_ |  |  |  |
| `status` _[DatabaseStatus](#databasestatus)_ |  |  |  |


#### DatabaseBackup



DatabaseBackup is the Schema for the databasebackups API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `dbops.dbprovision.io/v1alpha1` | | |
| `kind` _string_ | `DatabaseBackup` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[DatabaseBackupSpec](#databasebackupspec)_ |  |  |  |
| `status` _[DatabaseBackupStatus](#databasebackupstatus)_ |  |  |  |


#### DatabaseBackupSchedule



DatabaseBackupSchedule is the Schema for the databasebackupschedules API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `dbops.dbprovision.io/v1alpha1` | | |
| `kind` _string_ | `DatabaseBackupSchedule` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[DatabaseBackupScheduleSpec](#databasebackupschedulespec)_ |  |  |  |
| `status` _[DatabaseBackupScheduleStatus](#databasebackupschedulestatus)_ |  |  |  |


#### DatabaseBackupScheduleSpec



DatabaseBackupScheduleSpec defines the desired state of DatabaseBackupSchedule.



_Appears in:_
- [DatabaseBackupSchedule](#databasebackupschedule)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `schedule` _string_ | Schedule is the cron expression for the backup schedule |  | MinLength: 1 <br />Required: \{\} <br /> |
| `timezone` _string_ | Timezone is the timezone for the schedule (e.g., "Asia/Bangkok") | UTC |  |
| `paused` _boolean_ | Paused suspends the schedule |  | Optional: \{\} <br /> |
| `concurrencyPolicy` _[ConcurrencyPolicy](#concurrencypolicy)_ | ConcurrencyPolicy defines how to handle concurrent backups | Forbid | Enum: [Allow Forbid Replace] <br /> |
| `template` _[BackupTemplateSpec](#backuptemplatespec)_ | Template defines the DatabaseBackup to create |  | Required: \{\} <br /> |
| `retention` _[RetentionPolicy](#retentionpolicy)_ | Retention defines the backup retention policy |  | Optional: \{\} <br /> |
| `successfulBackupsHistoryLimit` _integer_ | SuccessfulBackupsHistoryLimit is the number of successful backups to keep in status | 5 | Minimum: 0 <br /> |
| `failedBackupsHistoryLimit` _integer_ | FailedBackupsHistoryLimit is the number of failed backups to keep in status | 3 | Minimum: 0 <br /> |
| `deletionProtection` _boolean_ | DeletionProtection prevents accidental deletion |  | Optional: \{\} <br /> |


#### DatabaseBackupScheduleStatus



DatabaseBackupScheduleStatus defines the observed state of DatabaseBackupSchedule.



_Appears in:_
- [DatabaseBackupSchedule](#databasebackupschedule)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `phase` _[Phase](#phase)_ | Phase represents the current state |  | Enum: [Active Paused] <br /> |
| `lastBackup` _[ScheduledBackupInfo](#scheduledbackupinfo)_ | LastBackup contains information about the last backup |  |  |
| `nextBackupTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#time-v1-meta)_ | NextBackupTime is the next scheduled backup time |  |  |
| `statistics` _[BackupStatistics](#backupstatistics)_ | Statistics contains backup statistics |  |  |
| `recentBackups` _[RecentBackupInfo](#recentbackupinfo) array_ | RecentBackups lists recent backup names and statuses |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#condition-v1-meta) array_ | Conditions represent the latest available observations |  | Optional: \{\} <br /> |


#### DatabaseBackupSpec



DatabaseBackupSpec defines the desired state of DatabaseBackup.



_Appears in:_
- [BackupTemplateSpec](#backuptemplatespec)
- [DatabaseBackup](#databasebackup)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `databaseRef` _[DatabaseReference](#databasereference)_ | DatabaseRef references the Database to backup |  | Required: \{\} <br /> |
| `storage` _[StorageConfig](#storageconfig)_ | Storage defines where to store the backup |  | Required: \{\} <br /> |
| `compression` _[CompressionConfig](#compressionconfig)_ | Compression configures backup compression |  | Optional: \{\} <br /> |
| `encryption` _[EncryptionConfig](#encryptionconfig)_ | Encryption configures backup encryption |  | Optional: \{\} <br /> |
| `ttl` _string_ | TTL is the time-to-live for the backup (e.g., "168h" for 7 days) |  | Optional: \{\} <br /> |
| `activeDeadlineSeconds` _integer_ | ActiveDeadlineSeconds is the timeout for the backup operation | 3600 | Minimum: 1 <br /> |
| `postgres` _[PostgresBackupConfig](#postgresbackupconfig)_ | PostgreSQL-specific backup configuration |  | Optional: \{\} <br /> |
| `mysql` _[MySQLBackupConfig](#mysqlbackupconfig)_ | MySQL-specific backup configuration |  | Optional: \{\} <br /> |


#### DatabaseBackupStatus



DatabaseBackupStatus defines the observed state of DatabaseBackup.



_Appears in:_
- [DatabaseBackup](#databasebackup)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `phase` _[Phase](#phase)_ | Phase represents the current state |  | Enum: [Pending Running Completed Failed] <br /> |
| `startedAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#time-v1-meta)_ | StartedAt is the backup start time |  |  |
| `completedAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#time-v1-meta)_ | CompletedAt is the backup completion time |  |  |
| `message` _string_ | Message provides additional information about the current state |  |  |
| `backup` _[BackupInfo](#backupinfo)_ | Backup contains backup-specific status information |  |  |
| `source` _[BackupSourceInfo](#backupsourceinfo)_ | Source contains information about the backup source |  |  |
| `expiresAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#time-v1-meta)_ | ExpiresAt is when the backup will be deleted |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#condition-v1-meta) array_ | Conditions represent the latest available observations |  | Optional: \{\} <br /> |


#### DatabaseGrant



DatabaseGrant is the Schema for the databasegrants API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `dbops.dbprovision.io/v1alpha1` | | |
| `kind` _string_ | `DatabaseGrant` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[DatabaseGrantSpec](#databasegrantspec)_ |  |  |  |
| `status` _[DatabaseGrantStatus](#databasegrantstatus)_ |  |  |  |


#### DatabaseGrantSpec



DatabaseGrantSpec defines the desired state of DatabaseGrant.



_Appears in:_
- [DatabaseGrant](#databasegrant)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `userRef` _[UserReference](#userreference)_ | UserRef references the DatabaseUser to grant permissions to |  | Required: \{\} <br /> |
| `databaseRef` _[DatabaseReference](#databasereference)_ | DatabaseRef references the Database for context (optional) |  | Optional: \{\} <br /> |
| `postgres` _[PostgresGrantConfig](#postgresgrantconfig)_ | PostgreSQL-specific grants |  | Optional: \{\} <br /> |
| `mysql` _[MySQLGrantConfig](#mysqlgrantconfig)_ | MySQL-specific grants |  | Optional: \{\} <br /> |
| `driftPolicy` _[DriftPolicy](#driftpolicy)_ | DriftPolicy overrides the instance-level drift policy for this grant.<br />If not specified, the instance's drift policy is used. |  | Optional: \{\} <br /> |
| `deletionProtection` _boolean_ | DeletionProtection prevents accidental deletion |  | Optional: \{\} <br /> |


#### DatabaseGrantStatus



DatabaseGrantStatus defines the observed state of DatabaseGrant.



_Appears in:_
- [DatabaseGrant](#databasegrant)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `phase` _[Phase](#phase)_ | Phase represents the current state |  | Enum: [Pending Creating Ready Failed Deleting] <br /> |
| `observedGeneration` _integer_ | ObservedGeneration is the last observed generation of the resource |  |  |
| `message` _string_ | Message provides additional information about the current state |  |  |
| `appliedGrants` _[AppliedGrantsInfo](#appliedgrantsinfo)_ | AppliedGrants contains information about applied grants |  |  |
| `drift` _[DriftStatus](#driftstatus)_ | Drift contains drift detection status information |  | Optional: \{\} <br /> |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#condition-v1-meta) array_ | Conditions represent the latest available observations |  | Optional: \{\} <br /> |


#### DatabaseInfo



DatabaseInfo contains general database information



_Appears in:_
- [DatabaseStatus](#databasestatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the actual database name |  |  |
| `owner` _string_ | Owner is the database owner |  |  |
| `sizeBytes` _integer_ | SizeBytes is the database size in bytes |  |  |
| `createdAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#time-v1-meta)_ | CreatedAt is the creation timestamp |  |  |


#### DatabaseInstance



DatabaseInstance is the Schema for the databaseinstances API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `dbops.dbprovision.io/v1alpha1` | | |
| `kind` _string_ | `DatabaseInstance` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[DatabaseInstanceSpec](#databaseinstancespec)_ |  |  |  |
| `status` _[DatabaseInstanceStatus](#databaseinstancestatus)_ |  |  |  |


#### DatabaseInstanceSpec



DatabaseInstanceSpec defines the desired state of DatabaseInstance.



_Appears in:_
- [DatabaseInstance](#databaseinstance)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `engine` _[EngineType](#enginetype)_ | Engine type (required, immutable) |  | Enum: [postgres mysql mariadb cockroachdb] <br />Required: \{\} <br /> |
| `connection` _[ConnectionConfig](#connectionconfig)_ | Connection configuration |  | Required: \{\} <br /> |
| `tls` _[TLSConfig](#tlsconfig)_ | TLS configuration |  | Optional: \{\} <br /> |
| `healthCheck` _[HealthCheckConfig](#healthcheckconfig)_ | Health check configuration |  | Optional: \{\} <br /> |
| `driftPolicy` _[DriftPolicy](#driftpolicy)_ | DriftPolicy defines the default drift detection policy for resources using this instance.<br />Individual resources can override this policy. |  | Optional: \{\} <br /> |
| `discovery` _[DiscoveryConfig](#discoveryconfig)_ | Discovery enables scanning for database resources not managed by Kubernetes CRs.<br />Discovered resources can be adopted via annotations. |  | Optional: \{\} <br /> |
| `postgres` _[PostgresInstanceConfig](#postgresinstanceconfig)_ | PostgreSQL-specific options (only valid when engine is "postgres") |  | Optional: \{\} <br /> |
| `mysql` _[MySQLInstanceConfig](#mysqlinstanceconfig)_ | MySQL-specific options (only valid when engine is "mysql") |  | Optional: \{\} <br /> |
| `deletionProtection` _boolean_ | DeletionProtection prevents accidental deletion |  | Optional: \{\} <br /> |


#### DatabaseInstanceStatus



DatabaseInstanceStatus defines the observed state of DatabaseInstance.



_Appears in:_
- [DatabaseInstance](#databaseinstance)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `phase` _[Phase](#phase)_ | Phase represents the current state of the instance |  | Enum: [Pending Ready Failed] <br /> |
| `version` _string_ | Version is the detected database server version |  |  |
| `message` _string_ | Message provides additional information about the current state |  |  |
| `lastCheckedAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#time-v1-meta)_ | LastCheckedAt is the timestamp of the last health check |  |  |
| `observedGeneration` _integer_ | ObservedGeneration is the last observed generation of the resource |  |  |
| `discoveredResources` _[DiscoveredResourcesStatus](#discoveredresourcesstatus)_ | DiscoveredResources contains resources found in the database that are not managed by CRs.<br />Only populated when discovery is enabled. |  | Optional: \{\} <br /> |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#condition-v1-meta) array_ | Conditions represent the latest available observations |  | Optional: \{\} <br /> |


#### DatabaseReference



DatabaseReference references a Database



_Appears in:_
- [DatabaseBackupSpec](#databasebackupspec)
- [DatabaseGrantSpec](#databasegrantspec)
- [RestoreTarget](#restoretarget)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the Database resource |  | Required: \{\} <br /> |
| `namespace` _string_ | Namespace of the Database (defaults to the resource namespace) |  | Optional: \{\} <br /> |


#### DatabaseRestore



DatabaseRestore is the Schema for the databaserestores API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `dbops.dbprovision.io/v1alpha1` | | |
| `kind` _string_ | `DatabaseRestore` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[DatabaseRestoreSpec](#databaserestorespec)_ |  |  |  |
| `status` _[DatabaseRestoreStatus](#databaserestorestatus)_ |  |  |  |


#### DatabaseRestoreSpec



DatabaseRestoreSpec defines the desired state of DatabaseRestore.



_Appears in:_
- [DatabaseRestore](#databaserestore)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `backupRef` _[BackupReference](#backupreference)_ | BackupRef references the DatabaseBackup to restore from |  | Optional: \{\} <br /> |
| `fromPath` _[RestoreFromPath](#restorefrompath)_ | FromPath allows restoring from a direct path instead of a backup reference |  | Optional: \{\} <br /> |
| `target` _[RestoreTarget](#restoretarget)_ | Target defines where to restore the backup |  | Required: \{\} <br /> |
| `confirmation` _[RestoreConfirmation](#restoreconfirmation)_ | Confirmation contains safety confirmations for destructive operations |  | Optional: \{\} <br /> |
| `activeDeadlineSeconds` _integer_ | ActiveDeadlineSeconds is the timeout for the restore operation | 7200 | Minimum: 1 <br /> |
| `postgres` _[PostgresRestoreConfig](#postgresrestoreconfig)_ | PostgreSQL-specific restore configuration |  | Optional: \{\} <br /> |
| `mysql` _[MySQLRestoreConfig](#mysqlrestoreconfig)_ | MySQL-specific restore configuration |  | Optional: \{\} <br /> |


#### DatabaseRestoreStatus



DatabaseRestoreStatus defines the observed state of DatabaseRestore.



_Appears in:_
- [DatabaseRestore](#databaserestore)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `phase` _[Phase](#phase)_ | Phase represents the current state |  | Enum: [Pending Running Completed Failed] <br /> |
| `startedAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#time-v1-meta)_ | StartedAt is the restore start time |  |  |
| `completedAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#time-v1-meta)_ | CompletedAt is the restore completion time |  |  |
| `message` _string_ | Message provides additional information about the current state |  |  |
| `restore` _[RestoreInfo](#restoreinfo)_ | Restore contains restore-specific status information |  |  |
| `progress` _[RestoreProgress](#restoreprogress)_ | Progress contains restore progress information |  |  |
| `warnings` _string array_ | Warnings contains any warnings encountered during restore |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#condition-v1-meta) array_ | Conditions represent the latest available observations |  | Optional: \{\} <br /> |


#### DatabaseRole



DatabaseRole is the Schema for the databaseroles API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `dbops.dbprovision.io/v1alpha1` | | |
| `kind` _string_ | `DatabaseRole` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[DatabaseRoleSpec](#databaserolespec)_ |  |  |  |
| `status` _[DatabaseRoleStatus](#databaserolestatus)_ |  |  |  |


#### DatabaseRoleSpec



DatabaseRoleSpec defines the desired state of DatabaseRole.



_Appears in:_
- [DatabaseRole](#databaserole)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `instanceRef` _[InstanceReference](#instancereference)_ | InstanceRef references the DatabaseInstance to use |  | Required: \{\} <br /> |
| `roleName` _string_ | RoleName is the role name in the database (immutable after creation) |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[a-zA-Z_][a-zA-Z0-9_]*$` <br />Required: \{\} <br /> |
| `postgres` _[PostgresRoleConfig](#postgresroleconfig)_ | PostgreSQL-specific configuration |  | Optional: \{\} <br /> |
| `mysql` _[MySQLRoleConfig](#mysqlroleconfig)_ | MySQL-specific configuration |  | Optional: \{\} <br /> |
| `driftPolicy` _[DriftPolicy](#driftpolicy)_ | DriftPolicy overrides the instance-level drift policy for this role.<br />If not specified, the instance's drift policy is used. |  | Optional: \{\} <br /> |


#### DatabaseRoleStatus



DatabaseRoleStatus defines the observed state of DatabaseRole.



_Appears in:_
- [DatabaseRole](#databaserole)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `phase` _[Phase](#phase)_ | Phase represents the current state |  | Enum: [Pending Creating Ready Failed Deleting] <br /> |
| `observedGeneration` _integer_ | ObservedGeneration is the last observed generation of the resource |  |  |
| `message` _string_ | Message provides additional information about the current state |  |  |
| `role` _[RoleInfo](#roleinfo)_ | Role contains role-specific status information |  |  |
| `drift` _[DriftStatus](#driftstatus)_ | Drift contains drift detection status information |  | Optional: \{\} <br /> |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#condition-v1-meta) array_ | Conditions represent the latest available observations |  | Optional: \{\} <br /> |


#### DatabaseSpec



DatabaseSpec defines the desired state of Database.



_Appears in:_
- [Database](#database)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `instanceRef` _[InstanceReference](#instancereference)_ | InstanceRef references the DatabaseInstance to use |  | Required: \{\} <br /> |
| `name` _string_ | Name is the database name in the database server (immutable after creation) |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[a-zA-Z_][a-zA-Z0-9_]*$` <br />Required: \{\} <br /> |
| `deletionPolicy` _[DeletionPolicy](#deletionpolicy)_ | DeletionPolicy defines what happens on CR deletion | Retain | Enum: [Retain Delete Snapshot] <br /> |
| `deletionProtection` _boolean_ | DeletionProtection prevents accidental deletion | true |  |
| `driftPolicy` _[DriftPolicy](#driftpolicy)_ | DriftPolicy overrides the instance-level drift policy for this database.<br />If not specified, the instance's drift policy is used. |  | Optional: \{\} <br /> |
| `postgres` _[PostgresDatabaseConfig](#postgresdatabaseconfig)_ | PostgreSQL-specific configuration (required when instance engine is "postgres") |  | Optional: \{\} <br /> |
| `mysql` _[MySQLDatabaseConfig](#mysqldatabaseconfig)_ | MySQL-specific configuration (required when instance engine is "mysql") |  | Optional: \{\} <br /> |


#### DatabaseStatus



DatabaseStatus defines the observed state of Database.



_Appears in:_
- [Database](#database)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `phase` _[Phase](#phase)_ | Phase represents the current state of the database |  | Enum: [Pending Creating Ready Failed Deleting] <br /> |
| `observedGeneration` _integer_ | ObservedGeneration is the last observed generation of the resource |  |  |
| `message` _string_ | Message provides additional information about the current state |  |  |
| `database` _[DatabaseInfo](#databaseinfo)_ | Database contains database-specific status information |  |  |
| `postgres` _[PostgresDatabaseStatus](#postgresdatabasestatus)_ | Postgres contains PostgreSQL-specific status information |  | Optional: \{\} <br /> |
| `mysql` _[MySQLDatabaseStatus](#mysqldatabasestatus)_ | MySQL contains MySQL-specific status information |  | Optional: \{\} <br /> |
| `drift` _[DriftStatus](#driftstatus)_ | Drift contains drift detection status information |  | Optional: \{\} <br /> |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#condition-v1-meta) array_ | Conditions represent the latest available observations |  | Optional: \{\} <br /> |


#### DatabaseUser



DatabaseUser is the Schema for the databaseusers API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `dbops.dbprovision.io/v1alpha1` | | |
| `kind` _string_ | `DatabaseUser` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[DatabaseUserSpec](#databaseuserspec)_ |  |  |  |
| `status` _[DatabaseUserStatus](#databaseuserstatus)_ |  |  |  |


#### DatabaseUserSpec



DatabaseUserSpec defines the desired state of DatabaseUser.



_Appears in:_
- [DatabaseUser](#databaseuser)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `instanceRef` _[InstanceReference](#instancereference)_ | InstanceRef references the DatabaseInstance to use |  | Required: \{\} <br /> |
| `username` _string_ | Username is the database username (immutable after creation) |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[a-zA-Z_][a-zA-Z0-9_]*$` <br />Required: \{\} <br /> |
| `passwordSecret` _[PasswordConfig](#passwordconfig)_ | PasswordSecret configures password generation and secret output |  | Optional: \{\} <br /> |
| `existingPasswordSecret` _[ExistingPasswordSecret](#existingpasswordsecret)_ | ExistingPasswordSecret references an existing secret containing the password |  | Optional: \{\} <br /> |
| `passwordRotation` _[PasswordRotationConfig](#passwordrotationconfig)_ | PasswordRotation configures automatic password rotation |  | Optional: \{\} <br /> |
| `postgres` _[PostgresUserConfig](#postgresuserconfig)_ | PostgreSQL-specific configuration |  | Optional: \{\} <br /> |
| `mysql` _[MySQLUserConfig](#mysqluserconfig)_ | MySQL-specific configuration |  | Optional: \{\} <br /> |
| `driftPolicy` _[DriftPolicy](#driftpolicy)_ | DriftPolicy overrides the instance-level drift policy for this user.<br />If not specified, the instance's drift policy is used. |  | Optional: \{\} <br /> |


#### DatabaseUserStatus



DatabaseUserStatus defines the observed state of DatabaseUser.



_Appears in:_
- [DatabaseUser](#databaseuser)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `phase` _[Phase](#phase)_ | Phase represents the current state |  | Enum: [Pending Creating Ready Failed Deleting] <br /> |
| `observedGeneration` _integer_ | ObservedGeneration is the last observed generation of the resource |  |  |
| `message` _string_ | Message provides additional information about the current state |  |  |
| `user` _[UserInfo](#userinfo)_ | User contains user-specific status information |  |  |
| `secret` _[SecretInfo](#secretinfo)_ | Secret contains generated secret information |  |  |
| `drift` _[DriftStatus](#driftstatus)_ | Drift contains drift detection status information |  | Optional: \{\} <br /> |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#condition-v1-meta) array_ | Conditions represent the latest available observations |  | Optional: \{\} <br /> |


#### DeletionPolicy

_Underlying type:_ _string_

DeletionPolicy defines what happens when a resource is deleted

_Validation:_
- Enum: [Retain Delete Snapshot]

_Appears in:_
- [DatabaseSpec](#databasespec)

| Field | Description |
| --- | --- |
| `Retain` |  |
| `Delete` |  |
| `Snapshot` |  |


#### DiscoveredResource



DiscoveredResource represents a resource found in the database
that is not managed by a Kubernetes CR.



_Appears in:_
- [DiscoveredResourcesStatus](#discoveredresourcesstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the discovered resource |  |  |
| `discovered` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#time-v1-meta)_ | Discovered is when this resource was first discovered |  |  |
| `adopted` _boolean_ | Adopted indicates if this resource has been adopted via annotation |  |  |


#### DiscoveredResourcesStatus



DiscoveredResourcesStatus contains discovered unmanaged resources.



_Appears in:_
- [DatabaseInstanceStatus](#databaseinstancestatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `databases` _[DiscoveredResource](#discoveredresource) array_ | Databases contains discovered database resources |  | Optional: \{\} <br /> |
| `users` _[DiscoveredResource](#discoveredresource) array_ | Users contains discovered user resources |  | Optional: \{\} <br /> |
| `roles` _[DiscoveredResource](#discoveredresource) array_ | Roles contains discovered role resources |  | Optional: \{\} <br /> |
| `lastScan` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#time-v1-meta)_ | LastScan is when the last discovery scan was performed |  | Optional: \{\} <br /> |


#### DiscoveryConfig



DiscoveryConfig defines configuration for resource discovery.
When enabled, the operator will scan the database for resources
that exist but are not managed by Kubernetes CRs.



_Appears in:_
- [DatabaseInstanceSpec](#databaseinstancespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled enables resource discovery | false |  |
| `interval` _string_ | Interval specifies how often to scan for unmanaged resources (Go duration string) | 30m | Optional: \{\} <br /> |


#### DriftDiff



DriftDiff represents a single difference between desired and actual state.



_Appears in:_
- [DriftStatus](#driftstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `field` _string_ | Field is the name of the field that differs |  |  |
| `expected` _string_ | Expected is the expected value from the CR spec |  |  |
| `actual` _string_ | Actual is the actual value in the database |  |  |
| `destructive` _boolean_ | Destructive indicates if correcting this drift would be destructive |  | Optional: \{\} <br /> |
| `immutable` _boolean_ | Immutable indicates if this field cannot be changed after creation |  | Optional: \{\} <br /> |


#### DriftMode

_Underlying type:_ _string_

DriftMode defines how drift is handled

_Validation:_
- Enum: [ignore detect correct]

_Appears in:_
- [DriftPolicy](#driftpolicy)

| Field | Description |
| --- | --- |
| `ignore` | DriftModeIgnore disables drift detection entirely<br /> |
| `detect` | DriftModeDetect detects drift and reports in status/events but does not auto-correct<br /> |
| `correct` | DriftModeCorrect detects drift and automatically corrects it<br /> |


#### DriftPolicy



DriftPolicy defines how drift detection and correction should be handled.
This can be set at the instance level (default for all child resources)
or overridden at the individual resource level.



_Appears in:_
- [DatabaseGrantSpec](#databasegrantspec)
- [DatabaseInstanceSpec](#databaseinstancespec)
- [DatabaseRoleSpec](#databaserolespec)
- [DatabaseSpec](#databasespec)
- [DatabaseUserSpec](#databaseuserspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `mode` _[DriftMode](#driftmode)_ | Mode determines how drift is handled | detect | Enum: [ignore detect correct] <br /> |
| `interval` _string_ | Interval specifies how often to check for drift (Go duration string)<br />This is only meaningful when mode is "detect" or "correct" | 5m | Optional: \{\} <br /> |


#### DriftStatus



DriftStatus represents the current drift detection status for a resource.



_Appears in:_
- [DatabaseGrantStatus](#databasegrantstatus)
- [DatabaseRoleStatus](#databaserolestatus)
- [DatabaseStatus](#databasestatus)
- [DatabaseUserStatus](#databaseuserstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `detected` _boolean_ | Detected indicates if drift was detected |  |  |
| `lastChecked` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#time-v1-meta)_ | LastChecked is when drift was last checked |  | Optional: \{\} <br /> |
| `diffs` _[DriftDiff](#driftdiff) array_ | Diffs contains the specific differences found |  | Optional: \{\} <br /> |


#### EncryptionAlgorithm

_Underlying type:_ _string_

EncryptionAlgorithm defines the encryption algorithm

_Validation:_
- Enum: [aes-256-gcm aes-256-cbc]

_Appears in:_
- [EncryptionConfig](#encryptionconfig)

| Field | Description |
| --- | --- |
| `aes-256-gcm` |  |
| `aes-256-cbc` |  |


#### EncryptionConfig



EncryptionConfig defines backup encryption settings



_Appears in:_
- [DatabaseBackupSpec](#databasebackupspec)
- [RestoreFromPath](#restorefrompath)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled enables encryption |  | Optional: \{\} <br /> |
| `algorithm` _[EncryptionAlgorithm](#encryptionalgorithm)_ | Algorithm specifies the encryption algorithm | aes-256-gcm | Enum: [aes-256-gcm aes-256-cbc] <br /> |
| `secretRef` _[SecretKeySelector](#secretkeyselector)_ | SecretRef references a secret containing the encryption key |  | Optional: \{\} <br /> |


#### EngineType

_Underlying type:_ _string_

EngineType defines the database engine type

_Validation:_
- Enum: [postgres mysql mariadb cockroachdb]

_Appears in:_
- [DatabaseInstanceSpec](#databaseinstancespec)

| Field | Description |
| --- | --- |
| `postgres` |  |
| `mysql` |  |
| `mariadb` |  |
| `cockroachdb` |  |


#### ExistingPasswordSecret



ExistingPasswordSecret references an existing secret containing a password



_Appears in:_
- [DatabaseUserSpec](#databaseuserspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the secret |  | Required: \{\} <br /> |
| `namespace` _string_ | Namespace of the secret (defaults to the resource namespace) |  | Optional: \{\} <br /> |
| `key` _string_ | Key within the secret containing the password |  | Required: \{\} <br /> |


#### GCSStorageConfig



GCSStorageConfig defines Google Cloud Storage configuration



_Appears in:_
- [StorageConfig](#storageconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `bucket` _string_ | Bucket name |  | Required: \{\} <br /> |
| `prefix` _string_ | Prefix (path prefix within the bucket) |  | Optional: \{\} <br /> |
| `secretRef` _[SecretKeySelector](#secretkeyselector)_ | SecretRef references a secret containing GCS credentials |  | Required: \{\} <br /> |


#### HealthCheckConfig



HealthCheckConfig defines health check settings



_Appears in:_
- [DatabaseInstanceSpec](#databaseinstancespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled enables periodic health checks | true |  |
| `intervalSeconds` _integer_ | IntervalSeconds defines how often to check (default: 30) | 30 | Minimum: 5 <br /> |
| `timeoutSeconds` _integer_ | TimeoutSeconds defines the health check timeout (default: 5) | 5 | Minimum: 1 <br /> |


#### InstanceReference



InstanceReference references a DatabaseInstance



_Appears in:_
- [DatabaseRoleSpec](#databaserolespec)
- [DatabaseSpec](#databasespec)
- [DatabaseUserSpec](#databaseuserspec)
- [RestoreTarget](#restoretarget)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the DatabaseInstance |  | Required: \{\} <br /> |
| `namespace` _string_ | Namespace of the DatabaseInstance (defaults to the resource namespace) |  | Optional: \{\} <br /> |


#### MySQLAuthPlugin

_Underlying type:_ _string_

MySQLAuthPlugin defines MySQL authentication plugins

_Validation:_
- Enum: [mysql_native_password caching_sha2_password sha256_password]

_Appears in:_
- [MySQLUserConfig](#mysqluserconfig)

| Field | Description |
| --- | --- |
| `mysql_native_password` |  |
| `caching_sha2_password` |  |
| `sha256_password` |  |


#### MySQLBackupConfig



MySQLBackupConfig defines MySQL-specific backup configuration



_Appears in:_
- [DatabaseBackupSpec](#databasebackupspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `method` _[MySQLBackupMethod](#mysqlbackupmethod)_ | Method specifies the backup method | mysqldump | Enum: [mysqldump xtrabackup mysqlpump] <br /> |
| `singleTransaction` _boolean_ | SingleTransaction uses a single transaction for InnoDB tables | true |  |
| `quick` _boolean_ | Quick retrieves rows one at a time instead of buffering | true |  |
| `lockTables` _boolean_ | LockTables locks all tables before backup |  | Optional: \{\} <br /> |
| `routines` _boolean_ | Routines includes stored procedures and functions | true |  |
| `triggers` _boolean_ | Triggers includes triggers | true |  |
| `events` _boolean_ | Events includes events | true |  |
| `extendedInsert` _boolean_ | ExtendedInsert uses extended INSERT statements | true |  |
| `setGtidPurged` _[MySQLGtidPurged](#mysqlgtidpurged)_ | SetGtidPurged controls SET @@GLOBAL.GTID_PURGED | AUTO | Enum: [OFF ON AUTO] <br /> |
| `databases` _string array_ | Databases lists specific databases to backup (empty = all) |  | Optional: \{\} <br /> |
| `tables` _string array_ | Tables lists specific tables to backup (empty = all) |  | Optional: \{\} <br /> |
| `excludeTables` _string array_ | ExcludeTables lists tables to exclude |  | Optional: \{\} <br /> |


#### MySQLBackupMethod

_Underlying type:_ _string_

MySQLBackupMethod defines MySQL backup methods

_Validation:_
- Enum: [mysqldump xtrabackup mysqlpump]

_Appears in:_
- [MySQLBackupConfig](#mysqlbackupconfig)

| Field | Description |
| --- | --- |
| `mysqldump` |  |
| `xtrabackup` |  |
| `mysqlpump` |  |


#### MySQLDatabaseConfig



MySQLDatabaseConfig defines MySQL-specific database configuration



_Appears in:_
- [DatabaseSpec](#databasespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `charset` _string_ | Charset sets the database character set | utf8mb4 |  |
| `collation` _string_ | Collation sets the database collation | utf8mb4_unicode_ci |  |
| `sqlMode` _string_ | SQLMode sets the SQL mode for the database |  | Optional: \{\} <br /> |
| `defaultStorageEngine` _string_ | DefaultStorageEngine sets the default storage engine | InnoDB |  |


#### MySQLDatabaseStatus



MySQLDatabaseStatus contains MySQL-specific database status



_Appears in:_
- [DatabaseStatus](#databasestatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `charset` _string_ | Charset is the database character set |  |  |
| `collation` _string_ | Collation is the database collation |  |  |


#### MySQLGrant



MySQLGrant defines a MySQL privilege grant



_Appears in:_
- [MySQLGrantConfig](#mysqlgrantconfig)
- [MySQLRoleConfig](#mysqlroleconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `level` _[MySQLGrantLevel](#mysqlgrantlevel)_ | Level is the grant level |  | Enum: [global database table column procedure function] <br /> |
| `database` _string_ | Database is the target database (for database/table/column/procedure/function levels) |  | Optional: \{\} <br /> |
| `table` _string_ | Table is the target table (for table/column levels) |  | Optional: \{\} <br /> |
| `columns` _string array_ | Columns lists target columns (for column level) |  | Optional: \{\} <br /> |
| `procedure` _string_ | Procedure is the target procedure (for procedure level) |  | Optional: \{\} <br /> |
| `function` _string_ | Function is the target function (for function level) |  | Optional: \{\} <br /> |
| `privileges` _string array_ | Privileges to grant (SELECT, INSERT, UPDATE, DELETE, CREATE, DROP, REFERENCES, INDEX, ALTER, etc.) |  | MinItems: 1 <br /> |
| `withGrantOption` _boolean_ | WithGrantOption allows the grantee to grant these privileges to others |  | Optional: \{\} <br /> |


#### MySQLGrantConfig



MySQLGrantConfig defines MySQL-specific grant configuration



_Appears in:_
- [DatabaseGrantSpec](#databasegrantspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `roles` _string array_ | Roles to assign to the user (MySQL 8.0+) |  | Optional: \{\} <br /> |
| `grants` _[MySQLGrant](#mysqlgrant) array_ | Grants defines direct privilege grants |  | Optional: \{\} <br /> |


#### MySQLGrantLevel

_Underlying type:_ _string_

MySQLGrantLevel defines the level of a MySQL grant

_Validation:_
- Enum: [global database table column procedure function]

_Appears in:_
- [MySQLGrant](#mysqlgrant)

| Field | Description |
| --- | --- |
| `global` |  |
| `database` |  |
| `table` |  |
| `column` |  |
| `procedure` |  |
| `function` |  |


#### MySQLGtidPurged

_Underlying type:_ _string_

MySQLGtidPurged defines GTID_PURGED setting

_Validation:_
- Enum: [OFF ON AUTO]

_Appears in:_
- [MySQLBackupConfig](#mysqlbackupconfig)

| Field | Description |
| --- | --- |
| `OFF` |  |
| `ON` |  |
| `AUTO` |  |


#### MySQLInstanceConfig



MySQLInstanceConfig defines MySQL-specific instance configuration



_Appears in:_
- [DatabaseInstanceSpec](#databaseinstancespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `charset` _string_ | Charset sets the default character set | utf8mb4 |  |
| `collation` _string_ | Collation sets the default collation | utf8mb4_unicode_ci |  |
| `parseTime` _boolean_ | ParseTime enables parsing of DATE and DATETIME to time.Time | true |  |
| `timeout` _string_ | Timeout is the connection timeout (e.g., "10s") | 10s |  |
| `readTimeout` _string_ | ReadTimeout is the read timeout (e.g., "30s") | 30s |  |
| `writeTimeout` _string_ | WriteTimeout is the write timeout (e.g., "30s") | 30s |  |
| `tls` _[MySQLTLSMode](#mysqltlsmode)_ | TLS specifies the TLS mode | preferred | Enum: [disabled preferred required skip-verify] <br /> |


#### MySQLRestoreConfig



MySQLRestoreConfig defines MySQL-specific restore configuration



_Appears in:_
- [DatabaseRestoreSpec](#databaserestorespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `dropExisting` _boolean_ | DropExisting drops existing database before restore |  | Optional: \{\} <br /> |
| `createDatabase` _boolean_ | CreateDatabase creates the database if it doesn't exist | true |  |
| `routines` _boolean_ | Routines restores stored procedures and functions | true |  |
| `triggers` _boolean_ | Triggers restores triggers | true |  |
| `events` _boolean_ | Events restores events | true |  |
| `disableForeignKeyChecks` _boolean_ | DisableForeignKeyChecks disables foreign key checks during restore | true |  |
| `disableBinlog` _boolean_ | DisableBinlog disables binary logging during restore | true |  |


#### MySQLRoleConfig



MySQLRoleConfig defines MySQL-specific role configuration



_Appears in:_
- [DatabaseRoleSpec](#databaserolespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `useNativeRoles` _boolean_ | UseNativeRoles enables MySQL 8.0+ native roles | true |  |
| `grants` _[MySQLGrant](#mysqlgrant) array_ | Grants defines the permissions this role grants |  | Optional: \{\} <br /> |


#### MySQLTLSMode

_Underlying type:_ _string_

MySQLTLSMode defines MySQL TLS modes

_Validation:_
- Enum: [disabled preferred required skip-verify]

_Appears in:_
- [MySQLInstanceConfig](#mysqlinstanceconfig)

| Field | Description |
| --- | --- |
| `disabled` |  |
| `preferred` |  |
| `required` |  |
| `skip-verify` |  |


#### MySQLUserConfig



MySQLUserConfig defines MySQL-specific user configuration



_Appears in:_
- [DatabaseUserSpec](#databaseuserspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `maxQueriesPerHour` _integer_ | MaxQueriesPerHour limits queries per hour (0 = unlimited) | 0 | Minimum: 0 <br /> |
| `maxUpdatesPerHour` _integer_ | MaxUpdatesPerHour limits updates per hour (0 = unlimited) | 0 | Minimum: 0 <br /> |
| `maxConnectionsPerHour` _integer_ | MaxConnectionsPerHour limits connections per hour (0 = unlimited) | 0 | Minimum: 0 <br /> |
| `maxUserConnections` _integer_ | MaxUserConnections limits concurrent connections (0 = unlimited) | 0 | Minimum: 0 <br /> |
| `authPlugin` _[MySQLAuthPlugin](#mysqlauthplugin)_ | AuthPlugin specifies the authentication plugin | caching_sha2_password | Enum: [mysql_native_password caching_sha2_password sha256_password] <br /> |
| `requireSSL` _boolean_ | RequireSSL requires SSL for connections |  | Optional: \{\} <br /> |
| `requireX509` _boolean_ | RequireX509 requires X509 certificate for connections |  | Optional: \{\} <br /> |
| `allowedHosts` _string array_ | AllowedHosts lists allowed host patterns for the user (e.g., "%", "localhost", "192.168.1.%") | [%] |  |
| `accountLocked` _boolean_ | AccountLocked locks the account |  | Optional: \{\} <br /> |
| `failedLoginAttempts` _integer_ | FailedLoginAttempts sets failed login attempts before locking (0 = disabled) | 0 | Minimum: 0 <br /> |
| `passwordLockTime` _integer_ | PasswordLockTime sets lock time in days after failed attempts (0 = permanent) | 0 | Minimum: 0 <br /> |


#### PVCStorageConfig



PVCStorageConfig defines PVC-based storage configuration



_Appears in:_
- [StorageConfig](#storageconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `claimName` _string_ | ClaimName is the name of the PersistentVolumeClaim |  | Required: \{\} <br /> |
| `subPath` _string_ | SubPath within the PVC |  | Optional: \{\} <br /> |


#### PasswordConfig



PasswordConfig defines password generation settings



_Appears in:_
- [DatabaseUserSpec](#databaseuserspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `generate` _boolean_ | Generate enables password generation | true |  |
| `length` _integer_ | Length of the generated password (default: 32) | 32 | Maximum: 128 <br />Minimum: 8 <br /> |
| `includeSpecialChars` _boolean_ | IncludeSpecialChars includes special characters in the password | true |  |
| `excludeChars` _string_ | ExcludeChars specifies characters to exclude from the password |  | Optional: \{\} <br /> |
| `secretName` _string_ | SecretName is the name of the generated secret |  | Required: \{\} <br /> |
| `secretNamespace` _string_ | SecretNamespace is the namespace for the generated secret (defaults to resource namespace) |  | Optional: \{\} <br /> |
| `format` _[SecretFormat](#secretformat)_ | Format specifies the secret format | kubernetes | Enum: [kubernetes vault external-secrets] <br /> |
| `secretTemplate` _[SecretTemplate](#secrettemplate)_ | SecretTemplate defines the secret template |  | Optional: \{\} <br /> |


#### PasswordRotationConfig



PasswordRotationConfig defines password rotation settings



_Appears in:_
- [DatabaseUserSpec](#databaseuserspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled enables automatic password rotation |  | Optional: \{\} <br /> |
| `schedule` _string_ | Schedule is a cron expression for rotation (e.g., "0 0 1 * *" for monthly) |  | Optional: \{\} <br /> |
| `maxAge` _string_ | MaxAge is the maximum age of a password before rotation (e.g., "90d") |  | Optional: \{\} <br /> |


#### Phase

_Underlying type:_ _string_

Phase represents the current state of a resource



_Appears in:_
- [DatabaseBackupScheduleStatus](#databasebackupschedulestatus)
- [DatabaseBackupStatus](#databasebackupstatus)
- [DatabaseGrantStatus](#databasegrantstatus)
- [DatabaseInstanceStatus](#databaseinstancestatus)
- [DatabaseRestoreStatus](#databaserestorestatus)
- [DatabaseRoleStatus](#databaserolestatus)
- [DatabaseStatus](#databasestatus)
- [DatabaseUserStatus](#databaseuserstatus)

| Field | Description |
| --- | --- |
| `Pending` |  |
| `Creating` |  |
| `Ready` |  |
| `Failed` |  |
| `Deleting` |  |
| `Running` |  |
| `Completed` |  |
| `Paused` |  |
| `Active` |  |


#### PostgresBackupConfig



PostgresBackupConfig defines PostgreSQL-specific backup configuration



_Appears in:_
- [DatabaseBackupSpec](#databasebackupspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `method` _[PostgresBackupMethod](#postgresbackupmethod)_ | Method specifies the backup method | pg_dump | Enum: [pg_dump pg_basebackup] <br /> |
| `format` _[PostgresDumpFormat](#postgresdumpformat)_ | Format specifies the output format (for pg_dump) | custom | Enum: [plain custom directory tar] <br /> |
| `jobs` _integer_ | Jobs sets the number of parallel jobs (for directory format) | 1 | Minimum: 1 <br /> |
| `dataOnly` _boolean_ | DataOnly backs up only data, not schema |  | Optional: \{\} <br /> |
| `schemaOnly` _boolean_ | SchemaOnly backs up only schema, not data |  | Optional: \{\} <br /> |
| `blobs` _boolean_ | Blobs includes large objects in the backup | true |  |
| `noOwner` _boolean_ | NoOwner omits ownership information |  | Optional: \{\} <br /> |
| `noPrivileges` _boolean_ | NoPrivileges omits privilege (GRANT/REVOKE) information |  | Optional: \{\} <br /> |
| `schemas` _string array_ | Schemas lists specific schemas to include (empty = all) |  | Optional: \{\} <br /> |
| `excludeSchemas` _string array_ | ExcludeSchemas lists schemas to exclude |  | Optional: \{\} <br /> |
| `tables` _string array_ | Tables lists specific tables to include (empty = all) |  | Optional: \{\} <br /> |
| `excludeTables` _string array_ | ExcludeTables lists tables to exclude (format: schema.table) |  | Optional: \{\} <br /> |
| `lockWaitTimeout` _string_ | LockWaitTimeout sets the lock wait timeout (e.g., "60s") | 60s |  |
| `noSync` _boolean_ | NoSync disables fsync after backup |  | Optional: \{\} <br /> |


#### PostgresBackupMethod

_Underlying type:_ _string_

PostgresBackupMethod defines PostgreSQL backup methods

_Validation:_
- Enum: [pg_dump pg_basebackup]

_Appears in:_
- [PostgresBackupConfig](#postgresbackupconfig)

| Field | Description |
| --- | --- |
| `pg_dump` |  |
| `pg_basebackup` |  |


#### PostgresDatabaseConfig



PostgresDatabaseConfig defines PostgreSQL-specific database configuration



_Appears in:_
- [DatabaseSpec](#databasespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `encoding` _string_ | Encoding sets the database encoding (default: UTF8) | UTF8 |  |
| `lcCollate` _string_ | LCCollate sets the collation order |  | Optional: \{\} <br /> |
| `lcCtype` _string_ | LCCtype sets the character classification |  | Optional: \{\} <br /> |
| `tablespace` _string_ | Tablespace sets the default tablespace | pg_default |  |
| `template` _string_ | Template is the template database to use | template0 |  |
| `connectionLimit` _integer_ | ConnectionLimit sets the maximum concurrent connections (-1 = unlimited) | -1 | Minimum: -1 <br /> |
| `isTemplate` _boolean_ | IsTemplate marks this as a template database |  | Optional: \{\} <br /> |
| `allowConnections` _boolean_ | AllowConnections allows/disallows connections to this database | true |  |
| `extensions` _[PostgresExtension](#postgresextension) array_ | Extensions to install in the database |  | Optional: \{\} <br /> |
| `schemas` _[PostgresSchema](#postgresschema) array_ | Schemas to create in the database |  | Optional: \{\} <br /> |
| `defaultPrivileges` _[PostgresDefaultPrivilege](#postgresdefaultprivilege) array_ | DefaultPrivileges sets default privileges for new objects |  | Optional: \{\} <br /> |


#### PostgresDatabaseStatus



PostgresDatabaseStatus contains PostgreSQL-specific database status



_Appears in:_
- [DatabaseStatus](#databasestatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `encoding` _string_ | Encoding is the database encoding |  |  |
| `collation` _string_ | Collation is the database collation |  |  |
| `installedExtensions` _[PostgresExtensionStatus](#postgresextensionstatus) array_ | InstalledExtensions lists installed extensions |  |  |
| `schemas` _string array_ | Schemas lists schemas in the database |  |  |


#### PostgresDefaultPrivilege



PostgresDefaultPrivilege defines default privileges for new objects



_Appears in:_
- [PostgresDatabaseConfig](#postgresdatabaseconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `role` _string_ | Role to grant privileges to |  | Required: \{\} <br /> |
| `schema` _string_ | Schema where the default applies |  | Required: \{\} <br /> |
| `objectType` _string_ | ObjectType is the type of objects (tables, sequences, functions, types) |  | Enum: [tables sequences functions types] <br /> |
| `privileges` _string array_ | Privileges to grant |  | MinItems: 1 <br /> |


#### PostgresDefaultPrivilegeGrant



PostgresDefaultPrivilegeGrant defines a default privilege grant



_Appears in:_
- [PostgresGrantConfig](#postgresgrantconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `database` _string_ | Database is the target database |  | Required: \{\} <br /> |
| `schema` _string_ | Schema is the target schema |  | Required: \{\} <br /> |
| `grantedBy` _string_ | GrantedBy is the role that creates the objects |  | Required: \{\} <br /> |
| `objectType` _string_ | ObjectType is the type of objects (tables, sequences, functions, types) |  | Enum: [tables sequences functions types] <br /> |
| `privileges` _string array_ | Privileges to grant |  | MinItems: 1 <br /> |


#### PostgresDumpFormat

_Underlying type:_ _string_

PostgresDumpFormat defines pg_dump output formats

_Validation:_
- Enum: [plain custom directory tar]

_Appears in:_
- [PostgresBackupConfig](#postgresbackupconfig)

| Field | Description |
| --- | --- |
| `plain` |  |
| `custom` |  |
| `directory` |  |
| `tar` |  |


#### PostgresExtension



PostgresExtension defines a PostgreSQL extension to install



_Appears in:_
- [PostgresDatabaseConfig](#postgresdatabaseconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the extension |  | Required: \{\} <br /> |
| `schema` _string_ | Schema to install the extension in (default: public) | public |  |
| `version` _string_ | Version of the extension (optional, uses default if not specified) |  | Optional: \{\} <br /> |


#### PostgresExtensionStatus



PostgresExtensionStatus contains extension status information



_Appears in:_
- [PostgresDatabaseStatus](#postgresdatabasestatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the extension |  |  |
| `version` _string_ | Version of the extension |  |  |


#### PostgresGrant



PostgresGrant defines a PostgreSQL privilege grant



_Appears in:_
- [PostgresGrantConfig](#postgresgrantconfig)
- [PostgresRoleConfig](#postgresroleconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `database` _string_ | Database is the target database |  | Required: \{\} <br /> |
| `schema` _string_ | Schema is the target schema (optional, for schema-level grants) |  | Optional: \{\} <br /> |
| `tables` _string array_ | Tables lists specific tables or "*" for all tables |  | Optional: \{\} <br /> |
| `sequences` _string array_ | Sequences lists specific sequences or "*" for all sequences |  | Optional: \{\} <br /> |
| `functions` _string array_ | Functions lists specific functions or "*" for all functions |  | Optional: \{\} <br /> |
| `privileges` _string array_ | Privileges to grant (SELECT, INSERT, UPDATE, DELETE, TRUNCATE, REFERENCES, TRIGGER, CREATE, CONNECT, TEMPORARY, EXECUTE, USAGE) |  | MinItems: 1 <br /> |
| `withGrantOption` _boolean_ | WithGrantOption allows the grantee to grant these privileges to others |  | Optional: \{\} <br /> |


#### PostgresGrantConfig



PostgresGrantConfig defines PostgreSQL-specific grant configuration



_Appears in:_
- [DatabaseGrantSpec](#databasegrantspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `roles` _string array_ | Roles to assign to the user |  | Optional: \{\} <br /> |
| `grants` _[PostgresGrant](#postgresgrant) array_ | Grants defines direct privilege grants |  | Optional: \{\} <br /> |
| `defaultPrivileges` _[PostgresDefaultPrivilegeGrant](#postgresdefaultprivilegegrant) array_ | DefaultPrivileges sets default privileges for future objects |  | Optional: \{\} <br /> |


#### PostgresInstanceConfig



PostgresInstanceConfig defines PostgreSQL-specific instance configuration



_Appears in:_
- [DatabaseInstanceSpec](#databaseinstancespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `sslMode` _[PostgresSSLMode](#postgressslmode)_ | SSLMode specifies the SSL mode for connections | prefer | Enum: [disable allow prefer require verify-ca verify-full] <br /> |
| `connectTimeout` _integer_ | ConnectTimeout is the connection timeout in seconds | 10 | Minimum: 1 <br /> |
| `statementTimeout` _string_ | StatementTimeout is the statement timeout (e.g., "30s") |  | Optional: \{\} <br /> |
| `applicationName` _string_ | ApplicationName is the application name for connections | db-provision-operator |  |


#### PostgresRestoreConfig



PostgresRestoreConfig defines PostgreSQL-specific restore configuration



_Appears in:_
- [DatabaseRestoreSpec](#databaserestorespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `dropExisting` _boolean_ | DropExisting drops existing database before restore |  | Optional: \{\} <br /> |
| `createDatabase` _boolean_ | CreateDatabase creates the database if it doesn't exist | true |  |
| `dataOnly` _boolean_ | DataOnly restores only data, not schema |  | Optional: \{\} <br /> |
| `schemaOnly` _boolean_ | SchemaOnly restores only schema, not data |  | Optional: \{\} <br /> |
| `noOwner` _boolean_ | NoOwner omits ownership restoration | true |  |
| `noPrivileges` _boolean_ | NoPrivileges omits privilege restoration |  | Optional: \{\} <br /> |
| `roleMapping` _object (keys:string, values:string)_ | RoleMapping maps old role names to new role names |  | Optional: \{\} <br /> |
| `schemas` _string array_ | Schemas lists specific schemas to restore (empty = all) |  | Optional: \{\} <br /> |
| `tables` _string array_ | Tables lists specific tables to restore (empty = all) |  | Optional: \{\} <br /> |
| `jobs` _integer_ | Jobs sets the number of parallel jobs for restore | 1 | Minimum: 1 <br /> |
| `disableTriggers` _boolean_ | DisableTriggers disables triggers during restore |  | Optional: \{\} <br /> |
| `analyze` _boolean_ | Analyze runs ANALYZE after restore | true |  |


#### PostgresRoleConfig



PostgresRoleConfig defines PostgreSQL-specific role configuration



_Appears in:_
- [DatabaseRoleSpec](#databaserolespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `login` _boolean_ | Login enables login capability (usually false for group roles) |  | Optional: \{\} <br /> |
| `inherit` _boolean_ | Inherit enables privilege inheritance | true |  |
| `createDB` _boolean_ | CreateDB allows the role to create databases |  | Optional: \{\} <br /> |
| `createRole` _boolean_ | CreateRole allows the role to create other roles |  | Optional: \{\} <br /> |
| `superuser` _boolean_ | Superuser grants superuser privileges |  | Optional: \{\} <br /> |
| `replication` _boolean_ | Replication enables replication privileges |  | Optional: \{\} <br /> |
| `bypassRLS` _boolean_ | BypassRLS allows bypassing row-level security |  | Optional: \{\} <br /> |
| `inRoles` _string array_ | InRoles lists roles this role should inherit from |  | Optional: \{\} <br /> |
| `grants` _[PostgresGrant](#postgresgrant) array_ | Grants defines the permissions this role grants |  | Optional: \{\} <br /> |


#### PostgresSSLMode

_Underlying type:_ _string_

PostgresSSLMode defines PostgreSQL SSL modes

_Validation:_
- Enum: [disable allow prefer require verify-ca verify-full]

_Appears in:_
- [PostgresInstanceConfig](#postgresinstanceconfig)

| Field | Description |
| --- | --- |
| `disable` |  |
| `allow` |  |
| `prefer` |  |
| `require` |  |
| `verify-ca` |  |
| `verify-full` |  |


#### PostgresSchema



PostgresSchema defines a schema to create



_Appears in:_
- [PostgresDatabaseConfig](#postgresdatabaseconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the schema |  | Required: \{\} <br /> |
| `owner` _string_ | Owner of the schema (optional) |  | Optional: \{\} <br /> |


#### PostgresUserConfig



PostgresUserConfig defines PostgreSQL-specific user configuration



_Appears in:_
- [DatabaseUserSpec](#databaseuserspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `connectionLimit` _integer_ | ConnectionLimit sets the maximum concurrent connections (-1 = unlimited) | -1 | Minimum: -1 <br /> |
| `validUntil` _string_ | ValidUntil sets the password expiration time (RFC3339 format) |  | Optional: \{\} <br /> |
| `superuser` _boolean_ | Superuser grants superuser privileges |  | Optional: \{\} <br /> |
| `createDB` _boolean_ | CreateDB allows the user to create databases |  | Optional: \{\} <br /> |
| `createRole` _boolean_ | CreateRole allows the user to create roles |  | Optional: \{\} <br /> |
| `inherit` _boolean_ | Inherit enables privilege inheritance | true |  |
| `login` _boolean_ | Login enables login capability | true |  |
| `replication` _boolean_ | Replication enables replication privileges |  | Optional: \{\} <br /> |
| `bypassRLS` _boolean_ | BypassRLS allows bypassing row-level security |  | Optional: \{\} <br /> |
| `inRoles` _string array_ | InRoles lists roles this user should be a member of |  | Optional: \{\} <br /> |
| `configParameters` _object (keys:string, values:string)_ | ConfigParameters sets session parameters for this user |  | Optional: \{\} <br /> |


#### RecentBackupInfo



RecentBackupInfo contains information about a recent backup



_Appears in:_
- [DatabaseBackupScheduleStatus](#databasebackupschedulestatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the backup |  |  |
| `status` _string_ | Status of the backup |  |  |


#### RestoreConfirmation



RestoreConfirmation contains safety confirmations



_Appears in:_
- [DatabaseRestoreSpec](#databaserestorespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `acknowledgeDataLoss` _string_ | AcknowledgeDataLoss must be set to "I-UNDERSTAND-DATA-LOSS" for destructive operations |  | Optional: \{\} <br /> |


#### RestoreFromPath



RestoreFromPath defines restoring from a direct path



_Appears in:_
- [DatabaseRestoreSpec](#databaserestorespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `storage` _[StorageConfig](#storageconfig)_ | Storage defines where the backup is stored |  | Required: \{\} <br /> |
| `backupPath` _string_ | BackupPath is the path to the backup file within the storage |  | Required: \{\} <br /> |
| `compression` _[CompressionConfig](#compressionconfig)_ | Compression settings used for the backup |  | Optional: \{\} <br /> |
| `encryption` _[EncryptionConfig](#encryptionconfig)_ | Encryption settings used for the backup |  | Optional: \{\} <br /> |


#### RestoreInfo



RestoreInfo contains restore-specific information



_Appears in:_
- [DatabaseRestoreStatus](#databaserestorestatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `sourceBackup` _string_ | SourceBackup is the source backup name |  |  |
| `targetInstance` _string_ | TargetInstance is the target instance name |  |  |
| `targetDatabase` _string_ | TargetDatabase is the target database name |  |  |


#### RestoreProgress



RestoreProgress contains restore progress information



_Appears in:_
- [DatabaseRestoreStatus](#databaserestorestatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `percentage` _integer_ | Percentage is the restore progress percentage (0-100) |  |  |
| `currentPhase` _string_ | CurrentPhase is the current restore phase |  |  |
| `tablesRestored` _integer_ | TablesRestored is the number of tables restored |  |  |
| `tablesTotal` _integer_ | TablesTotal is the total number of tables to restore |  |  |


#### RestoreTarget



RestoreTarget defines where to restore the backup



_Appears in:_
- [DatabaseRestoreSpec](#databaserestorespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `instanceRef` _[InstanceReference](#instancereference)_ | InstanceRef references the target DatabaseInstance |  | Optional: \{\} <br /> |
| `databaseName` _string_ | DatabaseName is the target database name (for restore to new database) |  | Optional: \{\} <br /> |
| `inPlace` _boolean_ | InPlace enables in-place restore (destructive!) |  | Optional: \{\} <br /> |
| `databaseRef` _[DatabaseReference](#databasereference)_ | DatabaseRef references the target Database for in-place restore |  | Optional: \{\} <br /> |


#### RetentionPolicy



RetentionPolicy defines backup retention settings



_Appears in:_
- [DatabaseBackupScheduleSpec](#databasebackupschedulespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `keepLast` _integer_ | KeepLast keeps the N most recent backups |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `keepHourly` _integer_ | KeepHourly keeps N hourly backups |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `keepDaily` _integer_ | KeepDaily keeps N daily backups |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `keepWeekly` _integer_ | KeepWeekly keeps N weekly backups |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `keepMonthly` _integer_ | KeepMonthly keeps N monthly backups |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `keepYearly` _integer_ | KeepYearly keeps N yearly backups |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `minAge` _string_ | MinAge is the minimum age before a backup can be deleted |  | Optional: \{\} <br /> |


#### RoleInfo



RoleInfo contains role status information



_Appears in:_
- [DatabaseRoleStatus](#databaserolestatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the actual role name |  |  |
| `createdAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#time-v1-meta)_ | CreatedAt is the role creation timestamp |  |  |


#### S3SecretKeys



S3SecretKeys defines the key names within an S3 secret



_Appears in:_
- [S3SecretRef](#s3secretref)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `accessKey` _string_ | Access key ID (default: "AWS_ACCESS_KEY_ID") | AWS_ACCESS_KEY_ID |  |
| `secretKey` _string_ | Secret access key (default: "AWS_SECRET_ACCESS_KEY") | AWS_SECRET_ACCESS_KEY |  |


#### S3SecretRef



S3SecretRef references S3 credentials in a secret



_Appears in:_
- [S3StorageConfig](#s3storageconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the secret |  | Required: \{\} <br /> |
| `keys` _[S3SecretKeys](#s3secretkeys)_ | Keys defines the key names for S3 credentials |  | Optional: \{\} <br /> |


#### S3StorageConfig



S3StorageConfig defines S3-compatible storage configuration



_Appears in:_
- [StorageConfig](#storageconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `bucket` _string_ | Bucket name |  | Required: \{\} <br /> |
| `region` _string_ | Region of the S3 bucket |  | Required: \{\} <br /> |
| `prefix` _string_ | Prefix (path prefix within the bucket) |  | Optional: \{\} <br /> |
| `endpoint` _string_ | Endpoint for S3-compatible storage (e.g., MinIO) |  | Optional: \{\} <br /> |
| `secretRef` _[S3SecretRef](#s3secretref)_ | SecretRef references a secret containing S3 credentials |  | Required: \{\} <br /> |
| `forcePathStyle` _boolean_ | ForcePathStyle enables path-style addressing (required for MinIO) |  | Optional: \{\} <br /> |


#### ScheduledBackupInfo



ScheduledBackupInfo contains information about a scheduled backup



_Appears in:_
- [DatabaseBackupScheduleStatus](#databasebackupschedulestatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the backup |  |  |
| `status` _string_ | Status of the backup |  |  |
| `startedAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#time-v1-meta)_ | StartedAt is when the backup started |  |  |
| `completedAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#time-v1-meta)_ | CompletedAt is when the backup completed |  |  |


#### SecretFormat

_Underlying type:_ _string_

SecretFormat defines the output secret format

_Validation:_
- Enum: [kubernetes vault external-secrets]

_Appears in:_
- [PasswordConfig](#passwordconfig)

| Field | Description |
| --- | --- |
| `kubernetes` |  |
| `vault` |  |
| `external-secrets` |  |


#### SecretInfo



SecretInfo contains generated secret information



_Appears in:_
- [DatabaseUserStatus](#databaseuserstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the secret name |  |  |
| `namespace` _string_ | Namespace is the secret namespace |  |  |
| `lastRotatedAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#time-v1-meta)_ | LastRotatedAt is the last password rotation timestamp |  |  |


#### SecretKeySelector



SecretKeySelector contains a reference to a secret key



_Appears in:_
- [EncryptionConfig](#encryptionconfig)
- [GCSStorageConfig](#gcsstorageconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the secret |  | Required: \{\} <br /> |
| `namespace` _string_ | Namespace of the secret (defaults to the resource namespace) |  | Optional: \{\} <br /> |
| `key` _string_ | Key within the secret |  | Required: \{\} <br /> |


#### SecretReference



SecretReference contains a reference to a secret with multiple keys



_Appears in:_
- [AzureStorageConfig](#azurestorageconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the secret |  | Required: \{\} <br /> |
| `namespace` _string_ | Namespace of the secret (defaults to the resource namespace) |  | Optional: \{\} <br /> |


#### SecretTemplate



SecretTemplate defines the template for generated secrets



_Appears in:_
- [PasswordConfig](#passwordconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _[SecretType](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#secrettype-v1-core)_ | Type is the secret type (default: Opaque) | Opaque |  |
| `labels` _object (keys:string, values:string)_ | Labels to add to the secret |  | Optional: \{\} <br /> |
| `annotations` _object (keys:string, values:string)_ | Annotations to add to the secret |  | Optional: \{\} <br /> |
| `data` _object (keys:string, values:string)_ | Data defines templated data keys<br />Available variables: .Username, .Password, .Host, .Port, .Database, .SSLMode |  | Optional: \{\} <br /> |


#### StorageConfig



StorageConfig defines backup storage configuration



_Appears in:_
- [DatabaseBackupSpec](#databasebackupspec)
- [RestoreFromPath](#restorefrompath)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _[StorageType](#storagetype)_ | Type of storage backend |  | Enum: [gcs s3 azure pvc] <br />Required: \{\} <br /> |
| `gcs` _[GCSStorageConfig](#gcsstorageconfig)_ | GCS configuration (required when type is "gcs") |  | Optional: \{\} <br /> |
| `s3` _[S3StorageConfig](#s3storageconfig)_ | S3 configuration (required when type is "s3") |  | Optional: \{\} <br /> |
| `azure` _[AzureStorageConfig](#azurestorageconfig)_ | Azure configuration (required when type is "azure") |  | Optional: \{\} <br /> |
| `pvc` _[PVCStorageConfig](#pvcstorageconfig)_ | PVC configuration (required when type is "pvc") |  | Optional: \{\} <br /> |


#### StorageType

_Underlying type:_ _string_

StorageType defines the backup storage type

_Validation:_
- Enum: [gcs s3 azure pvc]

_Appears in:_
- [StorageConfig](#storageconfig)

| Field | Description |
| --- | --- |
| `gcs` |  |
| `s3` |  |
| `azure` |  |
| `pvc` |  |


#### TLSConfig



TLSConfig defines TLS configuration for database connections



_Appears in:_
- [DatabaseInstanceSpec](#databaseinstancespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled enables TLS for connections |  | Optional: \{\} <br /> |
| `mode` _string_ | Mode specifies the TLS verification mode | disable | Enum: [disable require verify-ca verify-full] <br /> |
| `secretRef` _[TLSSecretRef](#tlssecretref)_ | SecretRef references a secret containing TLS certificates |  | Optional: \{\} <br /> |


#### TLSKeys



TLSKeys defines the key names within a TLS secret



_Appears in:_
- [TLSSecretRef](#tlssecretref)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `ca` _string_ | CA certificate key (default: "ca.crt") | ca.crt |  |
| `cert` _string_ | Client certificate key for mTLS (optional) |  | Optional: \{\} <br /> |
| `key` _string_ | Client key for mTLS (optional) |  | Optional: \{\} <br /> |


#### TLSSecretRef



TLSSecretRef references TLS certificates in a secret



_Appears in:_
- [TLSConfig](#tlsconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the secret containing TLS certificates |  | Required: \{\} <br /> |
| `keys` _[TLSKeys](#tlskeys)_ | Keys defines the key names for TLS certificates |  | Optional: \{\} <br /> |


#### UserInfo



UserInfo contains user status information



_Appears in:_
- [DatabaseUserStatus](#databaseuserstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `username` _string_ | Username is the actual database username |  |  |
| `createdAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#time-v1-meta)_ | CreatedAt is the user creation timestamp |  |  |


#### UserReference



UserReference references a DatabaseUser



_Appears in:_
- [DatabaseGrantSpec](#databasegrantspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the DatabaseUser resource |  | Required: \{\} <br /> |
| `namespace` _string_ | Namespace of the DatabaseUser (defaults to the resource namespace) |  | Optional: \{\} <br /> |
