package metrics

import "time"

// AuditRecord represents one audit/experiment record.
//
// Week 3 permits multiple records for the same IncidentID: an incident can
// contain multiple remediation attempts. AttemptNumber identifies the attempt.
// TerminalOutcome/TerminalAt are populated only when the incident reaches one
// of the five terminal outcomes.
type AuditRecord struct {
	IncidentID string `json:"incident_id"`

	// Week 3 experiment dimensions.
	// Workload: W1, W2, W3.
	// ExperimentArm: enabled or disabled.
	AttemptNumber   int    `json:"attempt_number,omitempty"`
	Workload        string `json:"workload,omitempty"`
	ExperimentArm   string `json:"experiment_arm"`
	FailureMode     string `json:"failure_mode"`
	TerminalOutcome string `json:"terminal_outcome,omitempty"`

	PodName   string `json:"pod_name"`
	Namespace string `json:"namespace"`
	Action    string `json:"action"`

	RemediationAttempt bool `json:"remediation_attempt"`

	InjectedAt time.Time `json:"injected_at"`
	DetectedAt time.Time `json:"detected_at"`
	TerminalAt time.Time `json:"terminal_at,omitempty"`

	ClassifierStartedAt   time.Time `json:"classifier_started_at"`
	ClassifierCompletedAt time.Time `json:"classifier_completed_at"`

	ClassifierProvider        string  `json:"classifier_provider"`
	ClassifierModel           string  `json:"classifier_model"`
	ClassifierDurationSeconds float64 `json:"classifier_duration_seconds"`

	ActionStartedAt   time.Time `json:"action_started_at"`
	ActionCompletedAt time.Time `json:"action_completed_at"`

	VerificationStartedAt   time.Time `json:"verification_started_at"`
	VerificationCompletedAt time.Time `json:"verification_completed_at"`

	RollbackStartedAt   time.Time `json:"rollback_started_at"`
	RollbackCompletedAt time.Time `json:"rollback_completed_at"`

	// Legacy Week-2 fields retained so older audit files remain readable.
	// New Week-3 runs should use TerminalOutcome and TerminalAt.
	MitigatedAt time.Time `json:"mitigated_at"`
	Success     bool      `json:"success"`
	RolledBack  bool      `json:"rolled_back"`

	SafeForAutomation  bool   `json:"safe_for_automation"`
	ClassifierDecision string `json:"classifier_decision"`

	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
	CostKnown        bool    `json:"cost_known"`

	ProposalAccepted bool `json:"proposal_accepted"`
	ProposalCorrect  bool `json:"proposal_correct"`

	// Optional explicit labels for the two-sided Week-3 validator set.
	// Expected values: adversarial, legitimate.
	ValidatorCaseType string `json:"validator_case_type,omitempty"`
}

// ArmSummary contains experiment-arm-specific metrics.
type ArmSummary struct {
	TotalIncidents       int `json:"total_incidents"`
	SuccessfulRecoveries int `json:"successful_recoveries"`
	FailedRecoveries     int `json:"failed_recoveries"`
	RemediationAttempts  int `json:"remediation_attempts"`
	RollbackTriggers     int `json:"rollback_triggers"`

	RecoverySuccessRate float64 `json:"recovery_success_rate"`
	RollbackTriggerRate float64 `json:"rollback_trigger_rate"`
}

// WorkloadArmSummary is the report unit for W1/W2/W3 × enabled/disabled.
type WorkloadArmSummary struct {
	TotalIncidents      int     `json:"total_incidents"`
	TerminalIncidents   int     `json:"terminal_incidents"`
	IncompleteIncidents int     `json:"incomplete_incidents"`
	RecoveredIncidents  int     `json:"recovered_incidents"`
	RecoveryRate        float64 `json:"recovery_rate"`
	RemediationAttempts int     `json:"remediation_attempts"`
	RollbackTriggers    int     `json:"rollback_triggers"`
	RollbackTriggerRate float64 `json:"rollback_trigger_rate"`
}

// WorkloadSummary compares enabled and disabled arms for one workload.
type WorkloadSummary struct {
	Enabled              WorkloadArmSummary `json:"enabled"`
	Disabled             WorkloadArmSummary `json:"disabled"`
	AttributableRecovery float64            `json:"attributable_recovery_percentage_points"`
}

// Summary contains calculated project metrics.
type Summary struct {
	TotalIncidents      int `json:"total_incidents"`
	TerminalIncidents   int `json:"terminal_incidents"`
	IncompleteIncidents int `json:"incomplete_incidents"`

	SuccessfulRecoveries int `json:"successful_recoveries"`
	FailedRecoveries     int `json:"failed_recoveries"`
	TotalRollbacks       int `json:"total_rollbacks"`

	// Week 3: this is the number of actual remediation attempts, not incidents.
	RemediationAttempts int `json:"remediation_attempts"`

	AverageTTDSeconds float64 `json:"average_ttd_seconds"`
	AverageTTMSeconds float64 `json:"average_ttm_seconds"`

	AverageInferenceLatencySeconds   float64 `json:"average_inference_latency_seconds"`
	AverageApplySeconds              float64 `json:"average_apply_seconds"`
	AverageVerificationSeconds       float64 `json:"average_verification_seconds"`
	AverageClusterConvergenceSeconds float64 `json:"average_cluster_convergence_seconds"`

	RecoverySuccessRate float64 `json:"recovery_success_rate"`

	// Legacy name retained; Week 3 interpretation is attempt-level rollback
	// trigger rate.
	RollbackRate float64 `json:"rollback_rate"`

	// Secondary Week-3 rollback metric: incidents with >=1 rollback /
	// terminal incidents.
	IncidentRollbackRate float64 `json:"incident_rollback_rate"`

	// Exactly five Week-3 terminal outcomes.
	OutcomeDistribution map[string]int `json:"outcome_distribution"`

	ControllerEnabled  ArmSummary `json:"controller_enabled"`
	ControllerDisabled ArmSummary `json:"controller_disabled"`
	// Legacy Week-2 arm retained for old audit files/tests.
	Unrecoverable ArmSummary `json:"unrecoverable"`

	W1 WorkloadSummary `json:"w1"`
	W2 WorkloadSummary `json:"w2"`
	W3 WorkloadSummary `json:"w3"`

	FalseAcceptCount int     `json:"false_accept_count"`
	FalseRejectCount int     `json:"false_reject_count"`
	FalseAcceptRate  float64 `json:"false_accept_rate"`
	FalseRejectRate  float64 `json:"false_reject_rate"`

	AdversarialCases         int     `json:"adversarial_cases"`
	AdversarialRejected      int     `json:"adversarial_rejected"`
	AdversarialRejectionRate float64 `json:"adversarial_rejection_rate"`
	LegitimateCases          int     `json:"legitimate_cases"`
	LegitimateAccepted       int     `json:"legitimate_accepted"`
	LegitimateAcceptanceRate float64 `json:"legitimate_acceptance_rate"`
}
