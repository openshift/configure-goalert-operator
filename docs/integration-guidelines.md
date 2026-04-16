# Integration Guidelines

## GoAlert API Client (`pkg/goalert/`)

### Authentication Pattern

The operator authenticates to GoAlert via HTTP basic auth (form-encoded POST to `/api/v2/identity/providers/basic`), then extracts a `goalert_session.2` cookie from the response's `set-cookie` headers. This cookie is passed to `goalert.NewClient()` and attached to every subsequent GraphQL request. Authentication happens once per reconcile loop, not per API call.

- The GoAlert endpoint is read from the `GOALERT_ENDPOINT_URL` environment variable at call time (`os.Getenv` inside `NewRequest`), not at startup. This means the env var must be set in the operator's Deployment spec.
- The session cookie is never persisted. A fresh login occurs on every reconciliation.

### GraphQL Convention

All GoAlert mutations use hand-built GraphQL query strings via `fmt.Sprintf`, not a GraphQL library. Follow this pattern exactly:

```go
query := fmt.Sprintf(`mutation {createService(input:{name:%s,description:%s,favorite:%t,escalationPolicyID:%s}){id}}`,
    strconv.Quote(data.Name), strconv.Quote(data.Description), data.Favorite, strconv.Quote(data.EscalationPolicyID))
query = strings.ReplaceAll(query, "\t", "")
body := Q{Query: query}
```

Rules:
- Use `strconv.Quote()` for string parameters (handles escaping).
- Strip tabs with `strings.ReplaceAll(query, "\t", "")`.
- POST to `/api/graphql` via `NewRequest`.
- Define a dedicated response struct for each operation (e.g., `RespSvcData`, `RespIntKeyData`).
- The `Client` interface must be updated when adding new GraphQL operations.

### Client Interface for Testability

The `goalert.Client` interface wraps all GoAlert operations. The reconciler stores a factory function `gclient func(sessionCookie *http.Cookie) goalert.Client` that defaults to `goalert.NewClient` in `SetupWithManager`. This allows injecting mock clients in tests without modifying the reconciler.

## Hive Integration

### Dual-Watch Pattern

The controller watches two resource types but reconciles on `GoalertIntegration`:

1. Primary: `GoalertIntegration` CRs (standard `For()` watch).
2. Secondary: `ClusterDeployment` CRs via a custom `enqueueRequestForClusterDeployment` event handler using `source.Kind` (note: `source.Kind` is deprecated in controller-runtime v0.15+; this codebase uses v0.13.0).

The custom handler maps ClusterDeployment events to GoalertIntegration reconcile requests by listing all GoalertIntegrations and checking which ones have a `clusterDeploymentSelector` matching the CD's labels. This is a many-to-many relationship: multiple GIs can match a single CD.

### Resource Creation Order

`handleCreate` creates resources in this strict sequence. Do not reorder:

1. Add finalizer to ClusterDeployment (via patch, then return -- the next reconcile continues)
2. Create high-severity GoAlert service
3. Create low-severity GoAlert service
4. Create integration keys (type `prometheusAlertmanager`) for both services
5. Create heartbeat monitor on the high-severity service (15-minute timeout)
6. Create ConfigMap (stores service IDs + heartbeat monitor ID)
7. Create Secret (stores integration key URLs + heartbeat key)
8. Create SyncSet (references the Secret for propagation)

All secondary Kubernetes resources (ConfigMap, Secret, SyncSet) have `SetControllerReference(cd, ...)` set to the ClusterDeployment, enabling garbage collection.

### Resource Naming Conventions

Names are generated via `config.Name()`:
- ConfigMap: `{servicePrefix}-{clusterDeploymentName}-goalert-config`
- Secret: always `goalert-secret` (constant `config.SecretName`)
- SyncSet: always `goalert-secret` (same constant, shares name with Secret)

The Secret and SyncSet share a fixed name per namespace. The ConfigMap is unique per GI+CD combination because it includes the service prefix.

### Existence Check Pattern

Before creating, the controller checks whether the ConfigMap, Secret, and SyncSet already exist using `cgaoResourcesExist()`. If any one is missing, `handleCreate` runs. This means partial creation failures are retried on the next reconcile since at least one resource will be absent.

## Secret Propagation via SyncSets

SyncSets use `ResourceApplyMode: "Sync"` and the `Secrets` field (not `Resources`), which maps a source Secret on the management cluster to a target Secret on the managed cluster:

```go
Secrets: []hivev1.SecretMapping{{
    SourceRef: hivev1.SecretReference{Namespace: secret.Namespace, Name: secret.Name},
    TargetRef: hivev1.SecretReference{Namespace: gi.Spec.TargetSecretRef.Namespace, Name: gi.Spec.TargetSecretRef.Name},
}}
```

The target namespace and name come from `GoalertIntegration.Spec.TargetSecretRef`. The Secret contains three keys: `GOALERT_URL_HIGH`, `GOALERT_URL_LOW`, `GOALERT_HEARTBEAT`.

When Secret data changes, the existing Secret is deleted and recreated (not patched). This is intentional -- see the comparison logic in `handleCreate`.

## Finalizer Pattern

Finalizer format: `goalert.managed.openshift.io/goalert-{gi-name}`

The same finalizer string (derived from the GoalertIntegration name) is added to both the GoalertIntegration CR and each matching ClusterDeployment CR. This means:
- Multiple GoalertIntegrations produce distinct finalizers on the same CD.
- Finalizers are added via `client.MergeFrom` patch (not direct update) on ClusterDeployments to avoid conflicts.
- Finalizers on GoalertIntegrations use `controllerutil.AddFinalizer` / `controllerutil.RemoveFinalizer`. Note: the existing code has inverted logic where it calls `Update` when the finalizer operation returns false (no change) rather than true (change made).

### Deletion Cleanup Order

`handleDelete` performs cleanup in this order:
1. Read service IDs from ConfigMap
2. Delete high GoAlert service via API
3. Delete low GoAlert service via API
4. Delete ConfigMap
5. Delete Secret
6. Delete SyncSet
7. Remove finalizer from ClusterDeployment (via patch)
8. Delete heartbeat metric from Prometheus registry

If the ConfigMap is already gone (not-found), GoAlert API deletion is skipped. Each subsequent resource deletion is independently guarded by a not-found check, so partial cleanup from a previous attempt does not block progress.

### Three Deletion Triggers

GoAlert services are deleted when:
1. The GoalertIntegration CR is deleted (bulk cleanup of all matching CDs -- note: there is a bug in the bulk cleanup loop where it iterates over `matchingClusterDeployments.Items` but accesses `allClusterDeployments.Items[i]`).
2. A ClusterDeployment CR is deleted (`DeletionTimestamp != nil`).
3. A CD's labels change so it no longer matches the GI's selector (detected by comparing all-CDs-with-finalizer against matching-CDs).

## Heartbeat Monitoring

- A heartbeat monitor is created on the high-severity service only, with a 15-minute timeout.
- The heartbeat monitor ID is stored in the ConfigMap under `HEARTBEATMONITOR_ID`.
- On every reconcile, `checkHeartbeatMonitor` queries GoAlert for the monitor's `lastState` and sets `cgao_heartbeat_inactive` to 1 if inactive, 0 if active.
- When a CD is deleted, the heartbeat metric is removed from the Prometheus registry via `DeleteMetricCGAOHeartbeat`.

## Prometheus Metrics

All metrics use the `cgao_` prefix and carry a constant label `name: configure-goalert-operator`. Metrics are registered via `openshift/operator-custom-metrics`, not the standard controller-runtime metrics server (the controller-runtime metrics bind address is set to `"0"` to disable it).

| Metric | Type | Labels | When Updated |
|---|---|---|---|
| `cgao_reconcile_duration_seconds` | Histogram | `controller` | Every reconcile (via defer) |
| `cgao_create_failure` | Gauge | `service_name` | GoAlert CreateService fails |
| `cgao_delete_failure` | Gauge | `service_name` | GoAlert DeleteService fails |
| `cgao_heartbeat_inactive` | Gauge | `service_name` | Every reconcile per matching CD |

Failure gauges are set to 1 on failure but never reset to 0. The heartbeat gauge is reset to 0 when the monitor becomes active again.

## Error Handling and Retry

- The controller does not set explicit requeue intervals. Errors returned from `Reconcile` trigger controller-runtime's default exponential backoff.
- GoAlert API errors during create/delete cause the handler to return an error, which propagates to `Reconcile` and triggers a requeue.
- The `cgaoResourcesExist` check means creation is idempotent at the Kubernetes resource level, but GoAlert services may be created again if the ConfigMap was lost (orphaning the previous GoAlert service).
