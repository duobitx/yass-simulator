# yass-simulator

Monorepo with the two components that together implement the YASS simulator:

- [`internal-components/`](./internal-components/) — runtime pieces injected into each experiment / pod: `experiment-executor`, `world-controller`, `geo-calculator`, `resource-to-json`, `events-webapp`, `web-ui`, `mosquitto-docker-image`, plus shared `go-common`. Packaged as a single image `ghcr.io/duobitx/yass-internal-components`.
- [`yass-operator/`](./yass-operator/) — Kubernetes operator that reconciles `Experiment`, `FsNode`, `Layout`, `ExperimentDefinition`, `HardwareDefinition` CRDs (group `int.esa.yass/v1`) and creates pods from them. Image `ghcr.io/duobitx/yass-operator`.

`internal-components` and `yass-operator` share Go types via the `go.work` file at this level.

## Limits

- A single experiment may declare at most **1,000,000 FsNodes** (satellites + ground stations combined). The geo-calculator (`internal-components/geo-calculator`) sizes its per-node arrays dynamically from the FsNode count in the experiment input and rejects inputs above this sanity bound.

## Build

```shell
task internal-components       # build & push internal-components image
task yass-operator             # build & push operator image
task                           # build & push both
task internal-components:build # build internal-components image only
task yass-operator:build       # build operator image only
```

See each subdirectory's README for details on the individual components.
