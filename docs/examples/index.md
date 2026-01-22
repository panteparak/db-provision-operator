# Examples

Ready-to-use examples for common scenarios.

## Quick Links

- [PostgreSQL Examples](postgresql.md) - Complete PostgreSQL setup
- [MySQL Examples](mysql.md) - Complete MySQL setup
- [Advanced Examples](advanced.md) - Cross-namespace, TLS, cloud backups

## Sample Manifests

All sample manifests are available in the repository:

```bash
# Clone the repository
git clone https://github.com/panteparak/db-provision-operator.git

# Navigate to examples
cd db-provision-operator/DOCS/examples/
```

### Directory Structure

```
DOCS/examples/
├── postgresql/
│   ├── 01-credentials.yaml
│   ├── 02-instance.yaml
│   ├── 03-database.yaml
│   ├── 04-user.yaml
│   ├── 05-role.yaml
│   ├── 06-grant.yaml
│   └── 07-backup.yaml
├── mysql/
│   ├── 01-credentials.yaml
│   ├── 02-instance.yaml
│   ├── 03-database.yaml
│   ├── 04-user.yaml
│   └── 05-grant.yaml
├── mariadb/
│   ├── 01-credentials.yaml
│   ├── 02-instance.yaml
│   └── 03-database.yaml
└── advanced/
    ├── cross-namespace.yaml
    ├── tls-connection.yaml
    └── backup-to-s3.yaml
```

## Common Patterns

### Minimal Setup

The simplest setup requires:

1. Admin credentials Secret
2. DatabaseInstance
3. Database
4. DatabaseUser

```yaml
# 1. Credentials
apiVersion: v1
kind: Secret
metadata:
  name: db-admin
type: Opaque
stringData:
  username: admin
  password: secretpassword
---
# 2. Instance
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: my-db
spec:
  engine: postgres
  connection:
    host: postgres.default.svc
    port: 5432
    secretRef:
      name: db-admin
---
# 3. Database
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: myapp
spec:
  instanceRef:
    name: my-db
---
# 4. User
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: myapp-user
spec:
  instanceRef:
    name: my-db
  username: myapp
  passwordSecret:
    generate: true
    secretName: myapp-credentials
```

### Production Setup

Production deployments should include:

1. TLS connections
2. Role-based permissions
3. Connection limits
4. Backup schedules
5. Deletion protection

See [Advanced Examples](advanced.md) for production patterns.

## Using Examples

### Apply All PostgreSQL Examples

```bash
# Apply in order
kubectl apply -f DOCS/examples/postgresql/01-credentials.yaml
kubectl apply -f DOCS/examples/postgresql/02-instance.yaml
kubectl apply -f DOCS/examples/postgresql/03-database.yaml
kubectl apply -f DOCS/examples/postgresql/04-user.yaml
kubectl apply -f DOCS/examples/postgresql/05-role.yaml
kubectl apply -f DOCS/examples/postgresql/06-grant.yaml
```

### Verify Resources

```bash
# Check all resources
kubectl get databaseinstances,databases,databaseusers,databaseroles,databasegrants

# Check specific resource
kubectl describe databaseinstance postgres-primary

# Get generated credentials
kubectl get secret myapp-user-credentials -o jsonpath='{.data.password}' | base64 -d
```

### Clean Up

```bash
# Delete in reverse order
kubectl delete -f DOCS/examples/postgresql/06-grant.yaml
kubectl delete -f DOCS/examples/postgresql/05-role.yaml
kubectl delete -f DOCS/examples/postgresql/04-user.yaml
kubectl delete -f DOCS/examples/postgresql/03-database.yaml
kubectl delete -f DOCS/examples/postgresql/02-instance.yaml
kubectl delete -f DOCS/examples/postgresql/01-credentials.yaml
```

## Next Steps

- [PostgreSQL Examples](postgresql.md) - Detailed PostgreSQL setup
- [MySQL Examples](mysql.md) - Detailed MySQL setup
- [Advanced Examples](advanced.md) - Production patterns
