package metrics

import (
	"math"
	"testing"
	"time"

	"github.com/aryausingh/k8s-selfheal/internal/classifier"
)

func TestAuditRecordAppliesClassificationOutcomeObservability(t *testing.T) {
	startedAt := time.Date(
		2026,
		time.August,
		21,
		10,
		0,
		0,
		0,
		time.UTC,
	)
	completedAt := startedAt.Add(1250 * time.Millisecond)

	outcome := classifier.ClassificationOutcome{
		ClassifierStartedAt:   startedAt,
		ClassifierCompletedAt: completedAt,
		ClassifierDuration:    completedAt.Sub(startedAt),
		ClassifierProvider:    "mock",
		ClassifierModel:       "mock",
		InputTokens:           120,
		OutputTokens:          30,
		TotalTokens:           150,
		EstimatedCostUSD:      0.00042,
		CostKnown:             true,
		RawResponse:           `{"id":"msg_test"}`,
	}

	record := AuditRecord{
		IncidentID: "incident-1",
	}

	ApplyClassificationOutcome(
		&record,
		outcome,
	)

	if !record.ClassifierStartedAt.Equal(startedAt) {
		t.Fatalf(
			"expected start timestamp %s, got %s",
			startedAt,
			record.ClassifierStartedAt,
		)
	}

	if !record.ClassifierCompletedAt.Equal(completedAt) {
		t.Fatalf(
			"expected completion timestamp %s, got %s",
			completedAt,
			record.ClassifierCompletedAt,
		)
	}

	if record.ClassifierProvider != "mock" {
		t.Fatalf(
			"expected mock provider, got: %s",
			record.ClassifierProvider,
		)
	}

	if record.ClassifierModel != "mock" {
		t.Fatalf(
			"expected mock model, got: %s",
			record.ClassifierModel,
		)
	}

	expectedSeconds := outcome.ClassifierDuration.Seconds()

	if math.Abs(
		record.ClassifierDurationSeconds-expectedSeconds,
	) > 0.000001 {
		t.Fatalf(
			"expected duration %.6f, got %.6f",
			expectedSeconds,
			record.ClassifierDurationSeconds,
		)
	}

	if record.InputTokens != outcome.InputTokens {
		t.Fatalf(
			"expected input tokens %d, got %d",
			outcome.InputTokens,
			record.InputTokens,
		)
	}

	if record.OutputTokens != outcome.OutputTokens {
		t.Fatalf(
			"expected output tokens %d, got %d",
			outcome.OutputTokens,
			record.OutputTokens,
		)
	}

	if record.TotalTokens != outcome.TotalTokens {
		t.Fatalf(
			"expected total tokens %d, got %d",
			outcome.TotalTokens,
			record.TotalTokens,
		)
	}

	if math.Abs(
		record.EstimatedCostUSD-outcome.EstimatedCostUSD,
	) > 0.000001 {
		t.Fatalf(
			"expected estimated cost %.6f, got %.6f",
			outcome.EstimatedCostUSD,
			record.EstimatedCostUSD,
		)
	}

	if record.CostKnown != outcome.CostKnown {
		t.Fatalf(
			"expected cost known %v, got %v",
			outcome.CostKnown,
			record.CostKnown,
		)
	}
	if record.RawClassifierResponse != outcome.RawResponse {
		t.Fatalf(
			"expected raw response %q, got %q",
			outcome.RawResponse,
			record.RawClassifierResponse,
		)
	}
}
