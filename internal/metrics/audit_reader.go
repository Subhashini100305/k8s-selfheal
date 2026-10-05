package metrics

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

func LoadAuditRecords(path string) ([]AuditRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open audit records file %q: %w", path, err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read audit records file %q: %w", path, err)
	}

	return parseAuditRecords(data)
}

func parseAuditRecords(data []byte) ([]AuditRecord, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return []AuditRecord{}, nil
	}

	if trimmed[0] == '[' {
		return parseAuditRecordsJSON(trimmed)
	}

	return parseAuditRecordsJSONL(trimmed)
}

func parseAuditRecordsJSON(data []byte) ([]AuditRecord, error) {
	var records []AuditRecord

	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("decode audit records JSON array: %w", err)
	}

	return records, nil
}

func parseAuditRecordsJSONL(data []byte) ([]AuditRecord, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var lines [][]byte
	lineNumber := 0

	for scanner.Scan() {
		lineNumber++

		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		lines = append(lines, bytes.Clone(line))
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read audit records JSONL: %w", err)
	}

	if len(lines) == 0 {
		return []AuditRecord{}, nil
	}

	if isWeek3AuditEventLine(lines[0]) {
		return parseWeek3AuditEventsJSONL(lines)
	}

	var records []AuditRecord

	for index, line := range lines {
		var record AuditRecord
		if err := json.Unmarshal(line, &record); err != nil {
			return nil, fmt.Errorf(
				"decode audit records JSONL line %d: %w",
				index+1,
				err,
			)
		}

		records = append(records, record)
	}

	return records, nil
}

type week3AuditEvent struct {
	IncidentID       string    `json:"incidentID"`
	AttemptNumber    int       `json:"attemptNumber"`
	Timestamp        time.Time `json:"timestamp"`
	Pod              string    `json:"pod"`
	State            string    `json:"state"`
	Action           string    `json:"action"`
	Result           string    `json:"result"`
	ClassifierMillis int64     `json:"classifierMillis"`
}

func isWeek3AuditEventLine(line []byte) bool {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(line, &fields); err != nil {
		return false
	}

	_, hasIncidentID := fields["incidentID"]
	_, hasAttemptNumber := fields["attemptNumber"]
	_, hasTimestamp := fields["timestamp"]
	_, hasState := fields["state"]

	return hasIncidentID &&
		hasAttemptNumber &&
		hasTimestamp &&
		hasState
}

func parseWeek3AuditEventsJSONL(lines [][]byte) ([]AuditRecord, error) {
	groups := make(map[string]*AuditRecord)
	order := make([]string, 0)

	for index, line := range lines {
		var event week3AuditEvent
		if err := json.Unmarshal(line, &event); err != nil {
			return nil, fmt.Errorf(
				"decode Week-3 audit event JSONL line %d: %w",
				index+1,
				err,
			)
		}

		if event.AttemptNumber < 0 {
			continue
		}

		key := week3AuditEventKey(
			event.IncidentID,
			event.AttemptNumber,
		)
		record, ok := groups[key]
		if !ok {
			record = &AuditRecord{
				IncidentID:    event.IncidentID,
				AttemptNumber: event.AttemptNumber,
				Pod:           event.Pod,
				Action:        event.Action,
			}
			if event.Pod != "" {
				record.Namespace, record.PodName = splitPodRef(event.Pod)
			}
			groups[key] = record
			order = append(order, key)
		}

		applyWeek3AuditEvent(record, event)
	}

	records := make([]AuditRecord, 0, len(order))
	for _, key := range order {
		records = append(records, *groups[key])
	}

	return records, nil
}

func week3AuditEventKey(
	incidentID string,
	attemptNumber int,
) string {
	return incidentID + "\x00" + strconv.Itoa(attemptNumber)
}

func applyWeek3AuditEvent(
	record *AuditRecord,
	event week3AuditEvent,
) {
	if record == nil {
		return
	}

	if record.Action == "" {
		record.Action = event.Action
	}
	if record.Pod == "" {
		record.Pod = event.Pod
		record.Namespace, record.PodName = splitPodRef(event.Pod)
	}

	switch event.State {
	case "DETECTED":
		record.DetectedAt = event.Timestamp

	case "REMEDIATING":
		record.RemediationAttempt = true
		record.ActionStartedAt = event.Timestamp

	case "VERIFYING":
		record.ActionCompletedAt = event.Timestamp
		record.VerificationStartedAt = event.Timestamp

	case "RECOVERED":
		record.VerificationCompletedAt = event.Timestamp
		record.Success = true

	case "ROLLING_BACK":
		record.VerificationCompletedAt = event.Timestamp
		record.RollbackStartedAt = event.Timestamp

	case "ROLLED_BACK":
		record.RolledBack = true
		record.RollbackCompletedAt = event.Timestamp

	case "LOGGED":
		// LOGGED records are attempt-level Owner-2 lifecycle notes. Incident
		// terminal state is authoritative only on the controller's CLOSED line.
		return

	case "CLOSED":
		applyWeek3ClosedEvent(record, event)
	}
}

func applyWeek3ClosedEvent(
	record *AuditRecord,
	event week3AuditEvent,
) {
	if event.ClassifierMillis > 0 {
		record.ClassifierDurationSeconds = float64(event.ClassifierMillis) / 1000
	}
	switch strings.ToLower(strings.TrimSpace(event.Result)) {
	case "recovered":
		record.TerminalOutcome = "recovered"
		record.TerminalAt = event.Timestamp
		record.Success = true

	case "exhausted":
		record.TerminalOutcome = "exhausted"
		record.TerminalAt = event.Timestamp
		record.Success = false

	case "escalated":
		record.TerminalOutcome = "escalated"
		record.TerminalAt = event.Timestamp
		record.Success = false
		record.RemediationAttempt = false
		record.Action = ""

	case "rejected":
		record.TerminalOutcome = "rejected"
		record.TerminalAt = event.Timestamp
		record.Success = false
		record.RemediationAttempt = false
		record.Action = ""
	}
}

func splitPodRef(pod string) (string, string) {
	namespace, name, ok := strings.Cut(pod, "/")
	if !ok {
		return "", pod
	}
	return namespace, name
}
