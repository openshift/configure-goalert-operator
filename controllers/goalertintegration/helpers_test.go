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

	goalertv1alpha1 "github.com/openshift/configure-goalert-operator/api/v1alpha1"
	"github.com/openshift/configure-goalert-operator/pkg/goalert"
	hivev1 "github.com/openshift/hive/apis/hive/v1"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

// stubGoalertClient is a hand-written test double implementing goalert.Client.
// Each method delegates to its matching function field when that field is set,
// otherwise it returns zero values so that methods a given test does not care
// about stay harmless.
type stubGoalertClient struct {
	getServiceIDByName     func(ctx context.Context, name string) (string, error)
	createService          func(ctx context.Context, data *goalert.Data) (string, error)
	getIntegrationKeyHref  func(ctx context.Context, serviceID, keyName, keyType string) (string, error)
	createIntegrationKey   func(ctx context.Context, data *goalert.Data) (string, error)
	getHeartbeatMonitor    func(ctx context.Context, serviceID, monitorName string) (string, string, error)
	createHeartbeatMonitor func(ctx context.Context, data *goalert.Data) (string, string, error)
}

// compile-time assertion that stubGoalertClient satisfies the goalert.Client interface.
var _ goalert.Client = &stubGoalertClient{}

// GetServiceIDByName delegates to the getServiceIDByName field when set.
func (s *stubGoalertClient) GetServiceIDByName(ctx context.Context, name string) (string, error) {
	if s.getServiceIDByName != nil {
		return s.getServiceIDByName(ctx, name)
	}
	return "", nil
}

// CreateService delegates to the createService field when set.
func (s *stubGoalertClient) CreateService(ctx context.Context, data *goalert.Data) (string, error) {
	if s.createService != nil {
		return s.createService(ctx, data)
	}
	return "", nil
}

// GetIntegrationKeyHref delegates to the getIntegrationKeyHref field when set.
func (s *stubGoalertClient) GetIntegrationKeyHref(ctx context.Context, serviceID, keyName, keyType string) (string, error) {
	if s.getIntegrationKeyHref != nil {
		return s.getIntegrationKeyHref(ctx, serviceID, keyName, keyType)
	}
	return "", nil
}

// CreateIntegrationKey delegates to the createIntegrationKey field when set.
func (s *stubGoalertClient) CreateIntegrationKey(ctx context.Context, data *goalert.Data) (string, error) {
	if s.createIntegrationKey != nil {
		return s.createIntegrationKey(ctx, data)
	}
	return "", nil
}

// GetHeartbeatMonitor delegates to the getHeartbeatMonitor field when set.
func (s *stubGoalertClient) GetHeartbeatMonitor(ctx context.Context, serviceID, monitorName string) (string, string, error) {
	if s.getHeartbeatMonitor != nil {
		return s.getHeartbeatMonitor(ctx, serviceID, monitorName)
	}
	return "", "", nil
}

// CreateHeartbeatMonitor delegates to the createHeartbeatMonitor field when set.
func (s *stubGoalertClient) CreateHeartbeatMonitor(ctx context.Context, data *goalert.Data) (string, string, error) {
	if s.createHeartbeatMonitor != nil {
		return s.createHeartbeatMonitor(ctx, data)
	}
	return "", "", nil
}

// DeleteService is unused by the current tests and returns a nil error.
func (s *stubGoalertClient) DeleteService(ctx context.Context, data *goalert.Data) error {
	return nil
}

// NewRequest is unused by the current tests and returns zero values.
func (s *stubGoalertClient) NewRequest(ctx context.Context, method string, body any) ([]byte, error) {
	return nil, nil
}

// IsHeartbeatMonitorInactive is unused by the current tests and returns zero values.
func (s *stubGoalertClient) IsHeartbeatMonitorInactive(ctx context.Context, data *goalert.Data) (bool, error) {
	return false, nil
}

// newTestScheme builds a runtime.Scheme registered with the same types main.go
// registers: core Kubernetes, Hive, and the operator's own CRD. It is used to
// back the controller-runtime fake client in controller-level tests.
func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(hivev1.AddToScheme(scheme))
	utilruntime.Must(goalertv1alpha1.AddToScheme(scheme))
	return scheme
}
