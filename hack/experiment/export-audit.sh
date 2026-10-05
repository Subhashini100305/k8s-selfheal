#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
Usage:
  export-audit.sh --output runs/A1-01/audit.jsonl [--namespace k8s-selfheal-system]

Exports /var/lib/sage/audit/audit.jsonl from the audit-data PVC using a short
BusyBox helper Pod. This avoids relying on shell tools inside the distroless
manager container.
USAGE
}

namespace="k8s-selfheal-system"
output=""
pvc="audit-data"
pod="audit-export-$(date +%s)"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --output) output="${2:-}"; shift 2 ;;
    --namespace) namespace="${2:-}"; shift 2 ;;
    --pvc) pvc="${2:-}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) usage; exit 2 ;;
  esac
done

if [[ -z "$output" ]]; then
  usage
  exit 2
fi

cleanup() {
  kubectl delete pod -n "$namespace" "$pod" --ignore-not-found=true >/dev/null 2>&1 || true
}
trap cleanup EXIT

kubectl run -n "$namespace" "$pod" \
  --image=busybox:1.36 \
  --restart=Never \
  --overrides='{"spec":{"volumes":[{"name":"audit","persistentVolumeClaim":{"claimName":"'"$pvc"'"}}],"containers":[{"name":"export","image":"busybox:1.36","command":["sleep","3600"],"volumeMounts":[{"name":"audit","mountPath":"/audit"}]}]}}'

kubectl wait -n "$namespace" --for=condition=Ready "pod/$pod" --timeout=60s
mkdir -p "$(dirname "$output")"
kubectl exec -n "$namespace" "$pod" -- cat /audit/audit.jsonl > "$output"
