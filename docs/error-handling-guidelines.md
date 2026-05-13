# Error Handling Guidelines

## Reconcile Return Helpers

The reconciler defines two private helper methods for returning from `Reconcile()`. Prefer using them instead of constructing `reconcile.Result` directly.

```go
// Ends reconciliation, no requeue.
return r.doNotRequeue()

// Requeues with exponential backoff via controller-runtime.
return r.requeueOnErr(err)
```

**When to use each:**
- `doNotRequeue` -- the primary resource (GoalertIntegration) was not found (deleted out from under us), or a GI deletion completed successfully.
- `requeueOnErr` -- any error fetching the GI, listing ClusterDeployments, updating finalizers, or failing `handleDelete` during GI/CD deletion.
- Direct `return reconcile.Result{}, err` -- used only from `cgaoResourcesExist` for unexpected K8s API errors. Keep this limited; prefer the helpers.

**Note:** The main reconcile loop ends with `return ctrl.Result{}, nil` (equivalent to `doNotRequeue`). Errors from `handleCreate` in the creation loop are logged but do **not** cause a requeue -- they use a log-and-continue pattern. This is intentional: a single failing CD should not block progress on others.

## Log-and-Continue vs Log-and-Return

The codebase uses two distinct error-handling strategies in the main `Reconcile()` function. Know which applies:

### Log-and-continue (do NOT return)

Used when a failure for one item should not block processing of other items, or when the operation will be retried on the next reconcile anyway:
- Credential loading failures (`LoadSecretData` for username/password)
- Heartbeat monitor checks per ClusterDeployment
- `handleCreate` failures for individual CDs in the creation loop
- `handleDelete` for unmatched CDs (label removed, not CD-deletion or GI-deletion)

### Log-and-return (stop and requeue)

Used when the failure is unrecoverable for this reconcile pass:
- Fetching the GoalertIntegration CR (except NotFound)
- Listing all or matching ClusterDeployments
- GoAlert authentication and session cookie failures
- `handleDelete` during GI deletion or CD deletion (returns `requeueOnErr`)
- Updating finalizers on the GI

## Kubernetes API Error Patterns

### NotFound Guard Pattern

When calling `r.Get()` for a resource that may legitimately not exist, use the standard double-check:

```go
err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, obj)
if err != nil {
    if !errors.IsNotFound(err) {
        return err  // unexpected error, propagate
    }
    // resource does not exist -- handle accordingly
}
```

This pattern appears throughout `cgaoResourcesExist`, `handleDelete`, `checkHeartbeatMonitor`, and `handleCreate`. The import is `"k8s.io/apimachinery/pkg/api/errors"`.

### AlreadyExists Handling in Create

When creating resources in `handleCreate`, handle `AlreadyExists` inline rather than checking existence first:

```go
if err := r.Create(ctx, resource); err != nil {
    if errors.IsAlreadyExists(err) {
        // update or skip
        return nil
    }
    return err
}
```

The ConfigMap uses this to fall through to an `Update` call. The Secret uses it to compare data and conditionally delete-then-recreate. The SyncSet does a `Get`-first approach instead (it queries before creating). Follow the existing pattern for the resource type you are modifying.

**Important:** `handleCreate` uses `"github.com/pingcap/errors"` for `IsAlreadyExists`/`IsNotFound`, while `handleDelete` and the main controller use `"k8s.io/apimachinery/pkg/api/errors"`. Both provide the same `IsNotFound`/`IsAlreadyExists` methods. New code should prefer `"k8s.io/apimachinery/pkg/api/errors"` (the standard K8s package) for consistency with the main controller and delete handler. The `pingcap/errors` usage in `clusterdeployment_created.go` and `heartbeatmonitor_check.go` is legacy.

## Error Wrapping Conventions

### In pkg/goalert (GraphQL client)

- HTTP/network errors from `NewRequest` propagate unwrapped.
- JSON unmarshal errors wrap with `fmt.Errorf("unable to unmarshal response %s: %w", ...)` to include the raw response body for debugging.
- Business logic failures use `errors.New` (e.g., `"failed to delete service"` when the GraphQL response indicates failure).
- The package uses stdlib `"errors"` and `"fmt"`, not `pingcap/errors`.

### In pkg/utils

- Uses `fmt.Errorf` without `%w` wrapping (plain formatted errors). These are terminal error messages describing missing or empty secret keys.

### In controllers

- Controller methods do **not** wrap errors from sub-calls. They log the error with context (`r.reqLogger.Error(err, "message", "key", value)`) and then either return the original error or continue.
- Avoid using `%s` or `%v` format verbs in log messages; put dynamic values in structured key-value pairs. Note: some existing code contains literal `%s` in log message strings (e.g., `"Checking %s heartbeat monitor"`), which should be avoided since logr doesn't use fmt-style formatting and these serve no purpose.

## Sentinel Errors

One sentinel error exists:

```go
var ErrSessionCookieMissing = fmt.Errorf("session cookie is missing")
```

Defined at package scope in `goalertintegration_controller.go`. Used in `fetchSessionCookie` when the `goalert_session.2` cookie is absent from the auth response. This is both logged and returned.

## Error Handling in handleDelete

`handleDelete` returns `error` but has an inconsistency at its tail: the finalizer removal and metric deletion steps log errors but **do not return them**. The function always returns `nil` after the delete-secret/delete-syncset block. This means finalizer-removal failures are silently swallowed. New code in the delete path should be aware that errors after the resource cleanup section are not propagated to the caller.

Additionally, in the finalizer error log (`r.reqLogger.Error(err, "failed to update cd finalizer")`), `err` is the variable from the previous `r.Delete` call, not from `RemoveFinalizer`. This is a known issue -- `RemoveFinalizer` returns a bool, not an error.

## Error Handling in Event Handlers

Event handlers (`event_handlers.go`) use a different pattern because they lack a reconciler receiver:
- Use the package-level `log` variable, not `r.reqLogger`.
- Errors from `Client.List` or `Client.Get` cause the handler to return an empty request slice or `continue` to the next iteration -- they never propagate errors upward since the `handler.EventHandler` interface has no error return.

## GoAlert Client Error Propagation

The `goalert.Client` interface methods return errors that bubble up through `handleCreate`/`handleDelete` into the reconcile loop. The controller is responsible for logging; the client methods return clean errors without logging. Keep this separation: **pkg/goalert should not log, only return errors**.

## Metrics on Error

When GoAlert API calls fail in the controller:
- **Create failures:** Call `localmetrics.UpdateMetricCGAOCreateFailure(1, serviceName)` before returning the error.
- **Delete failures:** Call `localmetrics.UpdateMetricCGAODeleteFailure(1, serviceID)` before returning the error.

These metrics are set on failure only. There is no corresponding "reset to 0" on success for create/delete metrics. The heartbeat metric (`UpdateMetricCGAOHeartbeatInactive`) does reset to 0 when the monitor becomes active again.
