package config

const (
	OperatorName             string = "configure-goalert-operator"
	OperatorNamespace        string = "openshift-configure-goalert-operator"
	GoalertUsernameSecretKey string = "USERNAME"
	GoalertPasswordSecretKey string = "PASSWORD"
)

// Name is used to generate the name of secondary resources (SyncSets,
// Secrets, ConfigMaps) for a ClusterDeployment that are created by
// the GoalertIntegration controller.
func Name(servicePrefix, clusterDeploymentName, suffix string) string {
	return servicePrefix + "-" + clusterDeploymentName + suffix
}
