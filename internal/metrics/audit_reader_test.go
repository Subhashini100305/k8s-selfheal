package metrics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadAuditRecordsJSONArray(t *testing.T) {
	path := writeAuditFile(
		t,
		mustMarshalAuditRecords(
			t,
			[]AuditRecord{
				{
					IncidentID:         "incident-1",
					PodName:            "checkoutservice-abc123",
					Namespace:          "default",
					ClassifierProvider: "mock",
					EstimatedCostUSD:   0.25,
					CostKnown:          true,
				},
				{
					IncidentID: "incident-2",
					PodName:    "paymentservice-def456",
					Namespace:  "default",
				},
			},
		),
	)

	records, err := LoadAuditRecords(path)
	if err != nil {
		t.Fatalf("expected JSON array to load: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}

	if records[0].IncidentID != "incident-1" {
		t.Fatalf(
			"expected first incident ID incident-1, got %s",
			records[0].IncidentID,
		)
	}

	if records[0].ClassifierProvider != "mock" {
		t.Fatalf(
			"expected classifier provider to be preserved, got %s",
			records[0].ClassifierProvider,
		)
	}

	if !records[0].CostKnown ||
		records[0].EstimatedCostUSD != 0.25 {

		t.Fatalf(
			"expected cost fields to be preserved, got known=%v cost=%.2f",
			records[0].CostKnown,
			records[0].EstimatedCostUSD,
		)
	}
}

func TestLoadAuditRecordsJSONL(t *testing.T) {
	first := mustMarshalAuditRecord(
		t,
		AuditRecord{
			IncidentID: "incident-1",
			PodName:    "checkoutservice-abc123",
		},
	)
	second := mustMarshalAuditRecord(
		t,
		AuditRecord{
			IncidentID: "incident-2",
			PodName:    "paymentservice-def456",
		},
	)

	path := writeAuditFile(
		t,
		first+"\n\n"+second+"\n",
	)

	records, err := LoadAuditRecords(path)
	if err != nil {
		t.Fatalf("expected JSONL to load: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}

	if records[1].IncidentID != "incident-2" {
		t.Fatalf(
			"expected second incident ID incident-2, got %s",
			records[1].IncidentID,
		)
	}
}

func TestLoadAuditRecordsMalformedJSONReturnsError(t *testing.T) {
	path := writeAuditFile(
		t,
		`[{"incident_id":"incident-1"}`,
	)

	if _, err := LoadAuditRecords(path); err == nil {
		t.Fatal("expected malformed JSON to return an error")
	}
}

func TestLoadAuditRecordsMissingFileReturnsError(t *testing.T) {
	path := filepath.Join(
		t.TempDir(),
		"missing.jsonl",
	)

	if _, err := LoadAuditRecords(path); err == nil {
		t.Fatal("expected missing file to return an error")
	}
}

func TestLoadAuditRecordsEmptyFile(t *testing.T) {
	path := writeAuditFile(t, "")

	records, err := LoadAuditRecords(path)
	if err != nil {
		t.Fatalf("expected empty file to load safely: %v", err)
	}

	if len(records) != 0 {
		t.Fatalf("expected 0 records, got %d", len(records))
	}
}

func TestLoadAuditRecordsCanBeCalculated(t *testing.T) {
	baseTime := time.Date(
		2026,
		time.August,
		21,
		10,
		0,
		0,
		0,
		time.UTC,
	)

	path := writeAuditFile(
		t,
		mustMarshalAuditRecords(
			t,
			[]AuditRecord{
				{
					IncidentID:         "incident-1",
					ExperimentArm:      "controller_enabled",
					RemediationAttempt: true,
					InjectedAt:         baseTime,
					DetectedAt:         baseTime.Add(5 * time.Second),
					MitigatedAt:        baseTime.Add(20 * time.Second),
					Success:            true,
					ProposalAccepted:   true,
					ProposalCorrect:    true,
				},
			},
		),
	)

	records, err := LoadAuditRecords(path)
	if err != nil {
		t.Fatalf("expected audit records to load: %v", err)
	}

	summary := Calculate(records)

	if summary.TotalIncidents != 1 {
		t.Fatalf(
			"expected 1 incident, got %d",
			summary.TotalIncidents,
		)
	}

	if summary.SuccessfulRecoveries != 1 {
		t.Fatalf(
			"expected 1 successful recovery, got %d",
			summary.SuccessfulRecoveries,
		)
	}
}

func writeAuditFile(
	t *testing.T,
	content string,
) string {
	t.Helper()

	path := filepath.Join(
		t.TempDir(),
		"audit.jsonl",
	)

	if err := os.WriteFile(
		path,
		[]byte(content),
		0o600,
	); err != nil {
		t.Fatalf("write audit file: %v", err)
	}

	return path
}

func mustMarshalAuditRecords(
	t *testing.T,
	records []AuditRecord,
) string {
	t.Helper()

	data, err := json.Marshal(records)
	if err != nil {
		t.Fatalf("marshal audit records: %v", err)
	}

	return string(data)
}

func mustMarshalAuditRecord(
	t *testing.T,
	record AuditRecord,
) string {
	t.Helper()

	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal audit record: %v", err)
	}

	return string(data)
}
