package kube

import (
	goalertv1alpha1 "github.com/openshift/configure-goalert-operator/api/v1alpha1"
	"github.com/openshift/configure-goalert-operator/config"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GenerateSyncSet returns a syncset that can be created with the oc client
func GenerateSyncSet(namespace string, clusterDeploymentName string, secret *corev1.Secret, gi *goalertv1alpha1.GoalertIntegration) *hivev1.SyncSet {
	return &hivev1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secret.Name,
			Namespace: namespace,
		},
		Spec: hivev1.SyncSetSpec{
			ClusterDeploymentRefs: []corev1.LocalObjectReference{
				{
					Name: clusterDeploymentName,
				},
			},
			SyncSetCommonSpec: hivev1.SyncSetCommonSpec{
				ResourceApplyMode: "Sync",
				Secrets: []hivev1.SecretMapping{
					{
						SourceRef: hivev1.SecretReference{
							Namespace: secret.Namespace,
							Name:      secret.Name,
						},
						TargetRef: hivev1.SecretReference{
							Namespace: gi.Spec.TargetSecretRef.Namespace,
							Name:      gi.Spec.TargetSecretRef.Name,
						},
					},
				},
			},
		},
	}
}

// GenerateGoalertSecret returns a secret that can be created with the oc client
func GenerateGoalertSecret(namespace string, name string, goalertHighIntegrationKey, goalertLowIntegrationKey, heartbeatKey string) *corev1.Secret {
	secret := &corev1.Secret{
		Type: "Opaque",
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			config.GoalertHighSecretKey:      []byte(goalertHighIntegrationKey),
			config.GoalertLowSecretKey:       []byte(goalertLowIntegrationKey),
			config.GoalertHeartbeatSecretKey: []byte(heartbeatKey),
		},
	}

	return secret
}
