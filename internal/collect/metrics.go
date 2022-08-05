package collect

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Collector specific metrics are defined and initialized here.

var (
	// stageLabel is the label included in all metrics collected by the getter
	stageLabel = prometheus.Labels{"stage": "collector"}

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
