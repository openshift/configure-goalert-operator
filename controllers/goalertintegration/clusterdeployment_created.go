package goalertintegration

import (
	"context"
	"strings"

	goalertv1alpha1 "github.com/openshift/configure-goalert-operator/api/v1alpha1"
	"github.com/openshift/configure-goalert-operator/config"
	"github.com/openshift/configure-goalert-operator/pkg/goalert"
	"github.com/openshift/configure-goalert-operator/pkg/kube"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/pingcap/errors"
	"github.com/pingcap/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Scaffold of func to handle creation of new clusters OSD-16306
func (r *GoalertIntegrationReconciler) handleCreate(gclient goalert.Client, gi *goalertv1alpha1.GoalertIntegration, cd *hivev1.ClusterDeployment) error {

	var (
		// secretName is the name of the Secret deployed to the target
		// cluster, and also the name of the SyncSet that causes it to
		// be deployed.
		secretName string = config.SecretName
		// There can be more than one GoalertIntegration that causes
		// creation of resources for a ClusterDeployment, and each one
		// will need a finalizer here. We add a suffix of the CR
		// name to distinguish them.
		finalizer string = config.GoalertFinalizerPrefix + gi.Name
		// configMapName is the name of the ConfigMap containing the
		// SERVICE_ID and INTEGRATION_ID
		configMapName          string = config.Name(gi.Spec.ServicePrefix, cd.Name, config.ConfigMapSuffix)
		highEscalationPolicyID string = gi.Spec.HighEscalationPolicy
		lowEscalationPolicyID  string = gi.Spec.LowEscalationPolicy
	)

	if !controllerutil.ContainsFinalizer(cd, finalizer) {
		baseToPatch := client.MergeFrom(cd.DeepCopy())
		controllerutil.AddFinalizer(cd, finalizer)
		return r.Patch(context.TODO(), cd, baseToPatch)
	}

	clusterID := GetClusterID(cd)

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

	highSvcID, err := gclient.CreateService(dataHighSvc)
	if err != nil {
		r.reqLogger.Error(err, "Failed to create service for High alerts")
		return err
	}
	lowSvcID, err := gclient.CreateService(dataLowSvc)
	if err != nil {
		r.reqLogger.Error(err, "Failed to create service for Low alerts")
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

	highIntKey, err := gclient.CreateIntegrationKey(dataIntKeyHighSvc)
	if err != nil {
		r.reqLogger.Error(err, "Failed to create integration key for high alerts")
		return err
	}
	lowIntKey, err := gclient.CreateIntegrationKey(dataIntKeyLowSvc)
	if err != nil {
		r.reqLogger.Error(err, "Failed to create integration key for low alerts")
		return err
	}

	// Load data to create heartbeatmonitor
	dataHeartbeatMonitor := &goalert.Data{
		Id:      highSvcID,
		Name:    clusterID,
		Timeout: 15,
	}

	heartbeatMonitorKey, err := gclient.CreateHeartbeatMonitor(dataHeartbeatMonitor)
	if err != nil {
		r.reqLogger.Error(err, "Failed to create heartbeat monitor")
		return err
	}

	// save config map
	newCM := kube.GenerateConfigMap(cd.Namespace, strings.ToLower(configMapName), highSvcID, lowSvcID, highEscalationPolicyID, lowEscalationPolicyID)
	if err = controllerutil.SetControllerReference(cd, newCM, r.Scheme); err != nil {
		r.reqLogger.Error(err, "Error setting controller reference on configmap")
		return err
	}

	if err := r.Create(context.TODO(), newCM); err != nil {
		if errors.IsAlreadyExists(err) {
			if updateErr := r.Update(context.TODO(), newCM); updateErr != nil {
				r.reqLogger.Error(err, "Error updating existing configmap", "Name", configMapName)
				return err
			}
			return nil
		}
		r.reqLogger.Error(err, "Error creating configmap", "Name", configMapName)
		return err
	}

	//add secret part
	secret := kube.GenerateGoalertSecret(cd.Namespace, secretName, highIntKey, lowIntKey, heartbeatMonitorKey)
	r.reqLogger.Info("creating goalert secret", "ClusterDeployment.Namespace", cd.Namespace)
	//add reference
	if err = controllerutil.SetControllerReference(cd, secret, r.Scheme); err != nil {
		r.reqLogger.Error(err, "Error setting controller reference on secret", "ClusterDeployment.Namespace", cd.Namespace)
		return err
	}
	if err = r.Create(context.TODO(), secret); err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}

		r.reqLogger.Info("the goalert secret exist, check if IntegrationKey are changed or not", "ClusterDeployment.Namespace", cd.Namespace)
		sc := &corev1.Secret{}
		err = r.Get(context.TODO(), types.NamespacedName{Name: secret.Name, Namespace: cd.Namespace}, sc)
		if err != nil {
			return nil
		}
		if string(sc.Data[config.GoalertHighIntKey]) != highIntKey ||
			string(sc.Data[config.GoalertLowIntKey]) != lowIntKey ||
			string(sc.Data[config.GoalertHeartbeatIntKey]) != heartbeatMonitorKey {
			r.reqLogger.Info("Secret data have changed, delete the secret first")
			if err = r.Delete(context.TODO(), secret); err != nil {
				log.Info("failed to delete existing goalert secret")
				return err
			}
			r.reqLogger.Info("creating goalert secret", "ClusterDeployment.Namespace", cd.Namespace)
			if err = r.Create(context.TODO(), secret); err != nil {
				return err
			}
		}
	}

	// Create syncset that will propagate secret to customer cluster
	r.reqLogger.Info("Creating syncset", "ClusterDeployment.Namespace", cd.Namespace)
	ss := &hivev1.SyncSet{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: cd.Namespace}, ss)
	if err != nil {
		r.reqLogger.Info("error finding the old syncset")
		if !errors.IsNotFound(err) {
			return err
		}
		r.reqLogger.Info("syncset not found , create a new one on this ")
		ss = kube.GenerateSyncSet(cd.Namespace, cd.Name, secret, gi)
		if err = controllerutil.SetControllerReference(cd, ss, r.Scheme); err != nil {
			r.reqLogger.Error(err, "Error setting controller reference on syncset", "ClusterDeployment.Namespace", cd.Namespace)
			return err
		}
		if err := r.Create(context.TODO(), ss); err != nil {
			return err
		}
	}

	return nil
}

func GetClusterID(cd *hivev1.ClusterDeployment) string {
	uid := strings.Split(cd.Namespace, "-")
	return "Fedramp-" + uid[len(uid)-1]
}
