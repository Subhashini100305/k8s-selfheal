package metrics

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunMetaValidationForDisabledObservation(t *testing.T) {
	injectedAt := time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)
	recovered := false
	meta := RunMeta{
		RunID:                    "W1-disabled-1",
		Workload:                 "W1",
		Arm:                      "disabled",
		InjectedAt:               injectedAt,
		ObservationCutoffSeconds: 300,
		Recovered:                &recovered,
		RecoverySource:           RecoverySourceNotRecoveredWithin,
	}
	if err := ValidateRunMeta(meta); err != nil {
		t.Fatalf("ValidateRunMeta() error = %v", err)
	}
}

func TestDisabledObservationRequires300SecondCutoff(t *testing.T) {
	injectedAt := time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)
	meta := RunMeta{
		RunID:      "W2-disabled-1",
		Workload:   "W2",
		Arm:        "disabled",
		InjectedAt: injectedAt,
	}
	if err := ValidateRunMeta(meta); err == nil {
		t.Fatal("expected disabled run without cutoff to fail")
	}
	meta.ObservationCutoffSeconds = 299
	if err := ValidateRunMeta(meta); err == nil {
		t.Fatal("expected disabled run with non-300 cutoff to fail")
	}
}

func TestReadyAtFourMinutesFiftyDoesNotQualifyWithinCutoff(t *testing.T) {
	injectedAt := time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)
	got, ok := RecoveryTimestampWithinCutoff([]ReadyObservation{
		{Timestamp: injectedAt.Add(4*time.Minute + 50*time.Second), Ready: true},
		{Timestamp: injectedAt.Add(5*time.Minute + 50*time.Second), Ready: true},
	}, injectedAt, DisabledObservationCutoff, ReadyStabilityWindow)
	if ok || !got.IsZero() {
		t.Fatalf("Ready at 4m50s must not qualify by 300s cutoff, got %s/%v", got, ok)
	}
}

func TestRecoveryTimestampWithinCutoffRequiresStableWindow(t *testing.T) {
	injectedAt := time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)
	got, ok := RecoveryTimestampWithinCutoff([]ReadyObservation{
		{Timestamp: injectedAt.Add(2 * time.Minute), Ready: true},
		{Timestamp: injectedAt.Add(3 * time.Minute), Ready: true},
	}, injectedAt, DisabledObservationCutoff, ReadyStabilityWindow)
	if !ok || !got.Equal(injectedAt.Add(3*time.Minute)) {
		t.Fatalf("recovery timestamp = %s/%v, want 3m true", got, ok)
	}
}

func TestFailedInjectionDoesNotCountTowardRecoveryN(t *testing.T) {
	yes := true
	failed := false
	ok := true
	injectedAt := time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)
	recoveredAt := injectedAt.Add(2 * time.Minute)
	summary, err := CalculateExperimentRecovery([]RunMeta{
		{RunID: "valid", Workload: "W1", Arm: "enabled", InjectedAt: injectedAt, InjectionSucceeded: &ok, Recovered: &yes, RecoveryTimestamp: &recoveredAt, RecoverySource: RecoverySourceController},
		{RunID: "failed", Workload: "W1", Arm: "enabled", InjectedAt: injectedAt, InjectionSucceeded: &failed},
	})
	if err != nil {
		t.Fatalf("CalculateExperimentRecovery() error = %v", err)
	}
	if summary.W1.Enabled.Runs != 1 || summary.W1.Enabled.RecoveredRuns != 1 {
		t.Fatalf("failed injection must not count toward N: %+v", summary.W1.Enabled)
	}
	if summary.ValidRuns != 1 || summary.InvalidRuns != 1 || summary.W1.Enabled.InvalidRuns != 1 {
		t.Fatalf("unexpected valid/invalid counts: %+v", summary)
	}
}

func TestMissingRecoveryObservationFailsClearly(t *testing.T) {
	injectedAt := time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)
	_, err := CalculateExperimentRecovery([]RunMeta{{
		RunID:      "missing-observation",
		Workload:   "W1",
		Arm:        "enabled",
		InjectedAt: injectedAt,
	}})
	if err == nil {
		t.Fatal("expected missing recovered observation to fail")
	}
}

func TestTransientReadyResetsStabilityTimer(t *testing.T) {
	injectedAt := time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)
	got, ok := RecoveryTimestampWithinCutoff([]ReadyObservation{
		{Timestamp: injectedAt.Add(30 * time.Second), Ready: true},
		{Timestamp: injectedAt.Add(45 * time.Second), Ready: false},
		{Timestamp: injectedAt.Add(2 * time.Minute), Ready: true},
		{Timestamp: injectedAt.Add(2*time.Minute + 59*time.Second), Ready: true},
		{Timestamp: injectedAt.Add(3 * time.Minute), Ready: true},
	}, injectedAt, DisabledObservationCutoff, ReadyStabilityWindow)
	if !ok || !got.Equal(injectedAt.Add(3*time.Minute)) {
		t.Fatalf("recovery timestamp = %s/%v, want stability reset and 3m true", got, ok)
	}
}

func TestW1AbandonedUnaidedRecoveryStaysControllerAbandoned(t *testing.T) {
	yes := true
	injectedAt := time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)
	recoveredAt := injectedAt.Add(3 * time.Minute)
	records, err := ApplyRunMeta([]AuditRecord{{
		IncidentID:         "incident-w1",
		AttemptNumber:      1,
		RemediationAttempt: true,
		RolledBack:         true,
	}}, RunMeta{
		RunID:             "w1-enabled",
		Workload:          "W1",
		Arm:               "enabled",
		InjectedAt:        injectedAt,
		Recovered:         &yes,
		RecoveryTimestamp: &recoveredAt,
		RecoverySource:    RecoverySourceUnaided,
	})
	if err != nil {
		t.Fatalf("ApplyRunMeta() error = %v", err)
	}
	controllerSummary := Calculate(records)
	if controllerSummary.AbandonedIncidents != 1 ||
		controllerSummary.SuccessfulRecoveries != 0 ||
		controllerSummary.TotalRollbacks != 1 {
		t.Fatalf("unexpected controller summary: %+v", controllerSummary)
	}
	experimentSummary, err := CalculateExperimentRecovery([]RunMeta{{
		RunID:             "w1-enabled",
		Workload:          "W1",
		Arm:               "enabled",
		InjectedAt:        injectedAt,
		Recovered:         &yes,
		RecoveryTimestamp: &recoveredAt,
		RecoverySource:    RecoverySourceUnaided,
	}})
	if err != nil {
		t.Fatalf("CalculateExperimentRecovery() error = %v", err)
	}
	if experimentSummary.W1.Enabled.RecoveryRate != 100 {
		t.Fatalf("W1 experiment recovery = %+v, want 100%%", experimentSummary.W1.Enabled)
	}
}

func TestPerWorkloadAttributableRecoveryAndFrozenMatrixRepresentable(t *testing.T) {
	yes := true
	no := false
	injectedAt := time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)
	recoveredAt := injectedAt.Add(2 * time.Minute)
	var metas []RunMeta
	add := func(workload, arm string, n int, recovered bool) {
		for i := 0; i < n; i++ {
			meta := RunMeta{
				RunID:      workload + "-" + arm,
				Workload:   workload,
				Arm:        arm,
				InjectedAt: injectedAt,
			}
			if arm == "disabled" {
				meta.ObservationCutoffSeconds = 300
			}
			if recovered {
				meta.Recovered = &yes
				meta.RecoveryTimestamp = &recoveredAt
				meta.RecoverySource = RecoverySourceController
				if arm == "disabled" {
					meta.RecoverySource = RecoverySourceUnaided
				}
			} else {
				meta.Recovered = &no
				meta.RecoverySource = RecoverySourceNone
				if arm == "disabled" {
					meta.RecoverySource = RecoverySourceNotRecoveredWithin
				}
			}
			metas = append(metas, meta)
		}
	}
	add("W1", "enabled", 5, true)
	add("W1", "disabled", 5, false)
	add("W2", "enabled", 5, true)
	add("W2", "disabled", 5, false)
	add("W3", "enabled", 5, false)
	add("W3", "disabled", 3, false)

	summary, err := CalculateExperimentRecovery(metas)
	if err != nil {
		t.Fatalf("CalculateExperimentRecovery() error = %v", err)
	}
	if summary.W1.Enabled.Runs != 5 || summary.W1.Disabled.Runs != 5 ||
		summary.W2.Enabled.Runs != 5 || summary.W2.Disabled.Runs != 5 ||
		summary.W3.Enabled.Runs != 5 || summary.W3.Disabled.Runs != 3 {
		t.Fatalf("frozen matrix not represented: %+v", summary)
	}
	if summary.ValidRuns != 28 || summary.InvalidRuns != 0 {
		t.Fatalf("valid/invalid runs = %d/%d, want 28/0", summary.ValidRuns, summary.InvalidRuns)
	}
	if summary.W1.AttributableRecovery != 100 ||
		summary.W2.AttributableRecovery != 100 ||
		summary.W3.AttributableRecovery != 0 {
		t.Fatalf("unexpected attributable recovery: %+v", summary)
	}
}

func TestLoadRunArchiveAppliesMetaAndAllowsDisabledNoAudit(t *testing.T) {
	dir := t.TempDir()
	runDir := filepath.Join(dir, "B1-03")
	if err := os.Mkdir(runDir, 0o700); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	injectedAt := time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)
	meta := []byte(`{"runID":"B1-03","workload":"W2","arm":"enabled","injectedAt":"2026-10-05T10:00:00Z"}`)
	if err := os.WriteFile(filepath.Join(runDir, "meta.json"), meta, 0o600); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	audit := []byte(`{"incidentID":"i1","attemptNumber":1,"timestamp":"2026-10-05T10:00:05Z","pod":"shop/pod-1","state":"DETECTED","action":"restart_pod","result":"received"}` + "\n")
	if err := os.WriteFile(filepath.Join(runDir, "audit.jsonl"), audit, 0o600); err != nil {
		t.Fatalf("write audit: %v", err)
	}
	archive, err := LoadRunArchive(runDir)
	if err != nil {
		t.Fatalf("LoadRunArchive() error = %v", err)
	}
	if len(archive.Records) != 1 ||
		archive.Records[0].Workload != "W2" ||
		archive.Records[0].ExperimentArm != "enabled" ||
		!archive.Records[0].InjectedAt.Equal(injectedAt) {
		t.Fatalf("metadata not applied: %+v", archive)
	}

	disabledDir := filepath.Join(dir, "C2-01")
	if err := os.Mkdir(disabledDir, 0o700); err != nil {
		t.Fatalf("mkdir disabled dir: %v", err)
	}
	disabledMeta := []byte(`{"runID":"C2-01","workload":"W3","arm":"disabled","injectedAt":"2026-10-05T10:00:00Z","observationCutoffSeconds":300}`)
	if err := os.WriteFile(filepath.Join(disabledDir, "meta.json"), disabledMeta, 0o600); err != nil {
		t.Fatalf("write disabled meta: %v", err)
	}
	disabledArchive, err := LoadRunArchive(disabledDir)
	if err != nil {
		t.Fatalf("LoadRunArchive(disabled) error = %v", err)
	}
	if len(disabledArchive.Records) != 0 {
		t.Fatalf("disabled run must have zero audit records: %+v", disabledArchive.Records)
	}
}

func TestLoadRunArchiveInfersEnabledControllerRecovery(t *testing.T) {
	dir := t.TempDir()
	runDir := filepath.Join(dir, "B1-01")
	if err := os.Mkdir(runDir, 0o700); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	meta := []byte(`{"runID":"B1-01","workload":"W2","arm":"enabled","injectedAt":"2026-10-05T10:00:00Z","injectionSucceeded":true}`)
	if err := os.WriteFile(filepath.Join(runDir, "meta.json"), meta, 0o600); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	audit := []byte(
		`{"incidentID":"i1","attemptNumber":1,"timestamp":"2026-10-05T10:00:05Z","pod":"shop/pod-1","state":"REMEDIATING","action":"rollout_undo","result":"started"}` + "\n" +
			`{"incidentID":"i1","attemptNumber":1,"timestamp":"2026-10-05T10:01:10Z","pod":"shop/pod-1","state":"CLOSED","action":"rollout_undo","result":"recovered","classifierMillis":250}` + "\n",
	)
	if err := os.WriteFile(filepath.Join(runDir, "audit.jsonl"), audit, 0o600); err != nil {
		t.Fatalf("write audit: %v", err)
	}

	archive, err := LoadRunArchive(runDir)
	if err != nil {
		t.Fatalf("LoadRunArchive() error = %v", err)
	}
	if archive.Meta.Recovered == nil || !*archive.Meta.Recovered ||
		archive.Meta.RecoverySource != RecoverySourceController {
		t.Fatalf("enabled controller recovery not inferred: %+v", archive.Meta)
	}
}

func TestDecodeRunMetaRejectsMalformedJSON(t *testing.T) {
	if _, err := DecodeRunMeta(bytes.NewBufferString("{")); err == nil {
		t.Fatal("expected malformed metadata JSON to fail")
	}
}
