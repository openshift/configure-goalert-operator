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

package config

import (
	"net/http"
	"time"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

// HTTPClient returns the shared HTTP client used for GoAlert API requests.
func HTTPClient() *http.Client {
	return httpClient
}

const (
	// OperatorName is the name of this operator, used for leader election and metrics.
	OperatorName string = "configure-goalert-operator"
	// OperatorNamespace is the namespace where the operator is deployed.
	OperatorNamespace string = "configure-goalert-operator"
	// GoalertUsernameSecretKey is the Secret data key for the GoAlert username.
	GoalertUsernameSecretKey string = "USERNAME"
	// GoalertPasswordSecretKey is the Secret data key for the GoAlert password.
	GoalertPasswordSecretKey string = "PASSWORD"
	// GoalertHighIntKey is the Secret data key for the high-severity integration URL.
	GoalertHighIntKey string = "GOALERT_URL_HIGH"
	// GoalertLowIntKey is the Secret data key for the low-severity integration URL.
	GoalertLowIntKey string = "GOALERT_URL_LOW"
	// GoalertHeartbeatIntKey is the Secret data key for the heartbeat monitor URL.
	GoalertHeartbeatIntKey string = "GOALERT_HEARTBEAT"
	// GoalertApiEndpointEnvVar is the environment variable holding the GoAlert API base URL.
	GoalertApiEndpointEnvVar string = "GOALERT_ENDPOINT_URL"
	// GoalertFinalizerPrefix is the prefix for finalizers added to ClusterDeployments.
	GoalertFinalizerPrefix string = "goalert.managed.openshift.io/goalert-"
	// ConfigMapSuffix is the suffix appended to ConfigMap names created per ClusterDeployment.
	ConfigMapSuffix string = "-goalert-config"
	// SecretName is the name of the Secret and SyncSet created per ClusterDeployment namespace.
	SecretName string = "goalert-secret"
	// GoalertHighServiceIDKey is the ConfigMap data key for the high-severity service ID.
	GoalertHighServiceIDKey string = "HIGH_SERVICE_ID"
	// GoalertLowServiceIDKey is the ConfigMap data key for the low-severity service ID.
	GoalertLowServiceIDKey string = "LOW_SERVICE_ID"
	// GoalertHeartbeatIDKey is the ConfigMap data key for the heartbeat monitor ID.
	GoalertHeartbeatIDKey string = "HEARTBEATMONITOR_ID"
)

// Name is used to generate the name of secondary resources (SyncSets,
// Secrets, ConfigMaps) for a ClusterDeployment that are created by
// the GoalertIntegration controller.
func Name(servicePrefix, clusterDeploymentName, suffix string) string {
	return servicePrefix + "-" + clusterDeploymentName + suffix
}
