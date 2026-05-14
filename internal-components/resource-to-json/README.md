# resource-to-json

InitContainer binary. Reads a Kubernetes resource by `RESOURCE_KIND` + `RESOURCE_NAME` from `NAMESPACE` and writes it to `DST_DIR/<kind>.json` so the rest of the pod has the spec without needing K8s API access at runtime.

Used by the FsNode pod's init phase: the operator schedules it as `resource-to-json-fsnode` with `DST_DIR=/mnt/shared`, mounting the pod-shared `EmptyDir` volume. `experiment-executor` and `world-controller` then read `/mnt/shared/experiment.json` / `/mnt/shared/fsnode.json`.

## Environment

| Var | Purpose |
|---|---|
| `RESOURCE_KIND` | e.g. `FsNode`, `Experiment` |
| `RESOURCE_NAME` | resource name |
| `NAMESPACE` | resource namespace (set via downward API) |
| `DST_DIR` | output directory |

## Build

```shell
task            # build + test
task build
task test
```

Builds a static `linux/amd64` binary as `./main` (`CGO_ENABLED=0`).
