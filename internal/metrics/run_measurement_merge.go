package metrics

import "fmt"

type ExperimentArmRecovery struct {
	Runs          int     `json:"runs"`
	InvalidRuns   int     `json:"invalid_runs"`
	RecoveredRuns int     `json:"recovered_runs"`
	RecoveryRate  float64 `json:"recovery_rate"`
}

type ExperimentWorkloadRecovery struct {
	Enabled              ExperimentArmRecovery `json:"enabled"`
	Disabled             ExperimentArmRecovery `json:"disabled"`
	AttributableRecovery float64               `json:"attributable_recovery_percentage_points"`
}

type ExperimentRecoverySummary struct {
	ValidRuns   int                        `json:"valid_runs"`
	InvalidRuns int                        `json:"invalid_runs"`
	W1          ExperimentWorkloadRecovery `json:"w1"`
	W2          ExperimentWorkloadRecovery `json:"w2"`
	W3          ExperimentWorkloadRecovery `json:"w3"`
}

func CalculateExperimentRecovery(metas []RunMeta) (ExperimentRecoverySummary, error) {
	var summary ExperimentRecoverySummary
	for _, meta := range metas {
		if err := ValidateRunMeta(meta); err != nil {
			return ExperimentRecoverySummary{}, err
		}
		arm := experimentArmFor(&summary, meta.Workload, meta.Arm)
		if arm == nil {
			continue
		}
		if invalidRun(meta) {
			summary.InvalidRuns++
			arm.InvalidRuns++
			continue
		}
		if meta.Recovered == nil {
			return ExperimentRecoverySummary{}, missingRecoveryObservationError(meta)
		}
		summary.ValidRuns++
		arm.Runs++
		if *meta.Recovered {
			arm.RecoveredRuns++
		}
	}

	finalizeExperimentWorkload(&summary.W1)
	finalizeExperimentWorkload(&summary.W2)
	finalizeExperimentWorkload(&summary.W3)
	return summary, nil
}

func invalidRun(meta RunMeta) bool {
	return (meta.SetupSucceeded != nil && !*meta.SetupSucceeded) ||
		(meta.InjectionSucceeded != nil && !*meta.InjectionSucceeded)
}

func missingRecoveryObservationError(meta RunMeta) error {
	id := meta.RunID
	if id == "" {
		id = meta.Run
	}
	return fmt.Errorf("run metadata %q: recovered observation is required for experiment recovery", id)
}

func experimentArmFor(
	summary *ExperimentRecoverySummary,
	workload string,
	arm string,
) *ExperimentArmRecovery {
	var workloadSummary *ExperimentWorkloadRecovery
	switch workload {
	case "W1":
		workloadSummary = &summary.W1
	case "W2":
		workloadSummary = &summary.W2
	case "W3":
		workloadSummary = &summary.W3
	default:
		return nil
	}
	switch arm {
	case "enabled":
		return &workloadSummary.Enabled
	case "disabled":
		return &workloadSummary.Disabled
	default:
		return nil
	}
}

func finalizeExperimentWorkload(summary *ExperimentWorkloadRecovery) {
	finalizeExperimentArm(&summary.Enabled)
	finalizeExperimentArm(&summary.Disabled)
	summary.AttributableRecovery =
		summary.Enabled.RecoveryRate - summary.Disabled.RecoveryRate
}

func finalizeExperimentArm(summary *ExperimentArmRecovery) {
	if summary.Runs > 0 {
		summary.RecoveryRate =
			float64(summary.RecoveredRuns) / float64(summary.Runs) * 100
	}
}
