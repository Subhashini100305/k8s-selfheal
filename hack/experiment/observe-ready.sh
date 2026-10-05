#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
Usage:
  observe-ready.sh --meta runs/A2-01/meta.json --selector app=w1-transient [--namespace default]

Polls Pods matching the selector until either one has been Ready continuously
for 60 seconds or the disabled-arm 300 second cutoff expires. Updates meta.json
with recovered/recoveryTimestamp/recoverySource/observedUntil.
USAGE
}

meta=""
selector=""
namespace="default"
poll_seconds=5

while [[ $# -gt 0 ]]; do
  case "$1" in
    --meta) meta="${2:-}"; shift 2 ;;
    --selector) selector="${2:-}"; shift 2 ;;
    --namespace) namespace="${2:-}"; shift 2 ;;
    --poll-seconds) poll_seconds="${2:-}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) usage; exit 2 ;;
  esac
done

if [[ -z "$meta" || -z "$selector" ]]; then
  usage
  exit 2
fi

if [[ ! -f "$meta" ]]; then
  echo "meta file not found: $meta" >&2
  exit 2
fi

read_meta_field() {
  python3 - "$meta" "$1" <<'PY'
import json,sys
with open(sys.argv[1], encoding="utf-8") as f:
    data=json.load(f)
value=data.get(sys.argv[2], "")
print(value)
PY
}

injected_at="$(read_meta_field injectedAt)"
cutoff="$(read_meta_field observationCutoffSeconds)"
if [[ -z "$injected_at" || "$cutoff" != "300" ]]; then
  echo "disabled observation requires injectedAt and observationCutoffSeconds=300" >&2
  exit 2
fi

injected_epoch="$(date -u -d "$injected_at" +%s)"
cutoff_epoch=$((injected_epoch + 300))
ready_since=0

pod_ready() {
  kubectl get pods -n "$namespace" -l "$selector" -o json |
    python3 -c 'import json,sys
data=json.load(sys.stdin)
for pod in data.get("items", []):
    for condition in pod.get("status", {}).get("conditions", []):
        if condition.get("type") == "Ready" and condition.get("status") == "True":
            sys.exit(0)
sys.exit(1)'
}

update_meta() {
  local recovered="$1"
  local timestamp="$2"
  local source="$3"
  python3 - "$meta" "$recovered" "$timestamp" "$source" <<'PY'
import json,sys
path,recovered,timestamp,source=sys.argv[1:5]
with open(path, encoding="utf-8") as f:
    data=json.load(f)
data["recovered"] = recovered == "true"
if recovered == "true":
    data["recoveryTimestamp"] = timestamp
else:
    data["observedUntil"] = timestamp
data["recoverySource"] = source
tmp = path + ".tmp"
with open(tmp, "w", encoding="utf-8") as f:
    json.dump(data, f, separators=(",", ":"))
    f.write("\n")
import os
os.replace(tmp, path)
PY
}

while true; do
  now_epoch="$(date -u +%s)"
  now_iso="$(date -u +"%Y-%m-%dT%H:%M:%S.%NZ")"

  if pod_ready; then
    if [[ "$ready_since" -eq 0 ]]; then
      ready_since="$now_epoch"
    fi
    if [[ $((ready_since + 60)) -le "$now_epoch" && "$now_epoch" -le "$cutoff_epoch" ]]; then
      update_meta true "$now_iso" unaided
      exit 0
    fi
  else
    ready_since=0
  fi

  if [[ "$now_epoch" -ge "$cutoff_epoch" ]]; then
    update_meta false "$now_iso" not_recovered_within_300s
    exit 0
  fi

  sleep "$poll_seconds"
done
