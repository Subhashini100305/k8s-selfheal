#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
Usage:
  reset-audit.sh [--namespace k8s-selfheal-system]

Truncates /var/lib/sage/audit/audit.jsonl on the audit-data PVC using a short
BusyBox helper Pod. Scale the controller manager to zero before running this so
there is exactly one writer/ownership domain for the audit file.
USAGE
}

namespace="k8s-selfheal-system"
pvc="audit-data"
pod="audit-reset-$(date +%s)"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --namespace) namespace="${2:-}"; shift 2 ;;
    --pvc) pvc="${2:-}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) usage; exit 2 ;;
  esac
done

cleanup() {
  kubectl delete pod -n "$namespace" "$pod" --ignore-not-found=true >/dev/null 2>&1 || true
}
trap cleanup EXIT

kubectl run -n "$namespace" "$pod" \
  --image=busybox:1.36 \
  --restart=Never \
  --overrides='{"spec":{"volumes":[{"name":"audit","persistentVolumeClaim":{"claimName":"'"$pvc"'"}}],"containers":[{"name":"reset","image":"busybox:1.36","command":["sleep","3600"],"volumeMounts":[{"name":"audit","mountPath":"/audit"}]}]}}'

kubectl wait -n "$namespace" --for=condition=Ready "pod/$pod" --timeout=60s
kubectl exec -n "$namespace" "$pod" -- sh -c ': > /audit/audit.jsonl'
