package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Collector specific metrics are defined and initialized here.

var (
	// stageLabelCollector is the label included in all metrics collected by the getter
	StageLabelCollector = prometheus.Labels{"stage": "collector"}

	// metricBMCQueryTimeSummary measures how the time spent querying the BMC.
	metricBMCQueryTimeSummary *prometheus.SummaryVec

	// metricBMCQueryErrorCount counts the number of query errors - when querying information from BMCs.
	metricBMCQueryErrorCount *prometheus.CounterVec
)

func init() {
	metricBMCQueryTimeSummary = promauto.NewSummaryVec(
		prometheus.SummaryOpts{
			Name: "alloy_bmc_query_duration_seconds",
			Help: "A counter metric to measure the duration to query information from BMCs",
		},
		[]string{"stage", "query_kind", "model", "vendor"},
	)

	metricBMCQueryErrorCount = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alloy_bmc_query_errors_total",
			Help: "A counter metric to measure the total count of errors when querying the BMC.",
		},
		[]string{"stage", "query_kind", "model", "vendor"},
	)
}

// collect BMC query count error if the BMC vendor, model attributes are available
func IncrementBMCQueryErrorCount(assetVendor, assetModel, queryKind string) {
	if assetModel == "" {
		assetModel = "unknown"
	}

	if assetVendor == "" {
		assetVendor = "unknown"
	}

	// count connection open error metric
	metricBMCQueryErrorCount.With(
		AddLabels(
			StageLabelCollector,
			prometheus.Labels{
				"query_kind": queryKind,
				"vendor":     assetVendor,
				"model":      assetModel,
			}),
	).Inc()
}

// collect BMC query time metrics
func ObserveBMCQueryTimeSummary(assetVendor, assetModel, queryKind string, startTS time.Time) {
	if assetModel == "" {
		assetModel = "unknown"
	}

	if assetVendor == "" {
		assetVendor = "unknown"
	}

	// measure BMC query time from the given startTS
	metricBMCQueryTimeSummary.With(
		AddLabels(
			StageLabelCollector,
			prometheus.Labels{
				"query_kind": queryKind,
				"vendor":     assetVendor,
				"model":      assetModel,
			}),
	).Observe(time.Since(startTS).Seconds())
}
