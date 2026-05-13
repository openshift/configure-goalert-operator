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

//goland:noinspection SpellCheckingInspection
import (
	"context"
	"strings"

	"github.com/openshift/configure-goalert-operator/pkg/localmetrics"

	goalertv1alpha1 "github.com/openshift/configure-goalert-operator/api/v1alpha1"
	"github.com/openshift/configure-goalert-operator/config"
	"github.com/openshift/configure-goalert-operator/pkg/goalert"
	"github.com/openshift/configure-goalert-operator/pkg/kube"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/pingcap/errors"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// handleCreate provisions GoAlert services, integration keys, and a heartbeat monitor for a ClusterDeployment, then creates the corresponding ConfigMap, Secret, and SyncSet.
func (r *GoalertIntegrationReconciler) handleCreate(ctx context.Context, gclient goalert.Client, gi *goalertv1alpha1.GoalertIntegration, cd *hivev1.ClusterDeployment) error {

	var (
		// secretName is the name of the Secret deployed to the target
		// cluster, and also the name of the SyncSet that causes it to
		// be deployed.
		secretName = config.SecretName
		// There can be more than one GoalertIntegration that causes
		// creation of resources for a ClusterDeployment, and each one
		// will need a finalizer here. We add a suffix of the CR
		// name to distinguish them.
		finalizer = config.GoalertFinalizerPrefix + gi.Name
		// configMapName is the name of the ConfigMap containing the
		// SERVICE_ID and INTEGRATION_ID
		configMapName = config.Name(gi.Spec.ServicePrefix, cd.Name, config.ConfigMapSuffix)
	)

	if !controllerutil.ContainsFinalizer(cd, finalizer) {
		baseToPatch := client.MergeFrom(cd.DeepCopy())
		controllerutil.AddFinalizer(cd, finalizer)
		return r.Patch(ctx, cd, baseToPatch)
	}

	clusterID := getClusterID(cd)

	// Load data to create new service in Goalert
	dataHighSvc := &goalert.Data{
		EscalationPolicyID: gi.Spec.HighEscalationPolicy,
		Name:               clusterID + " - High",
		Description:        cd.Spec.ClusterName,
		Favorite:           true,
	}

	dataLowSvc := &goalert.Data{
		EscalationPolicyID: gi.Spec.LowEscalationPolicy,
		Name:               clusterID + " - Low",
		Description:        cd.Spec.ClusterName,
		Favorite:           true,
	}

	highSvcID, err := gclient.CreateService(ctx, dataHighSvc)
	if err != nil {
		r.reqLogger.Error(err, "Failed to create service for High alerts")
		localmetrics.UpdateMetricCGAOCreateFailure(1, dataHighSvc.Name)
		return err
	}
	lowSvcID, err := gclient.CreateService(ctx, dataLowSvc)
	if err != nil {
		r.reqLogger.Error(err, "Failed to create service for Low alerts")
		localmetrics.UpdateMetricCGAOCreateFailure(1, dataLowSvc.Name)
		return err
	}

	// Load data to create integration key for alertmanager
	dataIntKeyHighSvc := &goalert.Data{
		Id:   highSvcID,
		Type: "prometheusAlertmanager",
		Name: "High alerts",
	}
	dataIntKeyLowSvc := &goalert.Data{
		Id:   lowSvcID,
		Type: "prometheusAlertmanager",
		Name: "Low alerts",
	}

	highIntKey, err := gclient.CreateIntegrationKey(ctx, dataIntKeyHighSvc)
	if err != nil {
		r.reqLogger.Error(err, "Failed to create integration key for high alerts")
		return err
	}
	lowIntKey, err := gclient.CreateIntegrationKey(ctx, dataIntKeyLowSvc)
	if err != nil {
		r.reqLogger.Error(err, "Failed to create integration key for low alerts")
		return err
	}

	// Load data to create heartbeat monitor
	dataHeartbeatMonitor := &goalert.Data{
		Id:      highSvcID,
		Name:    clusterID,
		Timeout: 15,
	}

	heartbeatMonitorKey, heartbeatMonitorId, err := gclient.CreateHeartbeatMonitor(ctx, dataHeartbeatMonitor)
	if err != nil {
		r.reqLogger.Error(err, "Failed to create heartbeat monitor")
		return err
	}

	if err := r.reconcileConfigMap(ctx, cd, configMapName, highSvcID, lowSvcID, heartbeatMonitorId); err != nil {
		return err
	}

	secret := kube.GenerateGoalertSecret(cd.Namespace, secretName, highIntKey, lowIntKey, heartbeatMonitorKey)
	if err := r.reconcileGoalertSecret(ctx, cd, secret, highIntKey, lowIntKey, heartbeatMonitorKey); err != nil {
		return err
	}

	return r.reconcileGoalertSyncSet(ctx, cd, secretName, secret, gi)
}

func (r *GoalertIntegrationReconciler) reconcileConfigMap(ctx context.Context, cd *hivev1.ClusterDeployment, configMapName, highSvcID, lowSvcID, heartbeatMonitorId string) error {
	if highSvcID == "" || lowSvcID == "" {
		return nil
	}
	newCM := kube.GenerateConfigMap(cd.Namespace, configMapName, highSvcID, lowSvcID, heartbeatMonitorId)
	if err := controllerutil.SetControllerReference(cd, newCM, r.Scheme); err != nil {
		r.reqLogger.Error(err, "Error setting controller reference on configmap")
		return err
	}
	if err := r.Create(ctx, newCM); err != nil {
		if !errors.IsAlreadyExists(err) {
			r.reqLogger.Error(err, "Error creating configmap", "Name", configMapName)
			return err
		}
		if updateErr := r.Update(ctx, newCM); updateErr != nil {
			r.reqLogger.Error(updateErr, "Error updating existing configmap", "Name", configMapName)
			return updateErr
		}
	}
	return nil
}

func (r *GoalertIntegrationReconciler) reconcileGoalertSecret(ctx context.Context, cd *hivev1.ClusterDeployment, secret *corev1.Secret, highIntKey, lowIntKey, heartbeatMonitorKey string) error {
	r.reqLogger.Info("creating goalert secret", "ClusterDeployment.Namespace", cd.Namespace)
	if err := controllerutil.SetControllerReference(cd, secret, r.Scheme); err != nil {
		r.reqLogger.Error(err, "Error setting controller reference on secret", "ClusterDeployment.Namespace", cd.Namespace)
		return err
	}
	if err := r.Create(ctx, secret); err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
		return r.updateGoalertSecretIfChanged(ctx, cd, secret, highIntKey, lowIntKey, heartbeatMonitorKey)
	}
	return nil
}

func (r *GoalertIntegrationReconciler) updateGoalertSecretIfChanged(ctx context.Context, cd *hivev1.ClusterDeployment, secret *corev1.Secret, highIntKey, lowIntKey, heartbeatMonitorKey string) error {
	r.reqLogger.Info("the goalert secret exist, check if IntegrationKey are changed or not", "ClusterDeployment.Namespace", cd.Namespace)
	sc := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: cd.Namespace}, sc); err != nil {
		return err
	}
	if string(sc.Data[config.GoalertHighIntKey]) == highIntKey &&
		string(sc.Data[config.GoalertLowIntKey]) == lowIntKey &&
		string(sc.Data[config.GoalertHeartbeatIntKey]) == heartbeatMonitorKey {
		return nil
	}
	r.reqLogger.Info("Secret data have changed, delete the secret first")
	if err := r.Delete(ctx, secret); err != nil {
		log.Info("failed to delete existing goalert secret")
		return err
	}
	r.reqLogger.Info("creating goalert secret", "ClusterDeployment.Namespace", cd.Namespace)
	return r.Create(ctx, secret)
}

func (r *GoalertIntegrationReconciler) reconcileGoalertSyncSet(ctx context.Context, cd *hivev1.ClusterDeployment, secretName string, secret *corev1.Secret, gi *goalertv1alpha1.GoalertIntegration) error {
	r.reqLogger.Info("Creating syncset", "ClusterDeployment.Namespace", cd.Namespace)
	ss := &hivev1.SyncSet{}
	err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: cd.Namespace}, ss)
	if err == nil {
		return nil
	}
	r.reqLogger.Info("error finding the old syncset")
	if !errors.IsNotFound(err) {
		return err
	}
	r.reqLogger.Info("syncset not found, creating a new one")
	ss = kube.GenerateSyncSet(cd.Namespace, cd.Name, secret, gi)
	if err := controllerutil.SetControllerReference(cd, ss, r.Scheme); err != nil {
		r.reqLogger.Error(err, "Error setting controller reference on syncset", "ClusterDeployment.Namespace", cd.Namespace)
		return err
	}
	return r.Create(ctx, ss)
}

// getClusterID derives a cluster identifier from the ClusterDeployment namespace for use in GoAlert service names.
func getClusterID(cd *hivev1.ClusterDeployment) string {
	uid := strings.Split(cd.Namespace, "-")
	return "fedramp-" + uid[len(uid)-1]
}
