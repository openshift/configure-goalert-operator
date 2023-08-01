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
	"net/url"
	"os"
	"strings"

	"golang.org/x/net/context/ctxhttp"

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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	ControllerName = "goalertintegration"
)

// GoalertIntegrationReconciler reconciles a GoalertIntegration object
type GoalertIntegrationReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	reqLogger logr.Logger
	gclient   func(sessionCookie *http.Cookie) goalert.Client
}

//+kubebuilder:rbac:groups=goalert.managed.openshift.io,resources=goalertintegrations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=goalert.managed.openshift.io,resources=goalertintegrations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=goalert.managed.openshift.io,resources=goalertintegrations/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// Modify the Reconcile function to compare the state specified by
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

	// fetch all CDs, so we can inspect if they're dropped out of the matching CD list
	allClusterDeployments, err := r.GetAllClusterDeployments(ctx)
	if err != nil {
		return r.requeueOnErr(err)
	}

	// Fetch ClusterDeployments matching the GI's ClusterDeployment label selector
	matchingClusterDeployments, err := r.GetMatchingClusterDeployments(ctx, gi)
	if err != nil {
		return r.requeueOnErr(err)
	}

	// Load creds for Goalert authentication
	goalertUsername, err := utils.LoadSecretData(
		ctx,
		r.Client,
		gi.Spec.GoalertCredsSecretRef.Name,
		gi.Spec.GoalertCredsSecretRef.Namespace,
		config.GoalertUsernameSecretKey,
	)
	if err != nil {
		r.reqLogger.Error(err, "Failed to load Goalert username key from Secret listed in GoalertIntegration CR")
	}
	goalertPassword, err := utils.LoadSecretData(
		ctx,
		r.Client,
		gi.Spec.GoalertCredsSecretRef.Name,
		gi.Spec.GoalertCredsSecretRef.Namespace,
		config.GoalertPasswordSecretKey,
	)
	if err != nil {
		r.reqLogger.Error(err, "Failed to load Goalert password key from Secret listed in GoalertIntegration CR")
	}

	// Log in to Goalert
	authenticateGoalert, err := r.authGoalert(ctx, goalertUsername, goalertPassword)
	if err != nil {
		r.reqLogger.Error(err, "Failed to auth to Goalert")
	}

	// Read session cookie from authentication response headers
	sessionCookie, err := r.fetchSessionCookie(authenticateGoalert)
	if err != nil {
		r.reqLogger.Error(err, "Error fetching goalert_session.2 cookie")
	}
	graphqlClient := r.gclient(sessionCookie)
	goalertFinalizer := config.GoalertFinalizerPrefix + gi.Name

	//If the GI is being deleted, clean up all ClusterDeployments with matching finalizers
	if gi.DeletionTimestamp != nil {
		if controllerutil.ContainsFinalizer(gi, goalertFinalizer) {
			for i := range matchingClusterDeployments.Items {
				clusterDeployment := allClusterDeployments.Items[i]
				if controllerutil.ContainsFinalizer(&clusterDeployment, goalertFinalizer) {
					if err = r.handleDelete(ctx, graphqlClient, gi, &clusterDeployment); err != nil {
						r.reqLogger.Error(err, "failing to remove cluster service from GoAlert")
						return r.requeueOnErr(err)
					}
				}
			}
			if !controllerutil.RemoveFinalizer(gi, goalertFinalizer) {
				if err = r.Update(ctx, gi); err != nil {
					return r.requeueOnErr(err)
				}
			}
		}
		return r.doNotRequeue()
	}

	//Make sure there's a finalizer on the GoalertIntegration
	if !controllerutil.ContainsFinalizer(gi, goalertFinalizer) {
		if !controllerutil.AddFinalizer(gi, goalertFinalizer) {
			if err = r.Update(ctx, gi); err != nil {
				return r.requeueOnErr(err)
			}
		}
	}

	for i := range allClusterDeployments.Items {
		cd := allClusterDeployments.Items[i]
		if controllerutil.ContainsFinalizer(&cd, goalertFinalizer) {
			cdDeleteTime := cd.DeletionTimestamp
			if cdDeleteTime != nil {
				if err = r.handleDelete(ctx, graphqlClient, gi, &cd); err != nil {
					r.reqLogger.Error(err, "failing to remove cluster service from GoAlert")
					return r.requeueOnErr(err)
				}
			}
			cdMatches := false
			for _, mcd := range matchingClusterDeployments.Items {
				if cd.Namespace == mcd.Namespace && cd.Name == mcd.Name {
					cdMatches = true
					break
				}
			}
			if !cdMatches {
				r.reqLogger.Info("cleaning up %s as it does not have a matching label", "clusterdeployment", cd.Name)
				err = r.handleDelete(ctx, graphqlClient, gi, &cd)
				if err != nil {
					r.reqLogger.Error(err, "unmatched clusterdeployment, failed to remove associated goalert service", "clusterdeployment", cd.Name)
				}
			}
		}
	}

	for i := range matchingClusterDeployments.Items {
		cd := matchingClusterDeployments.Items[i]
		if cd.DeletionTimestamp == nil {
			if err := r.handleCreate(ctx, graphqlClient, gi, &cd); err != nil {
				r.reqLogger.Error(err, "failing to register cluster with Goalert")
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *GoalertIntegrationReconciler) authGoalert(ctx context.Context, username string, password string) (*http.Response, error) {

	// Create authentication endpoint
	goalertApiEndpoint := os.Getenv(config.GoalertApiEndpointEnvVar)
	authUrl := goalertApiEndpoint + "/api/v2/identity/providers/basic"

	// Create form data to be sent in the request body
	form := url.Values{}
	form.Add("username", username)
	form.Add("password", password)

	// Encode form data and create HTTP request
	authReq, err := http.NewRequestWithContext(ctx, "POST", authUrl, bytes.NewBufferString(form.Encode()))
	if err != nil {
		r.reqLogger.Error(err, "Failed to create HTTP request to auth to Goalert")
	}

	authReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	authReq.Header.Set("Referer", goalertApiEndpoint+"/alerts")

	// Send HTTP request and get response
	authResp, err := ctxhttp.Do(ctx, http.DefaultClient, authReq)
	if err != nil {
		r.reqLogger.Error(err, "Error sending HTTP request")
	}

	defer authResp.Body.Close()
	return authResp.Request.Response, nil
}

var ErrSessionCookieMissing = fmt.Errorf("session cookie is missing")

func (r *GoalertIntegrationReconciler) fetchSessionCookie(response *http.Response) (*http.Cookie, error) {

	var strCookie string
	var httpCookie *http.Cookie
	for _, cookie := range response.Header.Values("set-cookie") {
		if strings.Contains(cookie, "goalert_session.2") {
			strCookie = substringAfter(cookie, "=")
		}
	}

	if strCookie != "" {
		httpCookie = &http.Cookie{
			Name:  "goalert_session.2",
			Value: strCookie,
		}
	} else {
		r.reqLogger.Error(ErrSessionCookieMissing, "goalert_session.2 is empty")
		return nil, ErrSessionCookieMissing
	}
	return httpCookie, nil
}

func substringAfter(s string, sep string) string {
	substrings := strings.SplitAfter(s, sep)
	if len(substrings) > 1 {
		return substrings[1]
	} else {
		return ""
	}
}

func (r *GoalertIntegrationReconciler) GetAllClusterDeployments(ctx context.Context) (*hivev1.ClusterDeploymentList, error) {
	allClusterDeployments := &hivev1.ClusterDeploymentList{}
	err := r.List(ctx, allClusterDeployments, &client.ListOptions{})
	return allClusterDeployments, err
}

func (r *GoalertIntegrationReconciler) GetMatchingClusterDeployments(ctx context.Context, gi *goalertv1alpha1.GoalertIntegration) (*hivev1.ClusterDeploymentList, error) {
	selector, err := metav1.LabelSelectorAsSelector(&gi.Spec.ClusterDeploymentSelector)
	if err != nil {
		return nil, err
	}

	matchingClusterDeployments := &hivev1.ClusterDeploymentList{}
	listOpts := &client.ListOptions{LabelSelector: selector}
	err = r.List(ctx, matchingClusterDeployments, listOpts)
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
	r.gclient = goalert.NewClient
	return ctrl.NewControllerManagedBy(mgr).
		For(&goalertv1alpha1.GoalertIntegration{}).
		Complete(r)
}
