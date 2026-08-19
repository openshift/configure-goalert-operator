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
	"fmt"
	"strings"

	"github.com/openshift/configure-goalert-operator/pkg/localmetrics"

	goalertv1alpha1 "github.com/openshift/configure-goalert-operator/api/v1alpha1"
	"github.com/openshift/configure-goalert-operator/config"
	"github.com/openshift/configure-goalert-operator/pkg/goalert"
	"github.com/openshift/configure-goalert-operator/pkg/kube"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/pingcap/errors"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// ensureService returns the ID of an existing GoAlert service matching data.Name, creating it if absent.
func ensureService(ctx context.Context, gclient goalert.Client, data *goalert.Data) (string, error) {
	id, err := gclient.GetServiceIDByName(ctx, data.Name)
	if err != nil {
		return "", err
	}
	if id != "" {
		return id, nil
	}
	return gclient.CreateService(ctx, data)
}

// ensureIntegrationKey returns the href of an existing integration key matching data.Name on data.Id, creating it if absent.
func ensureIntegrationKey(ctx context.Context, gclient goalert.Client, data *goalert.Data) (string, error) {
	href, err := gclient.GetIntegrationKeyHref(ctx, data.Id, data.Name, data.Type)
	if err != nil {
		return "", err
	}
	if href != "" {
		return href, nil
	}
	return gclient.CreateIntegrationKey(ctx, data)
}

// ensureHeartbeatMonitor returns the href and id of an existing heartbeat monitor matching data.Name on data.Id, creating it if absent.
func ensureHeartbeatMonitor(ctx context.Context, gclient goalert.Client, data *goalert.Data) (string, string, error) {
	href, id, err := gclient.GetHeartbeatMonitor(ctx, data.Id, data.Name)
	if err != nil {
		return "", "", err
	}
	if href != "" && id != "" {
		return href, id, nil
	}
	return gclient.CreateHeartbeatMonitor(ctx, data)
}

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

	highSvcID, err := ensureService(ctx, gclient, dataHighSvc)
	if err != nil {
		r.reqLogger.Error(err, "Failed to create service for High alerts")
		localmetrics.UpdateMetricCGAOCreateFailure(1, dataHighSvc.Name)
		return err
	}
	lowSvcID, err := ensureService(ctx, gclient, dataLowSvc)
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

	highIntKey, err := ensureIntegrationKey(ctx, gclient, dataIntKeyHighSvc)
	if err != nil {
		r.reqLogger.Error(err, "Failed to create integration key for high alerts")
		return err
	}
	lowIntKey, err := ensureIntegrationKey(ctx, gclient, dataIntKeyLowSvc)
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

	heartbeatMonitorKey, heartbeatMonitorId, err := ensureHeartbeatMonitor(ctx, gclient, dataHeartbeatMonitor)
	if err != nil {
		r.reqLogger.Error(err, "Failed to create heartbeat monitor")
		return err
	}

	if highSvcID != "" && lowSvcID != "" {
		// save config map
		newCM := kube.GenerateConfigMap(cd.Namespace, configMapName, highSvcID, lowSvcID, heartbeatMonitorId)
		if err := setControllerReferenceWithoutBlockingDeletion(cd, newCM, r.Scheme); err != nil {
			r.reqLogger.Error(err, "Error setting controller reference on configmap")
			return err
		}

		if err := r.Create(ctx, newCM); err != nil {
			if errors.IsAlreadyExists(err) {
				if updateErr := r.Update(ctx, newCM); updateErr != nil {
					r.reqLogger.Error(err, "Error updating existing configmap", "Name", configMapName)
					return err
				}
				return nil
			}
			r.reqLogger.Error(err, "Error creating configmap", "Name", configMapName)
			return err
		}
	}

	// add secret part
	secret := kube.GenerateGoalertSecret(cd.Namespace, secretName, highIntKey, lowIntKey, heartbeatMonitorKey)
	r.reqLogger.Info("creating goalert secret", "ClusterDeployment.Namespace", cd.Namespace)
	// add reference
	if err := setControllerReferenceWithoutBlockingDeletion(cd, secret, r.Scheme); err != nil {
		r.reqLogger.Error(err, "Error setting controller reference on secret", "ClusterDeployment.Namespace", cd.Namespace)
		return err
	}
	if err := r.Create(ctx, secret); err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}

		r.reqLogger.Info("the goalert secret exist, check if IntegrationKey are changed or not", "ClusterDeployment.Namespace", cd.Namespace)
		sc := &corev1.Secret{}
		err := r.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: cd.Namespace}, sc)
		if err != nil {
			return err
		}
		if string(sc.Data[config.GoalertHighIntKey]) != highIntKey ||
			string(sc.Data[config.GoalertLowIntKey]) != lowIntKey ||
			string(sc.Data[config.GoalertHeartbeatIntKey]) != heartbeatMonitorKey {
			r.reqLogger.Info("Secret data have changed, delete the secret first")
			if err := r.Delete(ctx, secret); err != nil {
				log.Info("failed to delete existing goalert secret")
				return err
			}
			r.reqLogger.Info("creating goalert secret", "ClusterDeployment.Namespace", cd.Namespace)
			if err := r.Create(ctx, secret); err != nil {
				return err
			}
		}
	}

	// Create syncset that will propagate secret to customer cluster
	r.reqLogger.Info("Creating syncset", "ClusterDeployment.Namespace", cd.Namespace)
	ss := &hivev1.SyncSet{}
	err = r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: cd.Namespace}, ss)
	if err != nil {
		r.reqLogger.Info("error finding the old syncset")
		if !errors.IsNotFound(err) {
			return err
		}
		r.reqLogger.Info("syncset not found , create a new one on this ")
		ss = kube.GenerateSyncSet(cd.Namespace, cd.Name, secret, gi)
		if err := setControllerReferenceWithoutBlockingDeletion(cd, ss, r.Scheme); err != nil {
			r.reqLogger.Error(err, "Error setting controller reference on syncset", "ClusterDeployment.Namespace", cd.Namespace)
			return err
		}
		if err := r.Create(ctx, ss); err != nil {
			return err
		}
	}

	return nil
}

// getClusterID derives a cluster identifier from the ClusterDeployment namespace for use in GoAlert service names.
func getClusterID(cd *hivev1.ClusterDeployment) string {
	uid := strings.Split(cd.Namespace, "-")
	return "fedramp-" + uid[len(uid)-1]
}

// setControllerReferenceWithoutBlockingDeletion sets the ClusterDeployment as controller/owner of the given object without blockOwnerDeletion.
// This is safe because the operator uses finalizers for cleanup, not garbage collection.
func setControllerReferenceWithoutBlockingDeletion(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
	ro, ok := owner.(runtime.Object)
	if !ok {
		return fmt.Errorf("%T is not a runtime.Object, cannot call SetControllerReference", owner)
	}

	gvk, err := apiutil.GVKForObject(ro, scheme)
	if err != nil {
		return err
	}

	ref := metav1.OwnerReference{
		APIVersion:         gvk.GroupVersion().String(),
		Kind:               gvk.Kind,
		Name:               owner.GetName(),
		UID:                owner.GetUID(),
		BlockOwnerDeletion: ptr.To(false),
		Controller:         ptr.To(true),
	}

	controlled.SetOwnerReferences(append(controlled.GetOwnerReferences(), ref))
	return nil
}
