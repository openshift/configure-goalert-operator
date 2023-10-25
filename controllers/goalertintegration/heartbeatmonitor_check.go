package goalertintegration

import (
	"context"

	goalertv1alpha1 "github.com/openshift/configure-goalert-operator/api/v1alpha1"
	"github.com/openshift/configure-goalert-operator/config"
	"github.com/openshift/configure-goalert-operator/pkg/goalert"
	"github.com/openshift/configure-goalert-operator/pkg/localmetrics"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/pingcap/errors"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (r *GoalertIntegrationReconciler) checkHeartbeatMonitor(ctx context.Context, gclient goalert.Client, gi *goalertv1alpha1.GoalertIntegration, cd *hivev1.ClusterDeployment) error {

	cmData := &v1.ConfigMap{Data: map[string]string{}}
	cmData.Name = config.Name(gi.Spec.ServicePrefix, cd.Name, config.ConfigMapSuffix)
	err := r.Get(ctx, types.NamespacedName{Name: cmData.Name, Namespace: cd.Namespace}, cmData)
	if err != nil {
		if !errors.IsNotFound(err) {
			// some error other than not found, requeue
			return err
		}
	}

	heartbeatmonitorId := cmData.Data["HEARTBEATMONITOR_ID"]

	isHeartbeatmonitorInactive, err := gclient.IsHeartbeatMonitorInactive(ctx, &goalert.Data{Id: heartbeatmonitorId})
	if err != nil {
		return err
	}

	getMetricValue := func(col prometheus.Collector) int {
		c := make(chan prometheus.Metric, 1)
		col.Collect(c)
		m := dto.Metric{}
		err := (<-c).Write(&m)
		if err != nil {
			return 0
		}
		return int(*m.Gauge.Value)
	}

	if isHeartbeatmonitorInactive {
		// Add metrics
		localmetrics.UpdateMetricCGAOHeartbeatInactive(1, cd.Name)
	} else {
		// If heartbeat is not inactive but metric value is more than 0, set to 0
		if gauge, err := localmetrics.MetricCGAOHeartbeatInactive.GetMetricWithLabelValues(cd.Name); gauge != nil && err == nil {
			if getMetricValue(gauge) > 0 {
				localmetrics.UpdateMetricCGAOHeartbeatInactive(0, cd.Name)
			}
		}
	}

	return nil

}
