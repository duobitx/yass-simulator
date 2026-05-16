# metrics-bridge

MQTT → Prometheus bridge. Deployed by `yass-operator` inside every
experiment namespace, alongside `messaging` (mosquitto). Subscribes to
the simulator's existing MQTT topics, derives Prometheus counters /
histograms / gauges, exposes them on `:9090/metrics` for the global
Prometheus instance in `yass-system` to scrape.

## Topics consumed

| Topic                          | Drives                                                          |
|--------------------------------|-----------------------------------------------------------------|
| `crud-events`                  | `yass_file_produced_*`, `..._received_*`, `..._delivery_seconds`, `..._lost_total` |
| `<fsNode>/resources`           | `yass_battery_*`, `yass_in_shadow`, `yass_low_power`, `yass_volume_*`, `yass_container_*` |
| `total-network-stats/<fsNode>` | `yass_network_{tx,rx}_{bytes,packets}_total`                    |
| `online-states/<fsNode>`       | IP → fsNode reverse map for `peer_node` label on network metrics |

## Env

| Var                      | Required | Default          | Meaning                                                          |
|--------------------------|----------|------------------|------------------------------------------------------------------|
| `EXPERIMENT_NAME`        | yes      | —                | populated as k8s pod label `yass-experiment`, becomes `experiment` Prometheus label |
| `ENGINE`                 | yes      | —                | `tus` / `edfs`                                                   |
| `RUN_ID`                 | yes      | —                | stable per experiment run (e.g. `forever@2026-05-16T14:02Z`)     |
| `NAMESPACE`              | no       | —                | informational only                                                |
| `MESSAGING_BROKER_HOST_PORT` | no   | `messaging:1883` | local mosquitto                                                  |
| `LISTEN_ADDR`            | no       | `:9090`          |                                                                  |
| `TARGET_GS_BY_FSNODE`    | no       | `{}`             | JSON map `{satellite: ground-station}`; drives `is_target_gs`    |
| `DELIVERY_DEADLINE`      | no       | `2h`             | files un-delivered after this become `yass_file_lost_total`      |
| `LOG_LEVEL`              | no       | `INFO`           |                                                                  |

The common labels `experiment`, `engine`, `run_id`, `namespace` are NOT
attached by the bridge itself — they are stamped on every scraped series
by Prometheus, via relabel rules in
`yass-flux/clusters/base/observability/prometheus.yml` that read pod
labels.

## Build & run

```
task build
EXPERIMENT_NAME=forever ENGINE=tus RUN_ID=run-1 \
  TARGET_GS_BY_FSNODE='{"oneweb-0008":"estrack-kiruna"}' \
  ./main
```
