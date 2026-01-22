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
