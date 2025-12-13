# Yass kubernetes operator

## Components

### Yass experiment controller

### Yass satellite controller


## Build

```shell
make docker-build docker-push build-installer
```

Commit and push `dist/install.yaml`.



## Yass Operator Installation

### Prerequisites

1. kubectl installed
2. Github account with access to PhiLab organisation
3. Kubernetes cluster with admin privileges.

#### Create kubernetes cluster using Kind
For local run consider using [Kind](https://kind.sigs.k8s.io/) project.
After installing Kind.
```shell
kind create cluster --name yass
```


### 1. Registry secret
Assuming you have working kubernetes cluster with admin role.

1. Generate GitHub access token to access `ghcr.io` registry.
2. Create namespace for yass operator
```shell
kubectl create namespace yass-system
```

3. Create `docker-registry` secret
```shell
kubectl -n yass-sytem create secret docker-registry docker-secret \
  --docker-server=https://ghcr.io/v1/ \
  --docker-username=YOUR_GITHUB_USERNAME \
  --docker-password=GENERATED_TOKEN \
  --docker-email=YOUR_EMAIL
```

### 2. Install cert-manager
```shell
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.19.1/cert-manager.yaml
# wait for cert manager to start
kubectl wait -namespace cert-manager --for=condition=Available deployment --all --timeout=300s
```


### 3. Install Yass operator
If you want to modify version of `internal components` edit envs-patch.yaml and execute `make manifests build-installer` first.

```shell
# Prepare namespace for yass operator
# Use server-side apply to avoid large annotation errors on big CRDs
kubectl -n yass-system apply -f dist/install.yaml
kubectl -n yass-system patch serviceaccount yass-controller-manager -p '{"imagePullSecrets": [{"name": "docker-secret"}]}'
# try to redownload image after imagePullSecrets is applied
kubectl -n yass-system delete `kubectl -n yass-system get pod -o name|grep yass-controller` 
```

### 4. Prepare namespace for an experiment ("default")
```shell
#kubect create namespace for an experiment
NS=namespace-name
kubectl create namespace "${NS}" && kubectl label namespace "${NS}" yass-namespace=true
```

### 5. Optional - Build local operator image
```shell
make docker-build
docker tag ghcr.io/duobitx/yass-operator:latest ghcr.io/duobitx/yass-operator:yourTAG
# edit and update image tag
kubectl -n yass-system edit deployments.apps yass-controller-manager 
```
