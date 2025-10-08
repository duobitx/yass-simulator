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
### Presentments
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
### 2. Install Yass operator
```shell
# Prepare namespace for yass operator
kubect create namespace yass-system
kubectl -n yass-system create -f docker-secret.yaml
kubectl -n yass-system patch serviceaccount default -p '{"imagePullSecrets": [{"name": "docker-secret"}]}'
# Install operator
kubectl apply -f dist/install.yaml
```

### 3. Prepare namespace for an experiment ("default")
```shell
#kubect create namespace default  
kubectl -n default create -f docker-secret.yaml
kubectl -n default patch serviceaccount default -p '{"imagePullSecrets": [{"name": "docker-secret"}]}'
```
