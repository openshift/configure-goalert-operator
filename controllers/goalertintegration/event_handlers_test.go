package goalertintegration

import (
	"fmt"
	"testing"

	"github.com/openshift/configure-goalert-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNilSelector(t *testing.T) {
	gai := &v1alpha1.GoalertIntegration{}
	s, _ := metav1.LabelSelectorAsSelector(&gai.Spec.ClusterDeploymentSelector)
	fmt.Printf("empty=%v", s.Empty())
}
