# DatabaseGrant

A `DatabaseGrant` manages fine-grained permissions for users and roles.

## Overview

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseGrant
metadata:
  name: myapp-user-grants
spec:
  userRef:
    name: myapp-user
  databaseRef:
    name: myapp-database
  postgres:
    roles:
      - readonly
    grants:
      - database: myapp
        schema: public
        tables: ["*"]
        privileges: [SELECT, INSERT, UPDATE]
```

## Spec Fields

### userRef (conditionally required)

Reference to a `DatabaseUser`. Required if `roleRef` is not specified.

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | DatabaseUser name |
| `namespace` | string | Namespace (optional) |

### roleRef (conditionally required)

Reference to a `DatabaseRole`. Required if `userRef` is not specified.

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | DatabaseRole name |
| `namespace` | string | Namespace (optional) |

### databaseRef (optional)

Reference to a `Database` for context.

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Database name |
| `namespace` | string | Namespace (optional) |

### postgres (optional)

PostgreSQL-specific grant configuration.

| Field | Type | Description |
|-------|------|-------------|
| `roles` | array | Roles to assign to the user |
| `grants` | array | Direct permission grants |
| `defaultPrivileges` | array | Default privileges for future objects |

#### grants

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `database` | string | Yes | Target database |
| `schema` | string | No | Target schema |
| `tables` | array | No | Tables or `["*"]` for all |
| `sequences` | array | No | Sequences or `["*"]` for all |
| `functions` | array | No | Functions |
| `privileges` | array | Yes | Privileges to grant |
| `withGrantOption` | bool | No | Allow grantee to grant to others |

**Table privileges:** `SELECT`, `INSERT`, `UPDATE`, `DELETE`, `TRUNCATE`, `REFERENCES`, `TRIGGER`

**Sequence privileges:** `USAGE`, `SELECT`, `UPDATE`

**Function privileges:** `EXECUTE`

#### defaultPrivileges

Apply privileges automatically to future objects:

| Field | Type | Description |
|-------|------|-------------|
| `database` | string | Target database |
| `schema` | string | Target schema |
| `grantedBy` | string | Role that creates objects |
| `objectType` | string | `tables`, `sequences`, `functions` |
| `privileges` | array | Privileges to grant |

### mysql (optional)

MySQL-specific grant configuration.

| Field | Type | Description |
|-------|------|-------------|
| `roles` | array | MySQL 8.0+ roles to assign |
| `grants` | array | Permission grants |

#### mysql grants

| Field | Type | Description |
|-------|------|-------------|
| `level` | string | `global`, `database`, `table` |
| `database` | string | Database name (for database/table level) |
| `table` | string | Table name (for table level) |
| `privileges` | array | Privileges to grant |

**MySQL privileges:** `SELECT`, `INSERT`, `UPDATE`, `DELETE`, `CREATE`, `DROP`, `INDEX`, `ALTER`, `EXECUTE`, `CREATE VIEW`, `SHOW VIEW`, etc.

## Status

| Field | Description |
|-------|-------------|
| `phase` | Current phase |
| `conditions` | Detailed conditions |

## Examples

### Assign Roles to User

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
      - readonly
      - analytics
```

### Direct Table Grants

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseGrant
metadata:
  name: myapp-user-table-grants
spec:
  userRef:
    name: myapp-user
  postgres:
    grants:
      - database: myapp
        schema: public
        tables: [users, orders, products]
        privileges: [SELECT, INSERT, UPDATE]
      - database: myapp
        schema: public
        tables: [audit_log]
        privileges: [SELECT]  # Read-only on audit
```

### Grant All Tables in Schema

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseGrant
metadata:
  name: myapp-user-all-tables
spec:
  userRef:
    name: myapp-user
  postgres:
    grants:
      - database: myapp
        schema: app
        tables: ["*"]
        privileges: [SELECT, INSERT, UPDATE, DELETE]
```

### Default Privileges for Future Tables

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseGrant
metadata:
  name: myapp-default-privileges
spec:
  userRef:
    name: myapp-user
  postgres:
    defaultPrivileges:
      - database: myapp
        schema: public
        grantedBy: myapp_admin
        objectType: tables
        privileges: [SELECT, INSERT, UPDATE, DELETE]
      - database: myapp
        schema: public
        grantedBy: myapp_admin
        objectType: sequences
        privileges: [USAGE, SELECT]
```

### Combined Roles and Grants

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseGrant
metadata:
  name: myapp-user-full-access
spec:
  userRef:
    name: myapp-user
  databaseRef:
    name: myapp-database
  postgres:
    roles:
      - readonly_base  # Inherit basic read
    grants:
      - database: myapp
        schema: app
        tables: ["*"]
        privileges: [INSERT, UPDATE, DELETE]  # Add write
    defaultPrivileges:
      - database: myapp
        schema: app
        grantedBy: app_admin
        objectType: tables
        privileges: [SELECT, INSERT, UPDATE, DELETE]
```

### MySQL Database-Level Grant

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseGrant
metadata:
  name: myapp-mysql-grants
spec:
  userRef:
    name: myapp-mysql-user
  mysql:
    grants:
      - level: database
        database: myapp
        privileges: [SELECT, INSERT, UPDATE, DELETE]
```

### MySQL Table-Level Grant

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseGrant
metadata:
  name: myapp-mysql-table-grants
spec:
  userRef:
    name: myapp-mysql-user
  mysql:
    grants:
      - level: table
        database: myapp
        table: users
        privileges: [SELECT, INSERT]
      - level: table
        database: myapp
        table: audit_log
        privileges: [SELECT]
```

## Deletion

### Deletion Protection

DatabaseGrant uses `spec.deletionProtection`:

```yaml
spec:
  deletionProtection: true
```

### Deletion Policy

DatabaseGrant has a **hardcoded Delete policy** — grants are always revoked when the CR is deleted. This is not configurable because leaving orphaned grants would be a security risk.

### Deletion Flow

1. **Deletion protection check**: Blocked if `spec.deletionProtection: true`, unless `force-delete` annotation is set.
2. **No dependency check**: DatabaseGrant is a leaf resource with no children, so there is no dependency blocking and no cascade confirmation.
3. **Revoke grants**: The operator revokes all grants managed by this resource. This always happens (hardcoded Delete policy).
4. **Force-delete and external failures**: If the REVOKE fails and force-delete is set, the operator continues with finalizer removal (grants may remain in the database).

## Cluster-Scoped Grants

`ClusterDatabaseGrant` is a cluster-scoped variant of `DatabaseGrant` for cross-namespace database access control. It uses `clusterInstanceRef` to reference a `ClusterDatabaseInstance`.

### Key Differences from DatabaseGrant

- **Cross-namespace references**: `userRef` requires an explicit `namespace` field (via `NamespacedUserReference`) to reference a `DatabaseUser` in any namespace.
- **Flexible role references**: `roleRef` can reference either a `ClusterDatabaseRole` (when `namespace` is empty) or a namespaced `DatabaseRole` (when `namespace` is provided).
- **Cluster-scoped instance**: Only `clusterInstanceRef` is supported — cluster-scoped grants require cluster-scoped instances.

### Example

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: ClusterDatabaseGrant
metadata:
  name: team-a-db-access  # No namespace - cluster-scoped
spec:
  clusterInstanceRef:
    name: shared-postgres
  userRef:
    name: team-a-user
    namespace: team-a  # Cross-namespace reference to DatabaseUser
  postgres:
    grants:
      - database: shared_db
        schema: public
        tables: ["*"]
        privileges: [SELECT, INSERT, UPDATE]
```

### Referencing a ClusterDatabaseRole

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: ClusterDatabaseGrant
metadata:
  name: platform-role-grant
spec:
  clusterInstanceRef:
    name: shared-postgres
  roleRef:
    name: platform-readonly  # ClusterDatabaseRole (no namespace = cluster-scoped)
  postgres:
    grants:
      - database: shared_db
        schema: public
        tables: ["*"]
        privileges: [SELECT]
```

## Grant vs Role

| Approach | Pros | Cons |
|----------|------|------|
| **Roles** | Reusable, hierarchical, easier to maintain | Additional indirection |
| **Direct Grants** | Simple, explicit | Harder to maintain at scale |

**Recommendation:** Use roles for common permission sets, direct grants for specific exceptions.

## Troubleshooting

### Grants not applied

- Verify the user/role exists and is Ready
- Check the database, schema, and tables exist
- Ensure proper privileges are specified
- Review operator logs

### Default privileges not working

- Verify `grantedBy` role exists and creates objects
- Check the schema exists
- Ensure the user has USAGE on the schema
