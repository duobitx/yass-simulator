# experiment-executor

Per-experiment controller pod. Spawns the orbital propagator `geo_calc` as a subprocess, reads its protobuf frames from `/dev/shm/geo_calc_shared_memory`, derives network parameters (delay = `1ms + distance/c`, plus a placeholder packet-loss/bandwidth model) and broadcasts `FsNodeUpdate` messages over MQTT. Each FsNode's `world-controller` consumes these and applies `tc` rules locally.

Also keeps the `Experiment` resource's `.status.experimentTime` ticking, transitions it to `TimedOut` when `MaxDuration` is exceeded, and listens on `experiment/end-request` so agents can end the experiment early with success/failure.

## Endpoints

The Go HTTP server on `:8080` is what the operator calls when transitioning the experiment to `Ongoing`:

- `POST /start` — begin the simulation loop.
- `POST /error-report` — record a component failure event.

## Required environment

| Var | Purpose |
|---|---|
| `YASS_EXPERIMENT` | name of the `Experiment` resource (used as MQTT identity and for status updates) |
| `NAMESPACE` | namespace of the `Experiment` (for `client.Get`/`Status().Update`) |
| `MESSAGING_BROKER_HOST_PORT` | mosquitto endpoint, default `messaging:1883` |
| `EXPERIMENT_JSON_FILE_PATH` | path passed to `geo_calc`, default `/mnt/shared/experiment.json` |
| `AUTOSTART` | start immediately without waiting for `POST /start` |
| `MOCK_K8S` | use a fake K8s client (for local runs) |

## Dependencies

- [`geo-calculator`](../geo-calculator/) — the orbital propagator binary (`./geo_calc`) is shipped in the same image and exec'd by the executor.
- [`go-common`](../go-common/) — slog setup, protobuf types, K8s scheme.
- Mosquitto MQTT broker, deployed as a sibling pod by the operator.
