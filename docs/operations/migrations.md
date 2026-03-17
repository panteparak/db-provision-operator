# Migrations

The `dbctl migrate` command group provides data migrations for operator version upgrades. These migrations are **idempotent** and safe to re-run.

## reverse-privileges

**Introduced in:** v0.13.0

Repairs the full ownership model for databases created before v0.13.0. This command performs comprehensive integrity checks and applies missing default privilege grants.

### Background

Before v0.13.0, `SetDefaultPrivileges` only applied **forward grants** (ownerRole → appUser). Starting in v0.13.0, the operator applies **bidirectional** grants so that objects created by the app user are accessible to the owner role and all rotated users that inherit from it.

The operator only runs ownership setup during **initial database creation** — it does not re-verify role existence, membership, or default privileges on subsequent reconciles. This means databases in a partially broken state (e.g., after manual role deletion or a failed initial setup) cannot self-heal through normal reconciliation.

This migration command fills that gap by acting as a **complete ownership repair tool**.

### What It Checks

The command performs 5 integrity check groups in order:

| # | Check | Fix | Destructive |
|---|-------|-----|-------------|
| 1 | Owner role exists | Creates role with `NOLOGIN INHERIT` | No |
| 2 | App user exists | Creates role with `LOGIN INHERIT` | No |
| 3 | Role membership (app user inherits from owner role) | Grants role membership | No |
| 4 | Database owner matches owner role | Transfers database ownership | Yes |
| 5 | Forward default privileges | Re-applies all forward grants (idempotent) | No |

After integrity checks, the command applies both **forward** and **reverse** default privilege grants across all configured schemas for tables, sequences, and functions.

### Usage

```bash
# Dry-run first — see what would be changed
dbctl migrate reverse-privileges my-database-cr -n my-namespace --dry-run

# Apply fixes
dbctl migrate reverse-privileges my-database-cr -n my-namespace

# With verbose output
dbctl migrate reverse-privileges my-database-cr -n my-namespace -v
```

### Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--namespace` | `-n` | Current kubeconfig context | Kubernetes namespace of the Database CR |
| `--kubeconfig` | | `$KUBECONFIG` or `~/.kube/config` | Path to kubeconfig file |
| `--dry-run` | | `false` | Print what would be done without executing |
| `--verbose` | `-v` | `false` | Enable verbose output |

### Example Output

#### Healthy database (no drift)

```
Integrity checks:
  [CHECK] Owner role "db_myapp_owner" .............. OK
  [CHECK] App user "db_myapp_app" .............. OK
  [CHECK] Role membership app→owner ................ OK
  [CHECK] Database "myapp" owner ................... OK

  Forward default privileges: applied 9 grants across 3 schema(s)

Ownership drift: no issues found

Forward default privileges:
  Applied 9 forward grants across 3 schema(s)

Reverse default privileges:
  Applied 9 reverse grants across 3 schema(s)
```

#### Database with drift (auto-fixed)

```
Integrity checks:
  [CHECK] Owner role "db_myapp_owner" .............. OK
  [CHECK] App user "db_myapp_app" .................. MISSING → created (LOGIN INHERIT)
  [CHECK] Role membership app→owner ................ MISSING → granted
  [CHECK] Database "myapp" owner ................... DRIFT (current: "postgres", expected: "db_myapp_owner") → transferred

  Forward default privileges: applied 9 grants across 3 schema(s)

Ownership drift: 3 issue(s) fixed

Forward default privileges:
  Applied 9 forward grants across 3 schema(s)

Reverse default privileges:
  Applied 9 reverse grants across 3 schema(s)
```

#### Dry-run mode

```
Integrity checks:
  [CHECK] Owner role "db_myapp_owner" .............. MISSING (would create with NOLOGIN INHERIT)
  [CHECK] App user "db_myapp_app" .................. OK
  [CHECK] Role membership app→owner ................ OK
  [CHECK] Database "myapp" owner ................... DRIFT (current: "postgres", expected: "db_myapp_owner") — would transfer

  Forward default privileges: would apply 9 grants across 3 schema(s)

Ownership drift: 2 issue(s) detected (dry-run, not corrected)

Forward default privileges (dry-run):
  ALTER DEFAULT PRIVILEGES FOR ROLE db_myapp_owner IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO db_myapp_app;
  ...

Reverse default privileges (dry-run):
  ALTER DEFAULT PRIVILEGES FOR ROLE db_myapp_app IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO db_myapp_owner;
  ...
```

### K8s Events

The command emits events on the Database CR for audit visibility:

| Event | Type | When |
|-------|------|------|
| `OwnershipDriftDetected` | Warning | Drift found in dry-run mode |
| `OwnershipDriftCorrected` | Normal | Drift found and fixed |

!!! info "Ownership events vs operator drift events"
    These events use the `Ownership` prefix to distinguish them from the operator's regular `DriftDetected` / `DriftCorrected` events. The event source is `dbctl-migrate` rather than the operator controller.

View migration events:

```bash
kubectl get events --field-selector reason=OwnershipDriftCorrected
kubectl get events --field-selector reason=OwnershipDriftDetected

# Or filter by source
kubectl get events --field-selector source=dbctl-migrate
```

### Status Updates

The command updates `status.drift` on the Database CR:

**When drift is found (dry-run):**

```yaml
status:
  drift:
    detected: true
    lastChecked: "2026-03-18T10:30:00Z"
    diffs:
      - field: "ownership.role"
        expected: "db_myapp_owner"
        actual: "<missing>"
      - field: "ownership.dbOwner"
        expected: "db_myapp_owner"
        actual: "postgres"
        destructive: true
```

**After successful fix (non-dry-run):**

```yaml
status:
  drift:
    detected: false
    lastChecked: "2026-03-18T10:30:00Z"
```

!!! note "Status overwrite on next reconcile"
    The operator's next reconcile will overwrite `status.drift` with its own drift detection results. The migration's status write is immediately useful for visibility (e.g., dashboards polling CR status) but is not persistent.

### Supported Engines

| Engine | Supported |
|--------|-----------|
| PostgreSQL | Yes |
| CockroachDB | Yes |
| MySQL | No (no `ALTER DEFAULT PRIVILEGES` support) |
| ClickHouse | No (no `ALTER DEFAULT PRIVILEGES` support) |

### Prerequisites

- `kubectl` access to the cluster with permissions to read Database CRs, DatabaseInstance/ClusterDatabaseInstance resources, and Secrets
- Network access from the machine running `dbctl` to the database server
- The Database CR must use auto-ownership (`spec.postgres.ownership.autoOwnership: true`)

### When to Run

Run this migration when:

- **Upgrading from pre-v0.13.0** — to add missing reverse default privileges
- **After manual role deletion** — to recreate missing roles and re-establish membership
- **Investigating ownership issues** — use `--dry-run` to diagnose without making changes
- **After failed initial database creation** — to complete the ownership setup that was interrupted

!!! tip "Safe to re-run"
    All operations are idempotent. `ALTER DEFAULT PRIVILEGES` grants that already exist are silently skipped by PostgreSQL. Role creation and membership grants check for existence before acting.

### Troubleshooting

#### "No ownership model configured — nothing to migrate."

The Database CR does not have `spec.postgres.ownership.autoOwnership: true`. This command only works with auto-ownership databases.

#### "Engine does not support ALTER DEFAULT PRIVILEGES"

The referenced DatabaseInstance uses MySQL or ClickHouse. Ownership management is only supported on PostgreSQL and CockroachDB.

#### "Default privileges are explicitly disabled"

The Database CR has `spec.postgres.ownership.setDefaultPrivileges: false`. Default privileges are intentionally disabled for this database.

#### Connection errors

Ensure the machine running `dbctl` has network access to the database server. The command connects using the credentials from the DatabaseInstance's `secretRef`.

```bash
# Verify the instance connection details
kubectl get databaseinstance my-instance -o yaml

# Check the credentials secret exists
kubectl get secret my-instance-credentials
```

#### RBAC errors on event emission

If the command prints a warning about event recording, ensure the kubeconfig user has permissions to create events. The command will still apply fixes even if event emission fails.
