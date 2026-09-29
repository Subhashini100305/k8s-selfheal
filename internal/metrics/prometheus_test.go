package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPrometheusRegistryRegistersWithoutPanic(t *testing.T) {
	summary, records := prometheusTestData()

	registry, err := NewPrometheusRegistry(
		summary,
		records,
	)
	if err != nil {
		t.Fatalf("expected registry to be created: %v", err)
	}

	if registry == nil {
		t.Fatal("expected registry")
	}
}

func TestPrometheusMetricsHandlerExposesExpectedMetrics(t *testing.T) {
	body := scrapePrometheusTestMetrics(t)

	expectedNames := []string{
		"selfheal_total_incidents",
		"selfheal_successful_recoveries_total",
		"selfheal_failed_recoveries_total",
		"selfheal_remediation_attempts_total",
		"selfheal_rollback_total",
		"selfheal_recovery_success_rate",
		"selfheal_average_ttd_seconds",
		"selfheal_average_ttm_seconds",
		"selfheal_classifier_inference_latency_seconds",
		"selfheal_cluster_convergence_seconds",
		"selfheal_false_accept_total",
		"selfheal_false_reject_total",
		"selfheal_false_accept_rate",
		"selfheal_false_reject_rate",
		"selfheal_classifier_estimated_cost_usd",
		"selfheal_validator_decisions_total",
	}

	for _, name := range expectedNames {
		if !strings.Contains(body, name) {
			t.Fatalf("expected metric %s in body:\n%s", name, body)
		}
	}
}

func TestPrometheusMetricsHandlerRepresentsSummaryValues(t *testing.T) {
	body := scrapePrometheusTestMetrics(t)

	expectedSamples := []string{
		"selfheal_total_incidents 3",
		"selfheal_successful_recoveries_total 2",
		"selfheal_failed_recoveries_total 1",
		"selfheal_remediation_attempts_total 2",
		"selfheal_rollback_total 1",
		"selfheal_false_accept_total 1",
		"selfheal_false_reject_total 1",
	}

	for _, sample := range expectedSamples {
		if !strings.Contains(body, sample) {
			t.Fatalf("expected sample %q in body:\n%s", sample, body)
		}
	}
}

func TestPrometheusMetricsHandlerValidatorDecisionCounts(t *testing.T) {
	body := scrapePrometheusTestMetrics(t)

	expectedSamples := []string{
		`selfheal_validator_decisions_total{decision="automate"} 1`,
		`selfheal_validator_decisions_total{decision="escalate"} 1`,
		`selfheal_validator_decisions_total{decision="fallback_miss"} 1`,
	}

	for _, sample := range expectedSamples {
		if !strings.Contains(body, sample) {
			t.Fatalf("expected sample %q in body:\n%s", sample, body)
		}
	}
}

func TestPrometheusMetricsHandlerUsesKnownAuditCostOnly(t *testing.T) {
	body := scrapePrometheusTestMetrics(t)

	if !strings.Contains(
		body,
		"selfheal_classifier_estimated_cost_usd 0.25",
	) {
		t.Fatalf("expected known cost total in body:\n%s", body)
	}

	if strings.Contains(
		body,
		"selfheal_classifier_estimated_cost_usd 99.25",
	) {
		t.Fatalf("unknown audit cost was exported:\n%s", body)
	}
}

func scrapePrometheusTestMetrics(t *testing.T) string {
	t.Helper()

	summary, records := prometheusTestData()

	handler, err := NewPrometheusHandler(
		summary,
		records,
	)
	if err != nil {
		t.Fatalf("expected handler to be created: %v", err)
	}

	request := httptest.NewRequest(
		http.MethodGet,
		"/metrics",
		nil,
	)
	response := httptest.NewRecorder()

	handler.ServeHTTP(
		response,
		request,
	)

	if response.Code != http.StatusOK {
		t.Fatalf(
			"expected HTTP 200, got %d",
			response.Code,
		)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}

	return string(body)
}

func prometheusTestData() (Summary, []AuditRecord) {
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

	records := []AuditRecord{
		{
			IncidentID:         "incident-1",
			ExperimentArm:      "controller_enabled",
			RemediationAttempt: true,
			InjectedAt:         baseTime,
			DetectedAt:         baseTime.Add(5 * time.Second),
			ClassifierStartedAt: baseTime.Add(
				6 * time.Second,
			),
			ClassifierCompletedAt: baseTime.Add(
				8 * time.Second,
			),
			ActionStartedAt: baseTime.Add(10 * time.Second),
			VerificationCompletedAt: baseTime.Add(
				20 * time.Second,
			),
			MitigatedAt:        baseTime.Add(20 * time.Second),
			Success:            true,
			ClassifierDecision: "automate",
			EstimatedCostUSD:   0.25,
			CostKnown:          true,
			ProposalAccepted:   true,
			ProposalCorrect:    true,
		},
		{
			IncidentID:         "incident-2",
			ExperimentArm:      "unrecoverable",
			RemediationAttempt: true,
			InjectedAt:         baseTime.Add(30 * time.Second),
			DetectedAt:         baseTime.Add(40 * time.Second),
			ClassifierStartedAt: baseTime.Add(
				41 * time.Second,
			),
			ClassifierCompletedAt: baseTime.Add(
				45 * time.Second,
			),
			ActionStartedAt: baseTime.Add(50 * time.Second),
			VerificationCompletedAt: baseTime.Add(
				70 * time.Second,
			),
			MitigatedAt:        baseTime.Add(80 * time.Second),
			Success:            false,
			RolledBack:         true,
			ClassifierDecision: "escalate",
			EstimatedCostUSD:   99,
			CostKnown:          false,
			ProposalAccepted:   true,
			ProposalCorrect:    false,
		},
		{
			IncidentID:         "incident-3",
			ExperimentArm:      "controller_enabled",
			InjectedAt:         baseTime.Add(90 * time.Second),
			DetectedAt:         baseTime.Add(100 * time.Second),
			MitigatedAt:        baseTime.Add(120 * time.Second),
			Success:            true,
			ClassifierDecision: "fallback_miss",
			ProposalAccepted:   false,
			ProposalCorrect:    true,
		},
	}

	return Calculate(records), records
}
