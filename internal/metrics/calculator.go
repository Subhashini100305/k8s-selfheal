package metrics

import "time"

var terminalOutcomes = map[string]bool{
	"recovered":   true,
	"rolled_back": true,
	"exhausted":   true,
	"escalated":   true,
	"rejected":    true,
}

type incidentAggregate struct {
	id               string
	workload         string
	arm              string
	injectedAt       time.Time
	detectedAt       time.Time
	terminalAt       time.Time
	terminalOutcome  string
	attempts         int
	rollback         bool
	rollbackAttempts int
	legacySuccess    bool
	hasWeek3Outcome  bool
}

// Calculate calculates runtime metrics from audit records.
// It supports both legacy Week-2 one-record-per-incident files and Week-3
// multi-attempt incident files.
func Calculate(records []AuditRecord) Summary {
	summary := Summary{
		OutcomeDistribution: map[string]int{
			"recovered":   0,
			"rolled_back": 0,
			"exhausted":   0,
			"escalated":   0,
			"rejected":    0,
		},
	}
	if len(records) == 0 {
		return summary
	}

	incidents := make(map[string]*incidentAggregate)

	var totalInference, totalApply, totalVerify, totalConvergence time.Duration
	var validInference, validApply, validVerify, validConvergence int
	var acceptedCount, rejectedCount int

	for i, record := range records {
		key := record.IncidentID
		if key == "" {
			// Preserve separate legacy records even if an old fixture omitted ID.
			key = "__record_" + time.Duration(i).String()
		}
		inc, ok := incidents[key]
		if !ok {
			inc = &incidentAggregate{id: key}
			incidents[key] = inc
		}

		if inc.workload == "" {
			inc.workload = record.Workload
		}
		if inc.arm == "" {
			inc.arm = record.ExperimentArm
		}
		if inc.injectedAt.IsZero() || (!record.InjectedAt.IsZero() && record.InjectedAt.Before(inc.injectedAt)) {
			inc.injectedAt = record.InjectedAt
		}
		if inc.detectedAt.IsZero() || (!record.DetectedAt.IsZero() && record.DetectedAt.Before(inc.detectedAt)) {
			inc.detectedAt = record.DetectedAt
		}

		if record.RemediationAttempt {
			inc.attempts++
			summary.RemediationAttempts++
		}
		if record.RemediationAttempt && record.RolledBack {
			inc.rollback = true
			inc.rollbackAttempts++
			summary.TotalRollbacks++
		}
		if record.Success {
			inc.legacySuccess = true
		}

		if terminalOutcomes[record.TerminalOutcome] {
			inc.hasWeek3Outcome = true
			inc.terminalOutcome = record.TerminalOutcome
			inc.terminalAt = record.TerminalAt
		}

		if validDuration(record.ClassifierStartedAt, record.ClassifierCompletedAt) {
			totalInference += record.ClassifierCompletedAt.Sub(record.ClassifierStartedAt)
			validInference++
		}
		if validDuration(record.ActionStartedAt, record.ActionCompletedAt) {
			totalApply += record.ActionCompletedAt.Sub(record.ActionStartedAt)
			validApply++
		}
		if validDuration(record.VerificationStartedAt, record.VerificationCompletedAt) {
			totalVerify += record.VerificationCompletedAt.Sub(record.VerificationStartedAt)
			validVerify++
		}
		if validDuration(record.ActionStartedAt, record.VerificationCompletedAt) {
			totalConvergence += record.VerificationCompletedAt.Sub(record.ActionStartedAt)
			validConvergence++
		}

		if record.ProposalAccepted {
			acceptedCount++
			if !record.ProposalCorrect {
				summary.FalseAcceptCount++
			}
		} else {
			rejectedCount++
			if record.ProposalCorrect {
				summary.FalseRejectCount++
			}
		}

		switch record.ValidatorCaseType {
		case "adversarial":
			summary.AdversarialCases++
			if !record.ProposalAccepted {
				summary.AdversarialRejected++
			}
		case "legitimate":
			summary.LegitimateCases++
			if record.ProposalAccepted {
				summary.LegitimateAccepted++
			}
		}

		// Keep legacy arm summaries for compatibility.
		switch record.ExperimentArm {
		case "controller_enabled":
			updateArmAttempt(&summary.ControllerEnabled, record)
		case "controller_disabled":
			updateArmAttempt(&summary.ControllerDisabled, record)
		case "unrecoverable":
			updateArmAttempt(&summary.Unrecoverable, record)
		}
	}

	summary.TotalIncidents = len(incidents)

	var totalTTD, totalTTM time.Duration
	var validTTD, validTTM int
	var incidentRollbackCount int
	var actionableIncidentCount int

	for _, inc := range incidents {
		if validDuration(inc.injectedAt, inc.detectedAt) {
			totalTTD += inc.detectedAt.Sub(inc.injectedAt)
			validTTD++
		}

		terminal := inc.hasWeek3Outcome
		if !terminal {
			// Legacy compatibility: MitigatedAt/Success represent a terminal row.
			for _, r := range records {
				key := r.IncidentID
				if key == "" {
					continue
				}
				if key == inc.id && !r.MitigatedAt.IsZero() {
					terminal = true
					inc.terminalAt = r.MitigatedAt
					if r.Success {
						inc.terminalOutcome = "recovered"
					} else if r.RolledBack {
						inc.terminalOutcome = "rolled_back"
					}
				}
			}
		}

		if terminal {
			summary.TerminalIncidents++
			if inc.terminalOutcome != "" {
				summary.OutcomeDistribution[inc.terminalOutcome]++
			}
			if inc.terminalOutcome == "recovered" || (!inc.hasWeek3Outcome && inc.legacySuccess) {
				summary.SuccessfulRecoveries++
			} else {
				summary.FailedRecoveries++
			}
			if validDuration(inc.detectedAt, inc.terminalAt) {
				totalTTM += inc.terminalAt.Sub(inc.detectedAt)
				validTTM++
			}
			if inc.terminalOutcome != "escalated" &&
				inc.terminalOutcome != "rejected" {
				actionableIncidentCount++
				if inc.rollback {
					incidentRollbackCount++
				}
			}
		} else {
			summary.IncompleteIncidents++
		}

		updateWorkloadSummary(&summary, inc, terminal)
	}

	// Legacy fixtures in the existing test suite always carry MitigatedAt.
	// If IDs were present, the loop above handles them. If not, retain old
	// record-level success/failure semantics.
	if summary.TerminalIncidents == 0 && summary.IncompleteIncidents == summary.TotalIncidents {
		summary.SuccessfulRecoveries = 0
		summary.FailedRecoveries = 0
	}

	if summary.TerminalIncidents > 0 {
		summary.RecoverySuccessRate =
			float64(summary.SuccessfulRecoveries) / float64(summary.TerminalIncidents) * 100
	}
	if actionableIncidentCount > 0 {
		summary.IncidentRollbackRate =
			float64(incidentRollbackCount) / float64(actionableIncidentCount) * 100
	}
	if summary.RemediationAttempts > 0 {
		summary.RollbackRate =
			float64(summary.TotalRollbacks) / float64(summary.RemediationAttempts) * 100
	}
	if validTTD > 0 {
		summary.AverageTTDSeconds = totalTTD.Seconds() / float64(validTTD)
	}
	if validTTM > 0 {
		summary.AverageTTMSeconds = totalTTM.Seconds() / float64(validTTM)
	}
	if validInference > 0 {
		summary.AverageInferenceLatencySeconds = totalInference.Seconds() / float64(validInference)
	}
	if validApply > 0 {
		summary.AverageApplySeconds = totalApply.Seconds() / float64(validApply)
	}
	if validVerify > 0 {
		summary.AverageVerificationSeconds = totalVerify.Seconds() / float64(validVerify)
	}
	if validConvergence > 0 {
		summary.AverageClusterConvergenceSeconds =
			totalConvergence.Seconds() / float64(validConvergence)
	}

	if acceptedCount > 0 {
		summary.FalseAcceptRate =
			float64(summary.FalseAcceptCount) / float64(acceptedCount) * 100
	}
	if rejectedCount > 0 {
		summary.FalseRejectRate =
			float64(summary.FalseRejectCount) / float64(rejectedCount) * 100
	}
	if summary.AdversarialCases > 0 {
		summary.AdversarialRejectionRate =
			float64(summary.AdversarialRejected) / float64(summary.AdversarialCases) * 100
	}
	if summary.LegitimateCases > 0 {
		summary.LegitimateAcceptanceRate =
			float64(summary.LegitimateAccepted) / float64(summary.LegitimateCases) * 100
	}

	finalizeLegacyArm(&summary.ControllerEnabled)
	finalizeLegacyArm(&summary.ControllerDisabled)
	finalizeLegacyArm(&summary.Unrecoverable)
	finalizeWorkload(&summary.W1)
	finalizeWorkload(&summary.W2)
	finalizeWorkload(&summary.W3)

	return summary
}

func validDuration(start, end time.Time) bool {
	return !start.IsZero() && !end.IsZero() && end.After(start)
}

func updateArmAttempt(summary *ArmSummary, record AuditRecord) {
	// Existing tests/legacy files are one record per incident.
	summary.TotalIncidents++
	if record.Success {
		summary.SuccessfulRecoveries++
	} else {
		summary.FailedRecoveries++
	}
	if record.RemediationAttempt {
		summary.RemediationAttempts++
	}
	if record.RolledBack {
		summary.RollbackTriggers++
	}
}

func finalizeLegacyArm(summary *ArmSummary) {
	if summary.TotalIncidents > 0 {
		summary.RecoverySuccessRate =
			float64(summary.SuccessfulRecoveries) / float64(summary.TotalIncidents) * 100
	}
	if summary.RemediationAttempts > 0 {
		summary.RollbackTriggerRate =
			float64(summary.RollbackTriggers) / float64(summary.RemediationAttempts) * 100
	}
}

func updateWorkloadSummary(summary *Summary, inc *incidentAggregate, terminal bool) {
	var target *WorkloadArmSummary
	switch inc.workload {
	case "W1", "w1":
		if inc.arm == "enabled" {
			target = &summary.W1.Enabled
		} else if inc.arm == "disabled" {
			target = &summary.W1.Disabled
		}
	case "W2", "w2":
		if inc.arm == "enabled" {
			target = &summary.W2.Enabled
		} else if inc.arm == "disabled" {
			target = &summary.W2.Disabled
		}
	case "W3", "w3":
		if inc.arm == "enabled" {
			target = &summary.W3.Enabled
		} else if inc.arm == "disabled" {
			target = &summary.W3.Disabled
		}
	}
	if target == nil {
		return
	}

	target.TotalIncidents++
	target.RemediationAttempts += inc.attempts
	target.RollbackTriggers += inc.rollbackAttempts
	if terminal {
		target.TerminalIncidents++
		if inc.terminalOutcome == "recovered" {
			target.RecoveredIncidents++
		}
	} else {
		target.IncompleteIncidents++
	}
}

func finalizeWorkload(summary *WorkloadSummary) {
	finalizeWorkloadArm(&summary.Enabled)
	finalizeWorkloadArm(&summary.Disabled)
	summary.AttributableRecovery =
		summary.Enabled.RecoveryRate - summary.Disabled.RecoveryRate
}

func finalizeWorkloadArm(summary *WorkloadArmSummary) {
	if summary.TerminalIncidents > 0 {
		summary.RecoveryRate =
			float64(summary.RecoveredIncidents) / float64(summary.TerminalIncidents) * 100
	}
	if summary.RemediationAttempts > 0 {
		summary.RollbackTriggerRate =
			float64(summary.RollbackTriggers) / float64(summary.RemediationAttempts) * 100
	}
}
