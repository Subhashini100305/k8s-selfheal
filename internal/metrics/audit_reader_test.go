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

func TestLoadAuditRecordsWeek3RecoveredEventJSONL(t *testing.T) {
	baseTime := time.Date(
		2026,
		time.October,
		4,
		10,
		0,
		0,
		0,
		time.UTC,
	)

	path := writeAuditFile(
		t,
		mustMarshalWeek3AuditEvents(
			t,
			[]week3AuditEvent{
				week3Event(baseTime, "incident-1", 1, "DETECTED"),
				week3Event(baseTime.Add(10*time.Second), "incident-1", 1, "REMEDIATING"),
				week3Event(baseTime.Add(12*time.Second), "incident-1", 1, "VERIFYING"),
				week3Event(baseTime.Add(72*time.Second), "incident-1", 1, "RECOVERED"),
				week3ClosedEvent(baseTime.Add(73*time.Second), "incident-1", 1, "recovered", 412),
			},
		),
	)

	records, err := LoadAuditRecords(path)
	if err != nil {
		t.Fatalf("expected Week-3 event JSONL to load: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	record := records[0]
	if record.IncidentID != "incident-1" {
		t.Fatalf("expected incident-1, got %s", record.IncidentID)
	}
	if record.AttemptNumber != 1 {
		t.Fatalf("expected attempt 1, got %d", record.AttemptNumber)
	}
	if record.Action != "restart_pod" {
		t.Fatalf("unexpected action: %q", record.Action)
	}
	if record.Pod != "default/checkout-pod" ||
		record.Namespace != "default" ||
		record.PodName != "checkout-pod" {
		t.Fatalf("pod not preserved/split: %+v", record)
	}
	if !record.RemediationAttempt {
		t.Fatal("expected remediation attempt")
	}
	if !record.ActionStartedAt.Equal(baseTime.Add(10 * time.Second)) {
		t.Fatalf("unexpected action start: %s", record.ActionStartedAt)
	}
	if !record.ActionCompletedAt.Equal(baseTime.Add(12 * time.Second)) {
		t.Fatalf("unexpected action completion: %s", record.ActionCompletedAt)
	}
	if !record.VerificationStartedAt.Equal(baseTime.Add(12 * time.Second)) {
		t.Fatalf("unexpected verification start: %s", record.VerificationStartedAt)
	}
	if !record.VerificationCompletedAt.Equal(baseTime.Add(72 * time.Second)) {
		t.Fatalf("unexpected verification completion: %s", record.VerificationCompletedAt)
	}
	if record.TerminalOutcome != "recovered" {
		t.Fatalf("expected recovered terminal outcome, got %q", record.TerminalOutcome)
	}
	if !record.TerminalAt.Equal(baseTime.Add(73 * time.Second)) {
		t.Fatalf("unexpected terminal time: %s", record.TerminalAt)
	}
	if record.ClassifierDurationSeconds != 0.412 {
		t.Fatalf("classifier duration = %.3f, want 0.412", record.ClassifierDurationSeconds)
	}
	if !record.Success {
		t.Fatal("expected success")
	}
	if !record.DetectedAt.Equal(baseTime) {
		t.Fatalf("DetectedAt = %s, want DETECTED timestamp %s", record.DetectedAt, baseTime)
	}
}

func TestLoadAuditRecordsWeek3RolledBackEventJSONL(t *testing.T) {
	baseTime := time.Date(
		2026,
		time.October,
		4,
		11,
		0,
		0,
		0,
		time.UTC,
	)

	path := writeAuditFile(
		t,
		mustMarshalWeek3AuditEvents(
			t,
			[]week3AuditEvent{
				week3Event(baseTime, "incident-rollback", 1, "DETECTED"),
				week3Event(baseTime.Add(5*time.Second), "incident-rollback", 1, "REMEDIATING"),
				week3Event(baseTime.Add(8*time.Second), "incident-rollback", 1, "VERIFYING"),
				week3Event(baseTime.Add(38*time.Second), "incident-rollback", 1, "ROLLING_BACK"),
				week3Event(baseTime.Add(40*time.Second), "incident-rollback", 1, "ROLLED_BACK"),
				week3LoggedEvent(baseTime.Add(41*time.Second), "incident-rollback", 1, "rolled_back"),
			},
		),
	)

	records, err := LoadAuditRecords(path)
	if err != nil {
		t.Fatalf("expected Week-3 rollback event JSONL to load: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	record := records[0]
	if !record.RolledBack {
		t.Fatal("expected rolled-back attempt")
	}
	if record.TerminalOutcome != "" {
		t.Fatalf(
			"rolled-back attempt must not become incident terminal outcome, got %q",
			record.TerminalOutcome,
		)
	}
	if !record.TerminalAt.IsZero() {
		t.Fatalf("rolled-back attempt must not set TerminalAt, got %s", record.TerminalAt)
	}
	if !record.VerificationCompletedAt.Equal(baseTime.Add(38 * time.Second)) {
		t.Fatalf("unexpected verification completion: %s", record.VerificationCompletedAt)
	}
	if !record.RollbackStartedAt.Equal(baseTime.Add(38 * time.Second)) {
		t.Fatalf("unexpected rollback start: %s", record.RollbackStartedAt)
	}
	if !record.RollbackCompletedAt.Equal(baseTime.Add(40 * time.Second)) {
		t.Fatalf("unexpected rollback completion: %s", record.RollbackCompletedAt)
	}
}

func TestLoadAuditRecordsWeek3MultipleAttemptsStaySeparate(t *testing.T) {
	baseTime := time.Date(
		2026,
		time.October,
		4,
		12,
		0,
		0,
		0,
		time.UTC,
	)

	path := writeAuditFile(
		t,
		mustMarshalWeek3AuditEvents(
			t,
			[]week3AuditEvent{
				week3Event(baseTime, "incident-multi", 1, "REMEDIATING"),
				week3Event(baseTime.Add(time.Second), "incident-multi", 1, "VERIFYING"),
				week3Event(baseTime.Add(30*time.Second), "incident-multi", 1, "ROLLING_BACK"),
				week3Event(baseTime.Add(31*time.Second), "incident-multi", 1, "ROLLED_BACK"),
				week3Event(baseTime.Add(70*time.Second), "incident-multi", 2, "REMEDIATING"),
				week3Event(baseTime.Add(72*time.Second), "incident-multi", 2, "VERIFYING"),
				week3Event(baseTime.Add(132*time.Second), "incident-multi", 2, "RECOVERED"),
				week3ClosedEvent(baseTime.Add(133*time.Second), "incident-multi", 2, "recovered", 200),
			},
		),
	)

	records, err := LoadAuditRecords(path)
	if err != nil {
		t.Fatalf("expected Week-3 multi-attempt event JSONL to load: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if records[0].IncidentID != "incident-multi" ||
		records[1].IncidentID != "incident-multi" {

		t.Fatalf(
			"expected same incident ID across attempts, got %q and %q",
			records[0].IncidentID,
			records[1].IncidentID,
		)
	}
	if records[0].AttemptNumber != 1 ||
		records[1].AttemptNumber != 2 {

		t.Fatalf(
			"expected attempts 1 and 2, got %d and %d",
			records[0].AttemptNumber,
			records[1].AttemptNumber,
		)
	}
	if !records[0].RolledBack {
		t.Fatal("expected first attempt to be rolled back")
	}
	if records[1].TerminalOutcome != "recovered" {
		t.Fatalf("expected second attempt recovered, got %q", records[1].TerminalOutcome)
	}
}

func TestLoadAuditRecordsWeek3MultipleRolledBackAttempts(t *testing.T) {
	baseTime := time.Date(
		2026,
		time.October,
		4,
		12,
		30,
		0,
		0,
		time.UTC,
	)

	path := writeAuditFile(
		t,
		mustMarshalWeek3AuditEvents(
			t,
			[]week3AuditEvent{
				week3Event(baseTime, "incident-retry", 1, "REMEDIATING"),
				week3Event(baseTime.Add(time.Second), "incident-retry", 1, "VERIFYING"),
				week3Event(baseTime.Add(30*time.Second), "incident-retry", 1, "ROLLING_BACK"),
				week3Event(baseTime.Add(31*time.Second), "incident-retry", 1, "ROLLED_BACK"),
				week3Event(baseTime.Add(70*time.Second), "incident-retry", 2, "REMEDIATING"),
				week3Event(baseTime.Add(71*time.Second), "incident-retry", 2, "VERIFYING"),
				week3Event(baseTime.Add(101*time.Second), "incident-retry", 2, "ROLLING_BACK"),
				week3Event(baseTime.Add(102*time.Second), "incident-retry", 2, "ROLLED_BACK"),
			},
		),
	)

	records, err := LoadAuditRecords(path)
	if err != nil {
		t.Fatalf("expected Week-3 retry event JSONL to load: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	for _, record := range records {
		if !record.RolledBack {
			t.Fatalf("expected attempt %d to be rolled back", record.AttemptNumber)
		}
		if record.TerminalOutcome != "" {
			t.Fatalf("ROLLED_BACK must not be terminal, got %q", record.TerminalOutcome)
		}
	}
}

func TestLoadAuditRecordsWeek3ClosedExhaustedMergesWithAttempt3(t *testing.T) {
	baseTime := time.Date(
		2026,
		time.October,
		4,
		13,
		0,
		0,
		0,
		time.UTC,
	)

	path := writeAuditFile(
		t,
		mustMarshalWeek3AuditEvents(
			t,
			[]week3AuditEvent{
				week3Event(baseTime, "incident-exhausted", 3, "REMEDIATING"),
				week3Event(baseTime.Add(2*time.Second), "incident-exhausted", 3, "VERIFYING"),
				week3Event(baseTime.Add(32*time.Second), "incident-exhausted", 3, "ROLLING_BACK"),
				week3Event(baseTime.Add(33*time.Second), "incident-exhausted", 3, "ROLLED_BACK"),
				week3ClosedEvent(baseTime.Add(34*time.Second), "incident-exhausted", 3, "exhausted", 900),
			},
		),
	)

	records, err := LoadAuditRecords(path)
	if err != nil {
		t.Fatalf("expected Week-3 exhausted event JSONL to load: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected exhausted CLOSED event to merge with attempt 3, got %d records", len(records))
	}

	record := records[0]
	if record.AttemptNumber != 3 {
		t.Fatalf("expected attempt 3, got %d", record.AttemptNumber)
	}
	if !record.RemediationAttempt {
		t.Fatal("exhausted CLOSED event must not clear the existing remediation attempt")
	}
	if record.Action != "restart_pod" {
		t.Fatalf("expected attempt action to be preserved, got %q", record.Action)
	}
	if !record.RolledBack {
		t.Fatal("expected rolled-back attempt data to be preserved")
	}
	if record.TerminalOutcome != "exhausted" {
		t.Fatalf("expected exhausted terminal outcome, got %q", record.TerminalOutcome)
	}
	if !record.TerminalAt.Equal(baseTime.Add(34 * time.Second)) {
		t.Fatalf("unexpected terminal time: %s", record.TerminalAt)
	}
	if record.Success {
		t.Fatal("exhausted must not be marked successful")
	}
}

func TestLoadAuditRecordsWeek3LoggedEscalatedAttempt0(t *testing.T) {
	baseTime := time.Date(
		2026,
		time.October,
		4,
		14,
		0,
		0,
		0,
		time.UTC,
	)

	path := writeAuditFile(
		t,
		mustMarshalWeek3AuditEvents(
			t,
			[]week3AuditEvent{
				week3ClosedEvent(baseTime, "incident-escalated", 0, "escalated", 100),
			},
		),
	)

	records, err := LoadAuditRecords(path)
	if err != nil {
		t.Fatalf("expected Week-3 escalated event JSONL to load: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	record := records[0]
	if record.AttemptNumber != 0 ||
		record.TerminalOutcome != "escalated" ||
		!record.TerminalAt.Equal(baseTime) ||
		record.Success ||
		record.RemediationAttempt ||
		record.Action != "" {

		t.Fatalf("unexpected escalated record: %+v", record)
	}
}

func TestLoadAuditRecordsWeek3LoggedRejectedAttempt0(t *testing.T) {
	baseTime := time.Date(
		2026,
		time.October,
		4,
		15,
		0,
		0,
		0,
		time.UTC,
	)

	path := writeAuditFile(
		t,
		mustMarshalWeek3AuditEvents(
			t,
			[]week3AuditEvent{
				week3ClosedEvent(baseTime, "incident-rejected", 0, "rejected", 100),
			},
		),
	)

	records, err := LoadAuditRecords(path)
	if err != nil {
		t.Fatalf("expected Week-3 rejected event JSONL to load: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	record := records[0]
	if record.AttemptNumber != 0 ||
		record.TerminalOutcome != "rejected" ||
		!record.TerminalAt.Equal(baseTime) ||
		record.Success ||
		record.RemediationAttempt ||
		record.Action != "" {

		t.Fatalf("unexpected rejected record: %+v", record)
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

func week3Event(
	timestamp time.Time,
	incidentID string,
	attemptNumber int,
	state string,
) week3AuditEvent {
	return week3AuditEvent{
		IncidentID:    incidentID,
		AttemptNumber: attemptNumber,
		Timestamp:     timestamp,
		Pod:           "default/checkout-pod",
		State:         state,
		Action:        "restart_pod",
		Result:        "test",
	}
}

func week3LoggedEvent(
	timestamp time.Time,
	incidentID string,
	attemptNumber int,
	result string,
) week3AuditEvent {
	return week3AuditEvent{
		IncidentID:    incidentID,
		AttemptNumber: attemptNumber,
		Timestamp:     timestamp,
		Pod:           "default/checkout-pod",
		State:         "LOGGED",
		Action:        "",
		Result:        result,
	}
}

func week3ClosedEvent(
	timestamp time.Time,
	incidentID string,
	attemptNumber int,
	result string,
	classifierMillis int64,
) week3AuditEvent {
	return week3AuditEvent{
		IncidentID:       incidentID,
		AttemptNumber:    attemptNumber,
		Timestamp:        timestamp,
		Pod:              "default/checkout-pod",
		State:            "CLOSED",
		Action:           "",
		Result:           result,
		ClassifierMillis: classifierMillis,
	}
}

func mustMarshalWeek3AuditEvents(
	t *testing.T,
	events []week3AuditEvent,
) string {
	t.Helper()

	var output string
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("marshal Week-3 audit event: %v", err)
		}
		output += string(data) + "\n"
	}

	return output
}
