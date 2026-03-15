# PostgreSQL Examples

Complete examples for PostgreSQL database provisioning.

## Basic Setup

### Step 1: Admin Credentials

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: postgres-admin-credentials
type: Opaque
stringData:
  username: postgres
  password: your-admin-password
```

### Step 2: Database Instance

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
    sslMode: prefer
    secretRef:
      name: postgres-admin-credentials
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
    name: postgres-primary
  name: myapp
  deletionPolicy: Retain
  postgres:
    encoding: UTF8
    extensions:
      - name: uuid-ossp
        schema: public
      - name: pgcrypto
        schema: public
```

### Step 4: Application User

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: myapp-user
spec:
  instanceRef:
    name: postgres-primary
  username: myapp_user
  passwordSecret:
    generate: true
    length: 32
    secretName: myapp-user-credentials
    secretTemplate:
      labels:
        app: myapp
      data:
        DATABASE_URL: "postgresql://{{ .Username }}:{{ .Password }}@{{ .Host }}:{{ .Port }}/myapp?sslmode=prefer"
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
    name: postgres-primary
  roleName: myapp_readonly
  postgres:
    login: false
    grants:
      - database: myapp
        schema: public
        tables: ["*"]
        privileges: [SELECT]
      - database: myapp
        schema: public
        sequences: ["*"]
        privileges: [USAGE, SELECT]
```

### Read-Write Role

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseRole
metadata:
  name: myapp-readwrite
spec:
  instanceRef:
    name: postgres-primary
  roleName: myapp_readwrite
  postgres:
    login: false
    inRoles: [myapp_readonly]
    grants:
      - database: myapp
        schema: public
        tables: ["*"]
        privileges: [INSERT, UPDATE, DELETE]
      - database: myapp
        schema: public
        sequences: ["*"]
        privileges: [UPDATE]
```

### Assign Role to User

```yaml
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
```

## Multi-Schema Setup

### Database with Multiple Schemas

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: enterprise-app
spec:
  instanceRef:
    name: postgres-primary
  name: enterprise
  postgres:
    encoding: UTF8
    extensions:
      - name: uuid-ossp
      - name: pg_stat_statements
    schemas:
      - name: core
        owner: enterprise_admin
      - name: billing
        owner: enterprise_admin
      - name: analytics
        owner: analytics_admin
      - name: audit
        owner: enterprise_admin
```

### Schema-Specific Grants

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseGrant
metadata:
  name: billing-service-grants
spec:
  userRef:
    name: billing-service
  postgres:
    grants:
      # Full access to billing schema
      - database: enterprise
        schema: billing
        tables: ["*"]
        privileges: [SELECT, INSERT, UPDATE, DELETE]
      # Read-only on core schema
      - database: enterprise
        schema: core
        tables: ["*"]
        privileges: [SELECT]
      # Write to audit
      - database: enterprise
        schema: audit
        tables: [billing_audit]
        privileges: [INSERT]
```

## Init SQL

Bootstrap your database with tables, indexes, and seed data using `initSQL`.

### Inline SQL

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: myapp-database
spec:
  instanceRef:
    name: postgres-primary
  name: myapp
  postgres:
    extensions:
      - name: uuid-ossp
      - name: pgcrypto
    schemas:
      - name: app
  initSQL:
    inline:
      - |
        CREATE TABLE IF NOT EXISTS app.users (
          id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
          email TEXT UNIQUE NOT NULL,
          password_hash TEXT NOT NULL,
          created_at TIMESTAMPTZ DEFAULT now(),
          updated_at TIMESTAMPTZ DEFAULT now()
        );
      - CREATE INDEX IF NOT EXISTS idx_users_email ON app.users (email);
      - |
        CREATE OR REPLACE FUNCTION app.update_updated_at()
        RETURNS TRIGGER AS $$
        BEGIN
          NEW.updated_at = now();
          RETURN NEW;
        END;
        $$ LANGUAGE plpgsql;
      - |
        CREATE TRIGGER trg_users_updated_at
          BEFORE UPDATE ON app.users
          FOR EACH ROW EXECUTE FUNCTION app.update_updated_at();
    failurePolicy: Block
```

### ConfigMap SQL with Block Policy

Store longer migration scripts in a ConfigMap. Statements are separated by `---`.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: myapp-bootstrap
data:
  init.sql: |
    CREATE TABLE IF NOT EXISTS app.organizations (
      id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
      name TEXT UNIQUE NOT NULL,
      plan TEXT NOT NULL DEFAULT 'free',
      created_at TIMESTAMPTZ DEFAULT now()
    );
    ---
    CREATE TABLE IF NOT EXISTS app.members (
      org_id UUID REFERENCES app.organizations(id) ON DELETE CASCADE,
      user_id UUID REFERENCES app.users(id) ON DELETE CASCADE,
      role TEXT NOT NULL DEFAULT 'member',
      PRIMARY KEY (org_id, user_id)
    );
    ---
    INSERT INTO app.organizations (name, plan)
    VALUES ('default', 'enterprise')
    ON CONFLICT (name) DO NOTHING;
---
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: myapp-database
spec:
  instanceRef:
    name: postgres-primary
  name: myapp
  initSQL:
    configMapRef:
      name: myapp-bootstrap
      key: init.sql
    failurePolicy: Block
```

!!! tip "Writing idempotent init SQL"
    - Use `CREATE TABLE IF NOT EXISTS` and `CREATE INDEX IF NOT EXISTS`
    - Use `INSERT ... ON CONFLICT DO NOTHING` for seed data
    - Use `CREATE OR REPLACE FUNCTION` for functions and triggers
    - Order statements so that referenced tables are created before foreign keys

## Backup Configuration

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
  name: myapp-manual-backup
spec:
  databaseRef:
    name: myapp-database
  storage:
    type: s3
    s3:
      bucket: my-database-backups
      region: us-east-1
      prefix: postgres/myapp
      secretRef:
        name: s3-backup-credentials
  compression:
    enabled: true
    algorithm: gzip
  postgres:
    format: custom
    jobs: 4
  ttl: "720h"  # 30 days
```

### Scheduled Backups

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseBackupSchedule
metadata:
  name: myapp-daily-backup
spec:
  databaseRef:
    name: myapp-database
  schedule: "0 2 * * *"  # Daily at 2 AM
  timezone: "UTC"
  retention:
    keepLast: 7
    keepDaily: 7
    keepWeekly: 4
    keepMonthly: 3
  concurrencyPolicy: Forbid
  backupTemplate:
    storage:
      type: s3
      s3:
        bucket: my-database-backups
        region: us-east-1
        prefix: postgres/myapp/scheduled
        secretRef:
          name: s3-backup-credentials
    compression:
      enabled: true
      algorithm: gzip
    postgres:
      format: custom
      jobs: 4
```

### Restore from Backup

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseRestore
metadata:
  name: myapp-restore
spec:
  backupRef:
    name: myapp-manual-backup
  targetDatabaseRef:
    name: myapp-database-restored
  createTarget: true
  postgres:
    noOwner: true
    jobs: 4
    analyze: true
```

## Complete Application Stack

### Full Example

```yaml
---
# Admin credentials
apiVersion: v1
kind: Secret
metadata:
  name: postgres-admin
type: Opaque
stringData:
  username: postgres
  password: super-secret-admin-password
---
# Database instance
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
---
# Application database
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: myapp-db
spec:
  instanceRef:
    name: postgres-primary
  name: myapp
  deletionPolicy: Retain
  deletionProtection: true
  postgres:
    encoding: UTF8
    extensions:
      - name: uuid-ossp
      - name: pgcrypto
    schemas:
      - name: app
---
# Read-only role
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseRole
metadata:
  name: myapp-readonly
spec:
  instanceRef:
    name: postgres-primary
  roleName: myapp_readonly
  postgres:
    login: false
    grants:
      - database: myapp
        schema: app
        tables: ["*"]
        privileges: [SELECT]
---
# Read-write role
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseRole
metadata:
  name: myapp-readwrite
spec:
  instanceRef:
    name: postgres-primary
  roleName: myapp_readwrite
  postgres:
    login: false
    inRoles: [myapp_readonly]
    grants:
      - database: myapp
        schema: app
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
    name: postgres-primary
  username: myapp_user
  passwordSecret:
    generate: true
    length: 32
    secretName: myapp-credentials
    secretTemplate:
      data:
        DATABASE_URL: "postgresql://{{ .Username }}:{{ .Password }}@{{ .Host }}:{{ .Port }}/myapp?sslmode=require"
  postgres:
    connectionLimit: 50
---
# Assign role to user
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseGrant
metadata:
  name: myapp-user-grant
spec:
  userRef:
    name: myapp-user
  postgres:
    roles:
      - myapp_readwrite
    defaultPrivileges:
      - database: myapp
        schema: app
        grantedBy: postgres
        objectType: tables
        privileges: [SELECT, INSERT, UPDATE, DELETE]
```

## Verify Setup

```bash
# Check instance status
kubectl get databaseinstance postgres-primary

# Check all resources
kubectl get databases,databaseusers,databaseroles,databasegrants

# Get connection string
kubectl get secret myapp-credentials -o jsonpath='{.data.DATABASE_URL}' | base64 -d

# Test connection
kubectl run psql --rm -it --image=postgres:15 -- \
  psql "$(kubectl get secret myapp-credentials -o jsonpath='{.data.DATABASE_URL}' | base64 -d)"
```
