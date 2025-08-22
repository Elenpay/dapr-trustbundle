# Dapr Trust Bundle Operator

This operator monitors the `dapr-trust-bundle-from-cert-manager` secret and automatically creates/updates a `dapr-trust-bundle` secret with the same data whenever the source secret changes.

## How it works

1. The operator watches for changes to secrets named `dapr-trust-bundle-from-cert-manager`
2. When this secret is created or updated, the operator:
   - Creates or updates a secret named `dapr-trust-bundle` in the same namespace
   - Copies all data from the source secret to the target secret
   - Maintains the same secret type
   - Adds management labels to track the created secret

## Running the operator

### Development (local)
```bash
# Install CRDs and deploy RBAC
make install

# Run the controller locally
make run
```

### Production (in-cluster)
```bash
# Build and push the container image
make docker-build docker-push IMG=your-registry/dapr-trustbundle:latest

# Deploy to cluster
make deploy IMG=your-registry/dapr-trustbundle:latest
```

## Testing

If you have [cert-manager](https://cert-manager.io/) operator running in your cluster, you can use it to issue and manage the certificates.
This will be ideal scenario so will cert-manager the one on charge of renew your certificate and keep your CA key safe, just be sure your CA lives long enough to avoid problems and this operator will take care of integrate cert-manager resulting secret into a secret valid for Dapr.

```
kubectl apply -f certificate.yaml
```

To force the operator to act over `dapr-trust-bundle` secret and configmap just delete cert-manager resulting secret so the train of operator will left the station and regenerate the new cert-manager secret which will be later moved to `dapr-trust-bundle`.

If for some reason you do not plan to use cert-manager or you want to test this operator without installing it we got you covered with an example using a plain secret:

1. Create a test secret (this will create it in the `dapr-system` namespace):
```bash
kubectl apply -f examples/test-secret.yaml
```

2. Check that the target secret was created:
```bash
kubectl get secret dapr-trust-bundle -n dapr-system -o yaml
```

3. Update the source secret and verify the target secret is updated:
```bash
kubectl patch secret dapr-trust-bundle-from-cert-manager -n dapr-system -p '{"data":{"new-key":"bmV3LXZhbHVl"}}'
kubectl get secret dapr-trust-bundle -n dapr-system -o yaml
```


## RBAC Permissions

The operator requires the following permissions:
- `get`, `list`, `watch` on secrets (to monitor the source secret)
- `create`, `update`, `patch`, `delete` on secrets (to manage the target secret)

## Secret Lifecycle

- **Source secret created/updated**: Target secret is created/updated with the same data
- **Source secret deleted**: Target secret is also deleted (configurable behavior)

## Configuration

The operator is configured to monitor secrets named `dapr-trust-bundle-from-cert-manager`. The target secret name is `dapr-trust-bundle`.

To modify this behavior, edit the controller logic in `internal/controller/secret_controller.go`.
