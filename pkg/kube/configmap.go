package kube

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GenerateConfigMap returns a configmap that can be created with the oc client
func GenerateConfigMap(namespace string, cmName string, goalertHighServiceID, goalertLowServiceID, goalertHighIntegrationID, goalertLowIntegrationID, goalertHighEscalationPolicyID, goalertLowEscalationPlicyID string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: namespace,
		},
		Data: map[string]string{
			"HIGH_SERVICE_ID":           goalertHighServiceID,
			"LOW_SERVICE_ID":            goalertLowServiceID,
			"HIGH_INTEGRATION_ID":       goalertHighIntegrationID,
			"LOW_INTEGRATION_ID":        goalertLowIntegrationID,
			"HIGH_ESCALATION_POLICY_ID": goalertHighEscalationPolicyID,
			"LOW_ESCALATION_POLICY_ID":  goalertLowEscalationPlicyID,
		},
	}
}
