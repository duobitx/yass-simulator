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
2. Create `docker-registry` secret
```shell
kubectl create secret docker-registry docker-secret \
  --docker-server=https://ghcr.io/v1/ \
  --docker-username=YOUR_GITHUB_USERNAME \
  --docker-password=GENERATED_TOKEN \
  --docker-email=YOUR_EMAIL \
  --dry-run=client -o yaml > docker-secret.yaml
```

### 2. Install certmanager
```shell
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.19.1/cert-manager.yaml
```

### 3. Install Yass operator
```shell
# Prepare namespace for yass operator
# Use server-side apply to avoid large annotation errors on big CRDs
kubectl -n yass-system apply --server-side -f dist/install.yaml
kubectl -n yass-system create -f docker-secret.yaml
kubectl -n yass-system patch serviceaccount yass-controller-manager -p '{"imagePullSecrets": [{"name": "docker-secret"}]}'
kubectl -n yass-system patch serviceaccount default -p '{"imagePullSecrets": [{"name": "docker-secret"}]}'
# try to redownload image after imagePullSecrets is applied
kubectl -n yass-system delete `kubectl -n yass-system get pod -o name|grep yass-controller` 
```

### 4. Prepare namespace for an experiment ("default")
```shell
#kubect create namespace default  
kubectl -n default create -f docker-secret.yaml
```

