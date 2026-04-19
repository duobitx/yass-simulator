#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME=yass

make docker-build
V="local-${RANDOM}"

docker tag ghcr.io/duobitx/yass/internal-components:latest ghcr.io/duobitx/yass-internal-components:$V
kind load docker-image ghcr.io/duobitx/yass-internal-components:$V --name $CLUSTER_NAME
kubectl -n yass-system set env deployment/yass-controller-manager INTERNAL_COMPONENTS_VERSION=${V}
echo "Internal components image ghcr.io/duobitx/yass-internal-components:$V"

