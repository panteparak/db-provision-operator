---
hide:
  - navigation
---

# DB Provision Operator

**Kubernetes operator for declarative database provisioning across PostgreSQL, MySQL, and MariaDB.**

---

## Overview

DB Provision Operator is a Kubernetes operator that provides a unified, declarative interface for managing database resources. Instead of manually creating databases, users, and permissions, you define them as Kubernetes Custom Resources and the operator handles the rest.

## Features

- **Unified API** - Single CRD-based interface for PostgreSQL, MySQL, and MariaDB
- **Declarative Management** - GitOps-friendly database provisioning
- **Automatic Credentials** - Secure password generation and Kubernetes Secret management
- **Fine-grained Access Control** - Users, roles, and grants management
- **Backup Automation** - Scheduled backups with configurable retention policies
- **Health Monitoring** - Configurable health checks with status reporting

## Quick Start

=== "Helm (Recommended)"

    ```bash
    # Add the Helm repository
    helm repo add db-provision https://panteparak.github.io/db-provision-operator/charts
    helm repo update

    # Install the operator
    helm install db-provision-operator db-provision/db-provision-operator \
      --namespace db-provision-operator-system \
      --create-namespace
    ```

=== "Kustomize"

    ```bash
    kubectl apply -f https://github.com/panteparak/db-provision-operator/releases/latest/download/install.yaml
    ```

## Custom Resource Definitions

| CRD | Description | Short Name |
|-----|-------------|------------|
| `DatabaseInstance` | Database server connection configuration | `dbi` |
| `Database` | Logical database within an instance | `db` |
| `DatabaseUser` | Database user with credentials | `dbu` |
| `DatabaseRole` | Group role for permission management | `dbr` |
| `DatabaseGrant` | Fine-grained permission grants | `dbg` |
| `DatabaseBackup` | One-time backup operation | `dbbak` |
| `DatabaseBackupSchedule` | Scheduled backup configuration | `dbbaksch` |
| `DatabaseRestore` | Restore from backup | `dbrest` |

## Example: Create a PostgreSQL Database

```yaml
# 1. Define credentials secret
apiVersion: v1
kind: Secret
metadata:
  name: postgres-admin-credentials
type: Opaque
stringData:
  username: postgres
  password: your-secure-password
---
# 2. Create DatabaseInstance
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: postgres-primary
spec:
  engine: postgres
  connection:
    host: postgres.database.svc.cluster.local
    port: 5432
    secretRef:
      name: postgres-admin-credentials
---
# 3. Create Database
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: myapp-database
spec:
  instanceRef:
    name: postgres-primary
  name: myapp
---
# 4. Create User with auto-generated password
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
    secretName: myapp-user-credentials
```

## Next Steps

<div class="grid cards" markdown>

-   :material-download:{ .lg .middle } **[Installation](getting-started/installation.md)**

    ---

    Detailed installation instructions for Helm, Kustomize, and from source.

-   :material-rocket-launch:{ .lg .middle } **[Quick Start](getting-started/quickstart.md)**

    ---

    Step-by-step tutorial to get your first database running.

-   :material-book-open-variant:{ .lg .middle } **[User Guide](user-guide/index.md)**

    ---

    Comprehensive guide for all CRDs and features.

-   :material-database:{ .lg .middle } **[Engine Guides](engines/index.md)**

    ---

    Engine-specific features for PostgreSQL, MySQL, and MariaDB.

</div>

## Support

- **GitHub Issues**: [Report bugs or request features](https://github.com/panteparak/db-provision-operator/issues)
- **Documentation**: You're here!

## License

DB Provision Operator is licensed under the Apache License 2.0.
