# Integration Guidelines

## GoAlert Client Usage

### Client Interface and Injection

The `goalert.Client` interface in `pkg/goalert/service.go` defines all GoAlert API operations. The reconciler stores a factory function `gclient func(sessionCookie *http.Cookie) goalert.Client` rather than a client instance directly. `SetupWithManager` assigns `goalert.NewClient` as the default factory; this pattern allows tests to inject mock clients.

All GraphQL calls go through `Client.NewRequest`, which reads `GOALERT_ENDPOINT_URL` from the environment, POSTs to `/api/graphql`, and attaches the session cookie. GoAlert GraphQL API calls should use this interface. Authentication to `/api/v2/identity/providers/basic` is performed in the controller's `authGoalert` method.

### GraphQL Mutation Pattern

Mutations are built as raw query strings using `fmt.Sprintf` with `strconv.Quote` for string parameters. The `Q{Query: query}` struct is JSON-marshaled as the request body. Tabs are stripped with `strings.ReplaceAll(query, "\t", "")`. Follow this exact pattern for new mutations -- do not use a GraphQL client library.

### Authentication Flow

Authentication is a two-step process performed every reconcile (the session cookie is not cached or reused across reconcile loops):
1. POST form-encoded credentials to `/api/v2/identity/providers/basic` with a `Referer` header set to `{endpoint}/alerts`.
2. Extract the `goalert_session.2` cookie from the redirect response's `Set-Cookie` header.

**Known issue:** If authentication fails, the controller logs the error but continues execution rather than returning early, risking nil-pointer panics on the session cookie. See "Auth failure continuation" in AGENTS.md Known Bugs.

### GoAlert Resource Creation Order

`handleCreate` creates GoAlert resources in this strict order:
1. **Add finalizer** to ClusterDeployment (via patch, returns early if added)
2. **Create High service** (`CreateService`)
3. **Create Low service** (`CreateService`)
4. **Create High integration key** (`CreateIntegrationKey` with type `prometheusAlertmanager`)
5. **Create Low integration key** (`CreateIntegrationKey`)
6. **Create heartbeat monitor** (`CreateHeartbeatMonitor` on the High service, 15-minute timeout)
7. **Create ConfigMap** (stores `HIGH_SERVICE_ID`, `LOW_SERVICE_ID`, `HEARTBEATMONITOR_ID`)
8. **Create Secret** (stores integration key URLs and heartbeat key)
9. **Create SyncSet** (references the Secret for propagation)

Each step depends on IDs/keys from previous steps. If any GoAlert API call fails, return the error immediately -- do not create partial K8s resources.

**Retry/idempotency caveat:** On failure, the reconciler requeues and re-enters `handleCreate` from the top. The GoAlert API calls are not idempotent -- there is no check for whether a service already exists before creating one. A failure after step 2 but before step 7 (ConfigMap creation) will produce orphaned GoAlert services on retry, since the operator has no record of the previously created IDs. There is no circuit breaker; retries continue indefinitely via the controller-runtime work queue.

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
