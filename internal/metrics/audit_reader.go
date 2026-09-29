package metrics

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
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

	var records []AuditRecord
	lineNumber := 0

	for scanner.Scan() {
		lineNumber++

		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var record AuditRecord
		if err := json.Unmarshal(line, &record); err != nil {
			return nil, fmt.Errorf(
				"decode audit records JSONL line %d: %w",
				lineNumber,
				err,
			)
		}

		records = append(records, record)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read audit records JSONL: %w", err)
	}

	return records, nil
}
