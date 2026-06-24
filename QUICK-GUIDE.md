# YASS — Quick Guide

From nothing to a running experiment in five steps. Need more detail later? See the
[Installation Guide](./INSTALLATION.md) and [User Guide](./USER-GUIDE.md).

You only need `kubectl` and a Kubernetes cluster. 
You can spin up a local one with [KinD](https://kind.sigs.k8s.io/docs/user/quick-start/):

```bash
kind create cluster --name yass
```

## 1. Install YASS

From the repository root:

```bash
./install.sh
```

This installs the operator and the observability stack (Prometheus, Loki, Grafana).
It is safe to re-run. Check it is up:

```bash
kubectl -n yass-system get pods
```

## 2. Run your first experiment

The repo ships a ready-to-run example — `big-scale` (50 satellites and seven ground
stations). Apply the shared hardware profiles once, then the example:

```bash
kubectl apply -f yass-experiments/experiments/_common_/hardware_specs.yaml
kubectl apply -k yass-experiments/experiments/big-scale/tus/
```

## 3. Watch it run

```bash
kubectl -n big-scale-experiment-tus get experiment -w   # STATE goes Init -> Ready -> Ongoing
kubectl -n big-scale-experiment-tus get fsnodes -w      # per-node phase, battery, position
```

## 4. Open the web UI

A live map of the constellation (positions and line-of-sight links) is served per
experiment while it runs:

```bash
kubectl -n big-scale-experiment-tus port-forward svc/yass-web-ui 8080:80
# -> open http://localhost:8080
```

## 5. Get results, then clean up

Once the run ends, download its results bundle (Parquet metrics + events):

```bash
URL=$(kubectl -n big-scale-experiment-tus get experiment big-scale-experiment -o jsonpath='{.status.apiServerURL}')
kubectl get --raw "$URL/results" > big-scale-experiment-tus-results.zip
```

Tear the experiment down:

```bash
kubectl delete -k yass-experiments/experiments/big-scale/tus/
```

That's it. To write your own experiment (your own constellation, agents and engine),
head to the [User Guide](./USER-GUIDE.md).
