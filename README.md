# yass-simulator

Monorepo with the two components that together implement the YASS simulator:

- [`internal-components/`](./internal-components/) — runtime pieces injected into each experiment / pod: `experiment-executor`, `world-controller`, `geo-calculator`, `resource-to-json`, `events-webapp`, `web-ui`, `mosquitto-docker-image`, plus shared `go-common`. Packaged as a single image `ghcr.io/duobitx/yass-internal-components`.
- [`yass-operator/`](./yass-operator/) — Kubernetes operator that reconciles `Experiment`, `FsNode`, `Layout`, `ExperimentDefinition`, `HardwareDefinition` CRDs (group `int.esa.yass/v1`) and creates pods from them. Image `ghcr.io/duobitx/yass-operator`.

`internal-components` and `yass-operator` share Go types via the `go.work` file at this level.

## Limits

- A single experiment may declare at most **1024 FsNodes** (satellites + ground stations combined).
- For larger experiments — many FsNodes, and/or several experiments running in parallel — the cluster **control plane (API server + etcd), not the worker nodes, is usually the first bottleneck**. Every FsNode is a Pod and the simulation drives a high rate of API operations (status updates, fault-event churn, pod lifecycle), so make sure the control-plane nodes are sized accordingly and limit how many experiments run at once. An undersized control plane can become unresponsive and stall or fail otherwise-healthy runs even when the workers are nearly idle.

## Build

```shell
task internal-components       # build & push internal-components image
task yass-operator             # build & push operator image
task                           # build & push both
task internal-components:build # build internal-components image only
task yass-operator:build       # build operator image only
```

See each subdirectory's README for details on the individual components.
