# yass-simulator

**YASS (Yet Another Satellite Simulator)** is a Kubernetes operator that simulates
a constellation of satellites and ground stations together with the line-of-sight
(LOS) networking between them. Each satellite or ground station is modelled as a
Pod; satellite positions are propagated from TLEs (SGP4), ground stations sit at
fixed coordinates, and the operator shapes every Pod's network with Linux `tc` so
peers are reachable only while they are actually in view.

It exists to run **reproducible, observable experiments** on that simulated world
— in particular, to compare distributed file-systems (an IPFS-based engine) with a
classic point-to-point transfer engine under realistic orbital connectivity,
hardware faults and resource limits. Typical uses:

- measure how fast, and how reliably, a file produced on a satellite reaches a
  ground station, and compare delivery strategies head-to-head;
- stress engines and routing under injected faults (network outage/throttle, disk
  full/failure, satellite destruction) and large-scale topology;
- quantify the cost of a run — CPU, memory, network TX/RX, battery/energy — per
  node and across the whole constellation.

## Features

- **Declarative experiments** — the whole world and scenario are Kubernetes custom
  resources (`Layout`, `HardwareDefinition`, `ExperimentDefinition`, `Experiment`),
  so every run is re-runnable YAML.
- **Realistic orbits and LOS networking** — TLE/SGP4-propagated satellites and
  fixed ground stations; `tc`-shaped bandwidth, latency and drop of out-of-LOS
  traffic, plus a headless broadcast channel that respects line-of-sight.
- **Pluggable file-system engines** — a classic point-to-point (TUS) engine and a
  distributed IPFS-based (EDFS) engine ship in-box; bring your own via a thin
  contract.
- **Pluggable agents** — the per-node workload (produce files, wait to receive,
  react to position/power) is any container image following a small contract.
- **Hardware fault injection** — scheduled, one-shot or recurring faults:
  bandwidth reduction, network failure, disk full, disk failure, and node destroy.
- **Simulated battery/energy model** — per-hardware-profile capacity, charge and
  per-byte TX/disk energy costs, with low-power-mode behaviour.
- **Built-in observability** — Prometheus metrics, Loki events and pre-provisioned
  Grafana dashboards (overview, per-node drill-down, TUS-vs-EDFS comparison).
- **Aggregated runtime API** — each experiment exposes live state and a
  downloadable **Parquet results bundle** through the Kubernetes API server.
- **Run lifecycle controls** — auto-start when ready, an optional `maxDuration`
  timeout, and automatic resource eviction after a run while keeping its results.

## Documentation

- [Installation guide](./INSTALLATION.md) — prerequisites (cluster / KinD,
  kubectl) and installing the operator, cert-manager and the observability stack on
  a clean cluster.
- [User guide (manual)](./USER-GUIDE.md) — the custom resources and how they
  relate, writing/running/monitoring experiments, FsNode and Experiment state
  machines, downloading Parquet results, and writing your own agent or engine.

## Repository layout

Monorepo with the two components that together implement the YASS simulator:

- [`internal-components/`](./internal-components/) — runtime pieces injected into each experiment / pod: `experiment-executor`, `world-controller`, `geo-calculator`, `resource-to-json`, `events-webapp`, `web-ui`, `mosquitto-docker-image`, plus shared `go-common`. Packaged as a single image `ghcr.io/duobitx/yass-internal-components`.
- [`yass-operator/`](./yass-operator/) — Kubernetes operator that reconciles `Experiment`, `FsNode`, `Layout`, `ExperimentDefinition`, `HardwareDefinition` CRDs (group `int.esa.yass/v1`) and creates pods from them. Image `ghcr.io/duobitx/yass-operator`.

`internal-components` and `yass-operator` share Go types via the `go.work` file at this level.

## Limits

- A single experiment may declare at most **1024 FsNodes** (satellites + ground stations combined).
- For larger experiments — many FsNodes, and/or several experiments running in parallel — the cluster **control plane (API server + etcd), not the worker nodes, is usually the first bottleneck**. Every FsNode is a Pod and the simulation drives a high rate of API operations (status updates, fault-event churn, pod lifecycle), so make sure the control-plane nodes are sized accordingly and limit how many experiments run at once. An undersized control plane can become unresponsive and stall or fail otherwise-healthy runs even when the workers are nearly idle.

## FsNode broadcast (`fsnode-broadcast` service)

Alongside the per-experiment infrastructure services (`messaging`, `experiment-executor`,
`mqtt2prom`, …), the operator creates a headless Service **`fsnode-broadcast`** in every
experiment namespace (template `yass-operator/obj-templates/fsnode-broadcast-service.yaml`,
selector `yass-experiment: <experiment>`). Being headless, its DNS name resolves to **every
FsNode pod IP**, so any component running on an FsNode can use it as a broadcast channel.

To broadcast, send a UDP datagram to each IP the name resolves to, on a port inside the
`world-controller`'s tc-managed range (**9000–9999**). tc then **drops the datagram for peers that
are not currently in line-of-sight** and delivers it only to current LOS neighbours — reproducing a
physical RF broadcast (only nodes in range hear it) without needing multicast. The short name
`fsnode-broadcast` resolves in-namespace, so no FQDN is needed.

This is a general-purpose facility — any component may use it. For example, the EDFS replication
protocol uses it (UDP port 9101) to recruit nearby FsNodes to pin a file.

## Build

```shell
task internal-components       # build & push internal-components image
task yass-operator             # build & push operator image
task                           # build & push both
task internal-components:build # build internal-components image only
task yass-operator:build       # build operator image only
```

See each subdirectory's README for details on the individual components.
