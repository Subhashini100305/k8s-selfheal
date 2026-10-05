#!/usr/bin/env python3

import argparse
import json
from collections import Counter, defaultdict
from datetime import datetime
from pathlib import Path
from statistics import mean

INPUT_USD_PER_M = 3.0
OUTPUT_USD_PER_M = 15.0
TERMINAL = {"recovered", "exhausted", "escalated", "rejected"}


def fail(msg):
    raise SystemExit("ERROR: " + msg)


def load_json(path):
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except Exception as e:
        fail(f"{path}: {e}")


def load_jsonl(path):
    if not path.exists():
        return []
    out = []
    for n, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
        if not line.strip():
            continue
        try:
            out.append(json.loads(line))
        except Exception as e:
            fail(f"{path}:{n}: {e}")
    return out


def ts(value):
    if not value:
        fail("missing timestamp")
    return datetime.fromisoformat(value.replace("Z", "+00:00"))


def seconds(a, b):
    return (ts(b) - ts(a)).total_seconds()


def avg(values):
    return mean(values) if values else None


def cost(inp, out):
    return inp * INPUT_USD_PER_M / 1_000_000 + out * OUTPUT_USD_PER_M / 1_000_000


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--runs-dir", default="sage-final-runs")
    ap.add_argument("--matrix", default="hack/experiment/week3-matrix.json")
    ap.add_argument("--json-out", default="week3-live-final-results.json")
    ap.add_argument("--md-out", default="docs/week3-live-final-results.md")
    args = ap.parse_args()

    root = Path(args.runs_dir)
    matrix = load_json(Path(args.matrix))

    if len(matrix) != 28:
        fail(f"matrix must contain 28 runs, found {len(matrix)}")

    matrix_ids = [x["runID"] for x in matrix]
    if len(set(matrix_ids)) != 28:
        fail("duplicate runID in matrix")

    actual = sorted(x.name for x in root.iterdir() if x.is_dir())
    expected = sorted(matrix_ids)

    if actual != expected:
        fail(
            "run directories do not exactly match matrix\n"
            f"missing={sorted(set(expected)-set(actual))}\n"
            f"extra={sorted(set(actual)-set(expected))}"
        )

    run_rows = []
    all_audit = []
    classifier_calls = []

    attempt_count = 0
    rolled_back_attempts = 0
    actionable_incidents = set()
    incidents_with_rollback = set()
    abandoned_incidents = set()
    terminal_outcomes = Counter()

    ttd = []
    ttm = []
    classify_times = []
    apply_times = []
    verify_times = []

    for spec in matrix:
        rid = spec["runID"]
        workload = spec["workload"]
        arm = spec["arm"]
        d = root / rid

        meta = load_json(d / "meta.json")

        if meta.get("runID") != rid:
            fail(f"{rid}: runID mismatch")
        if meta.get("workload") != workload:
            fail(f"{rid}: workload mismatch")
        if meta.get("arm") != arm:
            fail(f"{rid}: arm mismatch")
        if meta.get("injectionSucceeded") is not True:
            fail(f"{rid}: injection did not succeed")
        if meta.get("injectionExitCode") != 0:
            fail(f"{rid}: injectionExitCode != 0")

        audit = load_jsonl(d / "audit.jsonl")
        all_audit.extend(audit)

        for row in audit:
            row["_runID"] = rid

        # ----------------------------
        # Experiment recovery outcome
        # ----------------------------
        recovered = None
        source = meta.get("recoverySource")

        if isinstance(meta.get("recovered"), bool):
            recovered = meta["recovered"]
        elif arm == "enabled":
            closed = [x for x in audit if x.get("state") == "CLOSED"]
            if len(closed) != 1:
                fail(f"{rid}: expected exactly one CLOSED record")
            result = closed[0].get("result")
            if result not in TERMINAL:
                fail(f"{rid}: invalid CLOSED result {result!r}")
            recovered = result == "recovered"
            source = f"closed:{result}"
        else:
            observed = d / "observed-result.txt"
            if not observed.exists():
                fail(f"{rid}: disabled run missing observed-result.txt")
            value = observed.read_text(encoding="utf-8").strip()
            if value == "not_recovered_within_300s":
                recovered = False
                source = value
            elif value == "recovered":
                recovered = True
                source = value
            else:
                fail(f"{rid}: unknown observed result {value!r}")

        # ----------------------------
        # Incident/attempt accounting
        # ----------------------------
        incident_ids = {
            x.get("incidentID")
            for x in audit
            if x.get("incidentID")
        }

        actionable_incidents.update(incident_ids)

        remediating = [
            x for x in audit
            if x.get("state") == "REMEDIATING"
        ]
        attempt_count += len(remediating)

        rolled = [
            x for x in audit
            if x.get("state") == "ROLLED_BACK"
        ]
        rolled_back_attempts += len(rolled)
        incidents_with_rollback.update(
            x["incidentID"] for x in rolled if x.get("incidentID")
        )

        closed = [x for x in audit if x.get("state") == "CLOSED"]

        for x in closed:
            result = x.get("result")
            if result not in TERMINAL:
                fail(f"{rid}: invalid terminal result {result!r}")
            terminal_outcomes[result] += 1

        for iid in incident_ids:
            rows = [x for x in audit if x.get("incidentID") == iid]
            had_attempt = any(x.get("state") == "REMEDIATING" for x in rows)
            had_closed = any(x.get("state") == "CLOSED" for x in rows)
            if had_attempt and not had_closed:
                abandoned_incidents.add(iid)

        # ----------------------------
        # Timing
        # ----------------------------
        injected = meta.get("injectedAt")

        for iid in incident_ids:
            rows = sorted(
                [x for x in audit if x.get("incidentID") == iid],
                key=lambda x: ts(x["timestamp"])
            )

            detected = next(
                (x for x in rows if x.get("state") == "DETECTED"),
                None
            )

            if detected and injected:
                ttd.append(seconds(injected, detected["timestamp"]))

            closed_row = next(
                (x for x in rows if x.get("state") == "CLOSED"),
                None
            )

            if detected and closed_row:
                ttm.append(
                    seconds(detected["timestamp"], closed_row["timestamp"])
                )

        # attempt-level apply/verify
        attempts = defaultdict(list)
        for x in audit:
            iid = x.get("incidentID")
            num = x.get("attemptNumber")
            if iid and isinstance(num, int) and num > 0:
                attempts[(iid, num)].append(x)

        for rows in attempts.values():
            rem = next(
                (x for x in rows if x.get("state") == "REMEDIATING"),
                None
            )
            ver = next(
                (x for x in rows if x.get("state") == "VERIFYING"),
                None
            )

            if rem and ver:
                apply_times.append(
                    seconds(rem["timestamp"], ver["timestamp"])
                )

            if ver:
                end = next(
                    (
                        x for x in rows
                        if x.get("state") in {"RECOVERED", "ROLLING_BACK"}
                    ),
                    None
                )
                if end:
                    verify_times.append(
                        seconds(ver["timestamp"], end["timestamp"])
                    )

        # ----------------------------
        # Classifier accounting
        # ----------------------------
        call_file = d / "classifier-calls.jsonl"
        calls = load_jsonl(call_file)

        # New A1 recorder contains independent per-call records.
        if calls:
            for c in calls:
                classifier_calls.append({
                    "runID": rid,
                    "provider": c.get("classifierProvider"),
                    "model": c.get("classifierModel"),
                    "millis": c.get("classifierMillis"),
                    "inputTokens": c.get("inputTokens"),
                    "outputTokens": c.get("outputTokens"),
                    "totalTokens": c.get("totalTokens"),
                    "costUSD": c.get("estimatedCostUSD")
                        if c.get("costKnown") is True
                        else cost(c.get("inputTokens", 0), c.get("outputTokens", 0)),
                    "costSource": "recorded"
                        if c.get("costKnown") is True
                        else "backfilled_from_tokens",
                })
        else:
            # Older B/C live runs persist classifier metadata on CLOSED.
            for c in closed:
                inp = c.get("inputTokens")
                out = c.get("outputTokens")
                millis = c.get("classifierMillis")

                if inp is None and out is None and millis is None:
                    continue

                if not isinstance(inp, int) or inp <= 0:
                    fail(f"{rid}: invalid classifier inputTokens")
                if not isinstance(out, int) or out <= 0:
                    fail(f"{rid}: invalid classifier outputTokens")
                if not isinstance(millis, int) or millis <= 0:
                    fail(f"{rid}: invalid classifierMillis")

                classifier_calls.append({
                    "runID": rid,
                    "provider": c.get("classifierProvider"),
                    "model": c.get("classifierModel"),
                    "millis": millis,
                    "inputTokens": inp,
                    "outputTokens": out,
                    "totalTokens": c.get("totalTokens"),
                    "costUSD": (
                        c.get("estimatedCostUSD")
                        if c.get("costKnown") is True
                        else cost(inp, out)
                    ),
                    "costSource": (
                        "recorded"
                        if c.get("costKnown") is True
                        else "backfilled_from_tokens"
                    ),
                })

        # classification component available from persisted live call metadata
        if calls:
            classify_times.extend(
                c["classifierMillis"] / 1000.0
                for c in calls
                if isinstance(c.get("classifierMillis"), int)
            )
        else:
            classify_times.extend(
                c["classifierMillis"] / 1000.0
                for c in closed
                if isinstance(c.get("classifierMillis"), int)
            )

        run_rows.append({
            "runID": rid,
            "workload": workload,
            "arm": arm,
            "recovered": recovered,
            "recoverySource": source,
        })

    # ----------------------------
    # Experiment recovery matrix
    # ----------------------------
    recovery = {}

    for workload in ("W1", "W2", "W3"):
        recovery[workload] = {}

        for arm in ("enabled", "disabled"):
            rows = [
                x for x in run_rows
                if x["workload"] == workload and x["arm"] == arm
            ]

            if not rows:
                fail(f"{workload}/{arm}: no runs")

            successes = sum(1 for x in rows if x["recovered"])
            total = len(rows)

            recovery[workload][arm] = {
                "recovered": successes,
                "total": total,
                "ratePercent": successes / total * 100.0,
            }

        recovery[workload]["attributableRecoveryPercentagePoints"] = (
            recovery[workload]["enabled"]["ratePercent"]
            - recovery[workload]["disabled"]["ratePercent"]
        )

    terminal_total = sum(terminal_outcomes.values())

    rollback_attempt_rate = (
        rolled_back_attempts / attempt_count * 100.0
        if attempt_count else 0.0
    )

    # Incident-level rollback denominator is actionable terminal incidents only.
    # Abandoned incidents have no terminal outcome and are excluded.
    actionable_terminal_incidents = (
        terminal_outcomes["recovered"]
        + terminal_outcomes["exhausted"]
    )

    terminal_incidents_with_rollback = 0
    for iid in incidents_with_rollback:
        incident_rows = [
            x for x in all_audit
            if x.get("incidentID") == iid
        ]
        if any(x.get("state") == "CLOSED" for x in incident_rows):
            terminal_incidents_with_rollback += 1

    rollback_incident_rate = (
        terminal_incidents_with_rollback
        / actionable_terminal_incidents
        * 100.0
        if actionable_terminal_incidents else 0.0
    )

    # Five frozen terminal outcome buckets.
    outcome_counts = {
        "recovered": terminal_outcomes["recovered"],
        "exhausted": terminal_outcomes["exhausted"],
        "escalated": terminal_outcomes["escalated"],
        "rejected": terminal_outcomes["rejected"],
        "abandoned": len(abandoned_incidents),
    }

    classifier_summary = {
        "persistedCallRecords": len(classifier_calls),
        "averageLatencySeconds": avg([
            x["millis"] / 1000.0 for x in classifier_calls
        ]),
        "totalInputTokens": sum(x["inputTokens"] for x in classifier_calls),
        "totalOutputTokens": sum(x["outputTokens"] for x in classifier_calls),
        "totalTokens": sum(x["totalTokens"] for x in classifier_calls),
        "totalEstimatedCostUSD": sum(x["costUSD"] for x in classifier_calls),
        "pricing": {
            "model": "claude-sonnet-4-6",
            "inputPerMillionUSD": INPUT_USD_PER_M,
            "outputPerMillionUSD": OUTPUT_USD_PER_M,
        },
        "calls": classifier_calls,
        "note": (
            "A1 calls come from classifier-calls.jsonl. "
            "Older B/C artifacts persist classifier metadata on CLOSED; "
            "their costs are backfilled deterministically from stored token counts. "
            "For C1, the older CLOSED-only instrumentation preserves the terminal "
            "classifier call rather than every attempt-level call."
        ),
    }

    summary = {
        "dataset": {
            "runs": len(run_rows),
            "matrixRuns": 28,
            "valid": True,
        },
        "experimentRecovery": recovery,
        "controller": {
            "incidents": len(actionable_incidents),
            "terminalIncidents": terminal_total,
            "abandonedIncidents": len(abandoned_incidents),
            "remediationAttempts": attempt_count,
            "rolledBackAttempts": rolled_back_attempts,
            "incidentsWithRollback": len(incidents_with_rollback),
            "rollbackAttemptRatePercent": rollback_attempt_rate,
            "rollbackIncidentRatePercent": rollback_incident_rate,
            "terminalOutcomes": outcome_counts,
        },
        "timingSeconds": {
            "averageTTD": avg(ttd),
            "averageTTM": avg(ttm),
            "averageClassifier": avg(classify_times),
            "averageApply": avg(apply_times),
            "averageVerify": avg(verify_times),
        },
        "classifier": classifier_summary,
    }

    json_out = Path(args.json_out)
    json_out.write_text(
        json.dumps(summary, indent=2) + "\n",
        encoding="utf-8"
    )

    md = []
    md.append("# SAGE-K8s Week 3 Live Final Results")
    md.append("")
    md.append("## Dataset")
    md.append("")
    md.append(f"- Valid runs: **{len(run_rows)}/28**")
    md.append("")

    md.append("## Experiment recovery")
    md.append("")
    md.append("| Workload | Enabled | Disabled | Attributable recovery |")
    md.append("|---|---:|---:|---:|")

    for w in ("W1", "W2", "W3"):
        e = recovery[w]["enabled"]
        d = recovery[w]["disabled"]
        a = recovery[w]["attributableRecoveryPercentagePoints"]
        md.append(
            f"| {w} | {e['recovered']}/{e['total']} "
            f"({e['ratePercent']:.1f}%) | "
            f"{d['recovered']}/{d['total']} "
            f"({d['ratePercent']:.1f}%) | "
            f"{a:+.1f} pp |"
        )

    md.append("")
    md.append("## Controller outcomes")
    md.append("")
    md.append(f"- Incidents: **{len(actionable_incidents)}**")
    md.append(f"- Terminal incidents: **{terminal_total}**")
    md.append(f"- Abandoned incidents: **{len(abandoned_incidents)}**")
    md.append(f"- Remediation attempts: **{attempt_count}**")
    md.append(f"- Rolled-back attempts: **{rolled_back_attempts}**")
    md.append(f"- Rollback attempt rate: **{rollback_attempt_rate:.1f}%**")
    md.append(f"- Rollback incident rate: **{rollback_incident_rate:.1f}%**")
    md.append("")

    md.append("## Terminal outcomes")
    md.append("")
    for name in ("recovered", "exhausted", "escalated", "rejected", "abandoned"):
        md.append(f"- {name}: **{outcome_counts[name]}**")

    md.append("")
    md.append("## Timing")
    md.append("")
    md.append(f"- Average TTD: **{avg(ttd):.3f} s**")
    md.append(f"- Average TTM: **{avg(ttm):.3f} s**")
    md.append(f"- Average classifier latency: **{avg(classify_times):.3f} s**")
    md.append(f"- Average apply: **{avg(apply_times):.6f} s**")
    md.append(f"- Average verify: **{avg(verify_times):.3f} s**")
    md.append("")

    md.append("## Classifier")
    md.append("")
    md.append(
        f"- Persisted classifier call records represented: "
        f"**{len(classifier_calls)}**"
    )
    md.append(
        f"- Input tokens represented: "
        f"**{classifier_summary['totalInputTokens']}**"
    )
    md.append(
        f"- Output tokens represented: "
        f"**{classifier_summary['totalOutputTokens']}**"
    )
    md.append(
        f"- Total tokens represented: "
        f"**{classifier_summary['totalTokens']}**"
    )
    md.append(
        f"- Estimated cost represented: "
        f"**${classifier_summary['totalEstimatedCostUSD']:.6f}**"
    )
    md.append("")
    md.append(
        "> Note: A1 has independent per-call recorder data. "
        "Older B/C runs store classifier metadata on CLOSED. "
        "Therefore C1 preserves the terminal classifier call, not all "
        "three attempt-level calls. Cost figures are reported only for "
        "persisted call records and are not extrapolated."
    )
    md.append("")

    md_out = Path(args.md_out)
    md_out.parent.mkdir(parents=True, exist_ok=True)
    md_out.write_text("\n".join(md), encoding="utf-8")

    print("===== FINAL WEEK 3 RESULTS =====")
    print(f"runs: {len(run_rows)}/28")

    for w in ("W1", "W2", "W3"):
        e = recovery[w]["enabled"]
        d = recovery[w]["disabled"]
        a = recovery[w]["attributableRecoveryPercentagePoints"]
        print(
            f"{w}: enabled={e['ratePercent']:.1f}% "
            f"disabled={d['ratePercent']:.1f}% "
            f"attributable={a:+.1f}pp"
        )

    print(f"incidents: {len(actionable_incidents)}")
    print(f"terminal incidents: {terminal_total}")
    print(f"abandoned: {len(abandoned_incidents)}")
    print(f"attempts: {attempt_count}")
    print(f"rolled-back attempts: {rolled_back_attempts}")
    print(f"rollback attempt rate: {rollback_attempt_rate:.1f}%")
    print(f"rollback incident rate: {rollback_incident_rate:.1f}%")
    print(f"average TTD: {avg(ttd):.3f}s")
    print(f"average TTM: {avg(ttm):.3f}s")
    print(f"classifier records represented: {len(classifier_calls)}")
    print(
        "represented classifier cost: "
        f"${classifier_summary['totalEstimatedCostUSD']:.6f}"
    )
    print(f"JSON: {json_out}")
    print(f"Markdown: {md_out}")


if __name__ == "__main__":
    main()
