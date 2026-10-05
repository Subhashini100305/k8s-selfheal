# Integrated run — demo scenarios

Four workloads that drive the full pipeline end to end against a real cluster:
detect → collect evidence → classify → gate → act → verify → recover or roll
back → audit.

Each one exercises a different terminal outcome. Run them one at a time — the
in-flight guard is per Deployment, not global, so several at once will
interleave their logs and make the run hard to read.

| Manifest | Action taken | Expected outcome |
|---|---|---|
| `transient-recovers.yaml` | `restart_pod` | `recovered` |
| `transient-unrecoverable.yaml` | `restart_pod` | `rolled_back` |
| `rollout-fixable.yaml` | `rollout_undo` | `recovered` |
| `rollout-unrecoverable.yaml` | `rollout_undo` | `rolled_back` |

## Prerequisites

```bash
# Docker must be running, and a kind cluster must exist.
kind create cluster --name selfheal      # only if you don't already have one
kubectl config use-context kind-selfheal
```

Run the controller from your host in one terminal, and drive the scenarios
from another:

```bash
make run
```

`make run` uses your kubeconfig credentials, so the manager's RBAC does not
apply. Anything RBAC-related (`pods/log`, reading Events) only gets exercised
by `make docker-build deploy`, which is worth doing once before claiming the
generated `config/rbac/role.yaml` is correct.

Leave `CLASSIFIER_PROVIDER` unset. It defaults to the mock classifier, which is
deterministic and makes no network calls; the mock reaches the same decisions
as a real LLM on these four workloads, and costs nothing to re-run. Set it to
`anthropic` or `mistral` only when the point of the run is the LLM itself.

## What "working" looks like in the logs

```
DETECTED CrashLoopBackOff
Collected incident evidence   logBytes=54 eventCount=10     <- non-zero: evidence collection is live
remediation finished          action=restart_pod result=recovered mttr=1m5.12184925s
```

Those numbers are from an actual `transient-recovers` run, not an illustration.
`logBytes` is the one `connection refused` line; `eventCount` grows with each
incident on the same Deployment and is capped at 25.

`logBytes=0 eventCount=0` followed by `ESCALATED — not safe for automation`
means evidence collection is not working — check the `Evidence` wiring in
`cmd/main.go`, and RBAC if running deployed rather than via `make run`.

Timing tells you which verifier phase ended the run. The constants are in
`internal/safety/verifier.go`: a 30s readiness timeout, then a 60s stability
window polled every 5s.

| MTTR | Phase | `ROLLING_BACK` reason |
|---|---|---|
| ~65s | Passed both | — (`recovered`) |
| ~30.0s | Never became Ready | `pod did not become Ready before initial readiness timeout` |
| 5–60s | Came up Ready, then died | `pod lost Ready during stability window` |

The third row is the one that is easy to forget exists. A container that runs
for a second before exiting reaches Ready long enough to satisfy the readiness
phase, so the run gets *past* the timeout and then fails partway through the
stability window at whatever poll catches it. `transient-unrecoverable`
produces both of the bottom two rows depending on where in its own crash
back-off the replacement Pod happens to be when the verifier looks — so an
MTTR that is neither ~30s nor ~65s is not a bug.

An MTTR of near-exactly 30.0s on a workload you expected to recover means the
verifier never found a Ready replacement Pod, which is not the same as the
workload failing to come up. Check whether the replacement is Running before
concluding the remediation was wrong.

---

## 1. transient-recovers — `restart_pod` → `recovered`

```bash
# The node remembers the first pod that ever ran; reset it or the demo
# starts healthy and proves nothing. Node name comes from `kubectl get nodes`.
docker exec selfheal-control-plane rm -rf /tmp/k8s-selfheal

kubectl apply -f hack/manifests/transient-recovers.yaml
kubectl get pods -l app=transient-recovers-demo -w
```

The first pod logs `connection refused` and crash-loops. The classifier reads
that log, proposes `transient_failure` → `restart_pod`, the guard allows it,
the pod is deleted, and the replacement comes up healthy and stays Ready for
the full stability window.

```bash
kubectl delete -f hack/manifests/transient-recovers.yaml
```

## 2. transient-unrecoverable — `restart_pod` → `rolled_back`

```bash
kubectl apply -f hack/manifests/transient-unrecoverable.yaml
```

Same evidence, same automatable decision — but every replacement crashes too,
so the verifier fails and the service rolls back. The snapshot restore is a
no-op here (`restart_pod` never changed the Deployment spec), which is the
point: the outcome must still be `rolled_back`, not silently treated as
success because the spec already matched.

This scenario reaches the rollback through *either* verifier exit, depending on
timing — the container stays up for about a second, so a replacement caught
early is Ready when the verifier first looks and then dies inside the stability
window, while one caught mid-back-off never becomes Ready at all. Both are
correct; see the MTTR table above.

```bash
kubectl delete -f hack/manifests/transient-unrecoverable.yaml
```

## 3. rollout-fixable — `rollout_undo` → `recovered`

This one needs two revisions: a good one to roll back *to*, then a bad one.

The `sed` runs against the *file*, not against `kubectl -o yaml` output:
`-o yaml` re-renders the command as an unquoted YAML list, so a pattern with
quotes in it silently fails to match and you end up applying the broken
revision as revision 1 — after which the patch below is a no-op and no rollout
ever happens.

```bash
# Revision 1 — known good.
sed 's/"sleep 1; exit 1"/"sleep 3600"/' hack/manifests/rollout-fixable.yaml \
  | kubectl apply -f -
kubectl rollout status deployment/rollout-fixable-demo

# Revision 2 — the bad deploy.
kubectl patch deployment rollout-fixable-demo --type=json -p \
  '[{"op":"replace","path":"/spec/template/spec/containers/0/command","value":["sh","-c","sleep 1; exit 1"]}]'
kubectl get pods -l app=rollout-fixable-demo -w
```

There are no logs at all here — the container dies without printing anything.
Classification runs purely on Events: a `BackOff` on the pod plus a recent
`ScalingReplicaSet` on the Deployment is what identifies this as `bad_deploy`,
which is why the collector queries the Deployment and ReplicaSet and not just
the pod. `rollout_undo` reverts the template to revision 1 and the replacement
pod stays up.

```bash
kubectl delete deployment rollout-fixable-demo
```

## 4. rollout-unrecoverable — `rollout_undo` → `rolled_back`

```bash
# Revision 1 — already broken, on purpose. sed the file, not -o yaml output
# (see scenario 3 above for why).
sed 's/"echo bad-2; exit 1"/"echo bad-1; exit 1"/' hack/manifests/rollout-unrecoverable.yaml \
  | kubectl apply -f -

# Wait for CrashLoopBackOff, then roll out an equally broken revision 2.
kubectl patch deployment rollout-unrecoverable-demo --type=json -p \
  '[{"op":"replace","path":"/spec/template/spec/containers/0/command","value":["sh","-c","echo bad-2; exit 1"]}]'
```

`rollout_undo` reverts to revision 1, which is also broken, so verification
fails and the snapshot restore puts revision 2's spec back. This is the only
scenario where the rollback genuinely changes the cluster, so it is the one
that proves snapshot/restore works against a live API server.

```bash
kubectl delete deployment rollout-unrecoverable-demo
```

## Reading the audit trail

Audit entries are JSONL on the manager's stdout — the same terminal as
`make run`. One line per state transition:

```bash
make run 2>&1 | tee /tmp/selfheal-run.jsonl
grep -E '"state":' /tmp/selfheal-run.jsonl
```

A complete recovered run walks `DETECTED → SNAPSHOTTED → REMEDIATING →
VERIFYING → RECOVERED → LOGGED`; a rolled-back one substitutes `ROLLING_BACK →
ROLLED_BACK → LOGGED` for the last two.

## Cleanup

```bash
kubectl delete deployment -l 'app in (transient-recovers-demo,transient-unrecoverable-demo,rollout-fixable-demo,rollout-unrecoverable-demo)'
docker exec selfheal-control-plane rm -rf /tmp/k8s-selfheal
```

---

# The three named experiment workloads (Week 3)

The four scenarios above are the Week 2 *demo* set — they prove each terminal
outcome individually. The experiment runs on a different, frozen set of
**three** workloads, chosen so that every contribution claim has something to
measure. IDs are stable and appear in each run's `meta.json` `workload` field;
do not rename them.

| ID | Manifest | What it is | Without controller | With controller |
|---|---|---|---|---|
| **W1** | `w1-transient.yaml` | Transient crasher, self-recovers after 3 failed starts | **Recovers unaided** in ~36s (measured) | `restart_pod` resets the counter → attempt `rolled_back`; the replacement then self-heals, so the incident is usually *abandoned* rather than reaching `exhausted` |
| **W2** | `rollout-fixable.yaml` | Bad current revision, good previous | Never recovers | `rollout_undo` → `recovered` |
| **W3** | `rollout-unrecoverable.yaml` | Bad current **and** previous revision | Never recovers | Fix applied → verification fails → `rolled_back` ×3 → `exhausted` |

W1 is new. W2 and W3 are the two rollout scenarios above, under experiment IDs.
The two `transient-*` manifests stay as demo material and are **not** part of
the experiment set.

## Why W1 exists

W2 and W3 are both built so Kubernetes cannot self-heal them. Run with the
controller disabled, both return 0/5 — so "recovery net of a null-action
baseline" compares against zero and says nothing. W1 is the one workload whose
control arm is nonzero, which is the only reason that contribution produces a
finding at all.

## The W1 finding — expect this, do not fix it

W1 counts its starts in a file on an `emptyDir` volume. `emptyDir` lives for
the lifetime of the **pod**: a container restart inside the same pod keeps it,
so the counter advances across the kubelet's own back-off (10s, 20s, 40s) and
the workload comes up healthy on its own after roughly 70 seconds.

`restart_pod` deletes the pod. The replacement gets a brand-new, empty
`emptyDir`, so its counter starts again at 1 and it crash-loops three more
times from scratch. Two consequences:

1. **The controller makes W1 slower to recover than doing nothing.**
2. The replacement cannot reach Ready inside the 30s readiness timeout, so
   verification fails and the attempt is recorded `rolled_back`.

So arm A1 (enabled) should look *worse* than A2 (disabled), and attributable
recovery for W1 should come out **negative**.

### Measured, deployed mode, one run each

| Arm | Observed |
|---|---|
| Control (controller off) | Ready after **36s**, 3 restarts, reached `start 4` unaided |
| Enabled | Detected at +14s, `restart_pod`, attempt 1 → `rolled_back` at +44s (*pod did not become Ready before initial readiness timeout*). The replacement then self-healed at about +62s, so no second attempt was ever triggered. |

**The incident never reached a terminal outcome.** One attempt was recorded,
it rolled back, and then the workload healed itself — so the controller simply
stopped seeing a crash loop and the incident was abandoned mid-flight with
`terminalOutcome` unset. The attempt-level record is complete and correct; the
incident-level one is missing. See the note in
`docs/measurement-definitions.md` on how to count these.

Note also that 36s unaided against a 30s readiness timeout is a ~6s margin, so
W1 sits close to the decision boundary. Do not assume all five runs of A1 will
look the same — report the spread. That is a real result about the
limits of a blunt remediation action, it is why the paired control arm exists
at all, and it belongs in the report as a finding. Record it; do not retune the
workload to make the controller look better.

## Per-arm setup and teardown

Each run is one fault injection. Record the injection timestamp — it is the
start of TTD.

### W1 — both arms

```bash
# inject
kubectl apply -f hack/manifests/w1-transient.yaml
# observe until terminal, then
kubectl delete -f hack/manifests/w1-transient.yaml
```

No node-level state to reset: the counter lives on the pod's `emptyDir` and
dies with the Deployment. W1 is the only workload that is genuinely
repeatable with a single `apply`.

### W2 — both arms

Needs two revisions: a known-good one to roll back *to*, then a bad one.

```bash
# revision 1 — known good. sed the FILE, not `kubectl -o yaml` output:
# -o yaml re-renders the command as an unquoted YAML list, so a quoted
# pattern silently fails to match and revision 1 gets the broken command.
sed 's/"sleep 1; exit 1"/"sleep 3600"/' hack/manifests/rollout-fixable.yaml \
  | kubectl apply -f -
kubectl rollout status deployment/rollout-fixable-demo

# revision 2 — the fault injection. Timestamp this.
kubectl patch deployment rollout-fixable-demo --type=json -p \
  '[{"op":"replace","path":"/spec/template/spec/containers/0/command","value":["sh","-c","sleep 1; exit 1"]}]'

# observe until terminal, then
kubectl delete deployment rollout-fixable-demo
```

### W3 — both arms

```bash
# revision 1 — already broken, on purpose
sed 's/"echo bad-2; exit 1"/"echo bad-1; exit 1"/' hack/manifests/rollout-unrecoverable.yaml \
  | kubectl apply -f -

# wait for CrashLoopBackOff, then inject an equally broken revision 2
kubectl patch deployment rollout-unrecoverable-demo --type=json -p \
  '[{"op":"replace","path":"/spec/template/spec/containers/0/command","value":["sh","-c","echo bad-2; exit 1"]}]'

# observe until exhausted, then
kubectl delete deployment rollout-unrecoverable-demo
```

**Keep each workload to exactly two revisions.** `revisionHistoryLimit`
defaults to 10, so more rollouts than that garbage-collect the known-good
ReplicaSet `rollout_undo` needs as a target.

## Running the disabled arm

The control arm is the same injection with the controller not running. Stop it
rather than reconfiguring it, so there is no question of a partially-active
controller:

```bash
# deployed mode
kubectl scale -n k8s-selfheal-system deployment/k8s-selfheal-controller-manager --replicas=0
# ... run the injection, observe, teardown ...
kubectl scale -n k8s-selfheal-system deployment/k8s-selfheal-controller-manager --replicas=1
```

Disabled-arm runs produce no audit lines, so recovery has to be observed from
the cluster itself — watch for the pod reaching Ready and staying Ready for
60s, applying the same recovery definition by hand so the two arms are
comparable:

```bash
kubectl get pods -l app=w1-transient -w
```

## What "terminal" looks like per workload

Watch the manager log. Each incident ends exactly once.

```
recovered   → "remediation finished ... result=recovered"
rolled_back → "remediation finished ... result=rolled_back"   (may repeat up to 3×)
exhausted   → "EXHAUSTED — attempt budget spent, going quiet until the deployment changes"
escalated   → "ESCALATED — not safe for automation"
```

After `exhausted`, the controller goes **silent** on that Deployment until its
`metadata.generation` changes. If you see further activity on it, the attempt
budget is not working — that is the single most important thing to confirm
before the run block.
