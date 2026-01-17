# Unified DB Management Operator

## Project Overview

A Kubernetes operator that provides unified database management across multiple database engines. The operator manages logical databases, users, roles, permissions, backups, and restores through Kubernetes Custom Resource Definitions (CRDs).

### Supported Engines

| Engine | Status | Version Support |
|--------|--------|-----------------|
| PostgreSQL | ✅ Phase 1 | 12, 13, 14, 15, 16 |
| MySQL | ✅ Phase 1 | 5.7, 8.0, 8.4 |
| CockroachDB | 🔜 Future | TBD |

### Core Features

- **Database CRUD**: Create, update, delete logical databases
- **User Management**: Create users with auto-generated or imported passwords
- **Role Management**: Define reusable permission sets
- **Permission Management**: Grant/revoke privileges with fine-grained control
- **Backup & Restore**: Velero-style backup scheduling and point-in-time recovery
- **Secret Management**: Flexible secret generation with templating support
- **Multi-format Secrets**: Kubernetes native, Vault, External Secrets Operator

### Design Principles

1. **Engine-Specific Sections**: Each CRD uses dedicated sections (`postgres:`, `mysql:`) for engine-specific configuration. Only ONE section should be set per resource.
2. **Velero-Style Backups**: Separate `DatabaseBackup` (point-in-time) and `DatabaseBackupSchedule` (recurring) resources.
3. **Safety First**: Deletion protection, force-delete confirmation, dry-run support.
4. **GitOps Ready**: Designed for ArgoCD/Flux with proper sync waves and finalizers.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         Unified DB Operator                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐             │
│  │    PostgreSQL   │  │      MySQL      │  │   CockroachDB   │             │
│  │     Adapter     │  │     Adapter     │  │     Adapter     │  (future)   │
│  │                 │  │                 │  │                 │             │
│  │  - pg_dump      │  │  - mysqldump    │  │  - cockroach    │             │
│  │  - pgx driver   │  │  - go-sql-driver│  │    dump         │             │
│  └────────┬────────┘  └────────┬────────┘  └────────┬────────┘             │
│           │                    │                    │                       │
│           └────────────────────┼────────────────────┘                       │
│                                │                                            │
│                    ┌───────────▼───────────┐                               │
│                    │   Adapter Interface   │                               │
│                    │                       │                               │
│                    │ - CreateDatabase()    │                               │
│                    │ - DropDatabase()      │                               │
│                    │ - CreateUser()        │                               │
│                    │ - SetPermissions()    │                               │
│                    │ - Backup()            │                               │
│                    │ - Restore()           │                               │
│                    └───────────┬───────────┘                               │
│                                │                                            │
│  ┌─────────────────────────────┼─────────────────────────────┐             │
│  │                   Controllers                              │             │
│  │  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐       │             │
│  │  │ Instance     │ │  Database    │ │    User      │       │             │
│  │  │ Controller   │ │  Controller  │ │  Controller  │       │             │
│  │  └──────────────┘ └──────────────┘ └──────────────┘       │             │
│  │  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐       │             │
│  │  │    Role      │ │    Grant     │ │   Backup     │       │             │
│  │  │ Controller   │ │  Controller  │ │  Controller  │       │             │
│  │  └──────────────┘ └──────────────┘ └──────────────┘       │             │
│  │  ┌──────────────┐ ┌──────────────┐                        │             │
│  │  │  Schedule    │ │   Restore    │                        │             │
│  │  │ Controller   │ │  Controller  │                        │             │
│  │  └──────────────┘ └──────────────┘                        │             │
│  └───────────────────────────────────────────────────────────┘             │
│                                │                                            │
│  ┌─────────────────────────────┼─────────────────────────────┐             │
│  │                  Shared Services                           │             │
│  │  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐       │             │
│  │  │   Secret     │ │   Storage    │ │   Metrics    │       │             │
│  │  │   Manager    │ │   Manager    │ │   Exporter   │       │             │
│  │  └──────────────┘ └──────────────┘ └──────────────┘       │             │
│  └───────────────────────────────────────────────────────────┘             │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Tech Stack

| Component | Choice | Rationale |
|-----------|--------|-----------|
| Framework | Kubebuilder v4 | Standard K8s operator framework |
| Language | Go 1.22+ | K8s native, performance |
| PostgreSQL Driver | pgx/v5 | Modern, performant, full feature support |
| MySQL Driver | go-sql-driver/mysql | Mature, well-maintained |
| Testing | Ginkgo/Gomega + Testcontainers | BDD style + real DB testing |
| Metrics | controller-runtime/metrics | Built-in Prometheus metrics |
| Logging | zap (via controller-runtime) | Structured logging |

---

## Project Structure

```
unified-db-operator/
├── api/
│   └── v1alpha1/
│       ├── databaseinstance_types.go
│       ├── database_types.go
│       ├── databaseuser_types.go
│       ├── databaserole_types.go
│       ├── databasegrant_types.go
│       ├── databasebackup_types.go
│       ├── databasebackupschedule_types.go
│       ├── databaserestore_types.go
│       ├── common_types.go          # Shared types (storage, TLS, etc.)
│       ├── postgres_types.go        # PostgreSQL-specific types
│       ├── mysql_types.go           # MySQL-specific types
│       ├── groupversion_info.go
│       └── zz_generated.deepcopy.go
├── cmd/
│   └── main.go
├── config/
│   ├── crd/
│   │   └── bases/
│   ├── manager/
│   ├── rbac/
│   └── samples/
├── internal/
│   ├── controller/
│   │   ├── databaseinstance_controller.go
│   │   ├── databaseinstance_controller_test.go
│   │   ├── database_controller.go
│   │   ├── database_controller_test.go
│   │   ├── databaseuser_controller.go
│   │   ├── databaseuser_controller_test.go
│   │   ├── databaserole_controller.go
│   │   ├── databaserole_controller_test.go
│   │   ├── databasegrant_controller.go
│   │   ├── databasegrant_controller_test.go
│   │   ├── databasebackup_controller.go
│   │   ├── databasebackup_controller_test.go
│   │   ├── databasebackupschedule_controller.go
│   │   ├── databasebackupschedule_controller_test.go
│   │   ├── databaserestore_controller.go
│   │   ├── databaserestore_controller_test.go
│   │   └── suite_test.go
│   ├── adapter/
│   │   ├── interface.go             # Adapter interface definition
│   │   ├── factory.go               # Adapter factory
│   │   ├── postgres/
│   │   │   ├── adapter.go
│   │   │   ├── adapter_test.go
│   │   │   ├── database.go
│   │   │   ├── database_test.go
│   │   │   ├── user.go
│   │   │   ├── user_test.go
│   │   │   ├── role.go
│   │   │   ├── role_test.go
│   │   │   ├── grant.go
│   │   │   ├── grant_test.go
│   │   │   ├── backup.go
│   │   │   ├── backup_test.go
│   │   │   ├── restore.go
│   │   │   └── restore_test.go
│   │   └── mysql/
│   │       ├── adapter.go
│   │       ├── adapter_test.go
│   │       ├── database.go
│   │       ├── database_test.go
│   │       ├── user.go
│   │       ├── user_test.go
│   │       ├── role.go
│   │       ├── role_test.go
│   │       ├── grant.go
│   │       ├── grant_test.go
│   │       ├── backup.go
│   │       ├── backup_test.go
│   │       ├── restore.go
│   │       └── restore_test.go
│   ├── secret/
│   │   ├── manager.go
│   │   ├── manager_test.go
│   │   ├── generator.go
│   │   ├── generator_test.go
│   │   ├── template.go
│   │   └── template_test.go
│   ├── storage/
│   │   ├── interface.go
│   │   ├── gcs.go
│   │   ├── gcs_test.go
│   │   ├── s3.go
│   │   ├── s3_test.go
│   │   └── pvc.go
│   └── utils/
│       ├── conditions.go
│       ├── conditions_test.go
│       ├── finalizers.go
│       └── finalizers_test.go
├── test/
│   ├── e2e/
│   │   └── ...
│   └── integration/
│       ├── postgres_test.go
│       ├── mysql_test.go
│       └── suite_test.go
├── Dockerfile
├── Makefile
├── go.mod
├── go.sum
└── README.md
```

---

## CRD Specifications

### 1. DatabaseInstance

Represents a connection to an existing database server.

```yaml
apiVersion: dbops.example.com/v1alpha1
kind: DatabaseInstance
metadata:
  name: postgres-primary
spec:
  # Engine type (required, immutable)
  engine: postgres            # postgres | mysql | cockroachdb

  # Connection configuration
  connection:
    host: postgres.database.svc
    port: 5432
    database: postgres        # Admin database for connection

    # Credentials from secret
    secretRef:
      name: postgres-admin-creds
      keys:
        username: username    # Key in secret (default: username)
        password: password    # Key in secret (default: password)

    # OR import existing secret with custom keys
    # existingSecret:
    #   name: legacy-db-secret
    #   keys:
    #     username: PGUSER
    #     password: PGPASSWORD

  # TLS configuration
  tls:
    enabled: true
    mode: verify-full         # disable | require | verify-ca | verify-full
    secretRef:
      name: postgres-tls
      keys:
        ca: ca.crt
        cert: tls.crt         # For mTLS (optional)
        key: tls.key          # For mTLS (optional)

  # Health check configuration
  healthCheck:
    enabled: true
    intervalSeconds: 30
    timeoutSeconds: 5

  #-----------------------------------------
  # Engine-specific options (ONE OF)
  #-----------------------------------------

  postgres:
    sslMode: verify-full      # disable | allow | prefer | require | verify-ca | verify-full
    connectTimeout: 10
    statementTimeout: 30s
    applicationName: dbops-operator

  # --- OR ---

  mysql:
    charset: utf8mb4
    collation: utf8mb4_unicode_ci
    parseTime: true
    timeout: 10s
    readTimeout: 30s
    writeTimeout: 30s
    tls: preferred            # disabled | preferred | required | skip-verify

status:
  phase: Ready                # Pending | Ready | Failed
  version: "15.4"
  message: ""
  lastCheckedAt: "2025-01-05T00:00:00Z"
  conditions:
    - type: Connected
      status: "True"
      lastTransitionTime: "2025-01-05T00:00:00Z"
      reason: ConnectionSuccessful
      message: "Successfully connected to database server"
    - type: TLSVerified
      status: "True"
      lastTransitionTime: "2025-01-05T00:00:00Z"
      reason: CertificateValid
```

### 2. Database

Represents a logical database within an instance.

```yaml
apiVersion: dbops.example.com/v1alpha1
kind: Database
metadata:
  name: myapp-db
  annotations:
    # Force delete even with active connections
    dbops.example.com/force-delete: "true"
    dbops.example.com/force-delete-confirm: "I-UNDERSTAND-DATA-LOSS"
spec:
  # Reference to DatabaseInstance
  instanceRef:
    name: postgres-primary

  # Database name (immutable after creation)
  name: myapp_production

  # What happens on CR deletion
  deletionPolicy: Retain      # Retain | Delete | Snapshot

  # Prevent accidental deletion
  deletionProtection: true

  #-----------------------------------------
  # Engine-specific options (ONE OF)
  #-----------------------------------------

  postgres:
    encoding: UTF8
    lcCollate: en_US.UTF-8
    lcCtype: en_US.UTF-8
    tablespace: pg_default
    template: template0
    connectionLimit: -1       # -1 = unlimited
    isTemplate: false
    allowConnections: true

    # Extensions to install
    extensions:
      - name: uuid-ossp
        schema: public
      - name: pg_trgm
        schema: public
      - name: postgis
        version: "3.4"
        schema: public

    # Schemas to create
    schemas:
      - name: app
        owner: myapp_user
      - name: analytics
      - name: audit

    # Default privileges for new objects
    defaultPrivileges:
      - role: myapp_user
        schema: app
        objectType: tables    # tables | sequences | functions | types
        privileges: [SELECT, INSERT, UPDATE, DELETE]

  # --- OR ---

  mysql:
    charset: utf8mb4
    collation: utf8mb4_unicode_ci
    sqlMode: STRICT_TRANS_TABLES,NO_ENGINE_SUBSTITUTION,NO_ZERO_DATE
    defaultStorageEngine: InnoDB

  # --- OR ---

  cockroachdb:
    primaryRegion: asia-southeast1
    regions:
      - asia-southeast1
      - asia-east1
    survivalGoal: zone        # zone | region

status:
  phase: Ready                # Pending | Creating | Ready | Failed | Deleting
  observedGeneration: 1
  message: ""

  database:
    name: myapp_production
    owner: postgres
    sizeBytes: 1073741824

  postgres:
    encoding: UTF8
    collation: en_US.UTF-8
    installedExtensions:
      - name: uuid-ossp
        version: "1.1"
      - name: pg_trgm
        version: "1.6"
    schemas: [public, app, analytics, audit]

  conditions:
    - type: Ready
      status: "True"
      reason: DatabaseReady
    - type: ExtensionsInstalled
      status: "True"
    - type: SchemasCreated
      status: "True"
```

### 3. DatabaseUser

Represents a database user/role with login capability.

```yaml
apiVersion: dbops.example.com/v1alpha1
kind: DatabaseUser
metadata:
  name: myapp-user
spec:
  # Reference to DatabaseInstance
  instanceRef:
    name: postgres-primary

  # Username (immutable after creation)
  username: myapp_svc

  #-----------------------------------------
  # Password / Secret configuration
  #-----------------------------------------

  passwordSecret:
    # Generate new password
    generate: true
    length: 32
    includeSpecialChars: true
    excludeChars: "\"'\\`"

    # Output secret name
    secretName: myapp-db-credentials
    secretNamespace: ""       # Default: same as CR namespace

    # Secret format
    format: kubernetes        # kubernetes | vault | external-secrets

    # Custom secret template
    secretTemplate:
      type: Opaque
      labels:
        app: myapp
        managed-by: dbops-operator
      annotations:
        reloader.stakater.com/match: "true"

      # Templated data keys
      # Available: .Username, .Password, .Host, .Port, .Database, .SSLMode
      data:
        DATABASE_URL: "postgresql://{{ .Username }}:{{ .Password }}@{{ .Host }}:{{ .Port }}/{{ .Database }}?sslmode={{ .SSLMode }}"
        JDBC_URL: "jdbc:postgresql://{{ .Host }}:{{ .Port }}/{{ .Database }}"
        DB_HOST: "{{ .Host }}"
        DB_PORT: "{{ .Port }}"
        DB_NAME: "{{ .Database }}"
        DB_USER: "{{ .Username }}"
        DB_PASSWORD: "{{ .Password }}"

  # OR import existing password from secret
  # existingPasswordSecret:
  #   name: legacy-app-secret
  #   key: db_password

  # Password rotation
  passwordRotation:
    enabled: false
    schedule: "0 0 1 * *"     # Cron: Monthly
    maxAge: 90d

  #-----------------------------------------
  # Engine-specific options (ONE OF)
  #-----------------------------------------

  postgres:
    connectionLimit: 100      # -1 = unlimited
    validUntil: "2025-12-31T23:59:59Z"

    # Role attributes
    superuser: false
    createDB: false
    createRole: false
    inherit: true
    login: true
    replication: false
    bypassRLS: false

    # Member of roles
    inRoles:
      - app_readonly
      - monitoring

    # Session parameters
    configParameters:
      statement_timeout: "30s"
      lock_timeout: "10s"

  # --- OR ---

  mysql:
    # Resource limits
    maxQueriesPerHour: 0      # 0 = unlimited
    maxUpdatesPerHour: 0
    maxConnectionsPerHour: 0
    maxUserConnections: 100

    # Authentication
    authPlugin: caching_sha2_password

    # TLS requirement
    requireSSL: false
    requireX509: false

    # Allowed hosts (user@host)
    allowedHosts:
      - "%"                   # Any host

    # Account locking
    accountLocked: false
    failedLoginAttempts: 0
    passwordLockTime: 0

status:
  phase: Ready
  observedGeneration: 1

  user:
    username: myapp_svc
    createdAt: "2025-01-01T00:00:00Z"

  secret:
    name: myapp-db-credentials
    namespace: default
    lastRotatedAt: "2025-01-01T00:00:00Z"

  conditions:
    - type: Ready
      status: "True"
    - type: SecretCreated
      status: "True"
    - type: PasswordRotated
      status: "True"
```

### 4. DatabaseRole

Represents a reusable permission set (group role).

```yaml
apiVersion: dbops.example.com/v1alpha1
kind: DatabaseRole
metadata:
  name: app-readonly
spec:
  # Reference to DatabaseInstance
  instanceRef:
    name: postgres-primary

  # Role name in database
  roleName: app_readonly

  #-----------------------------------------
  # Engine-specific options (ONE OF)
  #-----------------------------------------

  postgres:
    # Role attributes
    login: false              # Group role, not for login
    inherit: true
    createDB: false
    createRole: false
    superuser: false
    replication: false
    bypassRLS: false

    # Inherit from other roles
    inRoles: []

    # Permissions this role grants
    grants:
      - database: myapp_production
        privileges: [CONNECT]

      - database: myapp_production
        schema: public
        privileges: [USAGE]

      - database: myapp_production
        schema: app
        privileges: [USAGE]

      - database: myapp_production
        schema: public
        tables: ["*"]
        privileges: [SELECT]

      - database: myapp_production
        schema: app
        tables: ["*"]
        privileges: [SELECT]

      - database: myapp_production
        schema: public
        sequences: ["*"]
        privileges: [SELECT]

  # --- OR ---

  mysql:
    # Use native MySQL 8.0+ roles
    useNativeRoles: true

    # Grants for this role
    grants:
      - level: database
        database: myapp_production
        privileges: [SELECT]

      - level: table
        database: myapp_production
        table: config
        privileges: [SELECT]

status:
  phase: Ready
  observedGeneration: 1

  role:
    name: app_readonly
    createdAt: "2025-01-01T00:00:00Z"

  conditions:
    - type: Ready
      status: "True"
    - type: GrantsApplied
      status: "True"
```

### 5. DatabaseGrant

Assigns permissions to a user.

```yaml
apiVersion: dbops.example.com/v1alpha1
kind: DatabaseGrant
metadata:
  name: myapp-user-grants
spec:
  # Target user
  userRef:
    name: myapp-user

  # Target database (for context)
  databaseRef:
    name: myapp-db

  #-----------------------------------------
  # Engine-specific grants (ONE OF)
  #-----------------------------------------

  postgres:
    # Assign roles
    roles:
      - app_readwrite
      - monitoring_readonly

    # Direct grants
    grants:
      - database: myapp_production
        privileges: [CREATE]

      - database: myapp_production
        schema: app
        privileges: [USAGE, CREATE]

      - database: myapp_production
        schema: app
        tables: ["*"]
        privileges: [SELECT, INSERT, UPDATE, DELETE]
        withGrantOption: false

      - database: myapp_production
        schema: audit
        tables: [audit_log]
        privileges: [INSERT]

      - database: myapp_production
        schema: app
        sequences: ["*"]
        privileges: [USAGE, SELECT, UPDATE]

      - database: myapp_production
        schema: app
        functions: ["*"]
        privileges: [EXECUTE]

    # Default privileges for future objects
    defaultPrivileges:
      - database: myapp_production
        schema: app
        grantedBy: postgres
        objectType: tables
        privileges: [SELECT, INSERT, UPDATE, DELETE]

      - database: myapp_production
        schema: app
        grantedBy: postgres
        objectType: sequences
        privileges: [USAGE, SELECT]

  # --- OR ---

  mysql:
    # Assign roles (MySQL 8.0+)
    roles:
      - app_readwrite

    # Direct grants
    grants:
      - level: database
        database: myapp_production
        privileges: [SELECT, INSERT, UPDATE, DELETE]
        withGrantOption: false

      - level: table
        database: myapp_production
        table: audit_log
        privileges: [INSERT, SELECT]

      - level: column
        database: myapp_production
        table: users
        columns: [email, name]
        privileges: [SELECT, UPDATE]

      - level: procedure
        database: myapp_production
        procedure: calculate_metrics
        privileges: [EXECUTE]

status:
  phase: Ready
  observedGeneration: 1

  appliedGrants:
    roles: [app_readwrite, monitoring_readonly]
    directGrants: 8
    defaultPrivileges: 2

  conditions:
    - type: Ready
      status: "True"
    - type: RolesAssigned
      status: "True"
    - type: GrantsApplied
      status: "True"
```

### 6. DatabaseBackup

Represents a single point-in-time backup (Velero-style).

```yaml
apiVersion: dbops.example.com/v1alpha1
kind: DatabaseBackup
metadata:
  name: myapp-db-20250105-0200
  labels:
    dbops.example.com/database: myapp-db
    dbops.example.com/schedule: myapp-daily    # If created by schedule
spec:
  # Target database
  databaseRef:
    name: myapp-db

  # Storage destination
  storage:
    type: gcs                 # gcs | s3 | azure | pvc

    gcs:
      bucket: my-db-backups
      prefix: postgres/myapp/
      secretRef:
        name: gcs-backup-credentials
        key: service-account.json

    # --- OR ---
    # s3:
    #   bucket: my-db-backups
    #   region: ap-southeast-1
    #   prefix: postgres/myapp/
    #   endpoint: ""          # For MinIO/custom S3
    #   secretRef:
    #     name: s3-backup-credentials
    #     keys:
    #       accessKey: AWS_ACCESS_KEY_ID
    #       secretKey: AWS_SECRET_ACCESS_KEY

    # --- OR ---
    # azure:
    #   container: db-backups
    #   storageAccount: mybackups
    #   prefix: postgres/myapp/
    #   secretRef:
    #     name: azure-backup-credentials

    # --- OR ---
    # pvc:
    #   claimName: backup-storage
    #   subPath: postgres/myapp

  # Compression
  compression:
    enabled: true
    algorithm: zstd           # gzip | lz4 | zstd
    level: 3

  # Encryption at rest
  encryption:
    enabled: true
    algorithm: aes-256-gcm
    secretRef:
      name: backup-encryption-key
      key: passphrase

  # Auto-delete after TTL
  ttl: 168h                   # 7 days

  # Timeout
  activeDeadlineSeconds: 3600

  #-----------------------------------------
  # Engine-specific options (ONE OF)
  #-----------------------------------------

  postgres:
    method: pg_dump           # pg_dump | pg_basebackup
    format: custom            # plain | custom | directory | tar
    jobs: 4                   # Parallel (directory format only)

    # What to include
    dataOnly: false
    schemaOnly: false
    blobs: true
    noOwner: false
    noPrivileges: false

    # Filtering
    schemas: []               # Empty = all
    excludeSchemas:
      - temp_data
    tables: []
    excludeTables:
      - public.large_logs
      - public.sessions

    # Options
    lockWaitTimeout: 60s
    noSync: false

  # --- OR ---

  mysql:
    method: mysqldump         # mysqldump | xtrabackup | mysqlpump
    singleTransaction: true
    quick: true
    lockTables: false

    # What to include
    routines: true
    triggers: true
    events: true

    # Options
    extendedInsert: true
    setGtidPurged: AUTO       # OFF | ON | AUTO

    # Filtering
    databases: []
    tables: []
    excludeTables:
      - sessions
      - cache

status:
  phase: Completed            # Pending | Running | Completed | Failed
  startedAt: "2025-01-05T02:00:00Z"
  completedAt: "2025-01-05T02:12:34Z"

  backup:
    path: "gs://my-db-backups/postgres/myapp/myapp-db-20250105-0200.dump.zst.enc"
    sizeBytes: 524288000
    compressedSizeBytes: 104857600
    checksum: "sha256:abcdef..."
    format: custom

  source:
    instance: postgres-primary
    database: myapp_production
    engine: postgres
    version: "15.4"
    timestamp: "2025-01-05T02:00:00Z"

  expiresAt: "2025-01-12T02:00:00Z"

  conditions:
    - type: Complete
      status: "True"
      reason: BackupSucceeded
```

### 7. DatabaseBackupSchedule

Creates DatabaseBackup resources on schedule (Velero-style).

```yaml
apiVersion: dbops.example.com/v1alpha1
kind: DatabaseBackupSchedule
metadata:
  name: myapp-daily
spec:
  # Cron schedule
  schedule: "0 2 * * *"       # Daily at 2 AM
  timezone: Asia/Bangkok

  # Pause scheduling
  paused: false

  # Concurrency policy
  concurrencyPolicy: Forbid   # Allow | Forbid | Replace

  # Template for DatabaseBackup resources
  template:
    metadata:
      labels:
        environment: production
        team: platform
    spec:
      databaseRef:
        name: myapp-db

      storage:
        type: gcs
        gcs:
          bucket: my-db-backups
          prefix: postgres/myapp/scheduled/
          secretRef:
            name: gcs-backup-credentials

      compression:
        enabled: true
        algorithm: zstd
        level: 3

      encryption:
        enabled: true
        secretRef:
          name: backup-encryption-key

      ttl: 336h               # 14 days

      postgres:
        method: pg_dump
        format: custom
        jobs: 4
        excludeTables:
          - public.sessions

  # Retention policy
  retention:
    keepLast: 7               # Always keep N most recent
    keepHourly: 0
    keepDaily: 7
    keepWeekly: 4
    keepMonthly: 6
    keepYearly: 1
    minAge: 1h                # Don't delete if younger

  # History limits (for status)
  successfulBackupsHistoryLimit: 5
  failedBackupsHistoryLimit: 3

status:
  phase: Active               # Active | Paused

  lastBackup:
    name: myapp-db-20250105-0200
    status: Completed
    startedAt: "2025-01-05T02:00:00Z"
    completedAt: "2025-01-05T02:12:34Z"

  nextBackupTime: "2025-01-06T02:00:00Z"

  statistics:
    totalBackups: 142
    successfulBackups: 140
    failedBackups: 2
    averageDurationSeconds: 754
    averageSizeBytes: 524288000
    totalStorageBytes: 36700160000

  recentBackups:
    - name: myapp-db-20250105-0200
      status: Completed
    - name: myapp-db-20250104-0200
      status: Completed

  conditions:
    - type: Scheduled
      status: "True"
    - type: RetentionEnforced
      status: "True"
```

### 8. DatabaseRestore

Restores a database from backup.

```yaml
apiVersion: dbops.example.com/v1alpha1
kind: DatabaseRestore
metadata:
  name: myapp-restore-20250105
spec:
  # Source: Reference to DatabaseBackup
  backupRef:
    name: myapp-db-20250105-0200

  # OR restore from direct path
  # fromPath:
  #   storage:
  #     type: gcs
  #     gcs:
  #       bucket: my-db-backups
  #       path: postgres/myapp/myapp-db-20250105-0200.dump.zst.enc
  #       secretRef:
  #         name: gcs-backup-credentials
  #   compression:
  #     algorithm: zstd
  #   encryption:
  #     secretRef:
  #       name: backup-encryption-key

  # Target
  target:
    # Restore to NEW database (safe)
    instanceRef:
      name: postgres-primary
    databaseName: myapp_restored

    # OR in-place restore (destructive!)
    # inPlace: true
    # databaseRef:
    #   name: myapp-db

  # Safety confirmation (required for destructive ops)
  confirmation:
    acknowledgeDataLoss: ""   # "I-UNDERSTAND-DATA-LOSS" for inPlace

  # Timeout
  activeDeadlineSeconds: 7200

  #-----------------------------------------
  # Engine-specific options (ONE OF)
  #-----------------------------------------

  postgres:
    # Pre-restore
    dropExisting: false
    createDatabase: true

    # What to restore
    dataOnly: false
    schemaOnly: false
    noOwner: true
    noPrivileges: false

    # Role mapping
    roleMapping:
      old_user: new_user

    # Filtering
    schemas: []
    tables: []

    # Performance
    jobs: 4
    disableTriggers: false

    # Post-restore
    analyze: true

  # --- OR ---

  mysql:
    dropExisting: false
    createDatabase: true
    routines: true
    triggers: true
    events: true
    disableForeignKeyChecks: true
    disableBinlog: true

status:
  phase: Completed            # Pending | Running | Completed | Failed
  startedAt: "2025-01-05T10:00:00Z"
  completedAt: "2025-01-05T10:32:15Z"

  restore:
    sourceBackup: myapp-db-20250105-0200
    targetInstance: postgres-primary
    targetDatabase: myapp_restored

  progress:
    percentage: 100
    currentPhase: "Complete"
    tablesRestored: 45
    tablesTotal: 45

  warnings:
    - "Role 'old_user' not found, mapped to 'postgres'"

  conditions:
    - type: Complete
      status: "True"
      reason: RestoreSucceeded
```

---

## Implementation Phases

### Phase 1: Core MVP
- [ ] Project scaffolding with Kubebuilder
- [ ] `DatabaseInstance` CRD + controller
- [ ] `Database` CRD + controller
- [ ] `DatabaseUser` CRD + basic secret generation
- [ ] PostgreSQL adapter (connection, database, user)
- [ ] MySQL adapter (connection, database, user)
- [ ] Unit tests for all components
- [ ] Integration tests with Testcontainers

### Phase 2: Permissions & Safety
- [ ] `DatabaseRole` CRD + controller
- [ ] `DatabaseGrant` CRD + controller
- [ ] Force delete with confirmation annotation
- [ ] Deletion protection
- [ ] Import existing secrets

### Phase 3: Secrets & Integration
- [ ] Secret template customization
- [ ] Vault integration
- [ ] External Secrets Operator support
- [ ] Password rotation

### Phase 4: Backup & Restore
- [ ] `DatabaseBackup` CRD + controller
- [ ] `DatabaseBackupSchedule` CRD + controller
- [ ] `DatabaseRestore` CRD + controller
- [ ] GCS storage backend
- [ ] S3 storage backend
- [ ] Retention policy enforcement

### Phase 5: Observability & Polish
- [ ] Prometheus metrics
- [ ] Grafana dashboards
- [ ] Helm chart
- [ ] Documentation

---

## Test Plan

### Test Strategy

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Test Pyramid                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│                         ┌─────────────────────┐                             │
│                         │   Integration Tests │  Testcontainers             │
│                         │   Real PG/MySQL     │  ~80-120 tests              │
│                         └──────────┬──────────┘                             │
│                    ┌───────────────┴───────────────┐                        │
│                    │         Unit Tests            │  Envtest + Mocks       │
│                    │   Controllers, Adapters       │  ~200-300 tests        │
│                    │   Validators, Helpers         │                        │
│                    └───────────────────────────────┘                        │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Test Categories

| Category | Framework | Database | Speed |
|----------|-----------|----------|-------|
| Unit | Ginkgo + Envtest | Mock/Fake | Fast (~seconds) |
| Integration | Ginkgo + Testcontainers | Real PG/MySQL | Medium (~minutes) |

---

## Test Cases by Component

### 1. DatabaseInstance Controller

#### 1.1 Unit Tests

| ID | Test Case | Expected Result |
|----|-----------|-----------------|
| U-DI-001 | Create DatabaseInstance with valid spec | CR created, reconcile queued |
| U-DI-002 | Validate engine field is required | Validation error |
| U-DI-003 | Validate engine must be postgres/mysql/cockroachdb | Validation error for invalid |
| U-DI-004 | Validate connection.host is required | Validation error |
| U-DI-005 | Validate connection.port is required | Validation error |
| U-DI-006 | Validate secretRef and existingSecret are mutually exclusive | Validation error if both set |
| U-DI-007 | Validate postgres section only allowed when engine=postgres | Validation error |
| U-DI-008 | Validate mysql section only allowed when engine=mysql | Validation error |
| U-DI-009 | Validate TLS secretRef required when tls.enabled=true | Validation error |
| U-DI-010 | Reconcile sets status.phase=Pending initially | Status updated |
| U-DI-011 | Reconcile with missing secret sets status.phase=Failed | Status updated with error |
| U-DI-012 | Finalizer added on create | Finalizer present |
| U-DI-013 | Finalizer removed on delete after cleanup | Finalizer removed |
| U-DI-014 | Health check interval configuration parsed correctly | Correct interval used |
| U-DI-015 | Engine-specific defaults applied | Defaults populated |

#### 1.2 Integration Tests

| ID | Test Case | Expected Result |
|----|-----------|-----------------|
| I-DI-001 | Create PostgreSQL instance with valid credentials | status.phase=Ready, version populated |
| I-DI-002 | Create PostgreSQL instance with invalid credentials | status.phase=Failed, clear error message |
| I-DI-003 | Create PostgreSQL instance with wrong port | status.phase=Failed, connection timeout |
| I-DI-004 | Create PostgreSQL instance with TLS | Connects with TLS, status.phase=Ready |
| I-DI-005 | Create MySQL instance with valid credentials | status.phase=Ready, version populated |
| I-DI-006 | Create MySQL instance with invalid credentials | status.phase=Failed, clear error message |
| I-DI-007 | Create MySQL instance with charset option | Option applied, status.phase=Ready |
| I-DI-008 | Update secret credentials | Operator reconnects, status.phase=Ready |
| I-DI-009 | Delete secret while instance exists | status.phase=Failed |
| I-DI-010 | Import existing secret with custom keys | Credentials read correctly |
| I-DI-011 | Health check updates lastCheckedAt | Timestamp updates periodically |
| I-DI-012 | Database server restart | Status temporarily Failed, recovers to Ready |

---

### 2. Database Controller

#### 2.1 Unit Tests

| ID | Test Case | Expected Result |
|----|-----------|-----------------|
| U-DB-001 | Create Database with valid spec | CR created, reconcile queued |
| U-DB-002 | Validate instanceRef.name is required | Validation error |
| U-DB-003 | Validate name (database name) is required | Validation error |
| U-DB-004 | Validate deletionPolicy enum values | Only Retain/Delete/Snapshot allowed |
| U-DB-005 | Validate only one engine section set | Validation error if multiple |
| U-DB-006 | Validate at least one engine section required | Validation error if none |
| U-DB-007 | Validate postgres.encoding values | Only valid encodings allowed |
| U-DB-008 | Validate postgres.extensions schema | Correct structure |
| U-DB-009 | Validate mysql.charset values | Only valid charsets |
| U-DB-010 | Reconcile with non-existent instanceRef | status.phase=Failed |
| U-DB-011 | Reconcile with instanceRef not Ready | Requeue, wait for instance |
| U-DB-012 | Finalizer blocks deletion when deletionPolicy=Retain | Database preserved |
| U-DB-013 | Force delete annotation parsing | Correctly parsed |
| U-DB-014 | Force delete requires confirmation annotation | Error without confirmation |
| U-DB-015 | DeletionProtection blocks deletion | Deletion rejected |
| U-DB-016 | Status conditions set correctly | All conditions present |

#### 2.2 Integration Tests

| ID | Test Case | Expected Result |
|----|-----------|-----------------|
| I-DB-001 | Create PostgreSQL database | Database exists, status.phase=Ready |
| I-DB-002 | Create PostgreSQL database with encoding UTF8 | Encoding set correctly |
| I-DB-003 | Create PostgreSQL database with extensions | Extensions installed |
| I-DB-004 | Create PostgreSQL database with schemas | Schemas created |
| I-DB-005 | Create PostgreSQL database with default privileges | Defaults applied |
| I-DB-006 | Create MySQL database | Database exists, status.phase=Ready |
| I-DB-007 | Create MySQL database with charset utf8mb4 | Charset set correctly |
| I-DB-008 | Create MySQL database with collation | Collation set correctly |
| I-DB-009 | Delete database with deletionPolicy=Delete | Database dropped |
| I-DB-010 | Delete database with deletionPolicy=Retain | Database preserved |
| I-DB-011 | Delete database with active connections (no force) | Deletion fails/waits |
| I-DB-012 | Delete database with force-delete annotation | Database dropped despite connections |
| I-DB-013 | Update database (add extension) | Extension added |
| I-DB-014 | Update database (add schema) | Schema created |
| I-DB-015 | Create duplicate database name | status.phase=Failed or succeeds idempotently |
| I-DB-016 | Database with invalid extension name | status.phase=Failed, clear error |
| I-DB-017 | Status shows correct sizeBytes | Size reported |
| I-DB-018 | Instance becomes unavailable | Database status reflects instance status |

---

### 3. DatabaseUser Controller

#### 3.1 Unit Tests

| ID | Test Case | Expected Result |
|----|-----------|-----------------|
| U-DU-001 | Create DatabaseUser with valid spec | CR created |
| U-DU-002 | Validate instanceRef.name required | Validation error |
| U-DU-003 | Validate username required | Validation error |
| U-DU-004 | Validate passwordSecret or existingPasswordSecret required | Validation error |
| U-DU-005 | Validate passwordSecret and existingPasswordSecret mutually exclusive | Validation error |
| U-DU-006 | Validate password length >= 8 | Validation error |
| U-DU-007 | Validate password length <= 128 | Validation error |
| U-DU-008 | Validate secretTemplate.data keys | Valid template syntax |
| U-DU-009 | Validate postgres section only with postgres engine | Validation error |
| U-DU-010 | Validate mysql section only with mysql engine | Validation error |
| U-DU-011 | Validate postgres.connectionLimit >= -1 | Validation error |
| U-DU-012 | Validate mysql.maxUserConnections >= 0 | Validation error |
| U-DU-013 | Password generator produces correct length | Correct length |
| U-DU-014 | Password generator includes special chars when enabled | Special chars present |
| U-DU-015 | Password generator excludes specified chars | Chars excluded |
| U-DU-016 | Secret template rendering | Template rendered correctly |
| U-DU-017 | Finalizer added on create | Finalizer present |
| U-DU-018 | User deletion cleans up secret | Secret deleted |

#### 3.2 Integration Tests

| ID | Test Case | Expected Result |
|----|-----------|-----------------|
| I-DU-001 | Create PostgreSQL user with generated password | User exists, can connect |
| I-DU-002 | Create PostgreSQL user with connectionLimit | Limit enforced |
| I-DU-003 | Create PostgreSQL user with validUntil | Expiry set correctly |
| I-DU-004 | Create PostgreSQL user with inRoles | Role membership set |
| I-DU-005 | Create MySQL user with generated password | User exists, can connect |
| I-DU-006 | Create MySQL user with maxUserConnections | Limit enforced |
| I-DU-007 | Create MySQL user with allowedHosts | Host restrictions applied |
| I-DU-008 | Create MySQL user with authPlugin | Auth plugin set |
| I-DU-009 | Generated secret contains all template keys | All keys present |
| I-DU-010 | Generated secret DATABASE_URL works | Connection succeeds |
| I-DU-011 | Import existing password from secret | Password used correctly |
| I-DU-012 | Update user (change connectionLimit) | Limit updated |
| I-DU-013 | Delete user with deletionPolicy | User dropped or retained |
| I-DU-014 | Delete user cleans up generated secret | Secret deleted |
| I-DU-015 | Create duplicate username | status.phase=Failed or idempotent |
| I-DU-016 | Password rotation creates new secret | New password works, old doesn't |
| I-DU-017 | Secret labels and annotations applied | Metadata present |

---

### 4. DatabaseRole Controller

#### 4.1 Unit Tests

| ID | Test Case | Expected Result |
|----|-----------|-----------------|
| U-DR-001 | Create DatabaseRole with valid spec | CR created |
| U-DR-002 | Validate instanceRef.name required | Validation error |
| U-DR-003 | Validate roleName required | Validation error |
| U-DR-004 | Validate postgres grants structure | Correct structure |
| U-DR-005 | Validate mysql grants structure | Correct structure |
| U-DR-006 | Validate privileges are valid PostgreSQL privileges | Validation error for invalid |
| U-DR-007 | Validate privileges are valid MySQL privileges | Validation error for invalid |
| U-DR-008 | Validate postgres.login default is false | Default applied |
| U-DR-009 | Finalizer added on create | Finalizer present |
| U-DR-010 | Status conditions reflect grant state | Conditions accurate |

#### 4.2 Integration Tests

| ID | Test Case | Expected Result |
|----|-----------|-----------------|
| I-DR-001 | Create PostgreSQL role (group) | Role exists, cannot login |
| I-DR-002 | Create PostgreSQL role with grants | Grants applied |
| I-DR-003 | Create PostgreSQL role with inRoles | Inherits from roles |
| I-DR-004 | Create MySQL role (8.0+) | Role exists |
| I-DR-005 | Create MySQL role with grants | Grants applied |
| I-DR-006 | Update role (add grant) | New grant applied |
| I-DR-007 | Update role (remove grant) | Grant revoked |
| I-DR-008 | Delete role | Role dropped |
| I-DR-009 | Delete role in use by user | Fails or cascades based on policy |
| I-DR-010 | Role with schema wildcard (*) | All tables granted |
| I-DR-011 | Role with specific tables | Only specified tables granted |

---

### 5. DatabaseGrant Controller

#### 5.1 Unit Tests

| ID | Test Case | Expected Result |
|----|-----------|-----------------|
| U-DG-001 | Create DatabaseGrant with valid spec | CR created |
| U-DG-002 | Validate userRef.name required | Validation error |
| U-DG-003 | Validate postgres roles list | Valid role names |
| U-DG-004 | Validate postgres grants privileges | Valid privileges |
| U-DG-005 | Validate mysql grants level enum | Only valid levels |
| U-DG-006 | Validate mysql column grants require columns | Validation error |
| U-DG-007 | Reconcile with non-existent userRef | status.phase=Failed |
| U-DG-008 | Reconcile with non-existent role | status.phase=Failed |
| U-DG-009 | Status reflects applied grants count | Count accurate |
| U-DG-010 | Finalizer added on create | Finalizer present |

#### 5.2 Integration Tests

| ID | Test Case | Expected Result |
|----|-----------|-----------------|
| I-DG-001 | Assign PostgreSQL role to user | User has role |
| I-DG-002 | Assign multiple PostgreSQL roles | User has all roles |
| I-DG-003 | Grant PostgreSQL table privileges | User can access tables |
| I-DG-004 | Grant PostgreSQL schema privileges | User can access schema |
| I-DG-005 | Grant PostgreSQL with withGrantOption | User can grant to others |
| I-DG-006 | Set PostgreSQL default privileges | Future objects get privileges |
| I-DG-007 | Assign MySQL role to user | User has role |
| I-DG-008 | Grant MySQL database privileges | User can access database |
| I-DG-009 | Grant MySQL table privileges | User can access table |
| I-DG-010 | Grant MySQL column privileges | User can access columns only |
| I-DG-011 | Revoke grant (delete DatabaseGrant) | Privileges removed |
| I-DG-012 | Update grant (add privilege) | New privilege added |
| I-DG-013 | Update grant (remove privilege) | Privilege revoked |
| I-DG-014 | User can perform granted operations | Operations succeed |
| I-DG-015 | User cannot perform non-granted operations | Operations fail |

---

### 6. DatabaseBackup Controller

#### 6.1 Unit Tests

| ID | Test Case | Expected Result |
|----|-----------|-----------------|
| U-BK-001 | Create DatabaseBackup with valid spec | CR created |
| U-BK-002 | Validate databaseRef.name required | Validation error |
| U-BK-003 | Validate storage.type required | Validation error |
| U-BK-004 | Validate storage.gcs when type=gcs | GCS fields required |
| U-BK-005 | Validate storage.s3 when type=s3 | S3 fields required |
| U-BK-006 | Validate compression.algorithm enum | Valid algorithms only |
| U-BK-007 | Validate compression.level range | 0-9 or engine-specific |
| U-BK-008 | Validate postgres.method enum | pg_dump/pg_basebackup |
| U-BK-009 | Validate postgres.format enum | plain/custom/directory/tar |
| U-BK-010 | Validate mysql.method enum | mysqldump/xtrabackup/mysqlpump |
| U-BK-011 | Validate TTL duration format | Valid duration |
| U-BK-012 | Validate activeDeadlineSeconds > 0 | Validation error |
| U-BK-013 | Status transitions: Pending -> Running -> Completed | Correct transitions |
| U-BK-014 | Status transitions: Running -> Failed | On error |
| U-BK-015 | TTL calculation sets expiresAt | Correct expiry time |
| U-BK-016 | Finalizer added for cleanup | Finalizer present |

#### 6.2 Integration Tests

| ID | Test Case | Expected Result |
|----|-----------|-----------------|
| I-BK-001 | Backup PostgreSQL with pg_dump custom format | Backup file created |
| I-BK-002 | Backup PostgreSQL with compression | Compressed file smaller |
| I-BK-003 | Backup PostgreSQL with encryption | File encrypted |
| I-BK-004 | Backup PostgreSQL with schema filter | Only specified schemas |
| I-BK-005 | Backup PostgreSQL with table exclusion | Tables excluded |
| I-BK-006 | Backup PostgreSQL with parallel jobs | Completes faster |
| I-BK-007 | Backup MySQL with mysqldump | Backup file created |
| I-BK-008 | Backup MySQL with singleTransaction | Consistent snapshot |
| I-BK-009 | Backup MySQL with routines/triggers | Included in backup |
| I-BK-010 | Backup to GCS | File uploaded to bucket |
| I-BK-011 | Backup to S3 | File uploaded to bucket |
| I-BK-012 | Backup to PVC | File on volume |
| I-BK-013 | Backup status shows size | sizeBytes populated |
| I-BK-014 | Backup status shows checksum | checksum populated |
| I-BK-015 | Backup timeout exceeded | status.phase=Failed |
| I-BK-016 | Backup with non-existent database | status.phase=Failed |
| I-BK-017 | Backup file is restorable | Restore succeeds |
| I-BK-018 | TTL expiry triggers deletion | Backup CR deleted |

---

### 7. DatabaseBackupSchedule Controller

#### 7.1 Unit Tests

| ID | Test Case | Expected Result |
|----|-----------|-----------------|
| U-BS-001 | Create DatabaseBackupSchedule with valid spec | CR created |
| U-BS-002 | Validate schedule cron format | Validation error for invalid |
| U-BS-003 | Validate timezone format | Validation error for invalid |
| U-BS-004 | Validate template.spec required | Validation error |
| U-BS-005 | Validate retention keepLast >= 0 | Validation error |
| U-BS-006 | Validate concurrencyPolicy enum | Allow/Forbid/Replace |
| U-BS-007 | Calculate next run time from cron | Correct time |
| U-BS-008 | Calculate next run time with timezone | Timezone applied |
| U-BS-009 | Paused schedule sets phase=Paused | Correct phase |
| U-BS-010 | Status.nextBackupTime calculated | Time populated |
| U-BS-011 | Retention policy: keepLast logic | Correct backups selected |
| U-BS-012 | Retention policy: keepDaily logic | Best per day kept |
| U-BS-013 | Retention policy: keepWeekly logic | Best per week kept |
| U-BS-014 | Retention policy: minAge respected | Young backups kept |
| U-BS-015 | History limit enforced | Old entries pruned |

#### 7.2 Integration Tests

| ID | Test Case | Expected Result |
|----|-----------|-----------------|
| I-BS-001 | Schedule creates DatabaseBackup at cron time | Backup CR created |
| I-BS-002 | Created backup has schedule label | Label present |
| I-BS-003 | Created backup inherits template | Spec matches template |
| I-BS-004 | Paused schedule does not create backups | No backup created |
| I-BS-005 | Resume schedule creates backup | Backup created |
| I-BS-006 | ConcurrencyPolicy=Forbid blocks concurrent | Second backup waits |
| I-BS-007 | ConcurrencyPolicy=Replace cancels running | Old backup cancelled |
| I-BS-008 | Retention deletes old backups (keepLast) | Excess deleted |
| I-BS-009 | Retention keeps daily backups | Daily backups retained |
| I-BS-010 | Retention respects minAge | Young backups not deleted |
| I-BS-011 | Statistics updated after backup | Counts accurate |
| I-BS-012 | Failed backup recorded in status | failedBackups incremented |
| I-BS-013 | Multiple schedules for same database | Both create backups |
| I-BS-014 | Schedule with timezone offset | Correct trigger time |

---

### 8. DatabaseRestore Controller

#### 8.1 Unit Tests

| ID | Test Case | Expected Result |
|----|-----------|-----------------|
| U-RS-001 | Create DatabaseRestore with valid spec | CR created |
| U-RS-002 | Validate backupRef or fromPath required | Validation error |
| U-RS-003 | Validate backupRef and fromPath mutually exclusive | Validation error |
| U-RS-004 | Validate target required | Validation error |
| U-RS-005 | Validate target.inPlace requires confirmation | Validation error |
| U-RS-006 | Validate confirmation.acknowledgeDataLoss value | Exact match required |
| U-RS-007 | Validate postgres.roleMapping format | Valid mapping |
| U-RS-008 | Status transitions: Pending -> Running -> Completed | Correct transitions |
| U-RS-009 | Status transitions: Running -> Failed | On error |
| U-RS-010 | Progress percentage calculated | 0-100 range |
| U-RS-011 | Warnings accumulated in status | Warnings captured |
| U-RS-012 | Restore is one-shot (no re-reconcile on complete) | No re-run |

#### 8.2 Integration Tests

| ID | Test Case | Expected Result |
|----|-----------|-----------------|
| I-RS-001 | Restore PostgreSQL to new database | Database created with data |
| I-RS-002 | Restore PostgreSQL with createDatabase=true | Database created |
| I-RS-003 | Restore PostgreSQL with dropExisting=true | Old database dropped |
| I-RS-004 | Restore PostgreSQL with noOwner=true | Ownership not restored |
| I-RS-005 | Restore PostgreSQL with roleMapping | Roles mapped correctly |
| I-RS-006 | Restore PostgreSQL with schema filter | Only schemas restored |
| I-RS-007 | Restore PostgreSQL with parallel jobs | Faster completion |
| I-RS-008 | Restore PostgreSQL with analyze=true | ANALYZE run post-restore |
| I-RS-009 | Restore MySQL to new database | Database created with data |
| I-RS-010 | Restore MySQL with routines | Stored procedures restored |
| I-RS-011 | Restore MySQL with disableForeignKeyChecks | No FK errors during restore |
| I-RS-012 | Restore from GCS backup | Data restored correctly |
| I-RS-013 | Restore from encrypted backup | Decryption works |
| I-RS-014 | Restore in-place with confirmation | Data overwritten |
| I-RS-015 | Restore in-place without confirmation | Rejected |
| I-RS-016 | Restore non-existent backup | status.phase=Failed |
| I-RS-017 | Restore timeout exceeded | status.phase=Failed |
| I-RS-018 | Restored data matches original | Data integrity verified |
| I-RS-019 | Restore captures warnings | Warnings in status |

---

### 9. Adapter Unit Tests

#### 9.1 PostgreSQL Adapter

| ID | Test Case | Expected Result |
|----|-----------|-----------------|
| U-PG-001 | BuildConnectionString with all options | Correct DSN |
| U-PG-002 | BuildConnectionString with TLS | sslmode included |
| U-PG-003 | CreateDatabase SQL generation | Valid CREATE DATABASE |
| U-PG-004 | CreateDatabase with encoding | ENCODING clause |
| U-PG-005 | CreateDatabase with LC_COLLATE | LC_COLLATE clause |
| U-PG-006 | CreateDatabase with template | TEMPLATE clause |
| U-PG-007 | DropDatabase SQL generation | Valid DROP DATABASE |
| U-PG-008 | DropDatabase with force | WITH (FORCE) clause |
| U-PG-009 | CreateUser SQL generation | Valid CREATE USER |
| U-PG-010 | CreateUser with password | PASSWORD clause |
| U-PG-011 | CreateUser with connectionLimit | CONNECTION LIMIT clause |
| U-PG-012 | CreateUser with validUntil | VALID UNTIL clause |
| U-PG-013 | CreateUser with role attributes | Correct attributes |
| U-PG-014 | CreateRole SQL generation | Valid CREATE ROLE |
| U-PG-015 | Grant privileges SQL generation | Valid GRANT statement |
| U-PG-016 | Grant on all tables in schema | GRANT ON ALL TABLES |
| U-PG-017 | Grant on specific tables | Correct table names |
| U-PG-018 | Revoke privileges SQL generation | Valid REVOKE statement |
| U-PG-019 | AlterDefaultPrivileges SQL | Valid ALTER DEFAULT PRIVILEGES |
| U-PG-020 | CreateExtension SQL generation | Valid CREATE EXTENSION |
| U-PG-021 | CreateSchema SQL generation | Valid CREATE SCHEMA |
| U-PG-022 | BuildPgDumpArgs with all options | Correct arguments |
| U-PG-023 | BuildPgRestoreArgs with all options | Correct arguments |
| U-PG-024 | ParsePgDumpOutput for progress | Progress extracted |
| U-PG-025 | EscapeIdentifier prevents injection | Properly escaped |
| U-PG-026 | EscapeLiteral prevents injection | Properly escaped |

#### 9.2 MySQL Adapter

| ID | Test Case | Expected Result |
|----|-----------|-----------------|
| U-MY-001 | BuildConnectionString with all options | Correct DSN |
| U-MY-002 | BuildConnectionString with TLS | tls parameter |
| U-MY-003 | CreateDatabase SQL generation | Valid CREATE DATABASE |
| U-MY-004 | CreateDatabase with charset | CHARACTER SET clause |
| U-MY-005 | CreateDatabase with collation | COLLATE clause |
| U-MY-006 | DropDatabase SQL generation | Valid DROP DATABASE |
| U-MY-007 | CreateUser SQL generation | Valid CREATE USER |
| U-MY-008 | CreateUser with host patterns | user@host format |
| U-MY-009 | CreateUser with authPlugin | IDENTIFIED WITH clause |
| U-MY-010 | CreateUser with resource limits | WITH clause |
| U-MY-011 | CreateRole SQL generation (8.0+) | Valid CREATE ROLE |
| U-MY-012 | Grant privileges SQL generation | Valid GRANT statement |
| U-MY-013 | Grant database level | ON database.* |
| U-MY-014 | Grant table level | ON database.table |
| U-MY-015 | Grant column level | (column1, column2) |
| U-MY-016 | Revoke privileges SQL generation | Valid REVOKE statement |
| U-MY-017 | BuildMysqldumpArgs with all options | Correct arguments |
| U-MY-018 | BuildMysqlArgs for restore | Correct arguments |
| U-MY-019 | ParseMysqldumpOutput for progress | Progress extracted |
| U-MY-020 | EscapeIdentifier prevents injection | Properly escaped |
| U-MY-021 | EscapeLiteral prevents injection | Properly escaped |

---

### 10. Secret Manager Unit Tests

| ID | Test Case | Expected Result |
|----|-----------|-----------------|
| U-SM-001 | GeneratePassword with length | Correct length |
| U-SM-002 | GeneratePassword with special chars | Special chars included |
| U-SM-003 | GeneratePassword without special chars | No special chars |
| U-SM-004 | GeneratePassword with excludeChars | Chars excluded |
| U-SM-005 | GeneratePassword entropy sufficient | High entropy |
| U-SM-006 | RenderTemplate with all variables | All placeholders replaced |
| U-SM-007 | RenderTemplate with missing variable | Error or empty |
| U-SM-008 | RenderTemplate escapes special chars | Properly escaped |
| U-SM-009 | CreateSecret with labels | Labels applied |
| U-SM-010 | CreateSecret with annotations | Annotations applied |
| U-SM-011 | CreateSecret with custom type | Type set correctly |
| U-SM-012 | UpdateSecret preserves immutable fields | Fields preserved |
| U-SM-013 | DeleteSecret succeeds | Secret removed |
| U-SM-014 | GetSecret returns data | Data retrieved |
| U-SM-015 | GetSecret non-existent | NotFound error |

---

### 11. Storage Manager Unit Tests

| ID | Test Case | Expected Result |
|----|-----------|-----------------|
| U-ST-001 | GCS: BuildPath with prefix | Correct path |
| U-ST-002 | GCS: Upload file | Upload initiated |
| U-ST-003 | GCS: Download file | Download initiated |
| U-ST-004 | GCS: Delete file | Delete initiated |
| U-ST-005 | GCS: List files with prefix | Files listed |
| U-ST-006 | S3: BuildPath with prefix | Correct path |
| U-ST-007 | S3: Upload file | Upload initiated |
| U-ST-008 | S3: Download file | Download initiated |
| U-ST-009 | S3: Delete file | Delete initiated |
| U-ST-010 | S3: Custom endpoint (MinIO) | Endpoint used |
| U-ST-011 | PVC: BuildPath with subPath | Correct path |
| U-ST-012 | PVC: Write file | File written |
| U-ST-013 | PVC: Read file | File read |
| U-ST-014 | PVC: Delete file | File deleted |
| U-ST-015 | Compression: gzip compress/decompress | Round-trip works |
| U-ST-016 | Compression: lz4 compress/decompress | Round-trip works |
| U-ST-017 | Compression: zstd compress/decompress | Round-trip works |
| U-ST-018 | Encryption: encrypt/decrypt | Round-trip works |
| U-ST-019 | Encryption: wrong key fails | Decryption fails |

---

### 12. Validation Unit Tests (CEL/Webhook)

| ID | Test Case | Expected Result |
|----|-----------|-----------------|
| U-VL-001 | Database: only one engine section | Error if multiple |
| U-VL-002 | Database: at least one engine section | Error if none |
| U-VL-003 | Database: postgres section matches instance engine | Error if mismatch |
| U-VL-004 | DatabaseUser: secretRef XOR existingPasswordSecret | Error if both/neither |
| U-VL-005 | DatabaseBackup: storage type matches config | Error if mismatch |
| U-VL-006 | DatabaseRestore: inPlace requires confirmation | Error without |
| U-VL-007 | Immutable fields cannot be changed | Error on update |
| U-VL-008 | Required fields are present | Error if missing |
| U-VL-009 | Enum fields have valid values | Error if invalid |
| U-VL-010 | Cross-field validations work | Complex rules enforced |

---

### 13. End-to-End Scenarios (Integration)

| ID | Test Case | Expected Result |
|----|-----------|-----------------|
| I-E2E-001 | Full PostgreSQL workflow: Instance -> Database -> User -> Grant | All resources Ready, user can connect and operate |
| I-E2E-002 | Full MySQL workflow: Instance -> Database -> User -> Grant | All resources Ready, user can connect and operate |
| I-E2E-003 | PostgreSQL backup and restore to new database | Data matches original |
| I-E2E-004 | MySQL backup and restore to new database | Data matches original |
| I-E2E-005 | Scheduled backup creates backups on time | Backups created |
| I-E2E-006 | Retention policy cleans up old backups | Old backups deleted |
| I-E2E-007 | Delete cascade: Database -> Users/Grants cleaned up | Dependents handled |
| I-E2E-008 | Instance unavailable: Dependent resources degrade | Status reflects issues |
| I-E2E-009 | Password rotation: Application continues working | No downtime |
| I-E2E-010 | GitOps: Apply all resources, expect Ready | All reconciled |

---

## Test Infrastructure

### Testcontainers Setup

```go
// test/integration/suite_test.go
package integration

import (
    "context"
    "testing"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "github.com/testcontainers/testcontainers-go"
    "github.com/testcontainers/testcontainers-go/modules/postgres"
    "github.com/testcontainers/testcontainers-go/modules/mysql"
)

var (
    postgresContainer *postgres.PostgresContainer
    mysqlContainer    *mysql.MySQLContainer
    ctx               context.Context
)

func TestIntegration(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "Integration Suite")
}

var _ = BeforeSuite(func() {
    ctx = context.Background()

    var err error

    // Start PostgreSQL container
    postgresContainer, err = postgres.Run(ctx,
        "postgres:15-alpine",
        postgres.WithDatabase("testdb"),
        postgres.WithUsername("postgres"),
        postgres.WithPassword("postgres"),
        testcontainers.WithWaitStrategy(
            wait.ForLog("database system is ready to accept connections").
                WithOccurrence(2).
                WithStartupTimeout(30*time.Second),
        ),
    )
    Expect(err).NotTo(HaveOccurred())

    // Start MySQL container
    mysqlContainer, err = mysql.Run(ctx,
        "mysql:8.0",
        mysql.WithDatabase("testdb"),
        mysql.WithUsername("root"),
        mysql.WithPassword("mysql"),
        testcontainers.WithWaitStrategy(
            wait.ForLog("ready for connections").
                WithOccurrence(2).
                WithStartupTimeout(60*time.Second),
        ),
    )
    Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
    if postgresContainer != nil {
        _ = postgresContainer.Terminate(ctx)
    }
    if mysqlContainer != nil {
        _ = mysqlContainer.Terminate(ctx)
    }
})
```

### Envtest Setup for Unit Tests

```go
// internal/controller/suite_test.go
package controller

import (
    "context"
    "path/filepath"
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "k8s.io/client-go/kubernetes/scheme"
    "k8s.io/client-go/rest"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/envtest"
    logf "sigs.k8s.io/controller-runtime/pkg/log"
    "sigs.k8s.io/controller-runtime/pkg/log/zap"

    dbopsv1alpha1 "github.com/example/unified-db-operator/api/v1alpha1"
)

var (
    cfg       *rest.Config
    k8sClient client.Client
    testEnv   *envtest.Environment
    ctx       context.Context
    cancel    context.CancelFunc
)

func TestControllers(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
    logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

    ctx, cancel = context.WithCancel(context.TODO())

    By("bootstrapping test environment")
    testEnv = &envtest.Environment{
        CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
        ErrorIfCRDPathMissing: true,
    }

    var err error
    cfg, err = testEnv.Start()
    Expect(err).NotTo(HaveOccurred())
    Expect(cfg).NotTo(BeNil())

    err = dbopsv1alpha1.AddToScheme(scheme.Scheme)
    Expect(err).NotTo(HaveOccurred())

    k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
    Expect(err).NotTo(HaveOccurred())
    Expect(k8sClient).NotTo(BeNil())
})

var _ = AfterSuite(func() {
    cancel()
    By("tearing down the test environment")
    err := testEnv.Stop()
    Expect(err).NotTo(HaveOccurred())
})
```

### Example Unit Test

```go
// internal/controller/database_controller_test.go
package controller

import (
    "context"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/types"

    dbopsv1alpha1 "github.com/example/unified-db-operator/api/v1alpha1"
)

var _ = Describe("Database Controller", func() {
    const (
        timeout  = time.Second * 10
        interval = time.Millisecond * 250
    )

    Context("When creating a Database", func() {
        It("Should set status to Pending initially", func() {
            ctx := context.Background()

            database := &dbopsv1alpha1.Database{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-database",
                    Namespace: "default",
                },
                Spec: dbopsv1alpha1.DatabaseSpec{
                    InstanceRef: dbopsv1alpha1.InstanceReference{
                        Name: "test-instance",
                    },
                    Name:             "testdb",
                    DeletionPolicy:   "Retain",
                    PostgresConfig: &dbopsv1alpha1.PostgresDatabaseConfig{
                        Encoding: "UTF8",
                    },
                },
            }

            Expect(k8sClient.Create(ctx, database)).Should(Succeed())

            lookupKey := types.NamespacedName{Name: "test-database", Namespace: "default"}
            createdDatabase := &dbopsv1alpha1.Database{}

            Eventually(func() bool {
                err := k8sClient.Get(ctx, lookupKey, createdDatabase)
                return err == nil
            }, timeout, interval).Should(BeTrue())

            Expect(createdDatabase.Spec.Name).Should(Equal("testdb"))
        })
    })

    Context("When validating Database spec", func() {
        It("Should reject if no engine section is set", func() {
            ctx := context.Background()

            database := &dbopsv1alpha1.Database{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "invalid-database",
                    Namespace: "default",
                },
                Spec: dbopsv1alpha1.DatabaseSpec{
                    InstanceRef: dbopsv1alpha1.InstanceReference{
                        Name: "test-instance",
                    },
                    Name:           "testdb",
                    DeletionPolicy: "Retain",
                    // No PostgresConfig or MySQLConfig
                },
            }

            err := k8sClient.Create(ctx, database)
            Expect(err).Should(HaveOccurred())
            Expect(err.Error()).Should(ContainSubstring("one of postgres, mysql"))
        })

        It("Should reject if multiple engine sections are set", func() {
            ctx := context.Background()

            database := &dbopsv1alpha1.Database{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "invalid-database-2",
                    Namespace: "default",
                },
                Spec: dbopsv1alpha1.DatabaseSpec{
                    InstanceRef: dbopsv1alpha1.InstanceReference{
                        Name: "test-instance",
                    },
                    Name:           "testdb",
                    DeletionPolicy: "Retain",
                    PostgresConfig: &dbopsv1alpha1.PostgresDatabaseConfig{
                        Encoding: "UTF8",
                    },
                    MySQLConfig: &dbopsv1alpha1.MySQLDatabaseConfig{
                        Charset: "utf8mb4",
                    },
                },
            }

            err := k8sClient.Create(ctx, database)
            Expect(err).Should(HaveOccurred())
            Expect(err.Error()).Should(ContainSubstring("only one of"))
        })
    })
})
```

### Example Integration Test

```go
// test/integration/postgres_test.go
package integration

import (
    "context"
    "database/sql"
    "fmt"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    _ "github.com/jackc/pgx/v5/stdlib"

    "github.com/example/unified-db-operator/internal/adapter/postgres"
)

var _ = Describe("PostgreSQL Adapter Integration", func() {
    var (
        adapter *postgres.Adapter
        db      *sql.DB
        connStr string
    )

    BeforeEach(func() {
        var err error
        connStr, err = postgresContainer.ConnectionString(ctx, "sslmode=disable")
        Expect(err).NotTo(HaveOccurred())

        db, err = sql.Open("pgx", connStr)
        Expect(err).NotTo(HaveOccurred())

        adapter = postgres.NewAdapter(db)
    })

    AfterEach(func() {
        if db != nil {
            db.Close()
        }
    })

    Describe("CreateDatabase", func() {
        It("should create a database with default settings", func() {
            err := adapter.CreateDatabase(ctx, postgres.CreateDatabaseParams{
                Name: "test_default_db",
            })
            Expect(err).NotTo(HaveOccurred())

            // Verify database exists
            exists, err := adapter.DatabaseExists(ctx, "test_default_db")
            Expect(err).NotTo(HaveOccurred())
            Expect(exists).To(BeTrue())

            // Cleanup
            err = adapter.DropDatabase(ctx, "test_default_db", false)
            Expect(err).NotTo(HaveOccurred())
        })

        It("should create a database with UTF8 encoding", func() {
            err := adapter.CreateDatabase(ctx, postgres.CreateDatabaseParams{
                Name:     "test_utf8_db",
                Encoding: "UTF8",
            })
            Expect(err).NotTo(HaveOccurred())

            // Verify encoding
            encoding, err := adapter.GetDatabaseEncoding(ctx, "test_utf8_db")
            Expect(err).NotTo(HaveOccurred())
            Expect(encoding).To(Equal("UTF8"))

            // Cleanup
            err = adapter.DropDatabase(ctx, "test_utf8_db", false)
            Expect(err).NotTo(HaveOccurred())
        })

        It("should create a database with extensions", func() {
            err := adapter.CreateDatabase(ctx, postgres.CreateDatabaseParams{
                Name: "test_ext_db",
            })
            Expect(err).NotTo(HaveOccurred())

            err = adapter.InstallExtension(ctx, "test_ext_db", postgres.ExtensionParams{
                Name:   "uuid-ossp",
                Schema: "public",
            })
            Expect(err).NotTo(HaveOccurred())

            // Verify extension
            extensions, err := adapter.ListExtensions(ctx, "test_ext_db")
            Expect(err).NotTo(HaveOccurred())
            Expect(extensions).To(ContainElement("uuid-ossp"))

            // Cleanup
            err = adapter.DropDatabase(ctx, "test_ext_db", false)
            Expect(err).NotTo(HaveOccurred())
        })
    })

    Describe("CreateUser", func() {
        It("should create a user with password", func() {
            err := adapter.CreateUser(ctx, postgres.CreateUserParams{
                Username: "test_user",
                Password: "securepassword123",
            })
            Expect(err).NotTo(HaveOccurred())

            // Verify user can connect
            userConnStr := fmt.Sprintf(
                "postgres://test_user:securepassword123@%s:%s/postgres?sslmode=disable",
                postgresContainer.Host(ctx),
                postgresContainer.MappedPort(ctx, "5432"),
            )
            userDB, err := sql.Open("pgx", userConnStr)
            Expect(err).NotTo(HaveOccurred())
            defer userDB.Close()

            err = userDB.Ping()
            Expect(err).NotTo(HaveOccurred())

            // Cleanup
            err = adapter.DropUser(ctx, "test_user")
            Expect(err).NotTo(HaveOccurred())
        })

        It("should create a user with connection limit", func() {
            err := adapter.CreateUser(ctx, postgres.CreateUserParams{
                Username:        "test_limited_user",
                Password:        "password123",
                ConnectionLimit: 5,
            })
            Expect(err).NotTo(HaveOccurred())

            // Verify connection limit
            limit, err := adapter.GetUserConnectionLimit(ctx, "test_limited_user")
            Expect(err).NotTo(HaveOccurred())
            Expect(limit).To(Equal(5))

            // Cleanup
            err = adapter.DropUser(ctx, "test_limited_user")
            Expect(err).NotTo(HaveOccurred())
        })
    })

    Describe("Grant Permissions", func() {
        BeforeEach(func() {
            // Create test database and user
            _ = adapter.CreateDatabase(ctx, postgres.CreateDatabaseParams{Name: "grant_test_db"})
            _ = adapter.CreateUser(ctx, postgres.CreateUserParams{
                Username: "grant_test_user",
                Password: "password123",
            })
        })

        AfterEach(func() {
            _ = adapter.DropDatabase(ctx, "grant_test_db", true)
            _ = adapter.DropUser(ctx, "grant_test_user")
        })

        It("should grant SELECT on all tables", func() {
            err := adapter.GrantPrivileges(ctx, postgres.GrantParams{
                Database:   "grant_test_db",
                Schema:     "public",
                Tables:     []string{"*"},
                Privileges: []string{"SELECT"},
                Grantee:    "grant_test_user",
            })
            Expect(err).NotTo(HaveOccurred())

            // Create a test table
            _, err = db.ExecContext(ctx, `
                CREATE TABLE IF NOT EXISTS grant_test_db.public.test_table (id int);
                INSERT INTO grant_test_db.public.test_table VALUES (1);
            `)
            // Note: In real test, would need to connect to grant_test_db

            // Verify user can SELECT
            // (Would connect as grant_test_user and verify access)
        })
    })
})
```

---

## Makefile Targets

```makefile
# Test targets
.PHONY: test test-unit test-integration test-coverage

# Run all tests
test: test-unit test-integration

# Run unit tests only (fast, no external dependencies)
test-unit:
	go test ./internal/... -v -short -coverprofile=coverage-unit.out

# Run integration tests (requires Docker for Testcontainers)
test-integration:
	go test ./test/integration/... -v -timeout 30m -coverprofile=coverage-integration.out

# Run tests with coverage report
test-coverage: test
	go tool cover -html=coverage-unit.out -o coverage-unit.html
	go tool cover -html=coverage-integration.out -o coverage-integration.html

# Run specific test
test-focus:
	go test ./internal/controller/... -v -ginkgo.focus="$(FOCUS)"

# Lint tests
lint-tests:
	golangci-lint run ./test/...
```

---

## CI Pipeline (GitHub Actions)

```yaml
# .github/workflows/test.yaml
name: Tests

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  unit-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.22'

      - name: Install kubebuilder
        run: |
          curl -L -o kubebuilder https://go.kubebuilder.io/dl/latest/$(go env GOOS)/$(go env GOARCH)
          chmod +x kubebuilder && sudo mv kubebuilder /usr/local/bin/

      - name: Run unit tests
        run: make test-unit

      - name: Upload coverage
        uses: codecov/codecov-action@v4
        with:
          files: coverage-unit.out
          flags: unit

  integration-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.22'

      - name: Run integration tests
        run: make test-integration

      - name: Upload coverage
        uses: codecov/codecov-action@v4
        with:
          files: coverage-integration.out
          flags: integration
```

---

## Coverage Requirements

| Component | Minimum Coverage |
|-----------|-----------------|
| Controllers | 80% |
| Adapters | 85% |
| Secret Manager | 90% |
| Storage Manager | 80% |
| Validators | 95% |
| Utils | 90% |

---

## Summary

This document provides:

1. **Complete CRD specifications** with engine-specific sections (Option 2)
2. **Velero-style backup/restore** with separate Schedule and Backup resources
3. **Comprehensive test plan** with 300+ test cases
4. **Test infrastructure** using Envtest and Testcontainers
5. **Example test code** for unit and integration tests

Use this as the reference for implementing the Unified DB Management Operator.
