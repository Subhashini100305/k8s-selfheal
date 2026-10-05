# Week-3 experiment runbook

This runbook prepares the 28-run Owner-3 archive without changing the frozen
measurement definitions. Do not run live classifier providers unless the run
plan explicitly calls for them.

## Archive shape

Each repetition is archived as:

```text
runs/<run-id>/
  meta.json
  audit.jsonl
```

Disabled-arm runs have `meta.json` only. Enabled-arm runs must have both files.

## Deploy and health check

```bash
IMG=<registry>/k8s-selfheal:<commit> make docker-build docker-push deploy
kubectl get pods -n k8s-selfheal-system
kubectl rollout status -n k8s-selfheal-system deployment/k8s-selfheal-controller-manager
kubectl get servicemonitor -n k8s-selfheal-system
```

The manager writes durable audit JSONL to
`/var/lib/sage/audit/audit.jsonl` on the `audit-data` PVC.

## Prometheus and Grafana

The Kubernetes controller exposes controller-runtime metrics through
`controller-manager-metrics-service`. The final Week-3 experiment metrics are
offline/report metrics served by:

```bash
go run ./cmd/report -runs-dir runs -serve -listen :9091
```

Point Prometheus at that local endpoint for the Week-3 report dashboard, then
load `config/grafana/dashboards/week3-owner3.json`.

Checklist:

- Grafana opens.
- Prometheus datasource is healthy.
- Controller metrics target is UP.
- `cmd/report -serve` target is UP for final experiment metrics.
- At least one incident is reflected in outcome/remediation panels.
- Classifier latency appears after CLOSED lines with `classifierMillis`.
- No dashboard panel has a nonexistent PromQL metric name.

## One enabled smoke

```bash
mkdir -p runs/A1-smoke
kubectl scale -n k8s-selfheal-system deployment/k8s-selfheal-controller-manager --replicas=0
hack/experiment/reset-audit.sh
kubectl scale -n k8s-selfheal-system deployment/k8s-selfheal-controller-manager --replicas=1
kubectl rollout status -n k8s-selfheal-system deployment/k8s-selfheal-controller-manager

hack/experiment/record-injection.sh \
  --output runs/A1-smoke/meta.json \
  --run-id A1-smoke \
  --workload W1 \
  --arm enabled \
  -- kubectl apply -f hack/manifests/w1-transient.yaml

# After the run reaches its expected lifecycle state, export the PVC audit file.
hack/experiment/export-audit.sh --output runs/A1-smoke/audit.jsonl
kubectl delete -f hack/manifests/w1-transient.yaml --ignore-not-found=true
go run ./cmd/report -run-dir runs/A1-smoke
```

## One disabled smoke

```bash
mkdir -p runs/A2-smoke
kubectl scale -n k8s-selfheal-system deployment/k8s-selfheal-controller-manager --replicas=0
kubectl rollout status -n k8s-selfheal-system deployment/k8s-selfheal-controller-manager

hack/experiment/record-injection.sh \
  --output runs/A2-smoke/meta.json \
  --run-id A2-smoke \
  --workload W1 \
  --arm disabled \
  -- kubectl apply -f hack/manifests/w1-transient.yaml

hack/experiment/observe-ready.sh \
  --meta runs/A2-smoke/meta.json \
  --selector app=w1-transient

kubectl delete -f hack/manifests/w1-transient.yaml --ignore-not-found=true
kubectl scale -n k8s-selfheal-system deployment/k8s-selfheal-controller-manager --replicas=1
go run ./cmd/report -run-dir runs/A2-smoke
```

## Frozen matrix

Use `hack/experiment/week3-matrix.json` as the run list:

- A1: W1 enabled, 5 valid runs
- A2: W1 disabled, 5 valid runs
- B1: W2 enabled, 5 valid runs
- B2: W2 disabled, 5 valid runs
- C1: W3 enabled, 5 valid runs
- C2: W3 disabled, 3 valid runs

Failed setup or failed injection leaves `injectionSucceeded=false` or
`setupSucceeded=false` in `meta.json`. That run is invalid and excluded from
N; repeat with the next deterministic suffix rather than silently retrying
inside the same archive directory.

## Workload commands

W1 injection:

```bash
kubectl apply -f hack/manifests/w1-transient.yaml
```

W2 setup and injection:

```bash
sed 's/"sleep 1; exit 1"/"sleep 3600"/' hack/manifests/rollout-fixable.yaml | kubectl apply -f -
kubectl rollout status deployment/rollout-fixable-demo
kubectl patch deployment rollout-fixable-demo --type=json -p \
  '[{"op":"replace","path":"/spec/template/spec/containers/0/command","value":["sh","-c","sleep 1; exit 1"]}]'
```

W3 setup and injection:

```bash
sed 's/"echo bad-2; exit 1"/"echo bad-1; exit 1"/' hack/manifests/rollout-unrecoverable.yaml | kubectl apply -f -
kubectl patch deployment rollout-unrecoverable-demo --type=json -p \
  '[{"op":"replace","path":"/spec/template/spec/containers/0/command","value":["sh","-c","echo bad-2; exit 1"]}]'
```

Cleanup:

```bash
kubectl delete -f hack/manifests/w1-transient.yaml --ignore-not-found=true
kubectl delete deployment rollout-fixable-demo rollout-unrecoverable-demo --ignore-not-found=true
```

## Final aggregation

```bash
go run ./cmd/report -runs-dir runs > week3-summary.json
go run ./cmd/report -runs-dir runs -serve -listen :9091
```

Verify `experiment_recovery.valid_runs == 28` and
`experiment_recovery.invalid_runs == 0` before using the results.
