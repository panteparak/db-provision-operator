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
