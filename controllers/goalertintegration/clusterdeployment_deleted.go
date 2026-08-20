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

	// Evaluate edge-cases where Goalert service no longer needs to be deleted
	deleteSvcBool := true

	cmData := &v1.ConfigMap{Data: map[string]string{}}
	cmData.Name = config.Name(gi.Spec.ServicePrefix, cd.Name, config.ConfigMapSuffix)
	err := r.Get(ctx, types.NamespacedName{Name: cmData.Name, Namespace: cd.Namespace}, cmData)
	if err != nil {
		if !errors.IsNotFound(err) {
			// some error other than not found, requeue
			return err
		}
		deleteSvcBool = false
	}

	if deleteSvcBool {
		goalertHighServiceID := cmData.Data[config.GoalertHighServiceIDKey]
		goalertLowServiceID := cmData.Data[config.GoalertLowServiceIDKey]

		if goalertHighServiceID != "" {
			r.reqLogger.Info("Deleting service", "goalert high service id", goalertHighServiceID)
			err = gclient.DeleteService(ctx, &goalert.Data{
				Id:      goalertHighServiceID,
				Timeout: 15,
			})
			if err != nil {
				r.reqLogger.Error(err, "unable to delete service", "goalert high service id", goalertHighServiceID)
				localmetrics.UpdateMetricCGAODeleteFailure(1, goalertHighServiceID)
				return err
			}
		}

		if goalertLowServiceID != "" {
			r.reqLogger.Info("Deleting service", "goalert low service id", goalertLowServiceID)
			err = gclient.DeleteService(ctx, &goalert.Data{
				Id:      goalertLowServiceID,
				Timeout: 15,
			})
			if err != nil {
				r.reqLogger.Error(err, "unable to delete service", "goalert low service id", goalertLowServiceID)
				localmetrics.UpdateMetricCGAODeleteFailure(1, goalertLowServiceID)
				return err
			}
		}

		r.reqLogger.Info("Deleting Goalert configmap for", "clusterdeployment", cd.Name)
		cmData.Namespace = cd.Namespace
		err = r.Delete(ctx, cmData)
		if err != nil {
			r.reqLogger.Error(err, "unable to remove goalert configmap", "configmap", cmData.Name)
			return err
		}
	}

	deleteSecret := true
	secretToRemove := &v1.Secret{}
	err = r.Get(ctx, types.NamespacedName{Name: config.SecretName, Namespace: cd.Namespace}, secretToRemove)
	if err != nil {
		if !errors.IsNotFound(err) {
			r.reqLogger.Error(err, "unable to reconcile secret for", "clusterdeployment", cd.Name)
			return err
		}
		r.reqLogger.Info("unable to locate goalert secret for cluster deployment, moving on", "clusterdeployment", cd.Name)
		deleteSecret = false
	}

	deleteSyncset := true
	ssToRemove := &hivev1.SyncSet{}
	err = r.Get(ctx, types.NamespacedName{Name: config.SecretName, Namespace: cd.Namespace}, ssToRemove)
	if err != nil {
		if !errors.IsNotFound(err) {
			r.reqLogger.Error(err, "unable to reconcile syncset for", "clusterdeployment name", cd.Name)
			return err
		}
		r.reqLogger.Info("unable to locate goalert syncset for cluster deployment, moving on", "clusterdeployment", cd.Name)
		deleteSyncset = false
	}

	if deleteSecret {
		r.reqLogger.Info("Deleting Goalert secret for", "clusterdeployment", cd.Name)
		secretToRemove.Name = config.SecretName
		secretToRemove.Namespace = cd.Namespace
		err = r.Delete(ctx, secretToRemove)
		if err != nil {
			r.reqLogger.Error(err, "unable to delete secret for", "clusterdeployment", cd.Name)
			return err
		}
	}

	if deleteSyncset {
		r.reqLogger.Info("Deleting Goalert syncset for", "clusterdeployment", cd.Name)
		ssToRemove.Name = config.SecretName
		ssToRemove.Namespace = cd.Namespace
		err = r.Delete(ctx, ssToRemove)
		if err != nil {
			r.reqLogger.Error(err, "unable to remove goalert syncset", "clusterdeployment", cd.Name)
			return err
		}
	}

	goalertFinalizer := config.GoalertFinalizerPrefix + gi.Name
	r.reqLogger.Info("removing Goalert finalizer from ClusterDeployment", "clusterdeployment", cd.Name)
	baseToPatch := client.MergeFrom(cd.DeepCopy())
	if controllerutil.RemoveFinalizer(cd, goalertFinalizer) {
		if patchErr := r.Patch(ctx, cd, baseToPatch); patchErr != nil {
			r.reqLogger.Error(patchErr, "failed to remove finalizer from cd", "clusterdeployment", cd.Name)
			return patchErr
		}
	}

	r.reqLogger.Info("cluster in deletion, deleting heartbeat metric", "clusterdeployment", cd.Name)
	if !localmetrics.DeleteMetricCGAOHeartbeat(cd.Name) {
		r.reqLogger.Info("heartbeat metric not found for deletion", "clusterdeployment", cd.Name)
	}
	return nil
}
