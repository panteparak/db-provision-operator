# Installation

This guide covers all installation methods for DB Provision Operator.

## Prerequisites

- Kubernetes 1.26+
- kubectl configured
- Helm 3.x (for Helm installation)
- cluster-admin privileges (for CRD installation)

## Helm Installation

Helm is the recommended installation method for production deployments.

### Add the Helm Repository

```bash
helm repo add db-provision https://panteparak.github.io/db-provision-operator/charts
helm repo update
```

### Install with Default Values

```bash
helm install db-provision-operator db-provision/db-provision-operator \
  --namespace db-provision-operator-system \
  --create-namespace
```

### Install with Custom Values

Create a `values.yaml` file:

```yaml
# values.yaml
replicaCount: 2

resources:
  limits:
    cpu: 500m
    memory: 256Mi
  requests:
    cpu: 100m
    memory: 128Mi

metrics:
  enabled: true

serviceMonitor:
  enabled: true
```

Then install:

```bash
helm install db-provision-operator db-provision/db-provision-operator \
  --namespace db-provision-operator-system \
  --create-namespace \
  -f values.yaml
```

### Upgrade an Existing Installation

```bash
helm repo update
helm upgrade db-provision-operator db-provision/db-provision-operator \
  --namespace db-provision-operator-system
```

### Uninstall

```bash
helm uninstall db-provision-operator --namespace db-provision-operator-system
```

!!! warning "CRDs are not removed"
    Helm does not remove CRDs on uninstall to prevent data loss. To remove CRDs manually:
    ```bash
    kubectl delete crds -l app.kubernetes.io/name=db-provision-operator
    ```

## Kustomize Installation

Kustomize is ideal for GitOps workflows.

### Install Latest Release

```bash
kubectl apply -f https://github.com/panteparak/db-provision-operator/releases/latest/download/install.yaml
```

### Install Specific Version

```bash
VERSION=v0.1.0
kubectl apply -f https://github.com/panteparak/db-provision-operator/releases/download/${VERSION}/db-provision-operator-${VERSION}.yaml
```

### Customize with Kustomize Overlay

Create a `kustomization.yaml`:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - https://github.com/panteparak/db-provision-operator/config/default?ref=main

namespace: db-provision-operator-system

patches:
  - patch: |-
      - op: replace
        path: /spec/template/spec/containers/0/resources/limits/memory
        value: 256Mi
    target:
      kind: Deployment
      name: db-provision-operator-controller-manager
```

Apply:

```bash
kubectl apply -k .
```

## Verify Installation

### Check the Deployment

```bash
kubectl get pods -n db-provision-operator-system
```

Expected output:

```
NAME                                                     READY   STATUS    RESTARTS   AGE
db-provision-operator-controller-manager-xxxxx-xxxxx     1/1     Running   0          1m
```

### Check CRDs

```bash
kubectl get crds | grep dbops
```

Expected output:

```
databasebackups.dbops.dbprovision.io          2024-01-01T00:00:00Z
databasebackupschedules.dbops.dbprovision.io  2024-01-01T00:00:00Z
databasegrants.dbops.dbprovision.io           2024-01-01T00:00:00Z
databaseinstances.dbops.dbprovision.io        2024-01-01T00:00:00Z
databaserestores.dbops.dbprovision.io         2024-01-01T00:00:00Z
databaseroles.dbops.dbprovision.io            2024-01-01T00:00:00Z
databases.dbops.dbprovision.io                2024-01-01T00:00:00Z
databaseusers.dbops.dbprovision.io            2024-01-01T00:00:00Z
```

### Check Logs

```bash
kubectl logs -n db-provision-operator-system deployment/db-provision-operator-controller-manager
```

## Configuration Options

### Helm Values Reference

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of operator replicas | `1` |
| `image.repository` | Container image repository | `ghcr.io/panteparak/db-provision-operator` |
| `image.tag` | Container image tag | Chart appVersion |
| `resources.limits.cpu` | CPU limit | `500m` |
| `resources.limits.memory` | Memory limit | `128Mi` |
| `resources.requests.cpu` | CPU request | `10m` |
| `resources.requests.memory` | Memory request | `64Mi` |
| `metrics.enabled` | Enable metrics endpoint | `false` |
| `serviceMonitor.enabled` | Create ServiceMonitor | `false` |
| `grafanaDashboards.enabled` | Install Grafana dashboards | `false` |
| `leaderElection.enabled` | Enable leader election | `true` |

For the complete list of values, see the [Helm chart values.yaml](https://github.com/panteparak/db-provision-operator/blob/main/charts/db-provision-operator/values.yaml).

## Next Steps

- [Quick Start Tutorial](quickstart.md) - Create your first database
- [User Guide](../user-guide/index.md) - Learn about all CRDs
- [Examples](../examples/index.md) - Ready-to-use configurations
