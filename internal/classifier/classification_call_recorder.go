package classifier

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const ClassifierCallLogPathEnv = "SAGE_CLASSIFIER_CALLS_FILE"

type ClassificationCallRecorder interface {
	RecordClassificationCall(
		input IncidentInput,
		outcome ClassificationOutcome,
	) error
}

type JSONLClassificationCallRecorder struct {
	path string
	mu   sync.Mutex
}

type ClassificationCallRecord struct {
	Timestamp time.Time `json:"timestamp"`

	PodName         string `json:"podName"`
	Namespace       string `json:"namespace"`
	OwnerDeployment string `json:"ownerDeployment,omitempty"`

	ClassifierProvider    string    `json:"classifierProvider,omitempty"`
	ClassifierModel       string    `json:"classifierModel,omitempty"`
	ClassifierStartedAt   time.Time `json:"classifierStartedAt,omitempty"`
	ClassifierCompletedAt time.Time `json:"classifierCompletedAt,omitempty"`
	ClassifierMillis      int64     `json:"classifierMillis,omitempty"`

	InputTokens      int     `json:"inputTokens,omitempty"`
	OutputTokens     int     `json:"outputTokens,omitempty"`
	TotalTokens      int     `json:"totalTokens,omitempty"`
	EstimatedCostUSD float64 `json:"estimatedCostUSD,omitempty"`
	CostKnown        bool    `json:"costKnown"`

	RawClassifierResponse string `json:"rawClassifierResponse,omitempty"`

	FallbackUsed   bool   `json:"fallbackUsed,omitempty"`
	FallbackReason string `json:"fallbackReason,omitempty"`

	RecommendedAction string `json:"recommendedAction,omitempty"`
	SubCause          string `json:"subCause,omitempty"`
}

func NewJSONLClassificationCallRecorderFromEnv() ClassificationCallRecorder {
	path := strings.TrimSpace(
		os.Getenv(ClassifierCallLogPathEnv),
	)
	if path == "" {
		return nil
	}

	return NewJSONLClassificationCallRecorder(path)
}

func NewJSONLClassificationCallRecorder(
	path string,
) *JSONLClassificationCallRecorder {
	return &JSONLClassificationCallRecorder{
		path: strings.TrimSpace(path),
	}
}

func (r *JSONLClassificationCallRecorder) RecordClassificationCall(
	input IncidentInput,
	outcome ClassificationOutcome,
) error {
	if r == nil ||
		strings.TrimSpace(r.path) == "" {

		return nil
	}

	record := NewClassificationCallRecord(
		input,
		outcome,
	)

	line, err := json.Marshal(record)
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if dir := filepath.Dir(r.path); dir != "." &&
		strings.TrimSpace(dir) != "" {

		if err := os.MkdirAll(
			dir,
			0o755,
		); err != nil {
			return err
		}
	}

	file, err := os.OpenFile(
		r.path,
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0o644,
	)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	if _, err := file.Write(
		append(line, '\n'),
	); err != nil {
		return err
	}

	return nil
}

func NewClassificationCallRecord(
	input IncidentInput,
	outcome ClassificationOutcome,
) ClassificationCallRecord {
	timestamp := outcome.ClassifierCompletedAt
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	return ClassificationCallRecord{
		Timestamp: timestamp,

		PodName:         input.PodName,
		Namespace:       input.Namespace,
		OwnerDeployment: input.OwnerDeployment,

		ClassifierProvider:    outcome.ClassifierProvider,
		ClassifierModel:       outcome.ClassifierModel,
		ClassifierStartedAt:   outcome.ClassifierStartedAt,
		ClassifierCompletedAt: outcome.ClassifierCompletedAt,
		ClassifierMillis:      outcome.ClassifierDuration.Milliseconds(),

		InputTokens:      outcome.InputTokens,
		OutputTokens:     outcome.OutputTokens,
		TotalTokens:      outcome.TotalTokens,
		EstimatedCostUSD: outcome.EstimatedCostUSD,
		CostKnown:        outcome.CostKnown,

		RawClassifierResponse: outcome.RawResponse,

		FallbackUsed:   outcome.FallbackUsed,
		FallbackReason: outcome.FallbackReason,

		RecommendedAction: outcome.Proposal.RecommendedAction,
		SubCause:          outcome.Proposal.SubCause,
	}
}
