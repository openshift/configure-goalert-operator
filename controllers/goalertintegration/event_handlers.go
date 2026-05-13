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

	goalertv1alpha1 "github.com/openshift/configure-goalert-operator/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ handler.EventHandler = &enqueueRequestForClusterDeployment{}

// enqueueRequestForClusterDeployment implements the handler.EventHandler interface.
// Heavily inspired by https://github.com/kubernetes-sigs/controller-runtime/blob/v0.12.1/pkg/handler/enqueue_mapped.go
type enqueueRequestForClusterDeployment struct {
	Client client.Client
}

// Create handles ClusterDeployment create events by enqueuing matching GoalertIntegration reconcile requests.
func (e *enqueueRequestForClusterDeployment) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	reqs := map[reconcile.Request]struct{}{}
	e.mapAndEnqueue(q, evt.Object, reqs)
}

// Update handles ClusterDeployment update events by enqueuing matching GoalertIntegration reconcile requests for both old and new objects.
func (e *enqueueRequestForClusterDeployment) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	reqs := map[reconcile.Request]struct{}{}
	e.mapAndEnqueue(q, evt.ObjectOld, reqs)
	e.mapAndEnqueue(q, evt.ObjectNew, reqs)
}

// Delete handles ClusterDeployment delete events by enqueuing matching GoalertIntegration reconcile requests.
func (e *enqueueRequestForClusterDeployment) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	reqs := map[reconcile.Request]struct{}{}
	e.mapAndEnqueue(q, evt.Object, reqs)
}

// Generic handles generic ClusterDeployment events by enqueuing matching GoalertIntegration reconcile requests.
func (e *enqueueRequestForClusterDeployment) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	reqs := map[reconcile.Request]struct{}{}
	e.mapAndEnqueue(q, evt.Object, reqs)
}

// toRequests receives a ClusterDeployment objects that have fired an event and checks if it can find an associated
// GoAlertIntegration object that has a matching label selector, if so it creates a request for the reconciler to
// take a look at that GoalertIntegration object.
func (e *enqueueRequestForClusterDeployment) toRequests(obj client.Object) []reconcile.Request {
	reqs := []reconcile.Request{}
	giList := &goalertv1alpha1.GoalertIntegrationList{}
	if err := e.Client.List(context.TODO(), giList, &client.ListOptions{}); err != nil {
		log.Error(err, "failed to list GoalertIntegration resources in event handler")
		return reqs
	}

	for _, gai := range giList.Items {
		gai := gai // gosec G601 compliance - avoid memory aliasing in for-loops
		selector, err := metav1.LabelSelectorAsSelector(&gai.Spec.ClusterDeploymentSelector)
		if err != nil {
			log.Error(err, "failed to parse label selector", "goalertintegration", gai.Name)
			continue
		}
		if selector.Matches(labels.Set(obj.GetLabels())) {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      gai.Name,
					Namespace: gai.Namespace,
				},
			})
		}
	}
	return reqs
}

// mapAndEnqueue maps an object to reconcile requests and adds them to the work queue, de-duplicating as needed.
func (e *enqueueRequestForClusterDeployment) mapAndEnqueue(q workqueue.RateLimitingInterface, obj client.Object, reqs map[reconcile.Request]struct{}) {
	for _, req := range e.toRequests(obj) {
		_, ok := reqs[req]
		if !ok {
			q.Add(req)
			// Used for de-duping requests
			reqs[req] = struct{}{}
		}
	}
}
