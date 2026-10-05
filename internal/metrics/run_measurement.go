package metrics

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	RecoverySourceController         = "controller"
	RecoverySourceUnaided            = "unaided"
	RecoverySourceNone               = "none"
	RecoverySourceNotRecoveredWithin = "not_recovered_within_300s"
)

const (
	DisabledObservationCutoff = 300 * time.Second
	ReadyStabilityWindow      = 60 * time.Second
)

// RunMeta is the per-run archive metadata stored beside audit.jsonl.
type RunMeta struct {
	RunID                    string     `json:"runID,omitempty"`
	Run                      string     `json:"run,omitempty"`
	Workload                 string     `json:"workload"`
	Arm                      string     `json:"arm"`
	InjectedAt               time.Time  `json:"injectedAt"`
	ObservationCutoffSeconds int        `json:"observationCutoffSeconds,omitempty"`
	ObservationCutoffAt      *time.Time `json:"observationCutoffAt,omitempty"`
	SetupSucceeded           *bool      `json:"setupSucceeded,omitempty"`
	SetupError               string     `json:"setupError,omitempty"`
	InjectionSucceeded       *bool      `json:"injectionSucceeded,omitempty"`
	InjectionExitCode        int        `json:"injectionExitCode,omitempty"`
	InjectionError           string     `json:"injectionError,omitempty"`
	Recovered                *bool      `json:"recovered,omitempty"`
	RecoveryTimestamp        *time.Time `json:"recoveryTimestamp,omitempty"`
	ObservedUntil            *time.Time `json:"observedUntil,omitempty"`
	RecoverySource           string     `json:"recoverySource,omitempty"`
}

type RunArchive struct {
	Meta    RunMeta
	Records []AuditRecord
}

func LoadRunMeta(path string) (RunMeta, error) {
	file, err := os.Open(path)
	if err != nil {
		return RunMeta{}, fmt.Errorf("open run metadata file %q: %w", path, err)
	}
	defer file.Close()
	return DecodeRunMeta(file)
}

func DecodeRunMeta(reader io.Reader) (RunMeta, error) {
	var meta RunMeta
	if err := json.NewDecoder(reader).Decode(&meta); err != nil {
		return RunMeta{}, fmt.Errorf("decode run metadata: %w", err)
	}
	if err := ValidateRunMeta(meta); err != nil {
		return RunMeta{}, err
	}
	return meta, nil
}

func ValidateRunMeta(meta RunMeta) error {
	if strings.TrimSpace(meta.Workload) == "" {
		return fmt.Errorf("run metadata: workload is required")
	}
	if err := validateWorkloadArm(meta.Workload, meta.Arm); err != nil {
		return err
	}
	if meta.InjectedAt.IsZero() {
		return fmt.Errorf("run metadata %q: injectedAt is required", meta.RunID)
	}
	switch meta.RecoverySource {
	case "", RecoverySourceController, RecoverySourceUnaided, RecoverySourceNone, RecoverySourceNotRecoveredWithin:
	default:
		return fmt.Errorf("run metadata %q: invalid recoverySource %q", meta.RunID, meta.RecoverySource)
	}
	if meta.Arm == "disabled" {
		if meta.ObservationCutoffSeconds == 0 {
			return fmt.Errorf("disabled run %q: observationCutoffSeconds is required", meta.RunID)
		}
		if time.Duration(meta.ObservationCutoffSeconds)*time.Second != DisabledObservationCutoff {
			return fmt.Errorf("disabled run %q: observationCutoffSeconds must be 300", meta.RunID)
		}
		if meta.RecoverySource == RecoverySourceController {
			return fmt.Errorf("disabled run %q: recoverySource cannot be controller", meta.RunID)
		}
	}
	if meta.InjectionSucceeded != nil && !*meta.InjectionSucceeded && meta.Recovered != nil {
		return fmt.Errorf("run metadata %q: failed injection must not carry recovery observation", meta.RunID)
	}
	if meta.SetupSucceeded != nil && !*meta.SetupSucceeded && meta.Recovered != nil {
		return fmt.Errorf("run metadata %q: failed setup must not carry recovery observation", meta.RunID)
	}
	if meta.Recovered != nil && *meta.Recovered {
		if meta.RecoveryTimestamp == nil {
			return fmt.Errorf("run metadata %q: recovered=true requires recoveryTimestamp", meta.RunID)
		}
		if meta.RecoverySource == "" || meta.RecoverySource == RecoverySourceNone ||
			meta.RecoverySource == RecoverySourceNotRecoveredWithin {
			return fmt.Errorf("run metadata %q: recovered=true requires controller or unaided recoverySource", meta.RunID)
		}
	}
	if meta.Recovered != nil && !*meta.Recovered &&
		meta.RecoverySource != "" && meta.RecoverySource != RecoverySourceNone &&
		meta.RecoverySource != RecoverySourceNotRecoveredWithin {
		return fmt.Errorf("run metadata %q: recovered=false requires none/not_recovered_within_300s recoverySource", meta.RunID)
	}
	return nil
}

func ApplyRunMeta(records []AuditRecord, meta RunMeta) ([]AuditRecord, error) {
	if err := ValidateRunMeta(meta); err != nil {
		return nil, err
	}
	joined := append([]AuditRecord(nil), records...)
	for i := range joined {
		if joined[i].Workload != "" && joined[i].Workload != meta.Workload {
			return nil, fmt.Errorf("audit workload mismatch for run %q: %q != %q", meta.RunID, joined[i].Workload, meta.Workload)
		}
		if joined[i].ExperimentArm != "" && joined[i].ExperimentArm != meta.Arm {
			return nil, fmt.Errorf("audit arm mismatch for run %q: %q != %q", meta.RunID, joined[i].ExperimentArm, meta.Arm)
		}
		joined[i].Workload = meta.Workload
		joined[i].ExperimentArm = meta.Arm
		joined[i].InjectedAt = meta.InjectedAt
	}
	return joined, nil
}

func LoadRunArchive(runDir string) (RunArchive, error) {
	meta, err := LoadRunMeta(filepath.Join(runDir, "meta.json"))
	if err != nil {
		return RunArchive{}, err
	}
	records, err := LoadAuditRecords(filepath.Join(runDir, "audit.jsonl"))
	if err != nil {
		if meta.Arm == "disabled" && errors.Is(err, os.ErrNotExist) {
			return RunArchive{Meta: meta}, nil
		}
		return RunArchive{}, err
	}
	records, err = ApplyRunMeta(records, meta)
	if err != nil {
		return RunArchive{}, err
	}
	meta = inferEnabledRecovery(meta, records)
	return RunArchive{Meta: meta, Records: records}, nil
}

func inferEnabledRecovery(meta RunMeta, records []AuditRecord) RunMeta {
	if meta.Arm != "enabled" || meta.Recovered != nil {
		return meta
	}
	for _, record := range records {
		switch record.TerminalOutcome {
		case "recovered":
			recovered := true
			meta.Recovered = &recovered
			meta.RecoverySource = RecoverySourceController
			if !record.TerminalAt.IsZero() {
				recoveredAt := record.TerminalAt
				meta.RecoveryTimestamp = &recoveredAt
			}
			return meta
		case "exhausted", "escalated", "rejected":
			recovered := false
			meta.Recovered = &recovered
			meta.RecoverySource = RecoverySourceNone
			if !record.TerminalAt.IsZero() {
				observedUntil := record.TerminalAt
				meta.ObservedUntil = &observedUntil
			}
			return meta
		}
	}
	return meta
}

func validateWorkloadArm(workload, arm string) error {
	switch workload {
	case "W1", "W2", "W3":
	default:
		return fmt.Errorf("invalid workload %q", workload)
	}
	switch arm {
	case "enabled", "disabled":
	default:
		return fmt.Errorf("invalid arm %q", arm)
	}
	return nil
}

type ReadyObservation struct {
	Timestamp time.Time
	Ready     bool
}

func RecoveryTimestamp(
	observations []ReadyObservation,
	stabilityWindow time.Duration,
) (time.Time, bool) {
	if stabilityWindow <= 0 {
		return time.Time{}, false
	}
	var readySince time.Time
	for _, observation := range observations {
		if observation.Timestamp.IsZero() {
			continue
		}
		if !observation.Ready {
			readySince = time.Time{}
			continue
		}
		if readySince.IsZero() {
			readySince = observation.Timestamp
		}
		if !observation.Timestamp.Before(readySince.Add(stabilityWindow)) {
			return observation.Timestamp, true
		}
	}
	return time.Time{}, false
}

func RecoveryTimestampWithinCutoff(
	observations []ReadyObservation,
	injectedAt time.Time,
	cutoff time.Duration,
	stabilityWindow time.Duration,
) (time.Time, bool) {
	recoveredAt, ok := RecoveryTimestamp(observations, stabilityWindow)
	if !ok || injectedAt.IsZero() || cutoff <= 0 {
		return time.Time{}, false
	}
	if recoveredAt.After(injectedAt.Add(cutoff)) {
		return time.Time{}, false
	}
	return recoveredAt, true
}
