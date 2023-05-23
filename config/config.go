package config

const (
	OperatorName              string = "configure-goalert-operator"
	OperatorNamespace         string = "openshift-configure-goalert-operator"
	GoalertUsernameSecretKey  string = "USERNAME"
	GoalertPasswordSecretKey  string = "PASSWORD"
	GoalertHighSecretKey      string = "GOALERT_URL_HIGH"
	GoalertLowSecretKey       string = "GOALERT_URL_LOW"
	GoalertHeartbeatSecretKey string = "GOALERT_HEARTBEAT"
	GoalertFinalizerPrefix    string = "goalert.managed.openshift.io/goalert-"
	ConfigMapSuffix           string = "-goalert-config"
	SecretSuffix              string = "-goalert-secret"
)

// Name is used to generate the name of secondary resources (SyncSets,
// Secrets, ConfigMaps) for a ClusterDeployment that are created by
// the GoalertIntegration controller.
func Name(servicePrefix, clusterDeploymentName, suffix string) string {
	return servicePrefix + "-" + clusterDeploymentName + suffix
}
