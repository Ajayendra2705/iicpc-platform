#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="iicpc"
CONFIG="infra/kind/cluster.yaml"

if ! command -v kind >/dev/null 2>&1; then
  echo "kind not installed. See https://kind.sigs.k8s.io/docs/user/quick-start/#installation" >&2
  exit 1
fi

if ! command -v kubectl >/dev/null 2>&1; then
  echo "kubectl not installed." >&2
  exit 1
fi

if kind get clusters | grep -q "^${CLUSTER_NAME}\$"; then
  echo "Cluster ${CLUSTER_NAME} already exists. Delete with: kind delete cluster --name ${CLUSTER_NAME}"
  exit 0
fi

echo "Creating kind cluster: ${CLUSTER_NAME}"
kind create cluster --name "${CLUSTER_NAME}" --config "${CONFIG}"

echo "Cluster ready. Nodes:"
kubectl get nodes -o wide

echo ""
echo "Note: gVisor (runsc) not auto-installed in kind. For local dev, contestant"
echo "pods will fall back to runc with strict seccomp. Production EKS uses gVisor."
