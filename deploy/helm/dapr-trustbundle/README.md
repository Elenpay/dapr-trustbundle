# Dapr Trust Bundle Operator Helm Chart

This Helm chart deploys the Dapr Trust Bundle Operator to a Kubernetes cluster.

## Description

The Dapr Trust Bundle Operator automatically synchronizes certificate data from a source secret to a target secret and ConfigMap, specifically designed for Dapr trust bundle management.

## Prerequisites

- Kubernetes 1.20+
- Helm 3.8.0+
- RBAC enabled in your cluster

## Installing the Chart

### Install from Local Chart

```bash
# Clone the repository
git clone https://github.com/elenpay/dapr-trustbundle.git
cd dapr-cert-manager/dapr-trustbundle/dist/helm

# Install the chart
helm install dapr-trustbundle ./dapr-trustbundle
```

### Install with Custom Values

```bash
helm install dapr-trustbundle ./dapr-trustbundle \
  --set image.tag=v1.0.0 \
  --set resources.requests.memory=128Mi \
  --set metrics.serviceMonitor.enabled=true
```

### Install in Custom Namespace

```bash
helm install dapr-trustbundle ./dapr-trustbundle \
  --namespace my-namespace \
  --create-namespace \
  --set namespace.name=my-namespace
```

## Uninstalling the Chart

```bash
helm uninstall dapr-trustbundle
```

## Configuration

The following table lists the configurable parameters and their default values.

### Image Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `image.repository` | Container image repository | `ghcr.io/elenpay/dapr-trustbundle/dapr-trustbundle` |
| `image.tag` | Container image tag | `""` (uses chart appVersion) |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `imagePullSecrets` | Image pull secrets | `[]` |

### Deployment Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of replicas | `1` |
| `nameOverride` | Override chart name | `""` |
| `fullnameOverride` | Override full chart name | `""` |

### Namespace Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `namespace.name` | Target namespace for deployment | `dapr-system` |
| `namespace.create` | Create namespace if it doesn't exist | `true` |

### Service Account Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `serviceAccount.create` | Create service account | `true` |
| `serviceAccount.annotations` | Service account annotations | `{}` |
| `serviceAccount.name` | Service account name | `""` |

### Security Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `podSecurityContext` | Pod security context | `{runAsNonRoot: true}` |
| `securityContext` | Container security context | See values.yaml |

### Resource Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `resources.limits.cpu` | CPU limit | `500m` |
| `resources.limits.memory` | Memory limit | `128Mi` |
| `resources.requests.cpu` | CPU request | `10m` |
| `resources.requests.memory` | Memory request | `64Mi` |

### Auto Scaling Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `autoscaling.enabled` | Enable HPA | `false` |
| `autoscaling.minReplicas` | Minimum replicas | `1` |
| `autoscaling.maxReplicas` | Maximum replicas | `100` |
| `autoscaling.targetCPUUtilizationPercentage` | Target CPU utilization | `80` |

### Metrics Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `metrics.enabled` | Enable metrics | `true` |
| `metrics.port` | Metrics port | `8443` |
| `metrics.serviceMonitor.enabled` | Create ServiceMonitor | `false` |
| `metrics.serviceMonitor.additionalLabels` | Additional labels for ServiceMonitor | `{}` |
| `metrics.serviceMonitor.namespace` | ServiceMonitor namespace | `""` |
| `metrics.serviceMonitor.interval` | Scrape interval | `30s` |
| `metrics.serviceMonitor.scrapeTimeout` | Scrape timeout | `10s` |

### RBAC Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `rbac.create` | Create RBAC resources | `true` |
| `rbac.useClusterRole` | Use cluster-wide permissions (ClusterRole) instead of namespace-scoped (Role) | `true` |
| `rbac.additionalRules` | Additional RBAC rules | `[]` |

**RBAC Scope Options:**

- **Cluster-wide RBAC** (`useClusterRole: true`): Grants cluster-wide permissions. Use when monitoring multiple namespaces or when the operator needs broad access.
- **Namespace-scoped RBAC** (`useClusterRole: false`): Grants permissions only in the target namespace. More secure option when the operator only monitors a single namespace.

Example for namespace-scoped RBAC:
```yaml
rbac:
  useClusterRole: false
controller:
  targetNamespace: "my-namespace"
```

### Leader Election Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `leaderElection.enabled` | Enable leader election | `true` |
| `leaderElection.id` | Leader election ID | `5f7b14a1.elenpay.tech` |

### Advanced Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `controller.enabled` | Enable controller | `true` |
| `controller.sourceSecretName` | Name of the source secret to monitor | `dapr-trust-bundle-from-cert-manager` |
| `controller.targetNamespace` | Target namespace to monitor and create resources in | `dapr-system` |
| `controller.args` | Controller arguments | See values.yaml |
| `logLevel` | Log level | `info` |
| `env` | Environment variables | `[]` |
| `extraVolumes` | Additional volumes | `[]` |
| `extraVolumeMounts` | Additional volume mounts | `[]` |
| `nodeSelector` | Node selector | `{}` |
| `tolerations` | Tolerations | `[]` |
| `affinity` | Affinity rules | `{}` |
| `podAnnotations` | Pod annotations | `{}` |

### High Availability Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `podDisruptionBudget.enabled` | Enable PDB | `false` |
| `podDisruptionBudget.minAvailable` | Minimum available pods | `1` |

### Network Policy Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `networkPolicy.enabled` | Enable NetworkPolicy | `false` |
| `networkPolicy.ingress` | Ingress rules | `[]` |
| `networkPolicy.egress` | Egress rules | `[]` |

## Examples

### Basic Installation

```bash
helm install dapr-trustbundle ./dapr-trustbundle
```

### Production Installation with Monitoring

```yaml
# values-production.yaml
replicaCount: 2

resources:
  limits:
    cpu: 1000m
    memory: 256Mi
  requests:
    cpu: 100m
    memory: 128Mi

autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 5
  targetCPUUtilizationPercentage: 70

metrics:
  enabled: true
  serviceMonitor:
    enabled: true
    additionalLabels:
      monitoring: "enabled"

podDisruptionBudget:
  enabled: true
  minAvailable: 1

affinity:
  podAntiAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
    - weight: 100
      podAffinityTerm:
        labelSelector:
          matchExpressions:
          - key: app.kubernetes.io/name
            operator: In
            values:
            - dapr-trustbundle
        topologyKey: kubernetes.io/hostname
```

```bash
helm install dapr-trustbundle ./dapr-trustbundle -f values-production.yaml
```

### Development Installation

```yaml
# values-dev.yaml
image:
  tag: "main"
  pullPolicy: Always

resources:
  limits:
    cpu: 200m
    memory: 64Mi
  requests:
    cpu: 10m
    memory: 32Mi

logLevel: debug
```

```bash
helm install dapr-trustbundle ./dapr-trustbundle -f values-dev.yaml
```

## Testing

After installation, test the operator:

1. **Create a test secret:**
   ```bash
   kubectl apply -f - <<EOF
   apiVersion: v1
   kind: Secret
   metadata:
     name: dapr-trust-bundle-from-cert-manager
     namespace: dapr-system
   type: kubernetes.io/tls
   data:
     ca.crt: $(echo "test-ca-cert" | base64 -w 0)
     tls.crt: $(echo "test-tls-cert" | base64 -w 0)
     tls.key: $(echo "test-tls-key" | base64 -w 0)
   EOF
   ```

2. **Verify target resources:**
   ```bash
   kubectl get secret dapr-trust-bundle -n dapr-system
   kubectl get configmap dapr-trust-bundle -n dapr-system
   ```

3. **Check operator logs:**
   ```bash
   kubectl logs -n dapr-system deployment/dapr-trustbundle-operator
   ```

## Upgrading

### Upgrade to a New Version

```bash
helm upgrade dapr-trustbundle ./dapr-trustbundle --version 1.1.0
```

### Upgrade with Value Changes

```bash
helm upgrade dapr-trustbundle ./dapr-trustbundle \
  --set image.tag=v1.1.0 \
  --set replicaCount=3
```

## Troubleshooting

### Check Deployment Status

```bash
kubectl get pods -n dapr-system -l app.kubernetes.io/name=dapr-trustbundle
```

### View Operator Logs

```bash
kubectl logs -n dapr-system -l control-plane=operator --tail=100
```

### Validate RBAC Permissions

```bash
kubectl auth can-i create secrets --as=system:serviceaccount:dapr-system:dapr-trustbundle-operator
```

### Debug Helm Release

```bash
helm status dapr-trustbundle
helm get values dapr-trustbundle
helm get manifest dapr-trustbundle
```

## Contributing

For information on contributing to this Helm chart, see the main project repository:
https://github.com/elenpay/dapr-trustbundle

## License

This Helm chart is licensed under the Apache License 2.0.
