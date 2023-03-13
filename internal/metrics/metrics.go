package metrics

import (
	"log"
	"net/http"
	"time"

	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// s shared across packages are defined and initialized here.

var (
	// TasksLockerDispatched measures the count of tasks dispatched to retrieve assets.
	TasksLockerDispatched *prometheus.CounterVec

	// TasksLockerCompleted measures the count of workers that returned after being spawned.
	TasksLockerCompleted *prometheus.CounterVec

	// ServerServiceAssetsRetrieved measures the count of assets retrieved from server service to collect inventory for.
	ServerServiceAssetsRetrieved *prometheus.CounterVec

	// AssetsSent measures the count of assets sent over the asset channel to the collector.
	AssetsSent *prometheus.CounterVec

	// AssetsReceived measures the count of assets received from the asset/collector channels.
	AssetsReceived *prometheus.CounterVec

	// TaskQueueSize measures the number of tasks waiting for a getter worker .
	TaskQueueSize *prometheus.GaugeVec

	// ServerServiceQueryErrorCount counts the number of query errors - when querying the asset store.
	ServerServiceQueryErrorCount *prometheus.CounterVec

	// OOBCollectScheduleTimestamp includes the timestamp of the next OOB collection scheduled run.
	OOBCollectScheduleTimestamp *prometheus.GaugeVec

	// CollectTotalTimeSummary measures how the total time spent collecting inventory in each alloy run.
	CollectTotalTimeSummary *prometheus.SummaryVec

	// OOBCollectionActive indicates when inventory collection is active.
	OOBCollectionActive prometheus.Gauge
)

func init() {
	TasksLockerDispatched = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alloy_task_dispatched_total",
			Help: "A counter metric to measure the total count of tasks dispatched to retrieve assets from serverService",
		},
		[]string{"stage"},
	)

	TasksLockerCompleted = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alloy_task_completed_total",
			Help: "A counter metric to measure the total count of tasks that completed retrieving assets from serverService",
		},
		[]string{"stage"},
	)

	ServerServiceAssetsRetrieved = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alloy_assets_retrieved_total",
			Help: "A counter metric to measure the total count of assets retrieved from serverService",
		},
		[]string{"stage"},
	)

	AssetsSent = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alloy_assets_sent_total",
			Help: "A counter metric to measure the total count of assets sent on the asset channel to the alloy collector stage",
		},
		[]string{"stage"},
	)

	AssetsReceived = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alloy_assets_received_total",
			Help: "A counter metric to measure the total count of assets received on the assets/collector channels",
		},
		[]string{"stage"},
	)
	prometheus.MustRegister(AssetsReceived)

	TaskQueueSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "alloy_task_queue_size",
			Help: "A gauge metric to measure the number of tasks waiting for a worker in the getter worker pool",
		},
		[]string{"stage"},
	)

	ServerServiceQueryErrorCount = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alloy_serverservice_query_errors_total",
			Help: "A counter metric to measure the total count of errors when the asset store.",
		},
		[]string{"stage"},
	)

	OOBCollectScheduleTimestamp = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "alloy_oob_collect_schedule_ts",
			Help: "A gauge metric that indicates when the next alloy run is scheduled.",
		},
		[]string{"timestamp"},
	)

	CollectTotalTimeSummary = promauto.NewSummaryVec(
		prometheus.SummaryOpts{
			Name: "alloy_collect_duration_seconds",
			Help: "A summary metric to measure the total time spent to complete collecting inventory in each run.",
		},
		// collect_kind is one of inband/outofband
		[]string{"collect_kind"},
	)

	OOBCollectionActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "alloy_oob_collection_active",
			Help: "A gauge metric that indicates OOB BMC based inventory collection is active.",
		},
	)
}

// ListenAndServeMetrics exposes prometheus metrics as /metrics
func ListenAndServe() {
	go func() {
		http.Handle("/metrics", promhttp.Handler())

		server := &http.Server{
			Addr:              model.MetricsEndpoint,
			ReadHeaderTimeout: 2 * time.Second, // nolint:gomnd // time duration value is clear as is.
		}

		if err := server.ListenAndServe(); err != nil {
			log.Println(err)
		}
	}()
}

// AddLabels returns a new map of labels with the current and add labels included.
func AddLabels(current, add prometheus.Labels) prometheus.Labels {
	returned := map[string]string{}

	for l, v := range current {
		returned[l] = v
	}

	for l, v := range add {
		returned[l] = v
	}

	return returned
}
