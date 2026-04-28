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

var HTTPClient = &http.Client{Timeout: 30 * time.Second}

const (
	OperatorName             string = "configure-goalert-operator"
	OperatorNamespace        string = "configure-goalert-operator"
	GoalertUsernameSecretKey string = "USERNAME"
	GoalertPasswordSecretKey string = "PASSWORD"
	GoalertHighIntKey        string = "GOALERT_URL_HIGH"
	GoalertLowIntKey         string = "GOALERT_URL_LOW"
	GoalertHeartbeatIntKey   string = "GOALERT_HEARTBEAT"
	GoalertApiEndpointEnvVar string = "GOALERT_ENDPOINT_URL"
	GoalertFinalizerPrefix   string = "goalert.managed.openshift.io/goalert-"
	ConfigMapSuffix          string = "-goalert-config"
	SecretName               string = "goalert-secret"
	GoalertHighServiceIDKey  string = "HIGH_SERVICE_ID"
	GoalertLowServiceIDKey   string = "LOW_SERVICE_ID"
	GoalertHeartbeatIDKey    string = "HEARTBEATMONITOR_ID"
)

// Name is used to generate the name of secondary resources (SyncSets,
// Secrets, ConfigMaps) for a ClusterDeployment that are created by
// the GoalertIntegration controller.
func Name(servicePrefix, clusterDeploymentName, suffix string) string {
	return servicePrefix + "-" + clusterDeploymentName + suffix
}
