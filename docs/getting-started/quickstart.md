# Quick Start

This tutorial walks you through creating a PostgreSQL database with a user in 5 minutes.

## Prerequisites

- DB Provision Operator [installed](installation.md) in your cluster
- A PostgreSQL server accessible from your cluster

!!! tip "No PostgreSQL server?"
    You can deploy a test PostgreSQL server:
    ```bash
    kubectl apply -f https://raw.githubusercontent.com/panteparak/db-provision-operator/main/test/e2e/fixtures/testdata/postgresql.yaml
    ```

## Step 1: Create Admin Credentials

First, create a Secret with the credentials to connect to your PostgreSQL server:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: postgres-admin-credentials
  namespace: default
type: Opaque
stringData:
  username: postgres
  password: your-admin-password
```

Apply:

```bash
kubectl apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: postgres-admin-credentials
  namespace: default
type: Opaque
stringData:
  username: postgres
  password: postgres
EOF
```

## Step 2: Create a DatabaseInstance

A `DatabaseInstance` represents the connection to your database server:

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseInstance
metadata:
  name: postgres-primary
  namespace: default
spec:
  engine: postgres
  connection:
    host: postgresql.default.svc.cluster.local  # Adjust to your server
    port: 5432
    database: postgres
    secretRef:
      name: postgres-admin-credentials
  healthCheck:
    enabled: true
    intervalSeconds: 30
```

Apply and verify:

```bash
kubectl apply -f instance.yaml

# Check status
kubectl get databaseinstance postgres-primary
```

Wait for the instance to become `Ready`:

```
NAME               ENGINE     PHASE   HOST                                      PORT   AGE
postgres-primary   postgres   Ready   postgresql.default.svc.cluster.local      5432   30s
```

## Step 3: Create a Database

Now create a database within the instance:

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: myapp-database
  namespace: default
spec:
  instanceRef:
    name: postgres-primary
  name: myapp
  deletionPolicy: Retain
```

Apply:

```bash
kubectl apply -f database.yaml

# Verify
kubectl get database myapp-database
```

## Step 4: Create a User

Create a user with an auto-generated password:

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: myapp-user
  namespace: default
spec:
  instanceRef:
    name: postgres-primary
  username: myapp_user
  passwordSecret:
    generate: true
    length: 24
    secretName: myapp-user-credentials
```

Apply:

```bash
kubectl apply -f user.yaml

# Check the generated credentials
kubectl get secret myapp-user-credentials -o jsonpath='{.data.password}' | base64 -d
```

## Step 5: Grant Permissions

Grant the user access to the database:

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseGrant
metadata:
  name: myapp-user-grants
  namespace: default
spec:
  userRef:
    name: myapp-user
  databaseRef:
    name: myapp-database
  postgres:
    grants:
      - database: myapp
        schema: public
        tables: ["*"]
        privileges: [SELECT, INSERT, UPDATE, DELETE]
```

Apply:

```bash
kubectl apply -f grant.yaml
```

## Verify Everything

Check all resources:

```bash
kubectl get databaseinstance,database,databaseuser,databasegrant
```

Expected output:

```
NAME                                               ENGINE     PHASE   AGE
databaseinstance.dbops.dbprovision.io/postgres-primary   postgres   Ready   5m

NAME                                        ENGINE     PHASE   DATABASE   AGE
database.dbops.dbprovision.io/myapp-database   postgres   Ready   myapp      4m

NAME                                         ENGINE     PHASE   USERNAME     AGE
databaseuser.dbops.dbprovision.io/myapp-user   postgres   Ready   myapp_user   3m

NAME                                               PHASE   AGE
databasegrant.dbops.dbprovision.io/myapp-user-grants   Ready   2m
```

## Connect to the Database

Use the generated credentials to connect:

```bash
# Get credentials
export PGHOST=postgresql.default.svc.cluster.local
export PGPORT=5432
export PGDATABASE=myapp
export PGUSER=myapp_user
export PGPASSWORD=$(kubectl get secret myapp-user-credentials -o jsonpath='{.data.password}' | base64 -d)

# Connect (from a pod with psql)
kubectl run -it --rm psql --image=postgres:16 --restart=Never -- psql
```

## Clean Up

To remove all resources:

```bash
kubectl delete databasegrant myapp-user-grants
kubectl delete databaseuser myapp-user
kubectl delete database myapp-database
kubectl delete databaseinstance postgres-primary
kubectl delete secret postgres-admin-credentials myapp-user-credentials
```

## Next Steps

- [User Guide](../user-guide/index.md) - Deep dive into each CRD
- [Examples](../examples/index.md) - More complex scenarios
- [Engine Guides](../engines/index.md) - PostgreSQL, MySQL, MariaDB specifics
