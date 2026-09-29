package metrics

import (
	"math"
	"testing"
	"time"
)

func TestCalculateMetrics(t *testing.T) {
	baseTime := time.Date(
		2026,
		time.July,
		26,
		10,
		0,
		0,
		0,
		time.UTC,
	)

	records := []AuditRecord{
		{
			IncidentID: "incident-1",
			PodName:    "checkoutservice-abc123",
			Namespace:  "default",
			Action:     "restart_pod",

			ExperimentArm:      "controller_enabled",
			FailureMode:        "fixable",
			RemediationAttempt: true,

			InjectedAt:      baseTime.Add(-5 * time.Second),
			DetectedAt:      baseTime,
			ActionStartedAt: baseTime.Add(10 * time.Second),
			MitigatedAt:     baseTime.Add(40 * time.Second),

			ClassifierStartedAt:   baseTime.Add(2 * time.Second),
			ClassifierCompletedAt: baseTime.Add(7 * time.Second),

			VerificationStartedAt:   baseTime.Add(15 * time.Second),
			VerificationCompletedAt: baseTime.Add(40 * time.Second),

			Success:          true,
			RolledBack:       false,
			ProposalAccepted: true,
			ProposalCorrect:  true,
		},
		{
			IncidentID: "incident-2",
			PodName:    "paymentservice-def456",
			Namespace:  "default",
			Action:     "restart_pod",

			ExperimentArm:      "unrecoverable",
			FailureMode:        "unrecoverable",
			RemediationAttempt: true,

			InjectedAt:      baseTime.Add(110 * time.Second),
			DetectedAt:      baseTime.Add(120 * time.Second),
			ActionStartedAt: baseTime.Add(130 * time.Second),
			MitigatedAt:     baseTime.Add(180 * time.Second),

			ClassifierStartedAt:   baseTime.Add(125 * time.Second),
			ClassifierCompletedAt: baseTime.Add(135 * time.Second),

			VerificationStartedAt:   baseTime.Add(140 * time.Second),
			VerificationCompletedAt: baseTime.Add(180 * time.Second),

			Success:          false,
			RolledBack:       true,
			ProposalAccepted: true,
			ProposalCorrect:  false,
		},
		{
			IncidentID: "incident-3",
			PodName:    "frontend-xyz789",
			Namespace:  "default",
			Action:     "rollout_undo",

			ExperimentArm:      "controller_enabled",
			FailureMode:        "fixable",
			RemediationAttempt: true,

			InjectedAt:      baseTime.Add(230 * time.Second),
			DetectedAt:      baseTime.Add(240 * time.Second),
			ActionStartedAt: baseTime.Add(250 * time.Second),
			MitigatedAt:     baseTime.Add(290 * time.Second),

			ClassifierStartedAt:   baseTime.Add(245 * time.Second),
			ClassifierCompletedAt: baseTime.Add(255 * time.Second),

			VerificationStartedAt:   baseTime.Add(260 * time.Second),
			VerificationCompletedAt: baseTime.Add(290 * time.Second),

			Success:          true,
			RolledBack:       false,
			ProposalAccepted: false,
			ProposalCorrect:  true,
		},
	}

	result := Calculate(records)

	if result.TotalIncidents != 3 {
		t.Fatalf(
			"expected 3 incidents, got %d",
			result.TotalIncidents,
		)
	}

	if result.SuccessfulRecoveries != 2 {
		t.Fatalf(
			"expected 2 successful recoveries, got %d",
			result.SuccessfulRecoveries,
		)
	}

	if result.FailedRecoveries != 1 {
		t.Fatalf(
			"expected 1 failed recovery, got %d",
			result.FailedRecoveries,
		)
	}

	if result.TotalRollbacks != 1 {
		t.Fatalf(
			"expected 1 rollback, got %d",
			result.TotalRollbacks,
		)
	}

	// ------------------------------------------------------------
	// TTD
	//
	// incident-1 = 5 seconds
	// incident-2 = 10 seconds
	// incident-3 = 10 seconds
	// average = 25 / 3
	// ------------------------------------------------------------

	expectedTTD := 25.0 / 3.0

	if math.Abs(
		result.AverageTTDSeconds-expectedTTD,
	) > 0.001 {
		t.Fatalf(
			"expected TTD %.2f seconds, got %.2f",
			expectedTTD,
			result.AverageTTDSeconds,
		)
	}

	// ------------------------------------------------------------
	// TTM
	//
	// incident-1 = 40 seconds
	// incident-2 = 60 seconds
	// incident-3 = 50 seconds
	// average = 150 / 3 = 50
	// ------------------------------------------------------------

	expectedTTM := 50.0

	if math.Abs(
		result.AverageTTMSeconds-expectedTTM,
	) > 0.001 {
		t.Fatalf(
			"expected TTM %.2f seconds, got %.2f",
			expectedTTM,
			result.AverageTTMSeconds,
		)
	}

	// ------------------------------------------------------------
	// Inference latency
	//
	// incident-1 = 7 - 2 = 5 seconds
	// incident-2 = 135 - 125 = 10 seconds
	// incident-3 = 255 - 245 = 10 seconds
	// average = 25 / 3
	// ------------------------------------------------------------

	expectedInferenceLatency := 25.0 / 3.0

	if math.Abs(
		result.AverageInferenceLatencySeconds-
			expectedInferenceLatency,
	) > 0.001 {
		t.Fatalf(
			"expected inference latency %.2f seconds, got %.2f",
			expectedInferenceLatency,
			result.AverageInferenceLatencySeconds,
		)
	}

	// ------------------------------------------------------------
	// Cluster convergence
	//
	// incident-1 = 40 - 10 = 30 seconds
	// incident-2 = 180 - 130 = 50 seconds
	// incident-3 = 290 - 250 = 40 seconds
	// average = 120 / 3 = 40
	// ------------------------------------------------------------

	expectedClusterConvergence := 40.0

	if math.Abs(
		result.AverageClusterConvergenceSeconds-
			expectedClusterConvergence,
	) > 0.001 {
		t.Fatalf(
			"expected cluster convergence %.2f seconds, got %.2f",
			expectedClusterConvergence,
			result.AverageClusterConvergenceSeconds,
		)
	}

	// ------------------------------------------------------------
	// Recovery success rate
	// ------------------------------------------------------------

	expectedSuccessRate := 66.6667

	if math.Abs(
		result.RecoverySuccessRate-expectedSuccessRate,
	) > 0.01 {
		t.Fatalf(
			"expected success rate %.2f, got %.2f",
			expectedSuccessRate,
			result.RecoverySuccessRate,
		)
	}

	// ------------------------------------------------------------
	// Rollback rate
	// ------------------------------------------------------------

	expectedRollbackRate := 33.3333

	if math.Abs(
		result.RollbackRate-expectedRollbackRate,
	) > 0.01 {
		t.Fatalf(
			"expected rollback rate %.2f, got %.2f",
			expectedRollbackRate,
			result.RollbackRate,
		)
	}

	// ------------------------------------------------------------
	// Remediation attempts
	// ------------------------------------------------------------

	if result.RemediationAttempts != 3 {
		t.Fatalf(
			"expected 3 remediation attempts, got %d",
			result.RemediationAttempts,
		)
	}

	// ------------------------------------------------------------
	// Controller-enabled arm
	// ------------------------------------------------------------

	if result.ControllerEnabled.TotalIncidents != 2 {
		t.Fatalf(
			"expected 2 controller-enabled incidents, got %d",
			result.ControllerEnabled.TotalIncidents,
		)
	}

	if result.ControllerEnabled.SuccessfulRecoveries != 2 {
		t.Fatalf(
			"expected 2 controller-enabled successful recoveries, got %d",
			result.ControllerEnabled.SuccessfulRecoveries,
		)
	}

	if result.ControllerEnabled.RollbackTriggers != 0 {
		t.Fatalf(
			"expected 0 controller-enabled rollback triggers, got %d",
			result.ControllerEnabled.RollbackTriggers,
		)
	}

	if result.ControllerEnabled.RecoverySuccessRate != 100 {
		t.Fatalf(
			"expected 100%% controller-enabled recovery success rate, got %.2f",
			result.ControllerEnabled.RecoverySuccessRate,
		)
	}

	// ------------------------------------------------------------
	// Unrecoverable arm
	// ------------------------------------------------------------

	if result.Unrecoverable.TotalIncidents != 1 {
		t.Fatalf(
			"expected 1 unrecoverable incident, got %d",
			result.Unrecoverable.TotalIncidents,
		)
	}

	if result.Unrecoverable.SuccessfulRecoveries != 0 {
		t.Fatalf(
			"expected 0 unrecoverable successful recoveries, got %d",
			result.Unrecoverable.SuccessfulRecoveries,
		)
	}

	if result.Unrecoverable.RollbackTriggers != 1 {
		t.Fatalf(
			"expected 1 unrecoverable rollback trigger, got %d",
			result.Unrecoverable.RollbackTriggers,
		)
	}

	expectedUnrecoverableRollbackRate := 100.0

	if math.Abs(
		result.Unrecoverable.RollbackTriggerRate-
			expectedUnrecoverableRollbackRate,
	) > 0.001 {
		t.Fatalf(
			"expected unrecoverable rollback rate %.2f, got %.2f",
			expectedUnrecoverableRollbackRate,
			result.Unrecoverable.RollbackTriggerRate,
		)
	}

	// ------------------------------------------------------------
	// False acceptance
	// ------------------------------------------------------------

	if result.FalseAcceptCount != 1 {
		t.Fatalf(
			"expected 1 false acceptance, got %d",
			result.FalseAcceptCount,
		)
	}

	// ------------------------------------------------------------
	// False rejection
	// ------------------------------------------------------------

	if result.FalseRejectCount != 1 {
		t.Fatalf(
			"expected 1 false rejection, got %d",
			result.FalseRejectCount,
		)
	}

	expectedFalseAcceptRate := 50.0

	if math.Abs(
		result.FalseAcceptRate-expectedFalseAcceptRate,
	) > 0.001 {
		t.Fatalf(
			"expected false acceptance rate %.2f, got %.2f",
			expectedFalseAcceptRate,
			result.FalseAcceptRate,
		)
	}

	expectedFalseRejectRate := 100.0

	if math.Abs(
		result.FalseRejectRate-expectedFalseRejectRate,
	) > 0.001 {
		t.Fatalf(
			"expected false rejection rate %.2f, got %.2f",
			expectedFalseRejectRate,
			result.FalseRejectRate,
		)
	}
}

func TestCalculateEmptyRecords(t *testing.T) {
	result := Calculate(nil)

	if result.TotalIncidents != 0 {
		t.Fatal("expected zero incidents")
	}

	if result.SuccessfulRecoveries != 0 {
		t.Fatal("expected zero successful recoveries")
	}

	if result.FailedRecoveries != 0 {
		t.Fatal("expected zero failed recoveries")
	}

	if result.RemediationAttempts != 0 {
		t.Fatal("expected zero remediation attempts")
	}

	if result.RecoverySuccessRate != 0 {
		t.Fatal("expected zero recovery success rate")
	}

	if result.RollbackRate != 0 {
		t.Fatal("expected zero rollback rate")
	}

	if result.AverageTTDSeconds != 0 {
		t.Fatal("expected zero TTD")
	}

	if result.AverageTTMSeconds != 0 {
		t.Fatal("expected zero TTM")
	}

	if result.AverageInferenceLatencySeconds != 0 {
		t.Fatal("expected zero inference latency")
	}

	if result.AverageClusterConvergenceSeconds != 0 {
		t.Fatal("expected zero cluster convergence")
	}

	if result.ControllerEnabled.TotalIncidents != 0 {
		t.Fatal("expected zero controller-enabled incidents")
	}

	if result.ControllerDisabled.TotalIncidents != 0 {
		t.Fatal("expected zero controller-disabled incidents")
	}

	if result.Unrecoverable.TotalIncidents != 0 {
		t.Fatal("expected zero unrecoverable incidents")
	}

	if result.FalseAcceptCount != 0 {
		t.Fatal("expected zero false accepts")
	}

	if result.FalseRejectCount != 0 {
		t.Fatal("expected zero false rejects")
	}

	if result.FalseAcceptRate != 0 {
		t.Fatal("expected zero false acceptance rate")
	}

	if result.FalseRejectRate != 0 {
		t.Fatal("expected zero false rejection rate")
	}
}

func TestCalculateIgnoresInvalidTTDTimestamps(t *testing.T) {
	baseTime := time.Date(
		2026,
		time.July,
		26,
		10,
		0,
		0,
		0,
		time.UTC,
	)

	records := []AuditRecord{
		{
			IncidentID:  "valid-ttd",
			InjectedAt:  baseTime,
			DetectedAt:  baseTime.Add(10 * time.Second),
			MitigatedAt: baseTime.Add(20 * time.Second),

			Success:          true,
			ProposalAccepted: true,
			ProposalCorrect:  true,
		},
		{
			IncidentID:  "zero-injected",
			InjectedAt:  time.Time{},
			DetectedAt:  baseTime.Add(20 * time.Second),
			MitigatedAt: baseTime.Add(30 * time.Second),

			Success:          true,
			ProposalAccepted: true,
			ProposalCorrect:  true,
		},
		{
			IncidentID:  "detected-before-injected",
			InjectedAt:  baseTime.Add(30 * time.Second),
			DetectedAt:  baseTime.Add(20 * time.Second),
			MitigatedAt: baseTime.Add(40 * time.Second),

			Success:          true,
			ProposalAccepted: true,
			ProposalCorrect:  true,
		},
	}

	result := Calculate(records)

	// Only the first record has valid TTD.
	expectedTTD := 10.0

	if math.Abs(
		result.AverageTTDSeconds-expectedTTD,
	) > 0.001 {
		t.Fatalf(
			"expected TTD %.2f seconds, got %.2f",
			expectedTTD,
			result.AverageTTDSeconds,
		)
	}
}

func TestCalculateIgnoresInvalidTTMTimestamps(t *testing.T) {
	baseTime := time.Date(
		2026,
		time.July,
		26,
		10,
		0,
		0,
		0,
		time.UTC,
	)

	records := []AuditRecord{
		{
			IncidentID:  "valid-ttm",
			InjectedAt:  baseTime,
			DetectedAt:  baseTime.Add(10 * time.Second),
			MitigatedAt: baseTime.Add(30 * time.Second),

			Success:          true,
			ProposalAccepted: true,
			ProposalCorrect:  true,
		},
		{
			IncidentID:  "zero-detected",
			InjectedAt:  baseTime,
			DetectedAt:  time.Time{},
			MitigatedAt: baseTime.Add(40 * time.Second),

			Success:          true,
			ProposalAccepted: true,
			ProposalCorrect:  true,
		},
		{
			IncidentID:  "mitigated-before-detected",
			InjectedAt:  baseTime,
			DetectedAt:  baseTime.Add(50 * time.Second),
			MitigatedAt: baseTime.Add(40 * time.Second),

			Success:          true,
			ProposalAccepted: true,
			ProposalCorrect:  true,
		},
	}

	result := Calculate(records)

	// Only the first record has valid TTM.
	expectedTTM := 20.0

	if math.Abs(
		result.AverageTTMSeconds-expectedTTM,
	) > 0.001 {
		t.Fatalf(
			"expected TTM %.2f seconds, got %.2f",
			expectedTTM,
			result.AverageTTMSeconds,
		)
	}
}

func TestCalculateAllInvalidTTDTimestamps(t *testing.T) {
	baseTime := time.Date(
		2026,
		time.July,
		26,
		10,
		0,
		0,
		0,
		time.UTC,
	)

	records := []AuditRecord{
		{
			IncidentID:  "invalid-1",
			InjectedAt:  time.Time{},
			DetectedAt:  baseTime,
			MitigatedAt: baseTime.Add(10 * time.Second),
		},
		{
			IncidentID:  "invalid-2",
			InjectedAt:  baseTime.Add(20 * time.Second),
			DetectedAt:  baseTime.Add(10 * time.Second),
			MitigatedAt: baseTime.Add(30 * time.Second),
		},
	}

	result := Calculate(records)

	if result.AverageTTDSeconds != 0 {
		t.Fatalf(
			"expected zero TTD when no valid TTD records exist, got %.2f",
			result.AverageTTDSeconds,
		)
	}
}

func TestCalculateAllInvalidTTMTimestamps(t *testing.T) {
	baseTime := time.Date(
		2026,
		time.July,
		26,
		10,
		0,
		0,
		0,
		time.UTC,
	)

	records := []AuditRecord{
		{
			IncidentID:  "invalid-1",
			InjectedAt:  baseTime,
			DetectedAt:  time.Time{},
			MitigatedAt: baseTime.Add(10 * time.Second),
		},
		{
			IncidentID:  "invalid-2",
			InjectedAt:  baseTime,
			DetectedAt:  baseTime.Add(30 * time.Second),
			MitigatedAt: baseTime.Add(20 * time.Second),
		},
	}

	result := Calculate(records)

	if result.AverageTTMSeconds != 0 {
		t.Fatalf(
			"expected zero TTM when no valid TTM records exist, got %.2f",
			result.AverageTTMSeconds,
		)
	}
}

func TestCalculateIgnoresInvalidInferenceTimestamps(t *testing.T) {
	baseTime := time.Date(
		2026,
		time.July,
		26,
		10,
		0,
		0,
		0,
		time.UTC,
	)

	records := []AuditRecord{
		{
			IncidentID: "valid-inference",

			ClassifierStartedAt:   baseTime,
			ClassifierCompletedAt: baseTime.Add(10 * time.Second),
		},
		{
			IncidentID: "zero-start",

			ClassifierStartedAt:   time.Time{},
			ClassifierCompletedAt: baseTime.Add(20 * time.Second),
		},
		{
			IncidentID: "completed-before-start",

			ClassifierStartedAt:   baseTime.Add(30 * time.Second),
			ClassifierCompletedAt: baseTime.Add(20 * time.Second),
		},
	}

	result := Calculate(records)

	expectedInference := 10.0

	if math.Abs(
		result.AverageInferenceLatencySeconds-expectedInference,
	) > 0.001 {
		t.Fatalf(
			"expected inference latency %.2f seconds, got %.2f",
			expectedInference,
			result.AverageInferenceLatencySeconds,
		)
	}
}

func TestCalculateIgnoresInvalidClusterConvergenceTimestamps(
	t *testing.T,
) {
	baseTime := time.Date(
		2026,
		time.July,
		26,
		10,
		0,
		0,
		0,
		time.UTC,
	)

	records := []AuditRecord{
		{
			IncidentID: "valid-convergence",

			ActionStartedAt:         baseTime,
			VerificationCompletedAt: baseTime.Add(20 * time.Second),
		},
		{
			IncidentID: "zero-action",

			ActionStartedAt:         time.Time{},
			VerificationCompletedAt: baseTime.Add(30 * time.Second),
		},
		{
			IncidentID: "verification-before-action",

			ActionStartedAt:         baseTime.Add(40 * time.Second),
			VerificationCompletedAt: baseTime.Add(30 * time.Second),
		},
	}

	result := Calculate(records)

	expectedConvergence := 20.0

	if math.Abs(
		result.AverageClusterConvergenceSeconds-
			expectedConvergence,
	) > 0.001 {
		t.Fatalf(
			"expected cluster convergence %.2f seconds, got %.2f",
			expectedConvergence,
			result.AverageClusterConvergenceSeconds,
		)
	}
}

func TestCalculateAllInvalidInferenceTimestamps(t *testing.T) {
	baseTime := time.Date(
		2026,
		time.July,
		26,
		10,
		0,
		0,
		0,
		time.UTC,
	)

	records := []AuditRecord{
		{
			IncidentID: "invalid-inference-1",

			ClassifierStartedAt:   time.Time{},
			ClassifierCompletedAt: baseTime,
		},
		{
			IncidentID: "invalid-inference-2",

			ClassifierStartedAt:   baseTime.Add(20 * time.Second),
			ClassifierCompletedAt: baseTime.Add(10 * time.Second),
		},
	}

	result := Calculate(records)

	if result.AverageInferenceLatencySeconds != 0 {
		t.Fatalf(
			"expected zero inference latency when no valid records exist, got %.2f",
			result.AverageInferenceLatencySeconds,
		)
	}
}

func TestCalculateAllInvalidClusterConvergenceTimestamps(
	t *testing.T,
) {
	baseTime := time.Date(
		2026,
		time.July,
		26,
		10,
		0,
		0,
		0,
		time.UTC,
	)

	records := []AuditRecord{
		{
			IncidentID: "invalid-convergence-1",

			ActionStartedAt:         time.Time{},
			VerificationCompletedAt: baseTime,
		},
		{
			IncidentID: "invalid-convergence-2",

			ActionStartedAt:         baseTime.Add(20 * time.Second),
			VerificationCompletedAt: baseTime.Add(10 * time.Second),
		},
	}

	result := Calculate(records)

	if result.AverageClusterConvergenceSeconds != 0 {
		t.Fatalf(
			"expected zero cluster convergence when no valid records exist, got %.2f",
			result.AverageClusterConvergenceSeconds,
		)
	}
}
