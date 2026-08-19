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

package goalertintegration

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	goalertv1alpha1 "github.com/openshift/configure-goalert-operator/api/v1alpha1"
	"github.com/openshift/configure-goalert-operator/config"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestCgaoResourcesExistSecretContentAware exercises the content-aware Secret
// check in cgaoResourcesExist: a Secret only counts as existing when it is
// present AND all three integration-key data keys are non-empty. It asserts the
// secretExist return position (the second of four return values).
func TestCgaoResourcesExistSecretContentAware(t *testing.T) {
	scheme := newTestScheme(t)

	gi := &goalertv1alpha1.GoalertIntegration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gi",
			Namespace: "test-ns",
		},
		Spec: goalertv1alpha1.GoalertIntegrationSpec{
			ServicePrefix: "test",
		},
	}
	cd := &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cd",
			Namespace: "cluster-ns-abc123",
		},
	}

	tests := []struct {
		name               string
		objects            []client.Object
		expectedSecretBool bool
	}{
		{
			name:               "no secret present returns false",
			objects:            nil,
			expectedSecretBool: false,
		},
		{
			name: "secret with empty data returns false",
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      config.SecretName,
						Namespace: cd.Namespace,
					},
				},
			},
			expectedSecretBool: false,
		},
		{
			name: "secret missing one integration key returns false",
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      config.SecretName,
						Namespace: cd.Namespace,
					},
					Data: map[string][]byte{
						config.GoalertHighIntKey: []byte("high-url"),
						config.GoalertLowIntKey:  []byte("low-url"),
					},
				},
			},
			expectedSecretBool: false,
		},
		{
			name: "secret with all three integration keys present but empty returns false",
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      config.SecretName,
						Namespace: cd.Namespace,
					},
					Data: map[string][]byte{
						config.GoalertHighIntKey:      {},
						config.GoalertLowIntKey:       {},
						config.GoalertHeartbeatIntKey: {},
					},
				},
			},
			expectedSecretBool: false,
		},
		{
			name: "secret with all three integration keys populated returns true",
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      config.SecretName,
						Namespace: cd.Namespace,
					},
					Data: map[string][]byte{
						config.GoalertHighIntKey:      []byte("high-url"),
						config.GoalertLowIntKey:       []byte("low-url"),
						config.GoalertHeartbeatIntKey: []byte("hb-url"),
					},
				},
			},
			expectedSecretBool: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(test.objects...).
				Build()
			r := &GoalertIntegrationReconciler{
				Client:    fakeClient,
				Scheme:    scheme,
				reqLogger: logr.Discard(),
			}

			_, secretExist, _, err := r.cgaoResourcesExist(ctx, gi, cd)

			assert.Nil(t, err)
			assert.Equal(t, test.expectedSecretBool, secretExist)
		})
	}
}
