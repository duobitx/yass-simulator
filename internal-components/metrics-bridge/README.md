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
| `K8S_EVENTS_SKIP_KINDS`  | no       | —                | comma-separated Loki-event kinds to skip when mirroring to k8s Events (e.g. `crud`) |

## Delivery detection

`yass_file_delivery_seconds`, `yass_file_received_*` and `yass_file_lost_total`
are derived by joining each `PUT` to the later `RECEIVED` events for the
**same file content hash (md5)**. A delivery is counted once per
`(md5, receiver)` pair (so a peer re-fetching the same content is not
double-counted), and `is_target_gs` marks receipts by a ground-station node.

> **Assumption: every generated file has unique content.** Files are keyed by
> md5, so two byte-identical files share a key: the second `PUT` is dropped and
> its delivery/loss is misattributed to the first. The experiment agents must
> produce distinct content per file (e.g. randomised payloads, not zero-filled
> blobs of a fixed size). All current scenarios satisfy this; keep it in mind
> when adding new producers.

## Kubernetes Events

Every event the bridge sends to Loki is also mirrored as a Kubernetes
Event on the `Experiment` CR named `$EXPERIMENT_NAME` in the bridge's
namespace, so `kubectl describe experiment <name>` and
`kubectl get events --field-selector involvedObject.kind=Experiment`
both show the in-experiment activity.

Severity (`Normal` / `Warning`) is derived from `(kind, eventType)`:

| kind         | Warning when                                                  |
|--------------|---------------------------------------------------------------|
| `lifecycle`  | state ∈ {`Failure`, `Errored`, `TimedOut`}                    |
| `online_state` | type = `offline`                                            |
| `power`      | type = `enter_low_power`                                      |
| `hardware`   | type contains `error`, `fail`, `fault`, `alert`               |
| `crud`       | never                                                         |

K8s `EventRecorder` aggregates within a 10-minute window by
(reason, involvedObject, type) — so a busy `crud.PUT` stream collapses
into a single event with an incrementing `count` instead of one event
per file. Use `K8S_EVENTS_SKIP_KINDS=crud` to opt out entirely.

The bridge needs `create events` on its namespace; it gets this from
the operator-installed `yass-experiment-sa` ServiceAccount (set via
`serviceAccountName` in the deployment template).

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
