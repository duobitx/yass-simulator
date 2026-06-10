# YASS — Installation Guide

YASS (Yet Another Satellite Simulator) is a Kubernetes operator that simulates a
constellation of satellites and ground stations and runs reproducible experiments
on top of them.

This guide gets YASS running on a **clean cluster**. To author and run experiments
once it is installed, see the [User Guide](./USER-GUIDE.md). For the fastest path
from nothing to a running experiment, see the [Quick Guide](./QUICK-GUIDE.md).

---

## Table of contents

1. [Prerequisites](#1-prerequisites)
2. [Installing the operator](#2-installing-the-operator)
   - [One-command install](#one-command-install)
   - [What gets installed](#what-gets-installed)
   - [Step-by-step alternative](#step-by-step-alternative)
   - [Private images (GHCR pull secret)](#private-images-ghcr-pull-secret)
   - [Pinning image versions](#pinning-image-versions)
   - [Sizing the control plane](#sizing-the-control-plane)
   - [Verifying the installation](#verifying-the-installation)
   - [Accessing the observability stack](#accessing-the-observability-stack)
   - [Uninstalling](#uninstalling)

---

## 1. Prerequisites

You need:

- **A Kubernetes cluster (v1.27+)** you can reach with `kubectl`, using a context
  that has cluster-admin rights — the installer creates CRDs, cluster-scoped RBAC,
  webhooks and an aggregated `APIService`.
- **Worker capacity** for your experiments — every simulated node is a Pod, and its
  CPU/memory requests come from the `HardwareDefinition` you assign to it.
- **`kubectl`** within one minor version of your cluster.

**No cluster yet?** For local development, use
[KinD](https://kind.sigs.k8s.io/docs/user/quick-start/) (Kubernetes in Docker) —
follow their quick-start to install it, then create a cluster:

```bash
kind create cluster --name yass
kubectl cluster-info --context kind-yass
```

A ready-made config is kept in the repository as `kind-cluster.yaml`
(`kind create cluster --config kind-cluster.yaml`). A single-node KinD cluster is
fine for small runs (the
[`networking-demo`](../../yass-experiments/experiments/networking-demo) example) but
**not** for large constellations or many parallel experiments — use a real
multi-node cluster for those.

---

## 2. Installing the operator

The repository ships an **installer** — `install.sh` plus the pre-built manifests
under `dist/`. There is no Helm chart; everything is plain `kubectl apply`, so the
install is idempotent and re-runnable.

The installer applies three layers, in order:

1. **cert-manager** — required by the operator's admission webhooks.
2. **yass-operator** (`dist/install.yaml`) — the system namespace, all CRDs, RBAC,
   webhooks, the operator (`yass-controller-manager`) and the runtime API
   (`yass-experiment-apiservice`).
3. **observability** (`dist/observability`) — Prometheus, Loki and Grafana.

### One-command install

From the repository root (where `install.sh` and `dist/` live):

```bash
./install.sh
```

Common options:

```bash
./install.sh \
  --kubeconfig /path/to/kubeconfig \      # target a specific cluster (else current context)
  --namespace yass-system \               # system namespace (default: yass-system)
  --operator-tag e6877834 \               # pin the operator image tag
  --ghcr-user <user> --ghcr-token <token> # pull secret for private images
```

To skip a layer you already manage yourself:

```bash
./install.sh --no-cert-manager      # cert-manager already present
./install.sh --no-observability     # bring your own Prometheus/Loki/Grafana
```

See `./install.sh --help` for the full list.

### What gets installed

| Layer | Object | Namespace |
|---|---|---|
| cert-manager | Deployments, CRDs, webhooks | `cert-manager` |
| Operator | `Namespace`, 5 CRDs (`int.esa.yass`), RBAC, webhooks, `Certificate`/`Issuer` | cluster + `yass-system` |
| Operator | `Deployment/yass-controller-manager` | `yass-system` |
| Runtime API | `Deployment/yass-experiment-apiservice` + `APIService v1.runtime.esa.yass` | `yass-system` + cluster |
| Observability | Prometheus, Loki, Grafana | `yass-system` |

The five CRDs are `experiments`, `fsnodes`, `experimentdefinitions`, `layouts` and
`hardwaredefinitions`.

### Step-by-step alternative

If you prefer to drive each step yourself instead of `install.sh`:

```bash
# 1. cert-manager
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.19.1/cert-manager.yaml
kubectl -n cert-manager rollout status deploy/cert-manager-webhook --timeout=300s

# 2. operator: namespace, CRDs, RBAC, webhooks, manager, apiservice
kubectl apply -f dist/install.yaml
kubectl wait --for=condition=Established \
  crd/experiments.int.esa.yass crd/fsnodes.int.esa.yass \
  crd/experimentdefinitions.int.esa.yass crd/layouts.int.esa.yass \
  crd/hardwaredefinitions.int.esa.yass --timeout=120s
kubectl -n yass-system rollout status deploy/yass-controller-manager --timeout=300s

# 3. observability
kubectl apply -k dist/observability
```

### Private images (GHCR pull secret)

The operator, internal components and engines are published to GHCR. If the images
are private, create a pull secret in the system namespace (the installer does this
for you when `--ghcr-user`/`--ghcr-token` are passed):

```bash
kubectl -n yass-system create secret docker-registry docker-secret \
  --docker-server=ghcr.io \
  --docker-username=<user> \
  --docker-password=<token>
```

### Pinning image versions

For reproducible installs, pin concrete tags rather than `latest`:

- **Operator image** — set at install time:

  ```bash
  ./install.sh --operator-tag e6877834
  # or, on an existing install:
  kubectl -n yass-system set image deploy/yass-controller-manager \
    manager=ghcr.io/duobitx/yass-operator:e6877834
  kubectl -n yass-system rollout status deploy/yass-controller-manager
  ```

- **Internal components** — the operator stamps every FsNode Pod with the
  `INTERNAL_COMPONENTS_VERSION` it carries. Pin it with:

  ```bash
  ./install.sh --internal-components-version <tag>
  ```

- **Runtime API image** — pin the `yass-experiment-apiservice` Deployment to the
  same `yass-internal-components` tag for consistency:

  ```bash
  kubectl -n yass-system set image deploy/yass-experiment-apiservice \
    apiservice=ghcr.io/duobitx/yass-internal-components:<tag>
  ```

### Sizing the control plane

> **Important.** For large experiments — many FsNodes — and/or several experiments
> running in parallel, the cluster **control plane (API server + etcd), not the
> worker nodes, is usually the first bottleneck.** Every FsNode is a Pod and the
> simulation drives a high rate of API operations (status updates, fault-event
> churn, Pod lifecycle). An undersized control plane can become unresponsive and
> stall otherwise-healthy runs even when the workers are nearly idle.
>
> Size the control-plane nodes with CPU and memory headroom on the API server and
> etcd, and cap how many experiments run at once. A single experiment may declare
> at most **256 FsNodes** (satellites + ground stations combined), enforced by the
> admission webhook.

### Verifying the installation

```bash
# Operator and runtime API up
kubectl -n yass-system get pods

# CRDs registered
kubectl get crds | grep int.esa.yass

# Aggregated runtime API available
kubectl get apiservices | grep runtime.esa.yass
```

You should see `yass-controller-manager` and `yass-experiment-apiservice`
`Running`, five `int.esa.yass` CRDs, and the `v1.runtime.esa.yass` APIService
reporting `Available: True`.

### Accessing the observability stack

Port-forward the services from the system namespace:

```bash
# Grafana dashboards
kubectl -n yass-system port-forward svc/grafana 3000:3000
# -> http://localhost:3000   (default login: admin / yass-admin)

# Prometheus (PromQL)
kubectl -n yass-system port-forward svc/prometheus 9090:9090
# -> http://localhost:9090
```

Grafana ships pre-provisioned dashboards (Overview, fsNode drill-down, TUS vs EDFS
comparison, Events, Timeline) — see the
[User Guide → Monitoring](./USER-GUIDE.md#3-monitoring-an-experiment).

### Uninstalling

```bash
# Remove observability and the operator
kubectl delete -k dist/observability
kubectl delete -f dist/install.yaml

# Optionally remove cert-manager
kubectl delete -f https://github.com/cert-manager/cert-manager/releases/download/v1.19.1/cert-manager.yaml
```

Deleting `dist/install.yaml` removes the CRDs, which cascades to every YASS custom
resource on the cluster. Delete your experiment namespaces first if you want a
graceful teardown (see
[User Guide → Deleting an experiment](./USER-GUIDE.md#7-deleting-an-experiment)).
