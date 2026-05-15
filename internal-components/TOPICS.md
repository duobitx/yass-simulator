# YASS MQTT topics

Broker: `messaging:1883` (mosquitto, deployed by `yass-operator` as a sibling pod
of the experiment-executor). Wire format below is whatever `Publish` is called
with — for `proto.*` types this is JSON-marshalled protobuf (snake_case
fields), since the producers call `json.Marshal(protoStruct)` rather than the
binary proto wire format.

| Topic | Producer | Subscribers | Payload | QoS | Retained |
|---|---|---|---|---|---|
| `online-states/<fsNode>` | world-controller | experiment-executor (`online-states/#`), world-controller self (`online-states/#`) | `proto.FsNodeOnlineState` (JSON) | 0 | true |
| `updates/<fsNode>` | experiment-executor | world-controller (per-node), events-webapp (prefix `updates`) | `proto.FsNodeUpdate` (JSON) | 0 | true |
| `updates/_time_` | experiment-executor | — (events-webapp explicitly filters out `*_`) | `proto.TimeUpdate` (JSON) | 0 | true |
| `experiment/end-request` | experiment-executor (init publish), agents | experiment-executor | `proto.AgentExperimentEndRequest` (JSON) or empty | 0 | true |
| `total-network-stats/<fsNode>` | world-controller | events-webapp | `[]proto.TrafficStats` (JSON) | 0 | false |
| `energy/<fsNode>` ⚠️ **deprecated** | world-controller | — | JSON of internal `NodeHwState` (battery, in-shadow, etc.) | 0 | false |
| `<fsNode>/resources` | world-controller (planned — proto schema in place, publisher not yet implemented) | events-webapp | `proto.FsNodeResources` (JSON) — power mode, per-volume disk usage, per-engine-container CPU/RAM | 0 | false |
| `crud-events` | fs-engines via `fs_engine_wrapper`'s `facadeNotifier` | — (retained for later retrieval) | `notifier.NotifyEvent` (JSON) — file `PUT` / `DELETE` / `RECEIVED` with size, md5, attributes | 0 | true |
| `edfs-name-server` | edfs_engine (each instance) | edfs_engine (each instance — full mesh) | `TopicMessage` (JSON) — CID ↔ name mapping for the EDFS cluster | 0 | true |

## Naming conventions

- Per-FsNode metrics produced by world-controller / experiment-executor use one
  of two shapes:
  - `<category>/<fsNode>` — older topics: `online-states/...`,
    `updates/...`, `total-network-stats/...`, `energy/...`.
  - `<fsNode>/<category>` — newer convention for telemetry consumed by
    engines/agents inside the FsNode (currently: `<fsNode>/resources`).
- `_` suffixes on a segment (e.g. `updates/_time_`) mark control-plane /
  out-of-band entries that some subscribers should skip; events-webapp uses
  `strings.HasSuffix(topic, "_")` as a filter.

## Deprecated

### `energy/<fsNode>`
- **Status**: deprecated as of 2026-05-15; still published by world-controller,
  no consumers in this repository.
- **Reason**: superseded by `<fsNode>/resources`, which carries the same power
  state plus per-volume disk usage and per-engine-container CPU/RAM under a
  unified, versioned proto schema.
- **Plan**: keep publishing on `energy/<fsNode>` for now (no consumers, no
  cost). Remove once `<fsNode>/resources` has a publisher and at least one
  external consumer relies on the new topic.
