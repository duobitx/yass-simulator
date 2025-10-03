# Yass kubernetes controllers

## Components
### Yass experiment controller

### Yass satellite controller


## Build
```shell
make docker-build docker-push build-installer
```

## Helm Chart
A Helm chart is provided under charts/yass-experiment-operator.

Install into a namespace (example yass-system):
```shell
helm upgrade -i yass charts/yass-experiment-operator \
  --namespace yass-system --create-namespace \
  --set image.repository=<your-repo/manager> \
  --set image.tag=<tag>
```

By default, the chart:
- Creates a ServiceAccount and necessary RBAC (ClusterRole/Binding and leader election Role/Binding);
- Deploys the controller manager Deployment with health probes and resource requests/limits;
- Exposes a metrics Service on port 8443 (configurable);
- Optionally creates a NetworkPolicy to restrict metrics access (enable via values).

CRDs are installed automatically from the chart's crds/ directory.