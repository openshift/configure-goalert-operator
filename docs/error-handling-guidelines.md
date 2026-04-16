# Error Handling Guidelines

These guidelines document the error handling conventions specific to the Configure GoAlert Operator (CGAO) codebase. They cover patterns found in the controller reconciliation loop, GoAlert client, and supporting packages.

## 1. Reconcile Return Value Conventions

The controller uses two helper methods to standardize reconcile returns. Prefer using them in the main `Reconcile()` method:

```go
r.doNotRequeue()   // returns reconcile.Result{}, nil
r.requeueOnErr(err) // returns reconcile.Result{}, err
```

**When to use which:**

- **`doNotRequeue()`** -- The target resource was deleted (IsNotFound on the primary CR), or the GI has a deletion timestamp and cleanup completed.
- **`requeueOnErr(err)`** -- Any error fetching the primary CR, listing ClusterDeployments, or during GI/CD deletion cleanup where the error is fatal to the reconcile.
- **`reconcile.Result{}, err`** (direct) -- Used in `cgaoResourcesExist` for unexpected Get errors. Equivalent to `requeueOnErr` but appears in code that returns a 4-tuple.
- **`ctrl.Result{}, nil`** (direct) -- End of the main reconcile loop; creation errors are logged but do not trigger a requeue (see Section 3).

Sub-handlers (`handleCreate`, `handleDelete`) return `error` only. The caller in `Reconcile()` decides whether to requeue.

## 2. Kubernetes API Error Checking

### NotFound Pattern

Use `k8s.io/apimachinery/pkg/api/errors` (imported as `errors` in the controller) for all K8s API status checks. The codebase uses two distinct idioms:

**Existence probe (fail on unexpected errors only):**
```go
err := r.Get(ctx, key, obj)
if err != nil && !errors.IsNotFound(err) {
    return err  // unexpected error, propagate
}
// NotFound is expected -- set a boolean flag
exists = !errors.IsNotFound(err)
```

This pattern is used in `cgaoResourcesExist`, `handleDelete` (ConfigMap/Secret/SyncSet lookups), and `checkHeartbeatMonitor`.

**Guard clause (stop reconciling a deleted CR):**
```go
if errors.IsNotFound(err) {
    return r.doNotRequeue()
}
return r.requeueOnErr(err)
```

Used only at the top of `Reconcile()` when fetching the primary GoalertIntegration CR.

### AlreadyExists Pattern

On `Create`, check `errors.IsAlreadyExists(err)` and either update the existing resource or silently continue. Two variants exist in `handleCreate`:

**ConfigMap -- update on conflict:**
```go
if err := r.Create(ctx, newCM); err != nil {
    if errors.IsAlreadyExists(err) {
        if updateErr := r.Update(ctx, newCM); updateErr != nil { ... }
        return nil
    }
    return err
}
```

**Secret -- compare data, delete-and-recreate if changed:**
```go
if !errors.IsAlreadyExists(err) {
    return err
}
// fetch existing, compare fields, delete + create if different
```

## 3. Log-and-Continue vs Log-and-Return

The codebase has two distinct error strategies. Which one applies depends on whether the failure is recoverable via requeue:

**Log-and-return (requeue):** Errors that are transient or block the entire reconcile -- fetching the primary CR, listing ClusterDeployments, GI deletion cleanup, GoAlert service deletion during CD deletion. These return the error so controller-runtime requeues.

**Log-and-continue (no requeue):** Errors that affect a single cluster but should not block processing of other clusters. Examples:
- `handleCreate` failures during the CD creation loop -- logged but the loop continues to the next CD.
- Heartbeat monitor check failures -- logged, loop continues.
- Unmatched CD cleanup failures -- logged, loop continues.

**Rule of thumb in this codebase:** Errors in a per-CD loop body are logged and swallowed. Errors that affect the entire reconcile are returned.

## 4. Error Logging Conventions

Use `r.reqLogger.Error(err, "message", kvPairs...)` for structured logging in controller methods. In event handlers (which lack a reconciler receiver), use the package-level `log.Error(...)`.

Key patterns observed:
- The first argument is always the error value, the second is a human-readable message.
- Key-value pairs follow: `"clusterdeployment", cd.Name` or `"Name", configMapName`.
- Avoid trailing colons or inconsistent formatting in log keys. Use bare key names without punctuation.
- Never use `%s` format verbs in structured log messages. Put dynamic values in key-value pairs instead.

## 5. Error Wrapping in the GoAlert Client

Unmarshal errors in `pkg/goalert` functions are wrapped with `fmt.Errorf` using the `%w` verb and include the raw response body for debugging:

```go
return fmt.Errorf("unable to unmarshal response %s: %w", string(respData), err)
```

HTTP transport errors from `NewRequest` are returned unwrapped. Business logic errors use `errors.New`:

```go
return errors.New("failed to delete service")
```

The `pkg/utils` package uses `fmt.Errorf` without `%w` (no wrapping) for validation errors like missing secret keys. These are terminal -- there is nothing to unwrap.

## 6. Sentinel Errors

One sentinel error exists: `ErrSessionCookieMissing` defined at package level in the controller. Use package-level `var` with `fmt.Errorf` (not `errors.New`) for sentinel errors, matching the existing convention:

```go
var ErrSessionCookieMissing = fmt.Errorf("session cookie is missing")
```

## 7. Deferred Cleanup and Error Handling

HTTP response bodies are closed in deferred functions. Two patterns exist:

**Discard the error (GoAlert client):**
```go
defer func() { _ = resp.Body.Close() }()
```

**Log the error (controller):**
```go
defer func() {
    if err := authenticateGoalert.Body.Close(); err != nil {
        r.reqLogger.Error(err, "Error closing http.Response Body")
    }
}()
```

Use the logging variant in controller code where the reqLogger is available. Use the discard variant in library code (`pkg/goalert`) where no logger is available.

## 8. Nil Guard Convention

`handleDelete` guards against nil ClusterDeployment at the top:

```go
if cd == nil {
    return nil
}
```

Treat this as the convention for any handler that receives a pointer to a K8s resource -- nil-check first, return nil (not an error) if the resource is absent.

## 9. Metrics on Error

When GoAlert API calls fail during create or delete, update the corresponding Prometheus gauge before returning:

```go
localmetrics.UpdateMetricCGAOCreateFailure(1, dataHighSvc.Name)
localmetrics.UpdateMetricCGAODeleteFailure(1, goalertHighServiceID)
```

This must happen before returning the error. The metric name identifies which service failed. These gauges are never reset to 0 by the operator -- they persist until the pod restarts.

## 10. Event Handler Error Strategy

Event handlers (`event_handlers.go`) silently swallow errors by returning empty request slices or using `continue`. They never return errors directly because the `handler.EventHandler` interface has no error return. Errors are logged via the package-level `log` variable, not `r.reqLogger`.

## 11. Known Patterns to Avoid

These patterns exist in the codebase but should not be replicated:

1. **Continuing after auth failure:** The controller logs auth/cookie errors but continues execution, risking nil-pointer panics on the session cookie. New auth-dependent code should return early on auth failure.
2. **Mixed error packages:** Some files import `github.com/pingcap/errors` instead of `k8s.io/apimachinery/pkg/api/errors` for `IsNotFound`/`IsAlreadyExists` checks. Prefer `k8s.io/apimachinery/pkg/api/errors` for K8s API status checks to maintain consistency.
