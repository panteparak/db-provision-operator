# DatabaseRole

A `DatabaseRole` creates group roles for permission management. Roles are primarily used in PostgreSQL for grouping permissions that can be inherited by users.

## Overview

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseRole
metadata:
  name: readonly-role
spec:
  instanceRef:
    name: postgres-primary
  roleName: readonly
  postgres:
    login: false
```

## Spec Fields

### instanceRef (required)

Reference to the parent `DatabaseInstance`.

### roleName (required)

The role name in the database. Immutable after creation.

### postgres (optional)

PostgreSQL-specific role configuration.

| Field | Type | Description |
|-------|------|-------------|
| `login` | bool | Can login (default: false for roles) |
| `inherit` | bool | Inherit privileges (default: true) |
| `createDB` | bool | Can create databases |
| `createRole` | bool | Can create roles |
| `superuser` | bool | Is superuser |
| `replication` | bool | Can replicate |
| `bypassRLS` | bool | Bypass row-level security |
| `connectionLimit` | int | Max connections (-1 = unlimited) |
| `inRoles` | array | Roles to inherit from |
| `grants` | array | Permissions to grant |

#### grants

| Field | Type | Description |
|-------|------|-------------|
| `database` | string | Database name |
| `schema` | string | Schema name |
| `tables` | array | Table names or `["*"]` for all |
| `sequences` | array | Sequence names or `["*"]` for all |
| `functions` | array | Function names |
| `privileges` | array | Privileges to grant |

**Available privileges:**

- Tables: `SELECT`, `INSERT`, `UPDATE`, `DELETE`, `TRUNCATE`, `REFERENCES`, `TRIGGER`
- Sequences: `USAGE`, `SELECT`, `UPDATE`
- Functions: `EXECUTE`

### mysql (optional)

MySQL role configuration (MySQL 8.0+ only).

| Field | Type | Description |
|-------|------|-------------|
| `grants` | array | Permissions to grant |

## Status

| Field | Description |
|-------|-------------|
| `phase` | Current phase |
| `conditions` | Detailed conditions |

## Examples

### Read-Only Role

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseRole
metadata:
  name: readonly-role
spec:
  instanceRef:
    name: postgres-primary
  roleName: readonly
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
  name: readwrite-role
spec:
  instanceRef:
    name: postgres-primary
  roleName: readwrite
  postgres:
    login: false
    inRoles:
      - readonly  # Inherit read permissions
    grants:
      - database: myapp
        schema: public
        tables: ["*"]
        privileges: [INSERT, UPDATE, DELETE]
```

### Admin Role

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseRole
metadata:
  name: admin-role
spec:
  instanceRef:
    name: postgres-primary
  roleName: app_admin
  postgres:
    login: false
    createDB: false
    createRole: true
    grants:
      - database: myapp
        schema: public
        tables: ["*"]
        privileges: [SELECT, INSERT, UPDATE, DELETE, TRUNCATE]
      - database: myapp
        schema: public
        sequences: ["*"]
        privileges: [USAGE, SELECT, UPDATE]
```

### Role Hierarchy

Create a hierarchy of roles for complex permission models:

```yaml
# Level 1: Basic read access
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseRole
metadata:
  name: l1-reader
spec:
  instanceRef:
    name: postgres-primary
  roleName: l1_reader
  postgres:
    login: false
    grants:
      - database: myapp
        schema: public
        tables: ["*"]
        privileges: [SELECT]
---
# Level 2: Read + Write
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseRole
metadata:
  name: l2-writer
spec:
  instanceRef:
    name: postgres-primary
  roleName: l2_writer
  postgres:
    login: false
    inRoles: [l1_reader]
    grants:
      - database: myapp
        schema: public
        tables: ["*"]
        privileges: [INSERT, UPDATE]
---
# Level 3: Full access
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseRole
metadata:
  name: l3-admin
spec:
  instanceRef:
    name: postgres-primary
  roleName: l3_admin
  postgres:
    login: false
    inRoles: [l2_writer]
    grants:
      - database: myapp
        schema: public
        tables: ["*"]
        privileges: [DELETE, TRUNCATE]
```

## Deletion

### Deletion Protection

DatabaseRole uses **annotations** (not spec fields) for deletion protection:

```yaml
metadata:
  annotations:
    dbops.dbprovision.io/deletion-protection: "true"
```

To remove protection:

```bash
kubectl annotate databaserole readonly-role dbops.dbprovision.io/deletion-protection-
```

### Deletion Policy

DatabaseRole uses an **annotation** for deletion policy:

```yaml
metadata:
  annotations:
    dbops.dbprovision.io/deletion-policy: "Delete"  # or Retain (default)
```

- **Retain** (default): CR is deleted, but the database role is kept
- **Delete**: Database role is dropped, then CR is deleted

### Deletion Flow

1. **Deletion protection check**: Blocked if annotation `deletion-protection: "true"` is present, unless `force-delete` annotation is set.
2. **Child dependency check**: If DatabaseGrant children reference this role, deletion is blocked (Phase=Failed, condition=DependenciesExist) unless force-delete is set.
3. **Cascade confirmation**: When force-delete is set and grants exist, the operator enters `PhasePendingDeletion` and requires the `confirm-force-delete` annotation with the hash from `status.deletionConfirmation.hash`. See [Force Delete with Children](deletion-protection.md#force-delete-with-children-cascade-confirmation).
4. **Deletion policy**: The annotation `dbops.dbprovision.io/deletion-policy` controls whether the external role is dropped.
5. **Force-delete and external failures**: If the role drop fails and force-delete is set, the operator continues with finalizer removal.

## Cluster-Scoped Roles

`ClusterDatabaseRole` is a cluster-scoped variant of `DatabaseRole`. It uses `clusterInstanceRef` to reference a `ClusterDatabaseInstance` and is useful for shared service accounts and cross-namespace access patterns.

### Example

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: ClusterDatabaseRole
metadata:
  name: platform-readonly  # No namespace - cluster-scoped
spec:
  clusterInstanceRef:
    name: shared-postgres
  roleName: platform_readonly
  postgres:
    login: false
    grants:
      - database: shared_db
        schema: public
        tables: ["*"]
        privileges: [SELECT]
```

`ClusterDatabaseRole` has its own spec type (`ClusterDatabaseRoleSpec`) but supports the same engine-specific configurations (`postgres`, `mysql`) as `DatabaseRole`. It also supports `driftPolicy` and `managedResourceComment`.

## Best Practices

1. **Use roles for grouping** - Create roles for permission sets, then assign roles to users
2. **Keep roles non-login** - Roles should not have login capability; users should
3. **Use inheritance** - Build role hierarchies with `inRoles` for maintainability
4. **Principle of least privilege** - Grant only necessary permissions

## Troubleshooting

### Role not created

- Verify the DatabaseInstance is Ready
- Check the admin user has CREATE ROLE permission
- Review operator logs for errors

### Grants not applied

- Ensure the database and schema exist
- Verify the tables/sequences exist
- Check for syntax errors in privilege names
