# Integration Guidelines

## GoAlert Client Usage

### Client Interface and Injection

The `goalert.Client` interface in `pkg/goalert/service.go` defines all GoAlert API operations (9 methods, including the 3 read queries described below). The reconciler stores a factory function `gclient func(sessionCookie *http.Cookie) goalert.Client` rather than a client instance directly. `SetupWithManager` assigns `goalert.NewClient` as the default factory; this pattern allows tests to inject mock clients.

All GraphQL calls go through `Client.NewRequest`, which reads `GOALERT_ENDPOINT_URL` from the environment, POSTs to `/api/graphql`, and attaches the session cookie. GoAlert GraphQL API calls should use this interface. Authentication to `/api/v2/identity/providers/basic` is performed in the controller's `authGoalert` method.

### Read Methods and Exact Name Matching

Three read methods enable idempotent resource creation:
- `GetServiceIDByName(ctx, name) (string, error)` -- searches for a service by name, returns its ID or `""` if not found
- `GetIntegrationKeyHref(ctx, serviceID, keyName, keyType) (string, error)` -- finds an integration key by name on a service, returns href or `""`
- `GetHeartbeatMonitor(ctx, serviceID, monitorName) (href, id, error)` -- finds a heartbeat monitor by name on a service, returns both href and id, or `("", "")`

GoAlert's search API is fuzzy/substring-based, so these methods perform **exact client-side name matching** after the query. A successful no-match returns an empty string (or empty strings) with `nil` error. Only transport failures, unmarshal errors, or GraphQL `errors` array entries produce a non-nil error. This contract allows callers to distinguish "resource does not exist" (empty string) from "lookup failed" (non-nil error).

### GraphQL Mutation Pattern

Mutations are built as raw query strings using `fmt.Sprintf` with `strconv.Quote` for string parameters. The `Q{Query: query}` struct is JSON-marshaled as the request body. Tabs are stripped with `strings.ReplaceAll(query, "\t", "")`. Follow this exact pattern for new mutations -- do not use a GraphQL client library.

### Authentication Flow

Authentication is a two-step process performed every reconcile (the session cookie is not cached or reused across reconcile loops):
1. POST form-encoded credentials to `/api/v2/identity/providers/basic` with a `Referer` header set to `{endpoint}/alerts`.
2. Extract the `goalert_session.2` cookie from the redirect response's `Set-Cookie` header.

### GoAlert Resource Creation Order

`handleCreate` creates GoAlert resources in this strict order:
1. **Add finalizer** to ClusterDeployment (via patch, returns early if added)
2. **Ensure High service** (`ensureService` helper: GET via `GetServiceIDByName`, create via `CreateService` only if absent)
3. **Ensure Low service** (`ensureService`)
4. **Ensure High integration key** (`ensureIntegrationKey` helper: GET via `GetIntegrationKeyHref`, create via `CreateIntegrationKey` only if absent; type `prometheusAlertmanager`)
5. **Ensure Low integration key** (`ensureIntegrationKey`)
6. **Ensure heartbeat monitor** (`ensureHeartbeatMonitor` helper: GET via `GetHeartbeatMonitor`, create via `CreateHeartbeatMonitor` only if absent; on the High service, 15-minute timeout)
7. **Create or update ConfigMap** (stores `HIGH_SERVICE_ID`, `LOW_SERVICE_ID`, `HEARTBEATMONITOR_ID`). If `AlreadyExists`, call `Update` and fall through (do not return early).
8. **Guard and create Secret** -- before calling `Create`, verify all three integration key hrefs are non-empty. If any are empty, return an error (never persist an empty secret, which would break alerting); the reconcile loop accumulates this error and, after processing every matching CD, requeues with backoff so the keys are re-fetched. If Secret `AlreadyExists`, compare data; if changed, delete and recreate.
9. **Create SyncSet** (references the Secret for propagation). Uses `Get`-then-create approach.

Each step depends on IDs/keys from previous steps. If any GoAlert API call fails, return the error immediately -- do not create partial K8s resources.

**Idempotency via get-or-create:** Steps 2-6 use `ensure*` helpers that adopt pre-existing GoAlert resources (by exact name match), making reconcile idempotent. A retry after partial failure will reuse existing services/keys/monitors instead of creating duplicates or hitting duplicate-name rejections. The ConfigMap and Secret creation logic (steps 7-8) similarly handle `AlreadyExists` to avoid errors on retry.

### GoAlert Resource Deletion Order

`handleDelete` performs cleanup in this order:
1. Read ConfigMap to get `HIGH_SERVICE_ID` and `LOW_SERVICE_ID`
2. Delete High service via `deleteAll` mutation (type: `service`) -- integration keys and heartbeat monitors are removed by GoAlert server-side cascade
3. Delete Low service via `deleteAll` mutation
4. Delete ConfigMap
5. Delete Secret
6. Delete SyncSet
7. Remove finalizer from ClusterDeployment (via patch)
8. Delete heartbeat metric via `localmetrics.DeleteMetricCGAOHeartbeat`

If the ConfigMap is not found, steps 1-3 are skipped (no GoAlert services to delete). Secret and SyncSet deletions are similarly guarded with NotFound checks.

## Hive Dual-Watch Pattern

### Controller Setup

The controller watches two resource types registered in `SetupWithManager`:
- **Primary:** `GoalertIntegration` (via `For`)
- **Secondary:** `ClusterDeployment` (via `Watches` with custom `enqueueRequestForClusterDeployment` handler)

When a ClusterDeployment event fires, the event handler lists all GoalertIntegrations, checks which ones have a `ClusterDeploymentSelector` matching the CD's labels, and enqueues reconcile requests for those GIs. The reconcile loop always receives a GI's `NamespacedName`, never a CD's.

### Two ClusterDeployment Lists

Every reconcile fetches two separate lists:
- `allClusterDeployments`: every CD in the cluster, used for finalizer-based cleanup
- `matchingClusterDeployments`: CDs matching `gi.Spec.ClusterDeploymentSelector`, used for creation and heartbeat checks

This dual-list approach handles label un-matching: a CD that previously matched (has a finalizer) but no longer matches is found in `allClusterDeployments` and cleaned up.

### Event Handler De-duplication

`enqueueRequestForClusterDeployment` uses a `map[reconcile.Request]struct{}` to de-duplicate requests within a single event. For Update events, both `ObjectOld` and `ObjectNew` are mapped, preventing duplicate enqueues when labels change.

## SyncSet Propagation

SyncSets are generated by `kube.GenerateSyncSet` and use Hive's `SecretMapping` (not raw resource apply) to sync the goalert Secret from the management cluster to the managed cluster. Key conventions:
- `ResourceApplyMode` is always `"Sync"` (Hive reconciles continuously)
- The SyncSet name equals `config.SecretName` ("goalert-secret"), not the CD name
- `ClusterDeploymentRefs` contains a single reference to the owning CD
- Source ref uses the Secret's namespace/name on the management cluster
- Target ref uses `gi.Spec.TargetSecretRef` (namespace/name on the managed cluster)

## Finalizer Lifecycle

### Naming Convention

Finalizers follow the pattern `goalert.managed.openshift.io/goalert-{gi.Name}`, constructed as `config.GoalertFinalizerPrefix + gi.Name`. This allows multiple GoalertIntegrations to independently manage the same ClusterDeployment.

### Three Deletion Triggers

Finalizer removal and GoAlert cleanup (`handleDelete`) is triggered by three conditions:
1. **GI deletion** (`gi.DeletionTimestamp != nil`): iterates CDs with the GI's finalizer, calls `handleDelete` for each, then removes the finalizer from the GI itself.
2. **CD deletion** (`cd.DeletionTimestamp != nil`): found by scanning `allClusterDeployments` for those with the finalizer.
3. **Label un-match**: CD has the finalizer but is no longer in `matchingClusterDeployments`.

### Add/Remove Mechanics

- **CD finalizer add:** Uses `client.MergeFrom(cd.DeepCopy())` patch, not a full Update. Returns early after patching so the next reconcile continues creation.
- **CD finalizer remove:** Also uses MergeFrom patch in `handleDelete`.
- **GI finalizer add/remove:** Uses `r.Update(ctx, gi)`.

## Heartbeat Monitoring

Heartbeat monitors are checked every reconcile for all matching, non-deleting CDs. The flow:
1. Read the ConfigMap to get `HEARTBEATMONITOR_ID`
2. Call `IsHeartbeatMonitorInactive` (GraphQL query for `heartbeatMonitor.lastState`)
3. If inactive: set `cgao_heartbeat_inactive` gauge to 1 for that CD
4. If active and gauge is currently > 0: reset gauge to 0
5. On CD deletion: call `localmetrics.DeleteMetricCGAOHeartbeat` to remove the metric entirely

The heartbeat monitor is always created on the **High** service with a **15-minute** timeout and named with the cluster ID (`fedramp-{last-segment-of-namespace}`).

## Resource Naming Conventions

All secondary resource names use `config.Name(servicePrefix, clusterDeploymentName, suffix)` which produces `{servicePrefix}-{cdName}{suffix}`. Specific names:
- ConfigMap: `config.Name(gi.Spec.ServicePrefix, cd.Name, config.ConfigMapSuffix)` -- e.g., `myprefix-mycluster-goalert-config`
- Secret: always `config.SecretName` ("goalert-secret") -- shared per namespace
- SyncSet: always `config.SecretName` ("goalert-secret") -- same name as the Secret

### Controller References

All secondary K8s resources (ConfigMap, Secret, SyncSet) must have their controller reference set to the **ClusterDeployment**, not the GoalertIntegration:
```go
controllerutil.SetControllerReference(cd, resource, r.Scheme)
```

## Existence Check Pattern

`cgaoResourcesExist` checks for ConfigMap, Secret, and SyncSet before calling `handleCreate`. Creation is triggered when **any** of the three is missing. Each check uses `r.Get` and distinguishes between NotFound (resource missing) and other errors (return error to requeue). This is the gating mechanism for idempotent reconciliation -- if all three exist, no GoAlert API calls are made.

**Content-aware Secret check (self-heal):** The Secret existence check is not a simple presence check. A Secret that is present but has empty or missing data for any of the three integration key fields (`GOALERT_URL_HIGH`, `GOALERT_URL_LOW`, `GOALERT_HEARTBEAT`) is treated as **not existing**, triggering a re-entry into `handleCreate` on the next reconcile. This self-heals a deployed-but-empty secret without requiring manual GoAlert resource cleanup.
