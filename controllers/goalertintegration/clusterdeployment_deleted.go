package goalertintegration

import (
	goalertv1alpha1 "github.com/openshift/configure-goalert-operator/api/v1alpha1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

func (r *GoalertIntegrationReconciler) handleDelete(gi *goalertv1alpha1.GoalertIntegration, cd *hivev1.ClusterDeployment) error {

	if cd == nil {
		return nil
	}
	// Retrieve service ID for input for DeleteService(data)

}

func getSvcId(cd hivev1.ClusterDeployment, isCommercial bool) string {

}
