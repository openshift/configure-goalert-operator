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

func (e *enqueueRequestForClusterDeployment) Create(ctx context.Context, evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	reqs := map[reconcile.Request]struct{}{}
	e.mapAndEnqueue(ctx, q, evt.Object, reqs)
}

func (e *enqueueRequestForClusterDeployment) Update(ctx context.Context, evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	reqs := map[reconcile.Request]struct{}{}
	e.mapAndEnqueue(ctx, q, evt.ObjectOld, reqs)
	e.mapAndEnqueue(ctx, q, evt.ObjectNew, reqs)
}

func (e *enqueueRequestForClusterDeployment) Delete(ctx context.Context, evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	reqs := map[reconcile.Request]struct{}{}
	e.mapAndEnqueue(ctx, q, evt.Object, reqs)
}

func (e *enqueueRequestForClusterDeployment) Generic(ctx context.Context, evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	reqs := map[reconcile.Request]struct{}{}
	e.mapAndEnqueue(ctx, q, evt.Object, reqs)
}

// toRequests receives a ClusterDeployment objects that have fired an event and checks if it can find an associated
// GoAlertIntegration object that has a matching label selector, if so it creates a request for the reconciler to
// take a look at that GoalertIntegration object.
func (e *enqueueRequestForClusterDeployment) toRequests(ctx context.Context, obj client.Object) []reconcile.Request {
	reqs := []reconcile.Request{}
	giList := &goalertv1alpha1.GoalertIntegrationList{}
	if err := e.Client.List(ctx, giList, &client.ListOptions{}); err != nil {
		return reqs
	}

	for _, gai := range giList.Items {
		gai := gai // gosec G601 compliance - avoid memory aliasing in for-loops
		selector, err := metav1.LabelSelectorAsSelector(&gai.Spec.ClusterDeploymentSelector)
		if err != nil {
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

func (e *enqueueRequestForClusterDeployment) mapAndEnqueue(ctx context.Context, q workqueue.RateLimitingInterface, obj client.Object, reqs map[reconcile.Request]struct{}) {
	for _, req := range e.toRequests(ctx, obj) {
		_, ok := reqs[req]
		if !ok {
			q.Add(req)
			// Used for de-duping requests
			reqs[req] = struct{}{}
		}
	}
}
