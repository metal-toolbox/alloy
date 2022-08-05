package asset

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Getter specific metrics are defined and initialized here

var (
	// stageLabel is the label included in all metrics collected by the getter
	stageLabel = prometheus.Labels{"stage": "getter"}
)
