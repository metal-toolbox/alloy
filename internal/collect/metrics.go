package collect

import (
	"time"

	"github.com/metal-toolbox/alloy/internal/metrics"
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

	// metricBMCInventoryActive indicates when inventory collection is active.
	metricBMCInventoryActive *prometheus.GaugeVec
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

	metricBMCInventoryActive = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "alloy_bmc_inventory_active",
			Help: "A gauge metric that indicates BMC inventory collection is active.",
		},
		[]string{"active"},
	)
}

// collect BMC query count error if the BMC vendor, model attributes are available
func metricIncrementBMCQueryErrorCount(assetVendor, assetModel, queryKind string) {
	if assetModel == "" {
		assetModel = "unknown"
	}

	if assetVendor == "" {
		assetVendor = "unknown"
	}

	// count connection open error metric
	metricBMCQueryErrorCount.With(
		metrics.AddLabels(
			stageLabel,
			prometheus.Labels{
				"query_kind": queryKind,
				"vendor":     assetVendor,
				"model":      assetModel,
			}),
	).Inc()
}

// collect BMC query time metrics
func metricObserveBMCQueryTimeSummary(assetVendor, assetModel, queryKind string, startTS time.Time) {
	if assetModel == "" {
		assetModel = "unknown"
	}

	if assetVendor == "" {
		assetVendor = "unknown"
	}

	// measure BMC query time from the given startTS
	metricBMCQueryTimeSummary.With(
		metrics.AddLabels(
			stageLabel,
			prometheus.Labels{
				"query_kind": queryKind,
				"vendor":     assetVendor,
				"model":      assetModel,
			}),
	).Observe(time.Since(startTS).Seconds())
}
