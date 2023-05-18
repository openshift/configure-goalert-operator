package goalertintegration

import (
	goalertv1alpha1 "github.com/openshift/configure-goalert-operator/api/v1alpha1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

// Scaffold of func to handle creation of new clusters OSD-16306
func (r *GoalertIntegrationReconciler) handleCreate(gi *goalertv1alpha1.GoalertIntegration, cd *hivev1.ClusterDeployment) error {

}
