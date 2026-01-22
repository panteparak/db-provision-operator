# MySQL Examples

Complete examples for MySQL database provisioning.

## Basic Setup

### Step 1: Admin Credentials

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: mysql-admin-credentials
type: Opaque
stringData:
  username: root
  password: your-admin-password
```

### Step 2: Database Instance

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: mysql-primary
spec:
  engine: mysql
  connection:
    host: mysql.database.svc.cluster.local
    port: 3306
    database: mysql
    secretRef:
      name: mysql-admin-credentials
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
    name: mysql-primary
  name: myapp
  deletionPolicy: Retain
  mysql:
    charset: utf8mb4
    collation: utf8mb4_unicode_ci
```

### Step 4: Application User

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: myapp-user
spec:
  instanceRef:
    name: mysql-primary
  username: myapp_user
  passwordSecret:
    generate: true
    length: 32
    secretName: myapp-user-credentials
    secretTemplate:
      labels:
        app: myapp
      data:
        DATABASE_URL: "mysql://{{ .Username }}:{{ .Password }}@{{ .Host }}:{{ .Port }}/myapp"
        JDBC_URL: "jdbc:mysql://{{ .Host }}:{{ .Port }}/myapp?user={{ .Username }}&password={{ .Password }}"
```

### Step 5: User Grants

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseGrant
metadata:
  name: myapp-user-grants
spec:
  userRef:
    name: myapp-user
  mysql:
    grants:
      - level: database
        database: myapp
        privileges: [SELECT, INSERT, UPDATE, DELETE]
```

## Role-Based Access (MySQL 8.0+)

### Read-Only Role

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseRole
metadata:
  name: myapp-readonly
spec:
  instanceRef:
    name: mysql-primary
  roleName: myapp_readonly
  mysql:
    grants:
      - level: database
        database: myapp
        privileges: [SELECT]
```

### Read-Write Role

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseRole
metadata:
  name: myapp-readwrite
spec:
  instanceRef:
    name: mysql-primary
  roleName: myapp_readwrite
  mysql:
    grants:
      - level: database
        database: myapp
        privileges: [SELECT, INSERT, UPDATE, DELETE]
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
  mysql:
    roles:
      - myapp_readwrite
```

## Host-Restricted Users

### Application User with Host Restrictions

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: app-user-restricted
spec:
  instanceRef:
    name: mysql-primary
  username: app_user
  passwordSecret:
    generate: true
    secretName: app-user-credentials
  mysql:
    maxUserConnections: 100
    authPlugin: caching_sha2_password
    requireSSL: true
    allowedHosts:
      - "10.0.0.%"      # Internal network
      - "192.168.1.%"   # Secondary network
```

### Admin User for Specific Host

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: admin-user
spec:
  instanceRef:
    name: mysql-primary
  username: admin
  passwordSecret:
    generate: true
    length: 48
    secretName: admin-credentials
  mysql:
    authPlugin: caching_sha2_password
    requireSSL: true
    allowedHosts:
      - "10.0.0.1"  # Only from specific admin host
```

## Table-Level Permissions

### Granular Table Access

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseGrant
metadata:
  name: reporting-user-grants
spec:
  userRef:
    name: reporting-user
  mysql:
    grants:
      # Full read access to specific tables
      - level: table
        database: myapp
        table: orders
        privileges: [SELECT]
      - level: table
        database: myapp
        table: customers
        privileges: [SELECT]
      - level: table
        database: myapp
        table: products
        privileges: [SELECT]
      # No access to sensitive tables (users, payments, etc.)
```

### Service Account with Limited Write

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseGrant
metadata:
  name: order-service-grants
spec:
  userRef:
    name: order-service
  mysql:
    grants:
      # Read from reference tables
      - level: table
        database: myapp
        table: products
        privileges: [SELECT]
      - level: table
        database: myapp
        table: customers
        privileges: [SELECT]
      # Full access to orders
      - level: table
        database: myapp
        table: orders
        privileges: [SELECT, INSERT, UPDATE]
      # Insert only to audit
      - level: table
        database: myapp
        table: order_audit
        privileges: [INSERT]
```

## Backup Configuration

### One-Time Backup

```yaml
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
      prefix: mysql/myapp
      secretRef:
        name: s3-backup-credentials
  compression:
    enabled: true
    algorithm: gzip
  mysql:
    method: mysqldump
    singleTransaction: true
    routines: true
    triggers: true
  ttl: "720h"
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
  schedule: "0 3 * * *"  # Daily at 3 AM
  timezone: "UTC"
  retention:
    keepLast: 7
    keepDaily: 7
    keepWeekly: 4
  backupTemplate:
    storage:
      type: s3
      s3:
        bucket: my-database-backups
        region: us-east-1
        prefix: mysql/myapp/scheduled
        secretRef:
          name: s3-backup-credentials
    compression:
      enabled: true
      algorithm: gzip
    mysql:
      method: mysqldump
      singleTransaction: true
      routines: true
      triggers: true
```

## Complete Application Stack

### Full Example

```yaml
---
# Admin credentials
apiVersion: v1
kind: Secret
metadata:
  name: mysql-admin
type: Opaque
stringData:
  username: root
  password: super-secret-admin-password
---
# Database instance
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: mysql-primary
spec:
  engine: mysql
  connection:
    host: mysql.database.svc.cluster.local
    port: 3306
    database: mysql
    secretRef:
      name: mysql-admin
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
    name: mysql-primary
  name: myapp
  deletionPolicy: Retain
  mysql:
    charset: utf8mb4
    collation: utf8mb4_unicode_ci
---
# Application user
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: myapp-user
spec:
  instanceRef:
    name: mysql-primary
  username: myapp_user
  passwordSecret:
    generate: true
    length: 32
    secretName: myapp-credentials
    secretTemplate:
      data:
        DATABASE_URL: "mysql://{{ .Username }}:{{ .Password }}@{{ .Host }}:{{ .Port }}/myapp"
  mysql:
    maxUserConnections: 100
    authPlugin: caching_sha2_password
    allowedHosts:
      - "10.0.0.%"
---
# User grants
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseGrant
metadata:
  name: myapp-user-grant
spec:
  userRef:
    name: myapp-user
  mysql:
    grants:
      - level: database
        database: myapp
        privileges: [SELECT, INSERT, UPDATE, DELETE]
---
# Read-only user for reporting
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: reporting-user
spec:
  instanceRef:
    name: mysql-primary
  username: reporting
  passwordSecret:
    generate: true
    secretName: reporting-credentials
  mysql:
    maxUserConnections: 10
---
# Reporting grants
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseGrant
metadata:
  name: reporting-grant
spec:
  userRef:
    name: reporting-user
  mysql:
    grants:
      - level: database
        database: myapp
        privileges: [SELECT]
```

## Verify Setup

```bash
# Check instance status
kubectl get databaseinstance mysql-primary

# Check all resources
kubectl get databases,databaseusers,databasegrants

# Get connection info
kubectl get secret myapp-credentials -o jsonpath='{.data.DATABASE_URL}' | base64 -d

# Test connection
kubectl run mysql-test --rm -it --image=mysql:8.0 -- \
  mysql -h mysql.database.svc.cluster.local -u myapp_user -p myapp
```

## MariaDB Compatibility

These examples work with MariaDB by changing the engine:

```yaml
spec:
  engine: mariadb  # Instead of mysql
```

All MySQL configuration options are compatible with MariaDB.
