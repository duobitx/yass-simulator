#!/usr/bin/env bash
#
# All-in-one installer for YASS on a fresh cluster (KinD or any Kubernetes).
#
# No Helm chart for YASS — it applies the kustomize-built artifacts under ./dist:
#   1. cert-manager            (cluster prerequisite, applied with --wait)
#   2. yass-operator           dist/install.yaml — Namespace, CRDs, RBAC,
#                              webhooks, cert-manager Certificate/Issuer, manager
#   3. observability           dist/observability — Prometheus, Loki, Grafana
#
# Re-runnable (kubectl apply is idempotent). dist/install.yaml is produced by
# `kubectl kustomize yass-operator/config/default` (kept in sync there).
#
# Usage:
#   ./install.sh [options]
#
# Options:
#   --kubeconfig PATH               kubeconfig to target (else current context)
#   --ghcr-user USER                GHCR username (private images)
#   --ghcr-token TOKEN              GHCR token/password (private images)
#   --internal-components-version T override INTERNAL_COMPONENTS_VERSION on the operator
#   --operator-tag TAG              override the yass-operator image tag
#   --cert-manager-version VER      cert-manager version (default: v1.19.1)
#   --namespace NAME                target namespace (default: yass-system)
#   --no-observability              skip Prometheus/Loki/Grafana
#   --no-cert-manager               skip cert-manager (already installed)
#   -h, --help                      show this help
#
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DIST="$HERE/dist"

NAMESPACE="yass-system"
CERT_MANAGER_VERSION="v1.19.1"
GHCR_USER=""; GHCR_TOKEN=""
INTCOMP_VERSION=""; OPERATOR_TAG=""
KUBECONFIG_ARG=""
DO_OBSERVABILITY=1
DO_CERT_MANAGER=1

while [[ $# -gt 0 ]]; do
  case "$1" in
    --kubeconfig) KUBECONFIG_ARG="$2"; shift 2 ;;
    --ghcr-user) GHCR_USER="$2"; shift 2 ;;
    --ghcr-token) GHCR_TOKEN="$2"; shift 2 ;;
    --internal-components-version) INTCOMP_VERSION="$2"; shift 2 ;;
    --operator-tag) OPERATOR_TAG="$2"; shift 2 ;;
    --cert-manager-version) CERT_MANAGER_VERSION="$2"; shift 2 ;;
    --namespace) NAMESPACE="$2"; shift 2 ;;
    --no-observability) DO_OBSERVABILITY=0; shift ;;
    --no-cert-manager) DO_CERT_MANAGER=0; shift ;;
    -h|--help) sed -n '2,38p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "Unknown option: $1" >&2; exit 1 ;;
  esac
done

KUBECTL=(kubectl)
[[ -n "$KUBECONFIG_ARG" ]] && KUBECTL=(kubectl --kubeconfig "$KUBECONFIG_ARG")

log() { printf '\n\033[1;36m==> %s\033[0m\n' "$*"; }

# --- Pre-flight -------------------------------------------------------------
command -v kubectl >/dev/null 2>&1 || { echo "kubectl not found" >&2; exit 1; }
[[ -f "$DIST/install.yaml" ]] || { echo "missing $DIST/install.yaml" >&2; exit 1; }
"${KUBECTL[@]}" cluster-info >/dev/null 2>&1 || { echo "no reachable cluster (check kubeconfig)" >&2; exit 1; }
log "Target: $("${KUBECTL[@]}" config current-context 2>/dev/null || echo "${KUBECONFIG_ARG:-current}")"

# --- 1. cert-manager --------------------------------------------------------
if [[ "$DO_CERT_MANAGER" == 1 ]]; then
  log "Installing cert-manager ${CERT_MANAGER_VERSION}"
  "${KUBECTL[@]}" apply -f "https://github.com/cert-manager/cert-manager/releases/download/${CERT_MANAGER_VERSION}/cert-manager.yaml"
  "${KUBECTL[@]}" -n cert-manager rollout status deploy/cert-manager-webhook --timeout=300s
  "${KUBECTL[@]}" -n cert-manager rollout status deploy/cert-manager --timeout=300s
fi

# --- 2. namespace + image pull secret --------------------------------------
log "Namespace ${NAMESPACE}"
"${KUBECTL[@]}" create namespace "$NAMESPACE" --dry-run=client -o yaml | "${KUBECTL[@]}" apply -f -
if [[ -n "$GHCR_USER" && -n "$GHCR_TOKEN" ]]; then
  log "docker-secret (GHCR pull secret)"
  "${KUBECTL[@]}" -n "$NAMESPACE" create secret docker-registry docker-secret \
    --docker-server=ghcr.io --docker-username="$GHCR_USER" --docker-password="$GHCR_TOKEN" \
    --dry-run=client -o yaml | "${KUBECTL[@]}" apply -f -
fi

# --- 3. yass-operator (CRDs, RBAC, webhooks, manager) ----------------------
log "Installing yass-operator (dist/install.yaml)"
"${KUBECTL[@]}" apply -f "$DIST/install.yaml"
"${KUBECTL[@]}" wait --for=condition=Established crd/fsnodes.int.esa.yass crd/experiments.int.esa.yass \
  crd/experimentdefinitions.int.esa.yass crd/layouts.int.esa.yass crd/hardwaredefinitions.int.esa.yass --timeout=120s

[[ -n "$INTCOMP_VERSION" ]] && { log "override INTERNAL_COMPONENTS_VERSION=$INTCOMP_VERSION"; \
  "${KUBECTL[@]}" -n "$NAMESPACE" set env deploy/yass-controller-manager INTERNAL_COMPONENTS_VERSION="$INTCOMP_VERSION"; }
[[ -n "$OPERATOR_TAG" ]] && { log "override operator image tag=$OPERATOR_TAG"; \
  "${KUBECTL[@]}" -n "$NAMESPACE" set image deploy/yass-controller-manager manager="ghcr.io/duobitx/yass-operator:$OPERATOR_TAG"; }

"${KUBECTL[@]}" -n "$NAMESPACE" rollout status deploy/yass-controller-manager --timeout=300s

# --- 4. observability -------------------------------------------------------
if [[ "$DO_OBSERVABILITY" == 1 ]]; then
  log "Installing observability (Prometheus / Loki / Grafana)"
  "${KUBECTL[@]}" apply -k "$DIST/observability"
fi

# --- Summary ----------------------------------------------------------------
log "Done. YASS installed in namespace '${NAMESPACE}'."
cat <<EOF

  kubectl ${KUBECONFIG_ARG:+--kubeconfig $KUBECONFIG_ARG} -n ${NAMESPACE} get pods
  kubectl ${KUBECONFIG_ARG:+--kubeconfig $KUBECONFIG_ARG} get crds | grep int.esa.yass

Grafana:    kubectl -n ${NAMESPACE} port-forward svc/grafana 3000:3000
Prometheus: kubectl -n ${NAMESPACE} port-forward svc/prometheus 9090:9090
EOF
