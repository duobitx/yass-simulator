# yass-operator Helm chart

Helm chart that installs the YASS operator and all its support objects (CRDs, RBAC, webhooks with cert-manager certificates, optional network policy, optional metrics).

The chart is normally consumed via Flux from [`yass-flux`](../../../../yass-flux/), but it can also be installed directly.

## Install

```shell
helm install yass-operator ./charts/yass-operator \
  --namespace yass-system --create-namespace
```

## Common values ([`values.yaml`](./values.yaml))

| Key | Purpose | Default |
|---|---|---|
| `controllerManager.container.image.{repository,tag}` | operator image | `ghcr.io/duobitx/yass-operator:latest` |
| `controllerManager.container.env.INTERNAL_COMPONENTS_VERSION` | tag of the `internal-components` image to inject into pods | `latest` |
| `controllerManager.container.env.EXPERIMENT_LOG_LEVEL` | log level for in-pod components | `INFO` |
| `controllerManager.container.env.DISABLE_NETWORKING_MANIPULATION` | turn off `tc`/netlink (no `NET_ADMIN` required) | `false` |
| `controllerManager.container.env.ENABLE_WEBHOOKS` | enable validating webhooks | `true` |
| `rbac.enable` / `crd.enable` / `metrics.enable` / `webhook.enable` / `certmanager.enable` / `networkPolicy.enable` | feature toggles | see file |

## Prerequisite

cert-manager must be installed in the cluster; the operator's validating webhooks rely on certs it issues.

## Templates

See [`templates/`](./templates/): `manager/`, `crd/`, `rbac/`, `webhook/`, `certmanager/`, `metrics/`, `network-policy/`, `prometheus/`.
