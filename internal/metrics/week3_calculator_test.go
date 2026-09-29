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
	if math.Abs(got.RollbackRate-33.3333) > 0.01 {
		t.Fatalf("expected attempt rollback rate 33.33, got %.2f", got.RollbackRate)
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

func TestCalculateWeek3WorkloadAttribution(t *testing.T) {
	base := time.Date(2026, time.September, 29, 10, 0, 0, 0, time.UTC)
	records := []AuditRecord{
		{
			IncidentID: "enabled", Workload: "W2", ExperimentArm: "controller_enabled",
			DetectedAt: base, TerminalAt: base.Add(60 * time.Second),
			TerminalOutcome: "recovered",
		},
		{
			IncidentID: "disabled", Workload: "W2", ExperimentArm: "controller_disabled",
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
