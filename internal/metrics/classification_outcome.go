package metrics

import "github.com/aryausingh/k8s-selfheal/internal/classifier"

func ApplyClassificationOutcome(
	record *AuditRecord,
	outcome classifier.ClassificationOutcome,
) {
	if record == nil {
		return
	}

	record.ClassifierStartedAt = outcome.ClassifierStartedAt
	record.ClassifierCompletedAt = outcome.ClassifierCompletedAt
	record.ClassifierProvider = outcome.ClassifierProvider
	record.ClassifierModel = outcome.ClassifierModel
	record.InputTokens = outcome.InputTokens
	record.OutputTokens = outcome.OutputTokens
	record.TotalTokens = outcome.TotalTokens
	record.EstimatedCostUSD = outcome.EstimatedCostUSD
	record.CostKnown = outcome.CostKnown
	record.RawClassifierResponse = outcome.RawResponse
	record.ClassifierDurationSeconds =
		outcome.ClassifierDuration.Seconds()

	if !outcome.ClassifierStartedAt.IsZero() &&
		!outcome.ClassifierCompletedAt.IsZero() &&
		!outcome.ClassifierCompletedAt.Before(
			outcome.ClassifierStartedAt,
		) {

		record.ClassifierDurationSeconds =
			outcome.ClassifierCompletedAt.Sub(
				outcome.ClassifierStartedAt,
			).Seconds()
	}
}
