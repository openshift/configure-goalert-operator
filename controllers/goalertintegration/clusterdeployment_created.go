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
	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/pingcap/errors"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Scaffold of func to handle creation of new clusters OSD-16306
func (r *GoalertIntegrationReconciler) handleCreate(ctx context.Context, gclient goalert.Client, gi *goalertv1alpha1.GoalertIntegration, cd client.Object) error {

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
		configMapName = config.Name(gi.Spec.ServicePrefix, cd.GetName(), config.ConfigMapSuffix)
	)

	if !controllerutil.ContainsFinalizer(cd, finalizer) {
		modified := cd.DeepCopyObject().(client.Object)
		controllerutil.AddFinalizer(modified, finalizer)
		patch := client.MergeFrom(modified)
		if err := r.Patch(ctx, cd, patch); err != nil {
			return fmt.Errorf("failed to add finalizer to cd %s: %w", cd.GetName(), err)
		}
		// TODO: Why not proceed with the reconcile?
		return nil
	}

	clusterID, clusterName, err := func(gi *goalertv1alpha1.GoalertIntegration, obj client.Object) (string, string, error) {
		switch gi.Spec.ClusterSelectorSpec.Type {
		case goalertv1alpha1.ClusterTypeHive:
			cd := obj.(*hivev1.ClusterDeployment)
			uid := strings.Split(cd.Namespace, "-")
			return "fedramp-" + uid[len(uid)-1], cd.Spec.ClusterName, nil
		case goalertv1alpha1.ClusterTypeHypershift:
			hc := obj.(*hyperv1.HostedCluster)
			return hc.Spec.ClusterID, hc.Name, nil
		default:
			return "", "", fmt.Errorf("unsupported cluster type %q", gi.Spec.ClusterSelectorSpec.Type)
		}
	}(gi, cd)
	if err != nil {
		return err
	}

	// Load data to create new service in Goalert
	dataHighSvc := &goalert.Data{
		EscalationPolicyID: gi.Spec.HighEscalationPolicy,
		Name:               clusterID + " - High",
		Description:        clusterName,
		Favorite:           true,
	}

	dataLowSvc := &goalert.Data{
		EscalationPolicyID: gi.Spec.LowEscalationPolicy,
		Name:               clusterID + " - Low",
		Description:        clusterName,
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

	if highSvcID != "" && lowSvcID != "" {
		// save config map
		newCM := kube.GenerateConfigMap(cd.GetNamespace(), configMapName, highSvcID, lowSvcID, heartbeatMonitorId)
		if err := controllerutil.SetControllerReference(cd, newCM, r.Scheme); err != nil {
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

	//add secret part
	secret := kube.GenerateGoalertSecret(cd.GetNamespace(), secretName, highIntKey, lowIntKey, heartbeatMonitorKey)
	r.reqLogger.Info("creating goalert secret", "ClusterDeployment.Namespace", cd.GetNamespace())
	//add reference
	if err := controllerutil.SetControllerReference(cd, secret, r.Scheme); err != nil {
		r.reqLogger.Error(err, "Error setting controller reference on secret", "ClusterDeployment.Namespace", cd.GetNamespace())
		return err
	}
	if err := r.Create(ctx, secret); err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}

		r.reqLogger.Info("the goalert secret exist, check if IntegrationKey are changed or not", "ClusterDeployment.Namespace", cd.GetNamespace())
		sc := &corev1.Secret{}
		err := r.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: cd.GetNamespace()}, sc)
		if err != nil {
			return nil
		}
		if string(sc.Data[config.GoalertHighIntKey]) != highIntKey ||
			string(sc.Data[config.GoalertLowIntKey]) != lowIntKey ||
			string(sc.Data[config.GoalertHeartbeatIntKey]) != heartbeatMonitorKey {
			r.reqLogger.Info("Secret data have changed, delete the secret first")
			if err := r.Delete(ctx, secret); err != nil {
				log.Info("failed to delete existing goalert secret")
				return err
			}
			r.reqLogger.Info("creating goalert secret", "ClusterDeployment.Namespace", cd.GetNamespace())
			if err := r.Create(ctx, secret); err != nil {
				return err
			}
		}
	}

	// Create syncset that will propagate secret to customer cluster
	r.reqLogger.Info("Creating syncset", "ClusterDeployment.Namespace", cd.GetNamespace())
	ss := &hivev1.SyncSet{}
	err = r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: cd.GetNamespace()}, ss)
	if err != nil {
		r.reqLogger.Info("error finding the old syncset")
		if !errors.IsNotFound(err) {
			return err
		}
		r.reqLogger.Info("syncset not found , create a new one on this ")
		ss = kube.GenerateSyncSet(cd.GetNamespace(), cd.GetName(), secret, gi)
		if err := controllerutil.SetControllerReference(cd, ss, r.Scheme); err != nil {
			r.reqLogger.Error(err, "Error setting controller reference on syncset", "ClusterDeployment.Namespace", cd.GetNamespace())
			return err
		}
		if err := r.Create(ctx, ss); err != nil {
			return err
		}
	}

	return nil
}
