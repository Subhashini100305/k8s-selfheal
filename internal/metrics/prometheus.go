package metrics

import (
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	validatorDecisionAutomate = "automate"
	validatorDecisionEscalate = "escalate"
	validatorDecisionFallback = "fallback_miss"
	validatorDecisionUnknown  = "unknown"
	validatorDecisionEmpty    = "empty"
	metricNamespace           = "selfheal"
)

func NewPrometheusRegistry(
	summary Summary,
	records []AuditRecord,
) (*prometheus.Registry, error) {
	registry := prometheus.NewRegistry()

	gauges := []struct {
		name  string
		help  string
		value float64
	}{
		{
			name:  "total_incidents",
			help:  "Total incidents in the loaded audit dataset.",
			value: float64(summary.TotalIncidents),
		},
		{
			name:  "successful_recoveries_total",
			help:  "Successful recoveries in the loaded audit dataset.",
			value: float64(summary.SuccessfulRecoveries),
		},
		{
			name:  "failed_recoveries_total",
			help:  "Failed recoveries in the loaded audit dataset.",
			value: float64(summary.FailedRecoveries),
		},
		{
			name:  "remediation_attempts_total",
			help:  "Remediation attempts in the loaded audit dataset.",
			value: float64(summary.RemediationAttempts),
		},
		{
			name:  "rollback_total",
			help:  "Rollbacks in the loaded audit dataset.",
			value: float64(summary.TotalRollbacks),
		},
		{
			name:  "recovery_success_rate",
			help:  "Recovery success rate percentage in the loaded audit dataset.",
			value: summary.RecoverySuccessRate,
		},
		{
			name:  "average_ttd_seconds",
			help:  "Average time to detection in seconds.",
			value: summary.AverageTTDSeconds,
		},
		{
			name:  "average_ttm_seconds",
			help:  "Average time to mitigation in seconds.",
			value: summary.AverageTTMSeconds,
		},
		{
			name:  "classifier_inference_latency_seconds",
			help:  "Average classifier inference latency in seconds.",
			value: summary.AverageInferenceLatencySeconds,
		},
		{
			name:  "cluster_convergence_seconds",
			help:  "Average cluster convergence time in seconds.",
			value: summary.AverageClusterConvergenceSeconds,
		},
		{
			name:  "false_accept_total",
			help:  "False accepts in the loaded audit dataset.",
			value: float64(summary.FalseAcceptCount),
		},
		{
			name:  "false_reject_total",
			help:  "False rejects in the loaded audit dataset.",
			value: float64(summary.FalseRejectCount),
		},
		{
			name:  "false_accept_rate",
			help:  "False accept rate percentage in the loaded audit dataset.",
			value: summary.FalseAcceptRate,
		},
		{
			name:  "false_reject_rate",
			help:  "False reject rate percentage in the loaded audit dataset.",
			value: summary.FalseRejectRate,
		},
		{
			name:  "classifier_estimated_cost_usd",
			help:  "Known estimated classifier cost in USD from the loaded audit dataset.",
			value: knownEstimatedCostUSD(records),
		},
	}

	for _, gauge := range gauges {
		if err := registerGauge(
			registry,
			gauge.name,
			gauge.help,
			gauge.value,
		); err != nil {
			return nil, err
		}
	}

	decisionGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Name:      "validator_decisions_total",
			Help:      "Validator decisions in the loaded audit dataset.",
		},
		[]string{"decision"},
	)

	if err := registry.Register(decisionGauge); err != nil {
		return nil, fmt.Errorf(
			"register Prometheus metric selfheal_validator_decisions_total: %w",
			err,
		)
	}

	decisionCounts := validatorDecisionCounts(records)
	for _, decision := range []string{
		validatorDecisionAutomate,
		validatorDecisionEscalate,
		validatorDecisionFallback,
		validatorDecisionUnknown,
		validatorDecisionEmpty,
	} {
		decisionGauge.WithLabelValues(decision).Set(
			float64(decisionCounts[decision]),
		)
	}

	return registry, nil
}

func NewPrometheusHandler(
	summary Summary,
	records []AuditRecord,
) (http.Handler, error) {
	registry, err := NewPrometheusRegistry(
		summary,
		records,
	)
	if err != nil {
		return nil, err
	}

	return promhttp.HandlerFor(
		registry,
		promhttp.HandlerOpts{},
	), nil
}

func registerGauge(
	registry *prometheus.Registry,
	name string,
	help string,
	value float64,
) error {
	gauge := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Name:      name,
			Help:      help,
		},
	)

	if err := registry.Register(gauge); err != nil {
		return fmt.Errorf(
			"register Prometheus metric %s_%s: %w",
			metricNamespace,
			name,
			err,
		)
	}

	gauge.Set(value)

	return nil
}

func knownEstimatedCostUSD(records []AuditRecord) float64 {
	var total float64

	for _, record := range records {
		if record.CostKnown {
			total += record.EstimatedCostUSD
		}
	}

	return total
}

func validatorDecisionCounts(records []AuditRecord) map[string]int {
	counts := map[string]int{
		validatorDecisionAutomate: 0,
		validatorDecisionEscalate: 0,
		validatorDecisionFallback: 0,
		validatorDecisionUnknown:  0,
		validatorDecisionEmpty:    0,
	}

	for _, record := range records {
		switch record.ClassifierDecision {
		case validatorDecisionAutomate:
			counts[validatorDecisionAutomate]++

		case validatorDecisionEscalate:
			counts[validatorDecisionEscalate]++

		case validatorDecisionFallback:
			counts[validatorDecisionFallback]++

		case "":
			counts[validatorDecisionEmpty]++

		default:
			counts[validatorDecisionUnknown]++
		}
	}

	return counts
}
