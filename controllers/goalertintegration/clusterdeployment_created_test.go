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
	"errors"
	"testing"

	"github.com/go-logr/logr"
	goalertv1alpha1 "github.com/openshift/configure-goalert-operator/api/v1alpha1"
	"github.com/openshift/configure-goalert-operator/config"
	"github.com/openshift/configure-goalert-operator/pkg/goalert"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestEnsureService covers the get-or-create behaviour of ensureService:
// an existing service is returned without creating, a missing service is
// created, and both GET and CREATE errors are propagated.
func TestEnsureService(t *testing.T) {
	tests := []struct {
		name         string
		getID        string
		getErr       error
		createID     string
		createErr    error
		expectedID   string
		expectedErr  bool
		expectCreate bool
	}{
		{
			name:         "existing service returns its id without creating",
			getID:        "existing-svc-id",
			expectedID:   "existing-svc-id",
			expectedErr:  false,
			expectCreate: false,
		},
		{
			name:         "missing service is created",
			getID:        "",
			createID:     "new-svc-id",
			expectedID:   "new-svc-id",
			expectedErr:  false,
			expectCreate: true,
		},
		{
			name:         "get error is propagated without creating",
			getErr:       errors.New("get failed"),
			expectedErr:  true,
			expectCreate: false,
		},
		{
			name:         "create error is propagated",
			getID:        "",
			createErr:    errors.New("create failed"),
			expectedErr:  true,
			expectCreate: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			createCalled := false
			stub := &stubGoalertClient{
				getServiceIDByName: func(_ context.Context, _ string) (string, error) {
					return test.getID, test.getErr
				},
				createService: func(_ context.Context, _ *goalert.Data) (string, error) {
					createCalled = true
					return test.createID, test.createErr
				},
			}

			id, err := ensureService(ctx, stub, &goalert.Data{Name: "fedramp-abc123 - High"})

			if test.expectedErr {
				assert.NotNil(t, err)
			} else {
				assert.Equal(t, test.expectedID, id)
				assert.Nil(t, err)
			}
			assert.Equal(t, test.expectCreate, createCalled)
		})
	}
}

// TestEnsureIntegrationKey covers the get-or-create behaviour of
// ensureIntegrationKey: an existing key href is returned without creating, a
// missing key is created, and both GET and CREATE errors are propagated.
func TestEnsureIntegrationKey(t *testing.T) {
	tests := []struct {
		name         string
		getHref      string
		getErr       error
		createHref   string
		createErr    error
		expectedHref string
		expectedErr  bool
		expectCreate bool
	}{
		{
			name:         "existing integration key returns its href without creating",
			getHref:      "/api/v2/generic/incoming?token=high",
			expectedHref: "/api/v2/generic/incoming?token=high",
			expectedErr:  false,
			expectCreate: false,
		},
		{
			name:         "missing integration key is created",
			getHref:      "",
			createHref:   "/api/v2/generic/incoming?token=new",
			expectedHref: "/api/v2/generic/incoming?token=new",
			expectedErr:  false,
			expectCreate: true,
		},
		{
			name:         "get error is propagated without creating",
			getErr:       errors.New("get failed"),
			expectedErr:  true,
			expectCreate: false,
		},
		{
			name:         "create error is propagated",
			getHref:      "",
			createErr:    errors.New("create failed"),
			expectedErr:  true,
			expectCreate: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			createCalled := false
			stub := &stubGoalertClient{
				getIntegrationKeyHref: func(_ context.Context, _, _, _ string) (string, error) {
					return test.getHref, test.getErr
				},
				createIntegrationKey: func(_ context.Context, _ *goalert.Data) (string, error) {
					createCalled = true
					return test.createHref, test.createErr
				},
			}

			href, err := ensureIntegrationKey(ctx, stub, &goalert.Data{Id: "svc-1", Name: "High alerts", Type: "prometheusAlertmanager"})

			if test.expectedErr {
				assert.NotNil(t, err)
			} else {
				assert.Equal(t, test.expectedHref, href)
				assert.Nil(t, err)
			}
			assert.Equal(t, test.expectCreate, createCalled)
		})
	}
}

// TestEnsureHeartbeatMonitor covers the get-or-create behaviour of
// ensureHeartbeatMonitor, including the case where a href is present but the id
// is empty (which must still trigger a create because both are required).
func TestEnsureHeartbeatMonitor(t *testing.T) {
	tests := []struct {
		name         string
		getHref      string
		getID        string
		getErr       error
		createHref   string
		createID     string
		createErr    error
		expectedHref string
		expectedID   string
		expectedErr  bool
		expectCreate bool
	}{
		{
			name:         "existing monitor returns its href and id without creating",
			getHref:      "/api/v2/heartbeat/abc",
			getID:        "hb-existing",
			expectedHref: "/api/v2/heartbeat/abc",
			expectedID:   "hb-existing",
			expectedErr:  false,
			expectCreate: false,
		},
		{
			name:         "missing monitor is created",
			createHref:   "/api/v2/heartbeat/new",
			createID:     "hb-new",
			expectedHref: "/api/v2/heartbeat/new",
			expectedID:   "hb-new",
			expectedErr:  false,
			expectCreate: true,
		},
		{
			name:         "href present but id empty triggers create",
			getHref:      "/api/v2/heartbeat/abc",
			getID:        "",
			createHref:   "/api/v2/heartbeat/new",
			createID:     "hb-new",
			expectedHref: "/api/v2/heartbeat/new",
			expectedID:   "hb-new",
			expectedErr:  false,
			expectCreate: true,
		},
		{
			name:         "get error is propagated without creating",
			getErr:       errors.New("get failed"),
			expectedErr:  true,
			expectCreate: false,
		},
		{
			name:         "create error is propagated",
			createErr:    errors.New("create failed"),
			expectedErr:  true,
			expectCreate: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			createCalled := false
			stub := &stubGoalertClient{
				getHeartbeatMonitor: func(_ context.Context, _, _ string) (string, string, error) {
					return test.getHref, test.getID, test.getErr
				},
				createHeartbeatMonitor: func(_ context.Context, _ *goalert.Data) (string, string, error) {
					createCalled = true
					return test.createHref, test.createID, test.createErr
				},
			}

			href, id, err := ensureHeartbeatMonitor(ctx, stub, &goalert.Data{Id: "svc-1", Name: "fedramp-abc123", Timeout: 15})

			if test.expectedErr {
				assert.NotNil(t, err)
			} else {
				assert.Equal(t, test.expectedHref, href)
				assert.Equal(t, test.expectedID, id)
				assert.Nil(t, err)
			}
			assert.Equal(t, test.expectCreate, createCalled)
		})
	}
}

// TestHandleCreateEmptySecretGuard verifies that handleCreate refuses to write a
// goalert Secret when any integration key or the heartbeat monitor key is empty.
// It returns an error and leaves no Secret behind so the next reconcile retries.
func TestHandleCreateEmptySecretGuard(t *testing.T) {
	ctx := context.Background()
	scheme := newTestScheme(t)

	gi := &goalertv1alpha1.GoalertIntegration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gi",
			Namespace: "test-ns",
		},
		Spec: goalertv1alpha1.GoalertIntegrationSpec{
			ServicePrefix:        "test",
			HighEscalationPolicy: "high-ep",
			LowEscalationPolicy:  "low-ep",
		},
	}

	// The ClusterDeployment must already carry the finalizer so handleCreate does
	// not return early after adding it via Patch, and instead reaches the guard.
	cd := &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-cd",
			Namespace:  "cluster-ns-abc123",
			Finalizers: []string{config.GoalertFinalizerPrefix + gi.Name},
		},
		Spec: hivev1.ClusterDeploymentSpec{
			ClusterName: "test-cluster",
		},
	}

	// Services resolve to a non-empty ID (so the ConfigMap is written), but the
	// integration keys and heartbeat monitor key come back empty -- exactly the
	// condition the guard must catch.
	stub := &stubGoalertClient{
		getServiceIDByName: func(_ context.Context, _ string) (string, error) {
			return "existing-svc-id", nil
		},
		getIntegrationKeyHref: func(_ context.Context, _, _, _ string) (string, error) {
			return "", nil
		},
		createIntegrationKey: func(_ context.Context, _ *goalert.Data) (string, error) {
			return "", nil
		},
		getHeartbeatMonitor: func(_ context.Context, _, _ string) (string, string, error) {
			return "", "", nil
		},
		createHeartbeatMonitor: func(_ context.Context, _ *goalert.Data) (string, string, error) {
			return "", "", nil
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &GoalertIntegrationReconciler{
		Client:    fakeClient,
		Scheme:    scheme,
		reqLogger: logr.Discard(),
	}

	err := r.handleCreate(ctx, stub, gi, cd)

	assert.NotNil(t, err)
	assert.ErrorContains(t, err, "refusing to write goalert secret")

	// The guard must prevent any Secret from being written.
	secretErr := r.Get(ctx, types.NamespacedName{Name: config.SecretName, Namespace: cd.Namespace}, &corev1.Secret{})
	assert.True(t, apierrors.IsNotFound(secretErr))
}
