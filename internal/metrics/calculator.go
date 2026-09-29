package metrics

import "time"

// Calculate calculates runtime metrics from incident records.
func Calculate(records []AuditRecord) Summary {
	summary := Summary{
		TotalIncidents: len(records),
	}

	if len(records) == 0 {
		return summary
	}

	var totalTTDDuration time.Duration
	var totalTTMDuration time.Duration
	var totalInferenceDuration time.Duration
	var totalClusterConvergenceDuration time.Duration

	var validTTDRecords int
	var validTTMRecords int
	var validInferenceRecords int
	var validClusterConvergenceRecords int

	var acceptedCount int
	var rejectedCount int

	for _, record := range records {
		// --------------------------------------------------------
		// Overall recovery metrics.
		// --------------------------------------------------------
		if record.Success {
			summary.SuccessfulRecoveries++
		} else {
			summary.FailedRecoveries++
		}

		if record.RolledBack {
			summary.TotalRollbacks++
		}

		if record.RemediationAttempt {
			summary.RemediationAttempts++
		}

		// --------------------------------------------------------
		// TTD = detected - injected.
		// --------------------------------------------------------
		if !record.InjectedAt.IsZero() &&
			!record.DetectedAt.IsZero() &&
			record.DetectedAt.After(record.InjectedAt) {

			totalTTDDuration +=
				record.DetectedAt.Sub(record.InjectedAt)

			validTTDRecords++
		}

		// --------------------------------------------------------
		// TTM = mitigated - detected.
		// --------------------------------------------------------
		if !record.DetectedAt.IsZero() &&
			!record.MitigatedAt.IsZero() &&
			record.MitigatedAt.After(record.DetectedAt) {

			totalTTMDuration +=
				record.MitigatedAt.Sub(record.DetectedAt)

			validTTMRecords++
		}

		// --------------------------------------------------------
		// Inference latency =
		// classifier completed - classifier started.
		// --------------------------------------------------------
		if !record.ClassifierStartedAt.IsZero() &&
			!record.ClassifierCompletedAt.IsZero() &&
			record.ClassifierCompletedAt.After(
				record.ClassifierStartedAt,
			) {

			totalInferenceDuration +=
				record.ClassifierCompletedAt.Sub(
					record.ClassifierStartedAt,
				)

			validInferenceRecords++
		}

		// --------------------------------------------------------
		// Cluster convergence =
		// verification completed - action started.
		// --------------------------------------------------------
		if !record.ActionStartedAt.IsZero() &&
			!record.VerificationCompletedAt.IsZero() &&
			record.VerificationCompletedAt.After(
				record.ActionStartedAt,
			) {

			totalClusterConvergenceDuration +=
				record.VerificationCompletedAt.Sub(
					record.ActionStartedAt,
				)

			validClusterConvergenceRecords++
		}

		// --------------------------------------------------------
		// Validator evaluation metrics.
		// --------------------------------------------------------
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

		// --------------------------------------------------------
		// Experimental-arm metrics.
		// --------------------------------------------------------
		switch record.ExperimentArm {
		case "controller_enabled":
			updateArmSummary(
				&summary.ControllerEnabled,
				record,
			)

		case "controller_disabled":
			updateArmSummary(
				&summary.ControllerDisabled,
				record,
			)

		case "unrecoverable":
			updateArmSummary(
				&summary.Unrecoverable,
				record,
			)
		}
	}

	// ------------------------------------------------------------
	// Overall rates.
	// ------------------------------------------------------------

	total := float64(summary.TotalIncidents)

	summary.RecoverySuccessRate =
		float64(summary.SuccessfulRecoveries) /
			total * 100

	// Rollback rate is specifically over remediation attempts,
	// not over the complete experimental population.
	if summary.RemediationAttempts > 0 {
		summary.RollbackRate =
			float64(summary.TotalRollbacks) /
				float64(summary.RemediationAttempts) * 100
	}

	// ------------------------------------------------------------
	// Existing TTD.
	// ------------------------------------------------------------

	if validTTDRecords > 0 {
		summary.AverageTTDSeconds =
			totalTTDDuration.Seconds() /
				float64(validTTDRecords)
	}

	// ------------------------------------------------------------
	// Existing TTM.
	// ------------------------------------------------------------

	if validTTMRecords > 0 {
		summary.AverageTTMSeconds =
			totalTTMDuration.Seconds() /
				float64(validTTMRecords)
	}

	// ------------------------------------------------------------
	// TTM decomposition: inference latency.
	// ------------------------------------------------------------

	if validInferenceRecords > 0 {
		summary.AverageInferenceLatencySeconds =
			totalInferenceDuration.Seconds() /
				float64(validInferenceRecords)
	}

	// ------------------------------------------------------------
	// TTM decomposition: cluster convergence.
	// ------------------------------------------------------------

	if validClusterConvergenceRecords > 0 {
		summary.AverageClusterConvergenceSeconds =
			totalClusterConvergenceDuration.Seconds() /
				float64(validClusterConvergenceRecords)
	}

	// ------------------------------------------------------------
	// Validator metrics.
	// ------------------------------------------------------------

	if acceptedCount > 0 {
		summary.FalseAcceptRate =
			float64(summary.FalseAcceptCount) /
				float64(acceptedCount) * 100
	}

	if rejectedCount > 0 {
		summary.FalseRejectRate =
			float64(summary.FalseRejectCount) /
				float64(rejectedCount) * 100
	}

	return summary
}

// updateArmSummary adds one incident to the selected arm's metrics.
func updateArmSummary(
	summary *ArmSummary,
	record AuditRecord,
) {
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

	if summary.TotalIncidents > 0 {
		summary.RecoverySuccessRate =
			float64(summary.SuccessfulRecoveries) /
				float64(summary.TotalIncidents) * 100
	}

	if summary.RemediationAttempts > 0 {
		summary.RollbackTriggerRate =
			float64(summary.RollbackTriggers) /
				float64(summary.RemediationAttempts) * 100
	}
}
