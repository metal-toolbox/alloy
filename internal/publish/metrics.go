package publish

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Publisher specific metrics are defined and initialized here.

var (
	// stageLabel is the label included in all metrics collected by the publisher
	stageLabel = prometheus.Labels{"stage": "publisher"}

	// metricAssetComponentsIdentified measures the count of hardware components in the device components data from the collector.
	metricAssetComponentsIdentified *prometheus.GaugeVec

	// metricServerServiceDataChanges measures the number of server component data additions, updates, deletes.
	metricServerServiceDataChanges *prometheus.GaugeVec
)

func init() {
	metricAssetComponentsIdentified = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "alloy_asset_components_identified",
			Help: "A gauge metric to count of number hardware components identified by the publisher.",
		},
		[]string{"stage", "vendor", "model"},
	)

	metricServerServiceDataChanges = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "alloy_serverservice_data_changes",
			Help: "A gauge metric to measure the number of additions, updates, deletions to server service data",
		},
		[]string{"stage", "change_kind"},
	)
}
