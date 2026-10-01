package metrics

import (
	"math"
	"testing"
	"time"
)

func TestCalculateWeek3MultiAttemptIncident(t *testing.T) {
	base := time.Date(2026, time.September, 29, 10, 0, 0, 0, time.UTC)

	records := []AuditRecord{
		{
			IncidentID: "w3-1", AttemptNumber: 1, Workload: "W3",
			ExperimentArm: "controller_enabled", RemediationAttempt: true,
			InjectedAt: base, DetectedAt: base.Add(5 * time.Second),
			RolledBack: true,
		},
		{
			IncidentID: "w3-1", AttemptNumber: 2, Workload: "W3",
			ExperimentArm: "controller_enabled", RemediationAttempt: true,
			InjectedAt: base, DetectedAt: base.Add(5 * time.Second),
			RolledBack: true,
		},
		{
			IncidentID: "w3-1", AttemptNumber: 3, Workload: "W3",
			ExperimentArm: "controller_enabled", RemediationAttempt: true,
			InjectedAt: base, DetectedAt: base.Add(5 * time.Second),
			TerminalOutcome: "exhausted", TerminalAt: base.Add(100 * time.Second),
		},
	}

	got := Calculate(records)
	if got.TotalIncidents != 1 {
		t.Fatalf("expected 1 unique incident, got %d", got.TotalIncidents)
	}
	if got.RemediationAttempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", got.RemediationAttempts)
	}
	if got.OutcomeDistribution["exhausted"] != 1 {
		t.Fatalf("expected exhausted=1, got %d", got.OutcomeDistribution["exhausted"])
	}
	if math.Abs(got.RollbackRate-66.6667) > 0.01 {
		t.Fatalf("expected attempt rollback rate 66.67, got %.2f", got.RollbackRate)
	}
	if got.IncidentRollbackRate != 100 {
		t.Fatalf("expected incident rollback rate 100, got %.2f", got.IncidentRollbackRate)
	}
	if got.AverageTTMSeconds != 95 {
		t.Fatalf("expected TTM 95s, got %.2f", got.AverageTTMSeconds)
	}
}

func TestCalculateWeek3IncompleteIncident(t *testing.T) {
	base := time.Date(2026, time.September, 29, 10, 0, 0, 0, time.UTC)
	got := Calculate([]AuditRecord{{
		IncidentID: "incomplete", Workload: "W1",
		ExperimentArm: "controller_enabled",
		InjectedAt:    base, DetectedAt: base.Add(time.Second),
	}})

	if got.TotalIncidents != 1 || got.TerminalIncidents != 0 || got.IncompleteIncidents != 1 {
		t.Fatalf("unexpected incident counts: %+v", got)
	}
	if got.FailedRecoveries != 0 {
		t.Fatalf("incomplete incident must not be counted as failed")
	}
}

func TestCalculateWeek3IncidentRollbackDenominatorExcludesEscalatedAndRejected(t *testing.T) {
	base := time.Date(2026, time.September, 29, 10, 0, 0, 0, time.UTC)

	got := Calculate([]AuditRecord{
		{
			IncidentID: "rolled", Workload: "W1", ExperimentArm: "enabled",
			AttemptNumber: 1, RemediationAttempt: true, RolledBack: true,
			DetectedAt: base, TerminalAt: base.Add(30 * time.Second),
			TerminalOutcome: "exhausted",
		},
		{
			IncidentID: "no-rollback", Workload: "W1", ExperimentArm: "enabled",
			AttemptNumber: 1, RemediationAttempt: true,
			DetectedAt: base, TerminalAt: base.Add(30 * time.Second),
			TerminalOutcome: "recovered",
		},
		{
			IncidentID: "escalated", Workload: "W1", ExperimentArm: "enabled",
			AttemptNumber: 0, RemediationAttempt: false,
			DetectedAt: base, TerminalAt: base.Add(30 * time.Second),
			TerminalOutcome: "escalated",
		},
		{
			IncidentID: "rejected", Workload: "W1", ExperimentArm: "enabled",
			AttemptNumber: 0, RemediationAttempt: false,
			DetectedAt: base, TerminalAt: base.Add(30 * time.Second),
			TerminalOutcome: "rejected",
		},
	})

	if got.RemediationAttempts != 2 {
		t.Fatalf("expected only 2 remediation attempts, got %d", got.RemediationAttempts)
	}
	if got.TotalRollbacks != 1 {
		t.Fatalf("expected 1 rolled-back attempt, got %d", got.TotalRollbacks)
	}
	if got.RollbackRate != 50 {
		t.Fatalf("expected attempt rollback rate 50, got %.2f", got.RollbackRate)
	}
	if got.IncidentRollbackRate != 50 {
		t.Fatalf("expected incident rollback rate 50, got %.2f", got.IncidentRollbackRate)
	}
}

func TestCalculateWeek3WorkloadAttribution(t *testing.T) {
	base := time.Date(2026, time.September, 29, 10, 0, 0, 0, time.UTC)
	records := []AuditRecord{
		{
			IncidentID: "enabled", Workload: "W2", ExperimentArm: "enabled",
			DetectedAt: base, TerminalAt: base.Add(60 * time.Second),
			TerminalOutcome: "recovered",
		},
		{
			IncidentID: "disabled", Workload: "W2", ExperimentArm: "disabled",
			DetectedAt: base, TerminalAt: base.Add(60 * time.Second),
			TerminalOutcome: "exhausted",
		},
	}

	got := Calculate(records)
	if got.W2.Enabled.RecoveryRate != 100 || got.W2.Disabled.RecoveryRate != 0 {
		t.Fatalf("unexpected W2 recovery rates: %+v", got.W2)
	}
	if got.W2.AttributableRecovery != 100 {
		t.Fatalf("expected attributable recovery 100pp, got %.2f", got.W2.AttributableRecovery)
	}
}

func TestCalculateWeek3WorkloadRollbackTriggersCountAttempts(t *testing.T) {
	base := time.Date(2026, time.September, 29, 10, 0, 0, 0, time.UTC)

	got := Calculate([]AuditRecord{
		{
			IncidentID: "w1-rollback", AttemptNumber: 1, Workload: "W1",
			ExperimentArm: "enabled", RemediationAttempt: true,
			DetectedAt: base, RolledBack: true,
		},
		{
			IncidentID: "w1-rollback", AttemptNumber: 2, Workload: "W1",
			ExperimentArm: "enabled", RemediationAttempt: true,
			DetectedAt: base, RolledBack: true,
		},
		{
			IncidentID: "w1-rollback", AttemptNumber: 3, Workload: "W1",
			ExperimentArm: "enabled", RemediationAttempt: true,
			DetectedAt: base, TerminalAt: base.Add(90 * time.Second),
			TerminalOutcome: "exhausted",
		},
	})

	if got.W1.Enabled.RemediationAttempts != 3 {
		t.Fatalf(
			"expected 3 W1 enabled attempts, got %d",
			got.W1.Enabled.RemediationAttempts,
		)
	}
	if got.W1.Enabled.RollbackTriggers != 2 {
		t.Fatalf(
			"expected 2 W1 enabled rollback triggers, got %d",
			got.W1.Enabled.RollbackTriggers,
		)
	}
	if math.Abs(got.W1.Enabled.RollbackTriggerRate-66.6667) > 0.01 {
		t.Fatalf(
			"expected W1 enabled rollback trigger rate 66.67, got %.2f",
			got.W1.Enabled.RollbackTriggerRate,
		)
	}
}

func TestCalculateWeek3ValidatorTwoSided(t *testing.T) {
	got := Calculate([]AuditRecord{
		{IncidentID: "a1", ValidatorCaseType: "adversarial", ProposalAccepted: false},
		{IncidentID: "a2", ValidatorCaseType: "adversarial", ProposalAccepted: false},
		{IncidentID: "a3", ValidatorCaseType: "adversarial", ProposalAccepted: true},
		{IncidentID: "l1", ValidatorCaseType: "legitimate", ProposalAccepted: true},
		{IncidentID: "l2", ValidatorCaseType: "legitimate", ProposalAccepted: true},
	})

	if got.AdversarialCases != 3 || got.AdversarialRejected != 2 {
		t.Fatalf("unexpected adversarial metrics")
	}
	if math.Abs(got.AdversarialRejectionRate-66.6667) > 0.01 {
		t.Fatalf("unexpected adversarial rejection rate %.2f", got.AdversarialRejectionRate)
	}
	if got.LegitimateCases != 2 || got.LegitimateAccepted != 2 ||
		got.LegitimateAcceptanceRate != 100 {
		t.Fatalf("unexpected legitimate metrics")
	}
}
