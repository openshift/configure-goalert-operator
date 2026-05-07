/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package localmetrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// ReconcileDuration tracks the duration of reconcile loops, broken down by controller.
	ReconcileDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        "cgao_reconcile_duration_seconds",
		Help:        "Distribution of the number of seconds a Reconcile takes, broken down by controller",
		ConstLabels: prometheus.Labels{"name": "configure-goalert-operator"},
		Buckets:     []float64{0.001, 0.01, 0.1, 1, 5, 10, 20},
	}, []string{"controller"})
	// MetricCGAOCreateFailure tracks failures when creating GoAlert services.
	MetricCGAOCreateFailure = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name:        "cgao_create_failure",
		Help:        "Metric for the number of failures creating Goalert service.",
		ConstLabels: prometheus.Labels{"name": "configure-goalert-operator"},
	}, []string{"service_name"})

	// MetricCGAODeleteFailure tracks failures when deleting GoAlert services.
	MetricCGAODeleteFailure = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name:        "cgao_delete_failure",
		Help:        "Metric for the number of failures deleting a Goalert service.",
		ConstLabels: prometheus.Labels{"name": "configure-goalert-operator"},
	}, []string{"service_name"})
	// MetricCGAOHeartbeatInactive tracks GoAlert heartbeat monitors that are in an inactive state.
	MetricCGAOHeartbeatInactive = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name:        "cgao_heartbeat_inactive",
		Help:        "Metric for inactive heartbeatmonitors in Goalert",
		ConstLabels: prometheus.Labels{"name": "configure-goalert-operator"},
	}, []string{"service_name"})
	// MetricsList is the list of all Prometheus collectors registered with the custom metrics server.
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
