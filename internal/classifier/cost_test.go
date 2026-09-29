package classifier

import (
	"math"
	"testing"
)

func TestEstimateCostUSDZeroTokens(t *testing.T) {
	cost := EstimateCostUSD(
		TokenUsage{},
		ModelPricing{
			InputPerMillionUSD:  3,
			OutputPerMillionUSD: 15,
		},
	)

	if cost != 0 {
		t.Fatalf("expected zero cost, got %.6f", cost)
	}
}

func TestEstimateCostUSDFakePricing(t *testing.T) {
	cost := EstimateCostUSD(
		TokenUsage{
			InputTokens:  500_000,
			OutputTokens: 250_000,
		},
		ModelPricing{
			InputPerMillionUSD:  2,
			OutputPerMillionUSD: 8,
		},
	)

	expected := 3.0

	if math.Abs(cost-expected) > 0.000001 {
		t.Fatalf(
			"expected cost %.6f, got %.6f",
			expected,
			cost,
		)
	}
}

func TestEstimateCostUSDClaudeSonnet5Pricing(t *testing.T) {
	pricing, ok := PricingForModel(
		claudeSonnet5Model,
	)
	if !ok {
		t.Fatal("expected Claude Sonnet 5 pricing")
	}

	cost := EstimateCostUSD(
		TokenUsage{
			InputTokens:  1000,
			OutputTokens: 200,
		},
		pricing,
	)

	expected := 0.004

	if math.Abs(cost-expected) > 0.000001 {
		t.Fatalf(
			"expected cost %.6f, got %.6f",
			expected,
			cost,
		)
	}
}

func TestPricingForClaudeSonnet45(t *testing.T) {
	pricing, ok := PricingForModel(claudeSonnet45Model)
	if !ok {
		t.Fatal("expected Claude Sonnet 4.5 pricing to be known")
	}

	if pricing.InputPerMillionUSD != 3 {
		t.Fatalf(
			"expected input price 3, got %.2f",
			pricing.InputPerMillionUSD,
		)
	}

	if pricing.OutputPerMillionUSD != 15 {
		t.Fatalf(
			"expected output price 15, got %.2f",
			pricing.OutputPerMillionUSD,
		)
	}
}

func TestWeek3ClaudeSonnet45RecordedCost(t *testing.T) {
	pricing, ok := PricingForModel(claudeSonnet45Model)
	if !ok {
		t.Fatal("expected Claude Sonnet 4.5 pricing to be known")
	}

	usage := TokenUsage{
		InputTokens:  26884,
		OutputTokens: 3653,
	}

	cost := EstimateCostUSD(usage, pricing)

	const expected = 0.135447

	const epsilon = 0.000000001

	if cost < expected-epsilon || cost > expected+epsilon {
		t.Fatalf(
			"expected Week-3 cost %.6f, got %.6f",
			expected,
			cost,
		)
	}

	average := cost / 20

	const expectedAverage = 0.00677235

	if average < expectedAverage-epsilon ||
		average > expectedAverage+epsilon {

		t.Fatalf(
			"expected average Week-3 cost %.8f, got %.8f",
			expectedAverage,
			average,
		)
	}
}

func TestPricingForModelUnknownClaudeModel(t *testing.T) {
	pricing, ok := PricingForModel(
		"claude-unconfigured-model",
	)

	if ok {
		t.Fatalf(
			"expected unknown Claude model to have no pricing, got %#v",
			pricing,
		)
	}
}

func TestCostTokenUsageTotalTokens(t *testing.T) {
	usage := TokenUsage{
		InputTokens:  123,
		OutputTokens: 456,
	}

	if usage.TotalTokens() != 579 {
		t.Fatalf(
			"expected total tokens 579, got %d",
			usage.TotalTokens(),
		)
	}
}

func TestEstimateCostUSDNegativeTokensAreClamped(t *testing.T) {
	usage := TokenUsage{
		InputTokens:  -100,
		OutputTokens: 200_000,
	}

	cost := EstimateCostUSD(
		usage,
		ModelPricing{
			InputPerMillionUSD:  10,
			OutputPerMillionUSD: 5,
		},
	)

	expected := 1.0

	if usage.TotalTokens() != 200_000 {
		t.Fatalf(
			"expected negative input tokens to be clamped in total, got %d",
			usage.TotalTokens(),
		)
	}

	if math.Abs(cost-expected) > 0.000001 {
		t.Fatalf(
			"expected cost %.6f, got %.6f",
			expected,
			cost,
		)
	}
}
