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
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	goalertv1alpha1 "github.com/openshift/configure-goalert-operator/api/v1alpha1"
	"github.com/openshift/configure-goalert-operator/config"
	"github.com/openshift/configure-goalert-operator/pkg/goalert"
	"github.com/openshift/configure-goalert-operator/pkg/utils"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ControllerName = "goalertintegration"
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
	r.reqLogger = log.FromContext(ctx).WithName("controller").WithName(ControllerName)

	// Fetch the GoalertIntegration instance
	gi := &goalertv1alpha1.GoalertIntegration{}
	var gclient goalert.Client
	err := r.Get(ctx, req.NamespacedName, gi)
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
	// allClusterDeployments, err := r.getAllClusterDeployments(ctx)
	// if err != nil {
	// 	return r.requeueOnErr(err)
	// }

	// Fetch ClusterDeployments matching the GI's ClusterDeployment label selector
	matchingClusterDeployments, err := r.GetMatchingClusterDeployments(gi)
	if err != nil {
		return r.requeueOnErr(err)
	}

	// Load creds for Goalert authentication
	goalertUsername, err := utils.LoadSecretData(
		r.Client,
		gi.Spec.GoalertCredsSecretRef.Name,
		gi.Spec.GoalertCredsSecretRef.Namespace,
		config.GoalertUsernameSecretKey,
	)
	if err != nil {
		r.reqLogger.Error(err, "Failed to load Goalert username key from Secret listed in GoalertIntegration CR")
	}
	goalertPassword, err := utils.LoadSecretData(
		r.Client,
		gi.Spec.GoalertCredsSecretRef.Name,
		gi.Spec.GoalertCredsSecretRef.Namespace,
		config.GoalertPasswordSecretKey,
	)
	if err != nil {
		r.reqLogger.Error(err, "Failed to load Goalert password key from Secret listed in GoalertIntegration CR")
	}

	// Log in to Goalert
	authenticateGoalert, err := r.authGoalert(goalertUsername, goalertPassword)
	if err != nil {
		r.reqLogger.Error(err, "Failed to auth to Goalert")
	}

	fmt.Println("HTTP Response:")
	fmt.Println(authenticateGoalert.Header.Values("location"))
	// Read session cookie from authentication response headers
	sessionCookie, err := authenticateGoalert.Request.Cookie("goalert_session.2")
	if err != nil {
		r.reqLogger.Error(err, "Error extracting goalert_session.2 cookie")
	}

	// goalertFinalizer := config.GoalertFinalizerPrefix + gi.Name
	// //If the GI is being deleted, clean up all ClusterDeployments with matching finalizers
	// if gi.DeletionTimestamp != nil {
	// 	for i := range matchingClusterDeployments.Items {
	// 		clusterdeployment := allClusterDeployments.Items[i]
	// 		// !! COMMENTED OUT FOR PROW -- NEED LOGIC FOR DELETION !! //
	// 		// if util.ContainsFinalizer(&clusterdeployment, goalertFinalizer) {
	// 		// 	// Handle deletion of cluster OSD-16305
	// 		// }
	// 	}
	// }

	for _, cd := range matchingClusterDeployments.Items {
		cd := cd
		if cd.DeletionTimestamp == nil {
			if err := r.handleCreate(gclient, gi, sessionCookie, &cd); err != nil {
				r.reqLogger.Error(err, "Failing to register cluster with Goalert")
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *GoalertIntegrationReconciler) authGoalert(username string, password string) (*http.Response, error) {

	// Create HTTP POST request for authentication
	goalertApiEndpoint := os.Getenv(config.GoalertApiEndpointEnvVar)
	authUrl := goalertApiEndpoint + "/api/v2/identity/providers/basic"
	reqBody := fmt.Sprintf("username=%s&password=%s", username, password)
	authReq, err := http.NewRequest("POST", authUrl, bytes.NewBuffer([]byte(reqBody)))
	if err != nil {
		r.reqLogger.Error(err, "Failed to create HTTP request to auth to Goalert")
	}
	cookie := &http.Cookie{
		Name:  "login_redir",
		Value: goalertApiEndpoint + "/users",
	}
	authReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	authReq.Header.Set("Referer", goalertApiEndpoint+"/alerts")
	authReq.AddCookie(cookie)
	client := &http.Client{}

	authResp, err := client.Do(authReq)
	if err != nil {
		r.reqLogger.Error(err, "Error sending HTTP request:", err)
	}

	return authResp, nil
}
func (r *GoalertIntegrationReconciler) GetAllClusterDeployments(ctx context.Context) (*hivev1.ClusterDeploymentList, error) {
	allClusterDeployments := &hivev1.ClusterDeploymentList{}
	err := r.List(ctx, allClusterDeployments, &client.ListOptions{})
	return allClusterDeployments, err
}

func (r *GoalertIntegrationReconciler) GetMatchingClusterDeployments(gi *goalertv1alpha1.GoalertIntegration) (*hivev1.ClusterDeploymentList, error) {
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
