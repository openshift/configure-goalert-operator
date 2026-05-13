# API Contracts Guidelines

## CRD Schema: GoalertIntegration

The CRD group is `goalert.managed.openshift.io`, version `v1alpha1`, kind `GoalertIntegration`. All six spec fields are required (enforced in the CRD manifest):

| Field | Type | Purpose |
|---|---|---|
| `clusterDeploymentSelector` | `metav1.LabelSelector` | Selects which ClusterDeployments get a GoAlert integration |
| `targetSecretRef` | `corev1.SecretReference` | Namespace and name where the secret is synced on the managed cluster |
| `highEscalationPolicy` | `string` | GoAlert escalation policy ID for high-severity alerts |
| `lowEscalationPolicy` | `string` | GoAlert escalation policy ID for low-severity alerts |
| `servicePrefix` | `string` | Prefix for GoAlert service names and derived resource names |
| `goalertCredsSecretRef` | `corev1.SecretReference` | Points to the Secret holding GoAlert API credentials |

The `Status` struct is empty -- no status fields are used today. If adding status fields, add them to `GoalertIntegrationStatus` in `api/v1alpha1/goalertintegration_types.go` and run `make generate && make manifests`.

## Resource Naming Formula

All secondary resources (ConfigMap, Secret, SyncSet) for a ClusterDeployment derive their names through `config.Name()`:

```
config.Name(servicePrefix, clusterDeploymentName, suffix) = "{servicePrefix}-{clusterDeploymentName}{suffix}"
```

The only suffix currently used is `config.ConfigMapSuffix` (`"-goalert-config"`). The Secret and SyncSet use the fixed name `config.SecretName` (`"goalert-secret"`), not the `Name()` function.

This means ConfigMap names are unique per GI+CD pair, but Secret and SyncSet names are shared across GI instances in the same namespace. Only one GoalertIntegration can effectively manage a given ClusterDeployment namespace.

## GoAlert Service Naming

GoAlert services are named using the cluster ID derived from the CD namespace, not the CD name:

```go
uid := strings.Split(cd.Namespace, "-")
clusterID := "fedramp-" + uid[len(uid)-1]
```

Two services are created per cluster: `"{clusterID} - High"` and `"{clusterID} - Low"`. The `Description` field is set to `cd.Spec.ClusterName`.

## GoAlert Credentials Secret Contract

The Secret referenced by `goalertCredsSecretRef` must contain exactly two keys, defined as constants in `config/config.go`:

| Constant | Key Value | Purpose |
|---|---|---|
| `GoalertUsernameSecretKey` | `USERNAME` | HTTP basic auth username |
| `GoalertPasswordSecretKey` | `PASSWORD` | HTTP basic auth password |

These are loaded via `utils.LoadSecretData()` which fails if the key is missing or empty.

## ConfigMap Data Keys

The ConfigMap (name: `{servicePrefix}-{cdName}-goalert-config`) stores GoAlert resource IDs with these hardcoded string keys (not constants):

| Key | Value |
|---|---|
| `HIGH_SERVICE_ID` | GoAlert service ID for high alerts |
| `LOW_SERVICE_ID` | GoAlert service ID for low alerts |
| `HEARTBEATMONITOR_ID` | GoAlert heartbeat monitor ID |

These keys are used as raw strings in `pkg/kube/configmap.go`, `clusterdeployment_deleted.go`, and `heartbeatmonitor_check.go`. Unlike the Secret data keys, they are not defined as constants in `config/config.go`. If you add new ConfigMap keys, follow the existing pattern (but consider extracting them to constants).

## Secret Data Keys (Managed Cluster)

The Secret synced to managed clusters (name: `"goalert-secret"`) contains integration URLs using constants from `config/config.go`:

| Constant | Key Value | Content |
|---|---|---|
| `GoalertHighIntKey` | `GOALERT_URL_HIGH` | Integration key URL for high alerts |
| `GoalertLowIntKey` | `GOALERT_URL_LOW` | Integration key URL for low alerts |
| `GoalertHeartbeatIntKey` | `GOALERT_HEARTBEAT` | Heartbeat monitor URL |

Always reference these via their constants, never as raw strings.

## SyncSet Contract

The SyncSet has the same name as the Secret (`"goalert-secret"`), lives in the CD namespace, and uses `ResourceApplyMode: "Sync"`. It maps a single `SecretMapping` from the management cluster Secret to the target specified in `gi.Spec.TargetSecretRef`. The SyncSet references the ClusterDeployment by name in `ClusterDeploymentRefs`.

## Finalizer Contract

Finalizers follow the pattern: `config.GoalertFinalizerPrefix + gi.Name`, producing `"goalert.managed.openshift.io/goalert-{giName}"`. Finalizers are placed on both the GoalertIntegration and on each matching ClusterDeployment. Each GI creates a distinct finalizer per CD, allowing multiple GIs to coexist on the same CD.

## GraphQL API Contract

All GraphQL calls go to `{GOALERT_ENDPOINT_URL}/api/graphql` as POST requests with JSON content type. Authentication uses a `goalert_session.2` cookie obtained from `/api/v2/identity/providers/basic`.

### Mutations

| Operation | GraphQL | Input Fields | Returns |
|---|---|---|---|
| Create Service | `createService` | `name`, `description`, `favorite`, `escalationPolicyID` | `id` (string) |
| Create Integration Key | `createIntegrationKey` | `serviceID`, `type`, `name` | `href` (string URL) |
| Create Heartbeat Monitor | `createHeartbeatMonitor` | `serviceID`, `name`, `timeoutMinutes` | `href` (string URL), `id` (string) |
| Delete Service | `deleteAll` | `id`, `type` (always `service`) | boolean |

### Queries

| Operation | GraphQL | Input | Returns |
|---|---|---|---|
| Heartbeat State | `heartbeatMonitor` | `id` | `lastState` (string; `"inactive"` = unhealthy) |

The `type` field for integration keys is always `prometheusAlertmanager` (unquoted enum in GraphQL, not a quoted string). The heartbeat monitor timeout is always `15` minutes. String parameters are quoted via `strconv.Quote()`.

### Response Structs

Each mutation/query has a dedicated response struct (`RespSvcData`, `RespIntKeyData`, `RespHeartBeatData`, `RespDelete`, `RespHeartbeatState`). Note the typo in `RespHeartbeatState`: the field is `Heatbeatmonitor` (missing 'r'), matching the JSON tag `heartbeatMonitor`. Do not "fix" this without updating the JSON unmarshaling.

## Prometheus Metrics Contract

All metrics use the `cgao_` prefix and include `ConstLabels: {"name": "configure-goalert-operator"}`. Metrics are served via `openshift/operator-custom-metrics` on port `8080` at `/metrics`, NOT via controller-runtime's built-in server.

| Metric | Type | Labels | Purpose |
|---|---|---|---|
| `cgao_reconcile_duration_seconds` | Histogram | `controller` | Reconcile loop duration. Buckets: 0.001, 0.01, 0.1, 1, 5, 10, 20 |
| `cgao_create_failure` | Gauge | `service_name` | Set to 1 on GoAlert service creation failure |
| `cgao_delete_failure` | Gauge | `service_name` | Set to 1 on GoAlert service deletion failure |
| `cgao_heartbeat_inactive` | Gauge | `service_name` | Set to 1 when heartbeat monitor state is inactive |

New metrics must be appended to `localmetrics.MetricsList` for automatic registration. Each metric should have a corresponding `Update*` helper function. The `cgao_heartbeat_inactive` metric also has a `DeleteMetricCGAOHeartbeat` function to remove label sets when clusters are deleted.

### Metric Label Values

- `controller` label: always `"goalertintegration"` (the `controllerName` constant)
- `service_name` label for create failures: the GoAlert service name (e.g., `"fedramp-abc123 - High"`)
- `service_name` label for delete failures: the GoAlert service ID (an opaque string from the ConfigMap)
- `service_name` label for heartbeat: the ClusterDeployment `.Name`

Note the inconsistency: create failures use the GoAlert service name, delete failures use the service ID, and heartbeat uses the CD name. Maintain this existing convention for compatibility.

## Environment Variables

| Constant | Value | Purpose |
|---|---|---|
| `GoalertApiEndpointEnvVar` | `GOALERT_ENDPOINT_URL` | Base URL for GoAlert API (no trailing slash) |

This is read via `os.Getenv()` at request time (not at startup), so it can theoretically change between reconciles.
