#!/usr/bin/bash

echo "If you want to replace INTERNAL_COMPONENTS version pleas have a look config/manager/envs-patch.yaml".
echo "If you are using kind you will need to load the image if it's not available in the registry."
sleep 1

set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-yass}"

kind delete cluster --name $CLUSTER_NAME || true
kind create cluster --name $CLUSTER_NAME

kubectl create namespace yass-system

#kubectl -n yass-system create secret docker-registry docker-secret \
#  --docker-server=https://ghcr.io/v1/ \
#  --docker-username=$(<.github/USER) \
#  --docker-password=$(<.github/TOKEN) \
#  --docker-email=$(<.github/EMAIL)
kubectl -n yass-system create -f docker-secret.yaml


kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.19.1/cert-manager.yaml
# wait for cert manager to start
kubectl wait -n cert-manager --for=condition=Available deployment --all --timeout=300s

make manifests build-installer
kubectl -n yass-system apply -f dist/install.yaml
kubectl -n yass-system patch serviceaccount yass-controller-manager -p '{"imagePullSecrets": [{"name": "docker-secret"}]}'

# use local version of yass operator
make docker-build
V="local-${RANDOM}"
docker tag ghcr.io/duobitx/yass-operator:latest ghcr.io/duobitx/yass-operator:$V
kind load docker-image ghcr.io/duobitx/yass-operator:$V --name $CLUSTER_NAME
kubectl -n yass-system patch deployments.apps yass-controller-manager \
  --type='json' \
  -p="[
    {
      \"op\": \"replace\",
      \"path\": \"/spec/template/spec/containers/0/image\",
      \"value\": \"ghcr.io/duobitx/yass-operator:$V\"
    }
  ]"

echo "Yass operator image ghcr.io/duobitx/yass-operator:$V"
kubectl wait -n yass-system --for=condition=Available deployment --all --timeout=300s

# Prepare default namespace
kubectl label namespace default yass-namespace=true
