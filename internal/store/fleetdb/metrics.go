package fleetdb

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// stageLabel is the label included in all metrics collected by the fleetdb store
	stageLabel = prometheus.Labels{"stage": "fleetdb"}

	// metricAssetComponentsIdentified measures the count of hardware components in the device components data from the collector.
	metricAssetComponentsIdentified *prometheus.GaugeVec

	// metricFleetDBDataChanges measures the number of server component data additions, updates, deletes.
	metricFleetDBDataChanges *prometheus.GaugeVec

	// metricInventorized count measures the number of assets inventorized - both successful and not.
	metricInventorized *prometheus.GaugeVec

	// metricBiosCfgCollected count measures the number of assets of which BIOS configuration was collected - both successful and not.
	metricBiosCfgCollected *prometheus.GaugeVec
)

func init() {
	metricAssetComponentsIdentified = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "alloy_asset_components_identified",
			Help: "A gauge metric to count of number hardware components identified by the publisher.",
		},
		[]string{"stage", "vendor", "model"},
	)

	metricFleetDBDataChanges = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "alloy_serverservice_data_changes",
			Help: "A gauge metric to measure the number of additions, updates, deletions to server service data",
		},
		[]string{"stage", "change_kind"},
	)

	metricInventorized = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "alloy_assets_inventoried_count",
			Help: "A gauge metric to count the total assets inventoried - successful and not.",
		},
		// status is one of success/failure
		[]string{"status"},
	)

	metricBiosCfgCollected = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "alloy_assets_bios_cfg_collected_count",
			Help: "A gauge metric to count measures the number of assets of which BIOS configuration was collected - both successful and not.",
		},
		// status is one of success/failure
		[]string{"status"},
	)
}
