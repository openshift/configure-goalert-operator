package localmetrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	ReconcileDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        "cgao_reconcile_duration_seconds",
		Help:        "Distribution of the number of seconds a Reconcile takes, broken down by controller",
		ConstLabels: prometheus.Labels{"name": "configure-goalert-operator"},
		Buckets:     []float64{0.001, 0.01, 0.1, 1, 5, 10, 20},
	}, []string{"controller"})
	MetricCGAOCreateFailure = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name:        "cgao_create_failure",
		Help:        "Metric for the number of failures creating Goalert service.",
		ConstLabels: prometheus.Labels{"name": "configure-goalert-operator"},
	}, []string{"service_name"})

	MetricCGAODeleteFailure = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name:        "cgao_delete_failure",
		Help:        "Metric for the number of failures deleting a Goalert service.",
		ConstLabels: prometheus.Labels{"name": "configure-goalert-operator"},
	}, []string{"service_name"})
	MetricCGAOHeartbeatInactive = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name:        "cgao_heartbeat_inactive",
		Help:        "Metric for inactive heartbeatmonitors in Goalert",
		ConstLabels: prometheus.Labels{"name": "configure-goalert-operator"},
	}, []string{"service_name"})
	MetricsList = []prometheus.Collector{
		ReconcileDuration,
		MetricCGAOCreateFailure,
		MetricCGAODeleteFailure,
		MetricCGAOHeartbeatInactive,
	}
)

// SetReconcileDuration tracks the duration of the reconcile loop
func SetReconcileDuration(controller string, duration float64) {
	ReconcileDuration.WithLabelValues(controller).Observe(duration)
}

// UpdateMetricCGAOCreateFailure updates gauge to 1 when creation fails
func UpdateMetricCGAOCreateFailure(x int, svc string) {
	MetricCGAOCreateFailure.With(prometheus.Labels{
		"service_name": svc,
	}).Set(float64(x))
}

// UpdateMetricCGAODeleteFailure updates gauge to 1 when deletion fails
func UpdateMetricCGAODeleteFailure(x int, svc string) {
	MetricCGAODeleteFailure.With(prometheus.Labels{
		"service_name": svc,
	}).Set(float64(x))
}

// UpdateMetricCGAOHeartbeatInactive updates gauge to 1 when heartbeat is inactive
func UpdateMetricCGAOHeartbeatInactive(x int, svc string) {
	MetricCGAOHeartbeatInactive.With(prometheus.Labels{
		"service_name": svc,
	}).Set(float64(x))
}

// DeleteMetricCGAOHeartbeat removes heartbeat metrics for clusters in deletion
func DeleteMetricCGAOHeartbeat(svc string) bool {
	return MetricCGAOHeartbeatInactive.Delete(prometheus.Labels{
		"service_name": svc,
	})
}
