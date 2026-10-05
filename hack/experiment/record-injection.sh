#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
Usage:
  record-injection.sh --output PATH --run-id ID --workload W1|W2|W3 --arm enabled|disabled -- kubectl ...

Writes Owner-3 run metadata JSON and executes the fault-injection command in
the same process. For disabled runs, later observation updates the same
meta.json with recovery/not_recovered_within_300s.
USAGE
}

json_string() {
  python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))'
}

output=""
run_id=""
workload=""
arm=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --output)
      output="${2:-}"
      shift 2
      ;;
    --run-id)
      run_id="${2:-}"
      shift 2
      ;;
    --workload)
      workload="${2:-}"
      shift 2
      ;;
    --arm)
      arm="${2:-}"
      shift 2
      ;;
    --)
      shift
      break
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage
      exit 2
      ;;
  esac
done

if [[ -z "$output" || -z "$run_id" || -z "$workload" || -z "$arm" || $# -eq 0 ]]; then
  usage
  exit 2
fi

case "$workload" in
  W1|W2|W3) ;;
  *) echo "invalid workload: $workload" >&2; exit 2 ;;
esac

case "$arm" in
  enabled|disabled) ;;
  *) echo "invalid arm: $arm" >&2; exit 2 ;;
esac

mkdir -p "$(dirname "$output")"
fault_injection_time="$(date -u +"%Y-%m-%dT%H:%M:%S.%NZ")"

write_meta() {
  local succeeded="$1"
  local exit_code="$2"
  local error="$3"
  local cutoff=""
  if [[ "$arm" == "disabled" ]]; then
    cutoff=',"observationCutoffSeconds":300'
  fi
  printf '{"runID":%s,"workload":%s,"arm":%s,"injectedAt":%s%s,"injectionSucceeded":%s,"injectionExitCode":%d' \
    "$(printf '%s' "$run_id" | json_string)" \
    "$(printf '%s' "$workload" | json_string)" \
    "$(printf '%s' "$arm" | json_string)" \
    "$(printf '%s' "$fault_injection_time" | json_string)" \
    "$cutoff" \
    "$succeeded" \
    "$exit_code" \
    > "$output.tmp"
  if [[ -n "$error" ]]; then
    printf ',"injectionError":%s' "$(printf '%s' "$error" | json_string)" >> "$output.tmp"
  fi
  printf '}\n' >> "$output.tmp"
  mv "$output.tmp" "$output"
}

write_meta false 0 "injection command not yet completed"

set +e
"$@"
exit_code=$?
set -e

if [[ $exit_code -eq 0 ]]; then
  write_meta true 0 ""
else
  write_meta false "$exit_code" "fault injection command failed"
fi

exit "$exit_code"
