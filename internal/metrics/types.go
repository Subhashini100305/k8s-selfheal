package metrics

import "time"

// AuditRecord represents one experimental incident.
//
// One record corresponds to one fault injection attempt.
// The record stores raw lifecycle timestamps and outcomes.
// Metric calculations are performed separately by Calculate().
type AuditRecord struct {
	// ------------------------------------------------------------
	// Experiment identity
	// ------------------------------------------------------------

	IncidentID string `json:"incident_id"`

	// Experimental arm.
	//
	// Expected values:
	//   controller_enabled
	//   controller_disabled
	//   unrecoverable
	ExperimentArm string `json:"experiment_arm"`

	// Failure configuration.
	//
	// Expected values may include:
	//   fixable
	//   unrecoverable
	//   null_action
	FailureMode string `json:"failure_mode"`

	PodName   string `json:"pod_name"`
	Namespace string `json:"namespace"`

	// Action actually selected/executed.
	Action string `json:"action"`

	// Whether a remediation action was actually attempted.
	RemediationAttempt bool `json:"remediation_attempt"`

	// ------------------------------------------------------------
	// Incident lifecycle timestamps
	// ------------------------------------------------------------

	InjectedAt time.Time `json:"injected_at"`
	DetectedAt time.Time `json:"detected_at"`

	// ------------------------------------------------------------
	// Classifier timing
	// ------------------------------------------------------------

	ClassifierStartedAt   time.Time `json:"classifier_started_at"`
	ClassifierCompletedAt time.Time `json:"classifier_completed_at"`

	ClassifierProvider string `json:"classifier_provider"`
	ClassifierModel    string `json:"classifier_model"`

	ClassifierDurationSeconds float64 `json:"classifier_duration_seconds"`

	// ------------------------------------------------------------
	// Remediation
	// ------------------------------------------------------------

	ActionStartedAt time.Time `json:"action_started_at"`

	// ------------------------------------------------------------
	// Verification
	// ------------------------------------------------------------

	VerificationStartedAt   time.Time `json:"verification_started_at"`
	VerificationCompletedAt time.Time `json:"verification_completed_at"`

	// ------------------------------------------------------------
	// Rollback
	// ------------------------------------------------------------

	RollbackStartedAt   time.Time `json:"rollback_started_at"`
	RollbackCompletedAt time.Time `json:"rollback_completed_at"`

	// ------------------------------------------------------------
	// Final outcome
	// ------------------------------------------------------------

	MitigatedAt time.Time `json:"mitigated_at"`

	Success bool `json:"success"`

	RolledBack bool `json:"rolled_back"`

	// ------------------------------------------------------------
	// Classifier / validator audit information
	// ------------------------------------------------------------

	SafeForAutomation bool `json:"safe_for_automation"`

	ClassifierDecision string `json:"classifier_decision"`

	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
	CostKnown        bool    `json:"cost_known"`

	ProposalAccepted bool `json:"proposal_accepted"`

	ProposalCorrect bool `json:"proposal_correct"`
}

// ArmSummary contains experiment-arm-specific metrics.
type ArmSummary struct {
	TotalIncidents int `json:"total_incidents"`

	SuccessfulRecoveries int `json:"successful_recoveries"`
	FailedRecoveries     int `json:"failed_recoveries"`

	RemediationAttempts int `json:"remediation_attempts"`

	RollbackTriggers int `json:"rollback_triggers"`

	RecoverySuccessRate float64 `json:"recovery_success_rate"`
	RollbackTriggerRate float64 `json:"rollback_trigger_rate"`
}

// Summary contains the calculated project metrics.
type Summary struct {
	TotalIncidents int `json:"total_incidents"`

	SuccessfulRecoveries int `json:"successful_recoveries"`
	FailedRecoveries     int `json:"failed_recoveries"`
	TotalRollbacks       int `json:"total_rollbacks"`

	// Number of incidents for which remediation was actually attempted.
	RemediationAttempts int `json:"remediation_attempts"`

	AverageTTDSeconds float64 `json:"average_ttd_seconds"`
	AverageTTMSeconds float64 `json:"average_ttm_seconds"`

	// TTM decomposition.

	// ClassifierCompletedAt - ClassifierStartedAt.
	AverageInferenceLatencySeconds float64 `json:"average_inference_latency_seconds"`

	// VerificationCompletedAt - ActionStartedAt.
	AverageClusterConvergenceSeconds float64 `json:"average_cluster_convergence_seconds"`

	RecoverySuccessRate float64 `json:"recovery_success_rate"`
	RollbackRate        float64 `json:"rollback_rate"`

	// Experiment arms.

	ControllerEnabled  ArmSummary `json:"controller_enabled"`
	ControllerDisabled ArmSummary `json:"controller_disabled"`
	Unrecoverable      ArmSummary `json:"unrecoverable"`

	// Validator metrics.

	FalseAcceptCount int `json:"false_accept_count"`
	FalseRejectCount int `json:"false_reject_count"`

	FalseAcceptRate float64 `json:"false_accept_rate"`
	FalseRejectRate float64 `json:"false_reject_rate"`
}
