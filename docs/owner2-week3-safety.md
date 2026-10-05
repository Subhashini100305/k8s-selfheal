# Owner 2 Week 3 safety instrumentation

## Durable audit sink

Deployed mode passes `--audit-path=/var/lib/sage/audit/audit.jsonl` to the
manager and mounts the `audit-data` PersistentVolumeClaim at that directory.
The writer opens the file with append semantics, serializes appends with a
mutex, emits one JSON object per line, and calls `fsync` after every event.

The PVC intentionally omits `storageClassName`, so the configured cluster
default is used. On the project K3s cluster this is the local-path provisioner.
The data therefore survives controller Pod restarts and replacements. It is
not a cross-node backup: loss of the node hosting the local-path volume can
lose the audit file. Raw files must still be copied into the experiment archive
after each run.

Local `make run` keeps `--audit-path` empty and writes JSONL to stdout.

## Frozen shared event schema

Every safety lifecycle transition contains:

```json
{
  "incidentID": "string",
  "attemptNumber": 1,
  "timestamp": "RFC3339 timestamp",
  "pod": "namespace/pod-name",
  "state": "DETECTED",
  "action": "restart_pod",
  "result": "received"
}
```

`incidentID` and `attemptNumber` come from Owner 1's `DetectionEvent`.
`pod` is written as `namespace/name`. Workload, arm, fault-injection time, and
external recovery observations live in the per-run `meta.json`, not on every
safety audit line.

The controller appends one incident-terminal line:

```json
{
  "incidentID": "string",
  "attemptNumber": 1,
  "timestamp": "RFC3339 timestamp",
  "pod": "namespace/pod-name",
  "state": "CLOSED",
  "action": "restart_pod",
  "result": "recovered",
  "classifierMillis": 125
}
```

`CLOSED/result` is authoritative for incident outcomes. `ROLLED_BACK` remains
attempt-level only.

Owner 3 adapts these camelCase transition events into its internal aggregate
metrics representation. Snake_case aggregate fields are deliberately not part
of the shared safety event.

## Timing evidence

Safety writes raw transition timestamps and does not serialize calculated
durations. In particular:

- `t_apply = VERIFYING.timestamp - REMEDIATING.timestamp`;
- verification begins at `VERIFYING` and finishes at `RECOVERED` or
  `ROLLING_BACK`;
- rollback begins at `ROLLING_BACK` and completes at `ROLLED_BACK`;
- TTD begins at `meta.injectedAt` and ends at `DETECTED.timestamp`;
- TTM begins at `DETECTED.timestamp` and ends at the incident's `CLOSED`
  timestamp.

Fault-injection and classifier timestamps originate outside safety and must be
joined by Owner 3's adapter. They are never fabricated from nearby safety
transitions.

## Exact replacement attribution

Immediately before executing the injected action, safety lists Pods through
Deployment -> ReplicaSet -> Pod controller ownership and captures their UIDs.
Verification rejects every captured UID. This supersedes the Week 2
second-truncated creation-time comparison and closes its same-second
attribution ambiguity.
