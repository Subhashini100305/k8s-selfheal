package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aryausingh/k8s-selfheal/internal/metrics"
)

func TestLoadRunArchivesSingleAndMultipleRunDirectories(t *testing.T) {
	root := t.TempDir()
	enabled := filepath.Join(root, "B1-01")
	disabled := filepath.Join(root, "B2-01")
	writeReportRun(
		t,
		enabled,
		`{"runID":"B1-01","workload":"W2","arm":"enabled","injectedAt":"2026-10-05T10:00:00Z","injectionSucceeded":true}`,
		`{"incidentID":"i1","attemptNumber":1,"timestamp":"2026-10-05T10:00:05Z","pod":"shop/pod-1","state":"REMEDIATING","action":"rollout_undo","result":"started"}`+"\n"+
			`{"incidentID":"i1","attemptNumber":1,"timestamp":"2026-10-05T10:01:05Z","pod":"shop/pod-1","state":"CLOSED","action":"rollout_undo","result":"recovered","classifierMillis":125}`+"\n",
	)
	writeReportRun(
		t,
		disabled,
		`{"runID":"B2-01","workload":"W2","arm":"disabled","injectedAt":"2026-10-05T10:00:00Z","observationCutoffSeconds":300,"injectionSucceeded":true,"recovered":false,"recoverySource":"not_recovered_within_300s"}`,
		"",
	)

	archives, err := loadRunArchives(enabled, "")
	if err != nil {
		t.Fatalf("loadRunArchives(single) error = %v", err)
	}
	if len(archives) != 1 || archives[0].Meta.RunID != "B1-01" {
		t.Fatalf("unexpected single archive load: %+v", archives)
	}

	archives, err = loadRunArchives("", root)
	if err != nil {
		t.Fatalf("loadRunArchives(multiple) error = %v", err)
	}
	if len(archives) != 2 {
		t.Fatalf("expected 2 archives, got %d", len(archives))
	}
	metas := []metrics.RunMeta{archives[0].Meta, archives[1].Meta}
	summary, err := metrics.CalculateExperimentRecovery(metas)
	if err != nil {
		t.Fatalf("CalculateExperimentRecovery() error = %v", err)
	}
	if summary.ValidRuns != 2 || summary.W2.Enabled.Runs != 1 || summary.W2.Disabled.Runs != 1 {
		t.Fatalf("unexpected report recovery summary: %+v", summary)
	}
}

func TestLoadRunArchivesRejectsEnabledMissingAudit(t *testing.T) {
	root := t.TempDir()
	enabled := filepath.Join(root, "A1-01")
	if err := os.Mkdir(enabled, 0o700); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(enabled, "meta.json"),
		[]byte(`{"runID":"A1-01","workload":"W1","arm":"enabled","injectedAt":"2026-10-05T10:00:00Z"}`),
		0o600,
	); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	if _, err := loadRunArchives(enabled, ""); err == nil {
		t.Fatal("expected enabled run without audit.jsonl to fail")
	}
}

func writeReportRun(t *testing.T, dir string, meta string, audit string) {
	t.Helper()
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), []byte(meta), 0o600); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	if audit != "" {
		if err := os.WriteFile(filepath.Join(dir, "audit.jsonl"), []byte(audit), 0o600); err != nil {
			t.Fatalf("write audit: %v", err)
		}
	}
}
