# Admin Account Setup

This guide explains how to create a dedicated **least-privilege admin account** for the DB Provision Operator. Using a least-privilege account instead of superuser/root is a security best practice for production environments.

## Overview

The operator needs an admin account with sufficient privileges to:

- Create and drop databases
- Create and manage users/roles
- Grant and revoke privileges
- Terminate connections (for force-drop operations)
- Perform backup operations (SELECT access)

Each database engine has a different privilege model. This guide provides complete SQL scripts for PostgreSQL, MySQL, and MariaDB.

!!! warning "Do Not Use Superuser/Root"
    While superuser accounts work, they pose unnecessary security risks. A compromised operator could affect all databases on the server. Always use a dedicated least-privilege account.

---

## PostgreSQL

### Required Privileges

| Privilege/Attribute | Purpose |
|---------------------|---------|
| `LOGIN` | Connect to the database |
| `CREATEDB` | Create and drop databases |
| `CREATEROLE` | Create and manage users/roles |
| `pg_signal_backend` | Terminate connections (force-drop) |
| `pg_read_all_data` (PG14+) | Backup operations |
| `CONNECT ON template1` | Create databases from template |

### SQL Script

Connect as a PostgreSQL superuser (e.g., `postgres`) and execute:

```sql
-- ============================================
-- PostgreSQL Admin Account for DB Provision Operator
-- ============================================

-- Step 1: Create the operator admin role
CREATE ROLE dbprovision_admin WITH
    LOGIN
    CREATEDB
    CREATEROLE
    PASSWORD 'your-secure-password-here';

-- Step 2: Grant connection termination capability
-- Required for force-dropping databases with active connections
GRANT pg_signal_backend TO dbprovision_admin;

-- Step 3: Grant read access for backup operations (PostgreSQL 14+)
-- For PostgreSQL 12/13, grant SELECT on specific databases instead
GRANT pg_read_all_data TO dbprovision_admin;

-- Step 4: Grant template1 access for database creation
GRANT CONNECT ON DATABASE template1 TO dbprovision_admin;

-- Step 5: (Optional) Allow creating extensions in new databases
-- Note: Some extensions require superuser; pre-install them if needed
-- GRANT CREATE ON DATABASE template1 TO dbprovision_admin;

-- Verify the setup
\du dbprovision_admin
```

### PostgreSQL 12/13 (Without pg_read_all_data)

For older PostgreSQL versions, grant SELECT explicitly:

```sql
-- After creating each database managed by the operator:
GRANT SELECT ON ALL TABLES IN SCHEMA public TO dbprovision_admin;
GRANT SELECT ON ALL SEQUENCES IN SCHEMA public TO dbprovision_admin;

-- For future tables (run as database owner):
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT ON TABLES TO dbprovision_admin;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT ON SEQUENCES TO dbprovision_admin;
```

### Extension Considerations

Some PostgreSQL extensions require superuser to install:

| Extension | Requires Superuser | Solution |
|-----------|-------------------|----------|
| `pg_stat_statements` | Yes | Pre-install by DBA |
| `uuid-ossp` | Yes | Pre-install by DBA |
| `pgcrypto` | No | Operator can install |
| `hstore` | No | Operator can install |
| `pg_trgm` | No | Operator can install |

For extensions requiring superuser, pre-install them in `template1`:

```sql
-- Run as superuser in template1
\c template1
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
```

### Verification

```sql
-- Check role attributes
SELECT rolname, rolsuper, rolcreaterole, rolcreatedb, rolcanlogin
FROM pg_roles
WHERE rolname = 'dbprovision_admin';

-- Expected output:
--      rolname       | rolsuper | rolcreaterole | rolcreatedb | rolcanlogin
-- -------------------+----------+---------------+-------------+-------------
--  dbprovision_admin | f        | t             | t           | t

-- Check role memberships
SELECT r.rolname AS role, m.rolname AS member
FROM pg_auth_members am
JOIN pg_roles r ON am.roleid = r.oid
JOIN pg_roles m ON am.member = m.oid
WHERE m.rolname = 'dbprovision_admin';

-- Expected: pg_signal_backend, pg_read_all_data
```

---

## MySQL

### Required Privileges

| Privilege | Purpose |
|-----------|---------|
| `CREATE, DROP, ALTER ON *.*` | Database operations |
| `CREATE USER ON *.*` | User management |
| `GRANT OPTION ON *.*` | Delegate privileges to users |
| `SELECT ON mysql.*` | Query user/grant metadata |
| `RELOAD ON *.*` | FLUSH PRIVILEGES |
| `CONNECTION_ADMIN ON *.*` | Kill connections (force-drop) |
| `ROLE_ADMIN ON *.*` | Role management (MySQL 8.0+) |
| `SELECT, SHOW VIEW, TRIGGER, LOCK TABLES ON *.*` | Backup operations |

### SQL Script

Connect as MySQL root and execute:

```sql
-- ============================================
-- MySQL Admin Account for DB Provision Operator
-- ============================================

-- Step 1: Create the operator admin user
CREATE USER 'dbprovision_admin'@'%' IDENTIFIED BY 'your-secure-password-here';

-- Step 2: Grant database operations
GRANT CREATE, DROP, ALTER ON *.* TO 'dbprovision_admin'@'%';

-- Step 3: Grant user management
GRANT CREATE USER ON *.* TO 'dbprovision_admin'@'%';
GRANT GRANT OPTION ON *.* TO 'dbprovision_admin'@'%';

-- Step 4: Grant system table access for metadata queries
GRANT SELECT ON mysql.user TO 'dbprovision_admin'@'%';
GRANT SELECT ON mysql.db TO 'dbprovision_admin'@'%';
GRANT SELECT ON mysql.tables_priv TO 'dbprovision_admin'@'%';
GRANT SELECT ON mysql.columns_priv TO 'dbprovision_admin'@'%';
GRANT SELECT ON mysql.procs_priv TO 'dbprovision_admin'@'%';
GRANT SELECT ON mysql.global_grants TO 'dbprovision_admin'@'%';

-- Step 5: Grant administrative operations
GRANT RELOAD ON *.* TO 'dbprovision_admin'@'%';
GRANT CONNECTION_ADMIN ON *.* TO 'dbprovision_admin'@'%';
GRANT PROCESS ON *.* TO 'dbprovision_admin'@'%';

-- Step 6: Grant role management (MySQL 8.0+)
GRANT ROLE_ADMIN ON *.* TO 'dbprovision_admin'@'%';

-- Step 7: Grant data operations for managed databases and backup operations
-- INSERT, UPDATE, DELETE are needed to grant these privileges to application users
GRANT SELECT, INSERT, UPDATE, DELETE, SHOW VIEW, TRIGGER, LOCK TABLES ON *.* TO 'dbprovision_admin'@'%';
-- Note: information_schema access is automatic based on privileges on actual database objects

-- Step 8: Apply changes
FLUSH PRIVILEGES;

-- Verify the setup
SHOW GRANTS FOR 'dbprovision_admin'@'%';
```

### Restricted Network Access (Recommended)

For production, restrict the admin account to specific hosts:

```sql
-- Create user accessible only from Kubernetes pod network
CREATE USER 'dbprovision_admin'@'10.0.0.0/255.0.0.0'
    IDENTIFIED BY 'your-secure-password-here';

-- Or specific namespace CIDR
CREATE USER 'dbprovision_admin'@'10.244.%.%'
    IDENTIFIED BY 'your-secure-password-here';

-- Grant same privileges as above to this restricted user
```

### Verification

```sql
-- Check user exists
SELECT User, Host FROM mysql.user WHERE User = 'dbprovision_admin';

-- Check grants (should NOT show ALL PRIVILEGES or SUPER)
SHOW GRANTS FOR 'dbprovision_admin'@'%';

-- Expected grants (partial list):
-- GRANT SELECT, RELOAD, CREATE USER, PROCESS,
--       CREATE, DROP, ALTER, LOCK TABLES, SHOW VIEW, TRIGGER,
--       CONNECTION_ADMIN, ROLE_ADMIN
-- ON *.* TO 'dbprovision_admin'@'%' WITH GRANT OPTION
```

---

## MariaDB

MariaDB is MySQL-compatible but has some differences in privilege handling.

### Differences from MySQL

| Feature | MySQL 8.0 | MariaDB |
|---------|-----------|---------|
| Role system | `CREATE ROLE` statement | Roles are users (same as older MySQL) |
| Role admin | `ROLE_ADMIN` privilege | `CREATE USER` + `WITH GRANT OPTION` |
| Default auth | `caching_sha2_password` | `mysql_native_password` |
| Global grants table | `mysql.global_grants` | Not available |

### SQL Script

Connect as MariaDB root and execute:

```sql
-- ============================================
-- MariaDB Admin Account for DB Provision Operator
-- ============================================

-- Step 1: Create the operator admin user
CREATE USER 'dbprovision_admin'@'%' IDENTIFIED BY 'your-secure-password-here';

-- Step 2: Grant database operations
GRANT CREATE, DROP, ALTER ON *.* TO 'dbprovision_admin'@'%';

-- Step 3: Grant user management with grant option for role delegation
GRANT CREATE USER ON *.* TO 'dbprovision_admin'@'%' WITH GRANT OPTION;

-- Step 4: Grant system table access for metadata queries
GRANT SELECT ON mysql.user TO 'dbprovision_admin'@'%';
GRANT SELECT ON mysql.db TO 'dbprovision_admin'@'%';
GRANT SELECT ON mysql.tables_priv TO 'dbprovision_admin'@'%';
GRANT SELECT ON mysql.columns_priv TO 'dbprovision_admin'@'%';
GRANT SELECT ON mysql.procs_priv TO 'dbprovision_admin'@'%';

-- Step 5: Grant administrative operations
GRANT RELOAD ON *.* TO 'dbprovision_admin'@'%';
GRANT PROCESS ON *.* TO 'dbprovision_admin'@'%';

-- Note: MariaDB uses SUPER or CONNECTION ADMIN for killing connections
-- CONNECTION ADMIN was added in MariaDB 10.5.2
GRANT CONNECTION ADMIN ON *.* TO 'dbprovision_admin'@'%';

-- Step 6: Grant data operations for managed databases and backup operations
-- INSERT, UPDATE, DELETE are needed to grant these privileges to application users
GRANT SELECT, INSERT, UPDATE, DELETE, SHOW VIEW, TRIGGER, LOCK TABLES ON *.* TO 'dbprovision_admin'@'%';
-- Note: information_schema access is automatic based on privileges on actual database objects

-- Step 7: Apply changes
FLUSH PRIVILEGES;

-- Verify the setup
SHOW GRANTS FOR 'dbprovision_admin'@'%';
```

### MariaDB 10.4 and Earlier

For older MariaDB versions without `CONNECTION ADMIN`:

```sql
-- Use SUPER for connection termination (less ideal but necessary)
-- WARNING: SUPER is a very broad privilege
-- Consider upgrading to MariaDB 10.5+ instead
GRANT SUPER ON *.* TO 'dbprovision_admin'@'%';
```

### Verification

```sql
-- Check user exists
SELECT User, Host FROM mysql.user WHERE User = 'dbprovision_admin';

-- Check grants
SHOW GRANTS FOR 'dbprovision_admin'@'%';

-- Verify role capabilities (create a test role)
CREATE ROLE IF NOT EXISTS test_role_check;
DROP ROLE IF EXISTS test_role_check;
```

---

## Kubernetes Secret

After creating the admin account, create a Kubernetes Secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: db-admin-credentials
  namespace: db-provision-operator-system
type: Opaque
stringData:
  username: dbprovision_admin
  password: your-secure-password-here
```

Or create it with kubectl:

```bash
kubectl create secret generic db-admin-credentials \
  --namespace=db-provision-operator-system \
  --from-literal=username=dbprovision_admin \
  --from-literal=password='your-secure-password-here'
```

### Password Generation

Generate a secure random password:

```bash
# Using openssl (recommended)
openssl rand -base64 32

# Using /dev/urandom
head -c 32 /dev/urandom | base64

# Using pwgen
pwgen -s 32 1
```

---

## DatabaseInstance Configuration

Reference the admin credentials in your DatabaseInstance:

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
    sslMode: require
    secretRef:
      name: db-admin-credentials
```

---

## Security Recommendations

### 1. Password Rotation

Regularly rotate the admin account password:

```bash
# 1. Generate new password
NEW_PASSWORD=$(openssl rand -base64 32)

# 2. Update in database (run appropriate SQL)
# PostgreSQL: ALTER ROLE dbprovision_admin WITH PASSWORD 'new-password';
# MySQL/MariaDB: ALTER USER 'dbprovision_admin'@'%' IDENTIFIED BY 'new-password';

# 3. Update Kubernetes secret
kubectl create secret generic db-admin-credentials \
  --namespace=db-provision-operator-system \
  --from-literal=username=dbprovision_admin \
  --from-literal=password="$NEW_PASSWORD" \
  --dry-run=client -o yaml | kubectl apply -f -

# 4. Restart operator to pick up new credentials
kubectl rollout restart deployment/db-provision-operator-controller-manager \
  -n db-provision-operator-system
```

### 2. Network Restrictions

- Use Kubernetes NetworkPolicies to restrict operator-to-database traffic
- Configure database firewall rules (pg_hba.conf for PostgreSQL)
- Use host restrictions in MySQL/MariaDB user definitions

### 3. TLS Encryption

Always enable TLS for production:

```yaml
spec:
  connection:
    sslMode: verify-full
    tls:
      secretRef:
        name: db-tls-certs
```

### 4. Audit Logging

Enable database audit logging for the admin account:

**PostgreSQL (pgaudit):**
```sql
ALTER ROLE dbprovision_admin SET pgaudit.log = 'ddl,role';
```

**MySQL:**
```sql
SET GLOBAL audit_log_include_accounts = 'dbprovision_admin@%';
```

### 5. Least Privilege Verification

Periodically verify the account doesn't have superuser:

```bash
# PostgreSQL
psql -U dbprovision_admin -c "SELECT rolsuper FROM pg_roles WHERE rolname = current_user;"
# Should return: f (false)

# MySQL
mysql -u dbprovision_admin -e "SELECT SUPER_PRIV FROM mysql.user WHERE User = 'dbprovision_admin';"
# Should return: N (No)
```

---

## Troubleshooting

### Permission Denied Errors

If the operator logs show permission errors:

1. Check the admin account has all required privileges
2. Verify the secret contains correct credentials
3. Test connectivity manually:

```bash
# PostgreSQL
kubectl run psql-test --rm -it --image=postgres:16 -- \
  psql "postgresql://dbprovision_admin:password@host:5432/postgres?sslmode=require"

# MySQL/MariaDB
kubectl run mysql-test --rm -it --image=mysql:8 -- \
  mysql -h host -u dbprovision_admin -ppassword -e "SHOW GRANTS;"
```

### Cannot Create Database

PostgreSQL:
```sql
-- Check CREATEDB attribute
SELECT rolcreatedb FROM pg_roles WHERE rolname = 'dbprovision_admin';
-- If false, grant it:
ALTER ROLE dbprovision_admin WITH CREATEDB;
```

MySQL/MariaDB:
```sql
-- Check CREATE privilege
SHOW GRANTS FOR 'dbprovision_admin'@'%';
-- If missing, grant it:
GRANT CREATE ON *.* TO 'dbprovision_admin'@'%';
```

### Cannot Terminate Connections

PostgreSQL:
```sql
-- Check pg_signal_backend membership
SELECT pg_has_role('dbprovision_admin', 'pg_signal_backend', 'MEMBER');
-- If false:
GRANT pg_signal_backend TO dbprovision_admin;
```

MySQL/MariaDB:
```sql
-- Check CONNECTION_ADMIN or SUPER
SHOW GRANTS FOR 'dbprovision_admin'@'%';
-- If missing:
GRANT CONNECTION_ADMIN ON *.* TO 'dbprovision_admin'@'%';
```

---

## Next Steps

- [PostgreSQL Guide](../engines/postgresql.md) - PostgreSQL-specific features
- [MySQL Guide](../engines/mysql.md) - MySQL-specific features
- [MariaDB Guide](../engines/mariadb.md) - MariaDB-specific features
- [Monitoring](monitoring.md) - Set up monitoring and alerting
