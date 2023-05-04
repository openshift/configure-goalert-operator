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

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	goalertv1alpha1 "github.com/openshift/configure-goalert-operator/api/v1alpha1"
	"github.com/openshift/configure-goalert-operator/pkg/utils"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	GoalertFinalizerPrefix = "goalert.managed.openshift.io/goalert-"
)

// GoalertIntegrationReconciler reconciles a GoalertIntegration object
type GoalertIntegrationReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	reqLogger logr.Logger
}

//+kubebuilder:rbac:groups=goalert.managed.openshift.io,resources=goalertintegrations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=goalert.managed.openshift.io,resources=goalertintegrations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=goalert.managed.openshift.io,resources=goalertintegrations/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the GoalertIntegration object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *GoalertIntegrationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// TODO(user): your logic here
	gi := &goalertv1alpha1.GoalertIntegration{}
	err := r.Get(context.TODO(), req.NamespacedName, gi)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return r.doNotRequeue()
		}
		// Error reading the object - requeue the request.
		return r.requeueOnErr(err)
	}

	// fetch all CDs so we can inspect if they're dropped out of the matching CD list
	allClusterDeployments, err := r.getAllClusterDeployments()
	if err != nil {
		return r.requeueOnErr(err)
	}

	// Fetch ClusterDeployments matching the GI's ClusterDeployment label selector
	matchingClusterDeployments, err := r.getMatchingClusterDeployments(gi)
	if err != nil {
		return r.requeueOnErr(err)
	}

	// Add authentication to GraphQL API OSD-16252
	if err != nil {
		r.reqLogger.Error(err, "Failed to load Goalert API key from Secret listed in GoalertIntegration CR")
	}

	goalertFinalizer := GoalertFinalizerPrefix + gi.Name
	//If the GI is being deleted, clean up all ClusterDeployments with matching finalizers
	if gi.DeletionTimestamp != nil {
		for i := range matchingClusterDeployments.Items {
			clusterdeployment := allClusterDeployments.Items[i]
			if utils.HasFinalizer(&clusterdeployment, goalertFinalizer) {
				// Handle deletion of cluster OSD-16305
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *GoalertIntegrationReconciler) getAllClusterDeployments() (*hivev1.ClusterDeploymentList, error) {
	allClusterDeployments := &hivev1.ClusterDeploymentList{}
	err := r.List(context.TODO(), allClusterDeployments, &client.ListOptions{})
	return allClusterDeployments, err
}

func (r *GoalertIntegrationReconciler) getMatchingClusterDeployments(gi *goalertv1alpha1.GoalertIntegration) (*hivev1.ClusterDeploymentList, error) {
	selector, err := metav1.LabelSelectorAsSelector(&gi.Spec.ClusterDeploymentSelector)
	if err != nil {
		return nil, err
	}

	matchingClusterDeployments := &hivev1.ClusterDeploymentList{}
	listOpts := &client.ListOptions{LabelSelector: selector}
	err = r.List(context.TODO(), matchingClusterDeployments, listOpts)
	return matchingClusterDeployments, err
}

func (r *GoalertIntegrationReconciler) doNotRequeue() (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

func (r *GoalertIntegrationReconciler) requeueOnErr(err error) (reconcile.Result, error) {
	return reconcile.Result{}, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *GoalertIntegrationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&goalertv1alpha1.GoalertIntegration{}).
		Complete(r)
}
