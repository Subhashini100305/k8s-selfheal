package classifier

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestWeek3MockVsLiveAgreement(t *testing.T) {
	if os.Getenv("RUN_CLAUDE_LIVE") != "1" {
		t.Skip("set RUN_CLAUDE_LIVE=1 to run Week-3 mock-vs-live evaluation")
	}

	if strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")) == "" {
		t.Skip("set ANTHROPIC_API_KEY to run Week-3 mock-vs-live evaluation")
	}

	liveClassifier, err := NewClaudeClassifier()
	if err != nil {
		t.Fatalf("create Claude classifier: %v", err)
	}

	mockClassifier := MockClassifier{}

	cases := labelledClassificationCases()
	if len(cases) != 20 {
		t.Fatalf("expected exactly 20 labelled cases, got %d", len(cases))
	}

	ctx := context.Background()

	agreement := 0
	successfulComparisons := 0

	var totalLiveLatency time.Duration
	var totalInputTokens int
	var totalOutputTokens int
	var totalTokens int
	var totalCost float64

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			mockProposal, err := mockClassifier.Classify(ctx, tc.Input)
			if err != nil {
				t.Errorf("mock classification failed: %v", err)
				return
			}

			start := time.Now()

			liveProposal, metadata, err :=
				liveClassifier.ClassifyWithMetadata(ctx, tc.Input)

			latency := time.Since(start)

			if err != nil {
				t.Errorf("live Claude classification failed: %v", err)
				return
			}

			successfulComparisons++

			totalLiveLatency += latency
			totalInputTokens += metadata.InputTokens
			totalOutputTokens += metadata.OutputTokens
			totalTokens += metadata.TotalTokens

			pricing, known := PricingForModel(liveClassifier.ModelName())
			if known {
				totalCost += EstimateCostUSD(
					TokenUsage{
						InputTokens:  metadata.InputTokens,
						OutputTokens: metadata.OutputTokens,
					},
					pricing,
				)
			}

			sameSubCause :=
				mockProposal.SubCause == liveProposal.SubCause

			sameAction :=
				mockProposal.RecommendedAction ==
					liveProposal.RecommendedAction

			sameSafety :=
				mockProposal.SafeForAutomation ==
					liveProposal.SafeForAutomation

			if sameSubCause && sameAction && sameSafety {
				agreement++
			}

			t.Logf(
				"mock=(%s,%s,%t) live=(%s,%s,%t) latency=%s",
				mockProposal.SubCause,
				mockProposal.RecommendedAction,
				mockProposal.SafeForAutomation,
				liveProposal.SubCause,
				liveProposal.RecommendedAction,
				liveProposal.SafeForAutomation,
				latency,
			)
		})
	}

	if successfulComparisons == 0 {
		t.Fatal("no successful mock-vs-live comparisons")
	}

	agreementRate :=
		float64(agreement) /
			float64(successfulComparisons) * 100

	avgLatency :=
		totalLiveLatency /
			time.Duration(successfulComparisons)

	avgCost :=
		totalCost /
			float64(successfulComparisons)

	t.Logf(
		"Week-3 mock-vs-live agreement: %d/%d = %.2f%%",
		agreement,
		successfulComparisons,
		agreementRate,
	)

	t.Logf(
		"Week-3 live Claude average latency: %s",
		avgLatency,
	)

	t.Logf(
		"Week-3 live Claude tokens: input=%d output=%d total=%d",
		totalInputTokens,
		totalOutputTokens,
		totalTokens,
	)

	t.Logf(
		"Week-3 live Claude total estimated cost: $%.6f",
		totalCost,
	)

	t.Logf(
		"Week-3 live Claude average estimated cost/call: $%.6f",
		avgCost,
	)
}
