# Dapr Trust Bundle Operator

A Kubernetes operator that helps Dapr to meet Cert-Manager so we can let cert-manager efficiently manage the lifecycle of Dapr's trust bundle certificates.

## The problem

Cert Manager is a great tool and pretty much the standard to certificates management in Kubernetes spaces. However Cert Manager comes with some strong opinion on resulting secrets format and key names which collision with Dapr having its own strong opinion on `dapr-trust-bundle` secret and keys to be used by Dapr Sentinel service.

## Description

This operator monitors the `dapr-trust-bundle-from-cert-manager` secret and automatically creates/updates a `dapr-trust-bundle` secret with the same data whenever the source secret changes. It's designed to work seamlessly with Dapr's certificate management workflow in the `dapr-system` namespace.
So you will simply generate that `dapr-trust-bundle-from-cert-manager` as a result of a chained cert-manager certificate issuance.

### How it works

1. The operator watches for changes to secrets named `dapr-trust-bundle-from-cert-manager`
2. When this secret is created or updated, the operator:
   - Creates or updates a secret named `dapr-trust-bundle` in the same namespace
   - Copies all data from the source secret to the target secret with key renaming:
     - `ca.crt` → `ca.crt` (unchanged)
     - `tls.key` → `issuer.key`
     - `tls.crt` → `issuer.crt`
     - All other keys are copied as-is
   - Sets the target secret type to `Opaque`
   - Adds management labels to track the created secret
3. When the source secret is deleted, the target secret is also cleaned up

``` mermaid
graph TD
    subgraph "Cert Manager"
        A[certificate : ca]
        B[certificate : bundle]
    end

    C{Dapr-Trustbundle}

    subgraph "Dapr"
        D[secrets : dapr-trust-bundle]
        E[configmap : dapr-trust-bundle]
    end

    subgraph cert-manager_bundle [secret : dapr-cert-bundle-from-cert-manager]
    end

    B --> cert-manager_bundle
    A --> cert-manager_bundle
    cert-manager_bundle --> C
    C -- "ca.crt => ca.crt<br>issuer.crt => tls.crt<br>issuer.key => tls.key" --> D
    C -- "ca.crt => ca.crt" --> E
```

### Configuration

The operator can be configured using command-line flags:

**Configurable Parameters:**
- **Source secret name**: Configurable via `--source-secret-name` flag (default: `dapr-trust-bundle-from-cert-manager`)
- **Target namespace**: Configurable via `--target-namespace` flag (default: `dapr-system`)
- **Target secret name**: Always `dapr-trust-bundle` (created in target namespace)
- **Target ConfigMap name**: Always `dapr-trust-bundle` (created in target namespace)

**Default Configuration:**
- **Source secret name**: `dapr-trust-bundle-from-cert-manager`
- **Target namespace**: `dapr-system`
- **Operator scope**: Single namespace (only monitors the configured target namespace)

**Example with custom configuration:**
```bash
# Using custom source secret and namespace
--source-secret-name=my-cert-secret --target-namespace=production
```

**Helm Configuration:**
```yaml
controller:
  sourceSecretName: "my-cert-secret"
  targetNamespace: "production"
rbac:
  useClusterRole: false  # Use namespace-scoped permissions
```

## RBAC Permissions

The operator requires different permissions based on the deployment mode:

### Cluster-wide RBAC (default)
- `get`, `list`, `watch` on secrets (to monitor source secrets in any namespace)
- `create`, `update`, `patch`, `delete` on secrets and configmaps (to manage target resources)

### Namespace-scoped RBAC (enhanced security)
- `get`, `list`, `watch` on specific source secret by name (e.g., `dapr-trust-bundle-from-cert-manager`)
- `create`, `delete`, `patch`, `update` on secrets and configmaps (for resource creation)
- `get`, `list`, `watch`, `patch`, `update` on `dapr-trust-bundle` secret and configmap specifically
- `update` on finalizers and status for specific secrets only

**Enable namespace-scoped RBAC:**
```yaml
# Helm configuration
rbac:
  useClusterRole: false
controller:
  sourceSecretName: "my-cert-secret"
  targetNamespace: "my-namespace"
```

## Cleanup

**Remove the operator:**

```sh
make undeploy
```

**For KiND clusters, you can also clean up the entire cluster:**

```sh
kind delete cluster --name kind
```

## Development

### Prerequisites
- go version v1.24.0+
- docker version 17.03+
- kubectl version v1.11.3+
- Access to a Kubernetes cluster ([KiND](https://kind.sigs.k8s.io/) cluster recommended for development)


**Build and push your image to a registry:**

```sh
make docker-build docker-push IMG=<some-registry>/dapr-trustbundle:tag
```

**Deploy the operator to the cluster:**

```sh
make deploy IMG=<some-registry>/dapr-trustbundle:tag
```

Or if you are using a a [KiND](https://kind.sigs.k8s.io/) cluster

**Quick deployment to KiND cluster:**

```sh
# Build Docker image, load to KiND, and deploy
make kind-deploy
```

**For step-by-step deployment:**

```sh
# Build and load image to KiND cluster
make kind-load

# Deploy to cluster
make deploy
```

### Testing the Operator

1. **Create a test secret** (this creates it in the `dapr-system` namespace):
   ```sh
   kubectl apply -f examples/test-secret.yaml
   ```

2. **Verify the target secret was created:**
   ```sh
   kubectl get secret dapr-trust-bundle -n dapr-system -o yaml
   ```

3. **Test automatic updates** by modifying the source secret:
   ```sh
   kubectl patch secret dapr-trust-bundle-from-cert-manager -n dapr-system -p '{"data":{"new-key":"bmV3LXZhbHVl"}}'
   kubectl get secret dapr-trust-bundle -n dapr-system -o yaml
   ```

4. **Monitor operator logs:**
   ```sh
   kubectl logs -n dapr-system -l control-plane=operator -f
   ```

For more details take a look to [./examples/README.md](./examples/README.md)

### CI/CD Pipeline

This project includes GitHub Actions workflows for automated building and releasing:

- **Linting** (`.github/workflows/lint.yml`):
  - Runs `golangci-lint` to check Go code for style and correctness issues

- **Testing** (`.github/workflows/test.yml`):
  - Runs unit tests and integration tests
  - Ensures code changes do not break existing functionality

- **End-to-End Testing** (`.github/workflows/test-e2e.yml`):
  - Deploys the operator to a test cluster
  - Runs end-to-end tests against the deployed operator

- **Docker Build & Push** (`.github/workflows/docker-build-push.yml`):
  - Triggers on pushes to `main` branch and tag creation
  - Builds multi-platform Docker images (linux/amd64, linux/arm64)
  - Pushes to GitHub Container Registry (`ghcr.io`)
  - Generates deployment manifests as artifacts
  
**Image Tagging Strategy:**
- `main` - Latest development version from main branch
- `v1.0.0` - Stable release versions
- `pr-123` - Pull request builds for testing

### Project Structure

```
├── cmd/main.go                    # Application entry point
├── internal/controller/           # Controller implementation
│   └── secret_controller.go       # Secret synchronization logic
├── config/                        # Kubernetes manifests
│   ├── default/                   # Default deployment configuration
│   ├── manager/                   # Manager deployment
│   └── rbac/                      # RBAC permissions
├── deploy/                        # Deployment artifacts
│   ├── install.yaml               # Complete installation manifest
│   ├── helm/                      # Helm chart
│   │   └── dapr-trustbundle/      # Helm chart directory
│   │       ├── Chart.yaml         # Chart metadata
│   │       ├── values.yaml        # Default values
│   │       ├── README.md          # Chart documentation
│   │       └── templates/         # Kubernetes templates
│   └── helm-packages/             # Packaged Helm charts (.tgz)
├── examples/                      # Example secrets and documentation
├── scripts/                       # Installation and utility scripts
│   └── install.sh                 # Automated installation script
├── .github/workflows/             # CI/CD workflows
│   ├── docker-build-push.yml      # Docker build and push
│   └── release.yml                # GitHub releases
└── Dockerfile                     # Container image definition
```

### Making Changes

1. **Modify controller logic** in `internal/controller/secret_controller.go`
2. **Update RBAC** by adding kubebuilder annotations and running `make manifests`
3. **Test locally** with `make kind-deploy`
4. **Check logs** with `kubectl logs -n dapr-system -l control-plane=operator -f`

### Custom Configuration

The operator now supports configuration via command-line flags, eliminating the need to modify source code:

**Runtime Configuration (Recommended):**
```bash
# Kubernetes deployment with custom flags
kubectl patch deployment controller-manager -n dapr-trustbundle-system \
  --patch '{"spec":{"template":{"spec":{"containers":[{"name":"manager","args":["--leader-elect","--health-probe-bind-address=:8081","--source-secret-name=my-cert-secret","--target-namespace=my-namespace"]}]}}}}'

# Helm deployment with custom values
helm install dapr-trustbundle deploy/helm/dapr-trustbundle \
  --set controller.sourceSecretName=my-cert-secret \
  --set controller.targetNamespace=my-namespace \
  --set rbac.useClusterRole=false
```

**Development Configuration:**
To modify the controller logic in `internal/controller/secret_controller.go`:

- Adjust the data transformation logic (key renaming)
- Modify cleanup behavior
- Add custom validation logic
- Extend monitoring capabilities

## Troubleshooting

### Common Issues

1. **ImagePullBackOff**: Ensure you send the newly built image to your cluster, we use `make kind-load` that takes care of this
2. **RBAC errors**: Run `make manifests` and `make deploy` to update permissions
3. **Secret not syncing**: Check operator logs and verify the source secret name matches exactly

### Debug Commands

```sh
# Check operator status
kubectl get pods -n dapr-system -l control-plane=operator

# View operator logs
kubectl logs -n dapr-system -l control-plane=operator --tail=50

# Check RBAC permissions
kubectl auth can-i create secrets --as=system:serviceaccount:dapr-system:dapr-trustbundle-operator

# List all secrets in dapr-system
kubectl get secrets,configmaps -n dapr-system
```

## Project Distribution

Following the options to release and provide this solution to the users.

### GitHub Releases (Recommended)

This project automatically builds and publishes Docker images to GitHub Container Registry when code is pushed to the main branch or when tags are created.

**Install the latest release:**

```sh
# Quick install with automated script
curl -sSL https://raw.githubusercontent.com/elenpay/dapr-trustbundle/main/dapr-trustbundle/scripts/install.sh | bash

# Or install and test
curl -sSL https://raw.githubusercontent.com/elenpay/dapr-trustbundle/main/dapr-trustbundle/scripts/install.sh | bash -s -- --test

# Install directly from GitHub releases
kubectl apply -f https://github.com/elenpay/dapr-trustbundle/releases/latest/download/install.yaml
```

**Docker Images:**

- **Latest (main branch)**: `ghcr.io/elenpay/dapr-trustbundle/dapr-trustbundle:main`
- **Stable releases**: `ghcr.io/elenpay/dapr-trustbundle/dapr-trustbundle:v1.0.0`
- **Development**: `ghcr.io/elenpay/dapr-trustbundle/dapr-trustbundle:pr-123`

### Manual Build and Distribution

#### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=ghcr.io/elenpay/dapr-trustbundle/dapr-trustbundle:main
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the deploy directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/elenpay/dapr-trustbundle/main/dapr-trustbundle/deploy/install.yaml
```

### By providing a Helm Chart

**Install using Helm:**

```sh
# Install from local chart
helm install dapr-trustbundle deploy/helm/dapr-trustbundle

# Install with custom values
helm install dapr-trustbundle deploy/helm/dapr-trustbundle \
  --set image.tag=v1.0.0 \
  --set metrics.serviceMonitor.enabled=true

# Install in custom namespace
helm install dapr-trustbundle deploy/helm/dapr-trustbundle \
  --namespace my-namespace \
  --create-namespace \
  --set namespace.name=my-namespace
```

**Package the Helm chart:**

```sh
make helm-package
```

**Template and verify the Helm chart:**

```sh
make helm-template
```

The Helm chart provides extensive configuration options including:
- Resource limits and requests
- Horizontal Pod Autoscaling
- ServiceMonitor for Prometheus
- Pod Disruption Budgets
- Network Policies
- Custom environment variables

See `deploy/helm/dapr-trustbundle/README.md` for complete configuration options.

### Installing with Pre-built Manifests

1. **Build the installer:**
   ```sh
   make build-installer
   ```

2. **Apply the generated manifests:**
   ```sh
   kubectl apply -f deploy/install.yaml
   ```

### Installing from Source

1. **Clone the repository:**
   ```sh
   git clone <repository-url>
   cd dapr-trustbundle
   ```

2. **Deploy with KiND:**
   ```sh
   make kind-deploy
   ```

## License

Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

