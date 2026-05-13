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

	"github.com/openshift/configure-goalert-operator/pkg/localmetrics"

	goalertv1alpha1 "github.com/openshift/configure-goalert-operator/api/v1alpha1"
	"github.com/openshift/configure-goalert-operator/config"
	"github.com/openshift/configure-goalert-operator/pkg/goalert"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// handleDelete removes GoAlert services, the ConfigMap, Secret, SyncSet, and finalizer for a ClusterDeployment.
func (r *GoalertIntegrationReconciler) handleDelete(ctx context.Context, gclient goalert.Client, gi *goalertv1alpha1.GoalertIntegration, cd *hivev1.ClusterDeployment) error {
	if cd == nil {
		return nil
	}

	if err := r.deleteGoalertServicesAndConfigMap(ctx, cd, gi, gclient); err != nil {
		return err
	}

	if err := r.deleteGoalertSecretAndSyncSet(ctx, cd); err != nil {
		return err
	}

	goalertFinalizer := config.GoalertFinalizerPrefix + gi.Name
	r.reqLogger.Info("removing Goalert finalizer from ClusterDeployment", "clusterdeployment", cd.Name)
	baseToPatch := client.MergeFrom(cd.DeepCopy())
	if !controllerutil.RemoveFinalizer(cd, goalertFinalizer) {
		r.reqLogger.Info("finalizer not found on ClusterDeployment", "clusterdeployment", cd.Name)
	}
	if err := r.Patch(ctx, cd, baseToPatch); err != nil {
		r.reqLogger.Error(err, "failed to remove finalizer from cd", "clusterdeployment:", cd.Name)
	}

	r.reqLogger.Info("Cluster %s in deletion, deleting heartbeat metric", "clusterdeployment", cd.Name)
	if !localmetrics.DeleteMetricCGAOHeartbeat(cd.Name) {
		r.reqLogger.Info("failed to delete heartbeat monitor metric", "clusterdeployment", cd.Name)
	}
	return nil
}

func (r *GoalertIntegrationReconciler) deleteGoalertServicesAndConfigMap(ctx context.Context, cd *hivev1.ClusterDeployment, gi *goalertv1alpha1.GoalertIntegration, gclient goalert.Client) error {
	cmData := &v1.ConfigMap{Data: map[string]string{}}
	cmData.Name = config.Name(gi.Spec.ServicePrefix, cd.Name, config.ConfigMapSuffix)
	err := r.Get(ctx, types.NamespacedName{Name: cmData.Name, Namespace: cd.Namespace}, cmData)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		return nil
	}

	for _, svcEntry := range []struct{ key, label string }{
		{"HIGH_SERVICE_ID", "goalert high service id"},
		{"LOW_SERVICE_ID", "goalert low service id"},
	} {
		svcID := cmData.Data[svcEntry.key]
		if svcID == "" {
			continue
		}
		r.reqLogger.Info("Deleting service", svcEntry.label, svcID)
		if err := gclient.DeleteService(ctx, &goalert.Data{Id: svcID, Timeout: 15}); err != nil {
			r.reqLogger.Error(err, "unable to delete service", svcEntry.label, svcID)
			localmetrics.UpdateMetricCGAODeleteFailure(1, svcID)
			return err
		}
	}

	r.reqLogger.Info("Deleting Goalert configmap for", "clusterdeployment:", cd.Name)
	cmData.Namespace = cd.Namespace
	if err := r.Delete(ctx, cmData); err != nil {
		r.reqLogger.Error(err, "unable to remove goalert configmap", "configmap", cmData.Name)
		return err
	}
	return nil
}

func (r *GoalertIntegrationReconciler) deleteGoalertSecretAndSyncSet(ctx context.Context, cd *hivev1.ClusterDeployment) error {
	secretToRemove := &v1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: config.SecretName, Namespace: cd.Namespace}, secretToRemove)
	if err != nil && !errors.IsNotFound(err) {
		r.reqLogger.Error(err, "unable to reconcile secret for", "clusterdeployment", cd.Name)
		return err
	}
	if err == nil {
		r.reqLogger.Info("Deleting Goalert secret for", "clusterdeployment: ", cd.Name)
		if err := r.Delete(ctx, secretToRemove); err != nil {
			r.reqLogger.Error(err, "unable to delete secret for", "clusterdeployment", cd.Name)
			return err
		}
	}

	ssToRemove := &hivev1.SyncSet{}
	err = r.Get(ctx, types.NamespacedName{Name: config.SecretName, Namespace: cd.Namespace}, ssToRemove)
	if err != nil && !errors.IsNotFound(err) {
		r.reqLogger.Error(err, "unable to reconcile syncset for", "clusterdeployment name", cd.Name)
		return err
	}
	if err == nil {
		r.reqLogger.Info("Deleting Goalert syncset for", "clusterdeployment:", cd.Name)
		if err := r.Delete(ctx, ssToRemove); err != nil {
			r.reqLogger.Error(err, "unable to remove goalert syncset", "clusterdeployment", cd.Name)
			return err
		}
	}
	return nil
}
