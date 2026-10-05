# Week 3 final experiment results

Final Azure experiment artifacts:

- `runs/` contains the archived per-run `meta.json` files and enabled-run
  `audit.jsonl` files.
- `week3-summary.json` is the `cmd/report -runs-dir runs` output.
- No smoke-run directories are included in `runs/`.

The frozen matrix was completed with 28 valid runs and 0 invalid runs.

## Recovery by arm

| Arm | Workload | Controller | Valid N | Recovered runs | Experiment recovery rate | Attributable recovery |
|---|---|---:|---:|---:|---:|---:|
| A1 | W1 transient crasher | enabled | 5 | 5 | 100% | W1: 0 pp |
| A2 | W1 transient crasher | disabled | 5 | 5 | 100% | baseline |
| B1 | W2 bad current revision | enabled | 5 | 5 | 100% | W2: +100 pp |
| B2 | W2 bad current revision | disabled | 5 | 0 | 0% | baseline |
| C1 | W3 bad current and previous | enabled | 5 | 0 | 0% | W3: 0 pp |
| C2 | W3 bad current and previous | disabled | 3 | 0 | 0% | baseline |

Interpretation:

- W1 recovered in both arms. In the enabled arm, every controller incident was
  abandoned after a rolled-back restart attempt, while the workload later
  self-recovered unaided. This is experiment-level recovery, not controller
  recovery.
- W2 recovered only with the controller enabled. The attributable recovery was
  +100 percentage points.
- W3 did not recover in either arm. Enabled W3 incidents exhausted the
  controller attempt budget; disabled W3 observations ended as
  `not_recovered_within_300s`.

## Controller incident outcomes

From `week3-summary.json`:

| Metric | Value |
|---|---:|
| Total controller incidents | 15 |
| Terminal incidents | 10 |
| Incomplete incidents | 5 |
| Abandoned incidents | 5 |
| Successful controller recoveries | 5 |
| Failed controller recoveries | 5 |
| Recovery success rate over terminal incidents | 50% |

Terminal outcome distribution:

| Outcome | Count |
|---|---:|
| `recovered` | 5 |
| `exhausted` | 5 |
| `escalated` | 0 |
| `rejected` | 0 |
| `rolled_back` | 0 |

`rolled_back` remains an attempt outcome, not an incident-terminal outcome.
The 5 W1 enabled incidents had rolled-back attempts and no `CLOSED` line, so
they are counted as abandoned controller incidents. The 5 W3 enabled incidents
ended with `CLOSED/result=exhausted`.

## Attempts and rollback

| Metric | Value |
|---|---:|
| Remediation attempts | 25 |
| Rolled-back attempts | 20 |
| Primary rollback rate, attempts | 80% |
| Secondary rollback rate, incidents | 50% |

Breakdown:

- W1 enabled: 5 attempts, 5 rolled back, 5 abandoned incidents.
- W2 enabled: 5 attempts, 0 rolled back, 5 recovered incidents.
- W3 enabled: 15 attempts, 15 rolled back, 5 exhausted incidents.
- Disabled arms have no controller audit, classifier call, or remediation
  attempt.

## Timing

All duration values are seconds, calculated by `cmd/report` from the archived
audit and run metadata.

| Metric | Value |
|---|---:|
| Average TTD | 14.2608834842 |
| Average TTM | 147.1448679232 |
| Average `t_classify` / inference latency | 0 |
| Average `t_apply` | 0.00898166232 |
| Average `t_verify` | 36.206473798880005 |
| Average action-to-verification convergence | 36.2154554612 |

The experiment audit data did not include non-zero `classifierMillis` values,
so the experiment summary reports average inference latency as 0. The separate
live Claude evaluation below is the source for live model latency and cost.

## Classifier and validator evaluation

These evaluations were completed before the final Azure run block and were not
rerun during this documentation pass.

| Evaluation | Result |
|---|---:|
| Validator adversarial rejection | 15/15 = 100% |
| Validator legitimate acceptance | 30/30 = 100% |
| Mock-vs-live Claude successful calls | 20/20 |
| Mock-vs-live agreement | 13/20 = 65% |
| Average live Claude latency | 5.474884915 s |
| Live model | `claude-sonnet-4-5-20250929` |
| Input tokens | 26,884 |
| Output tokens | 3,653 |
| Total tokens | 30,537 |
| Estimated total cost | $0.135447 |
| Average cost per call | $0.00677235 |

## Runtime observability verification

Manual Azure observability verification completed after the final experiment
run:

- The report server served `/metrics` on `:9091`.
- A Kubernetes pod successfully reached `172.16.0.4:9091/metrics`.
- Prometheus `ServiceMonitor` `selfheal-report-metrics` was created.
- Prometheus query `up{job="selfheal-report-metrics"}` returned `1`.
- Prometheus Targets UI showed `selfheal-report-metrics` as `1/1 UP`.
- Grafana dashboard `SAGE Week 3 Owner 3 Metrics` was imported.
- All five prepared dashboard panels rendered real experiment data.

No screenshot files are committed in this repository.

## Artifact verification

Verification performed from the repository artifacts:

- `runs/` contains exactly:
  `A1-01`..`A1-05`, `A2-01`..`A2-05`,
  `B1-01`..`B1-05`, `B2-01`..`B2-05`,
  `C1-01`..`C1-05`, and `C2-01`..`C2-03`.
- `runs/` contains no `A1-smoke` or `A2-smoke` directories.
- Every `runs/*/meta.json` parses successfully.
- Enabled runs have `audit.jsonl`; disabled runs do not.
- `cmd/report -runs-dir runs` resolves `experiment_recovery.valid_runs = 28`
  and `experiment_recovery.invalid_runs = 0`.
