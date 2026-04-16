# API Contracts Guidelines

## CRD: GoalertIntegration (v1alpha1)

**API Group:** `goalert.managed.openshift.io`
**Version:** `v1alpha1`
**Kind:** `GoalertIntegration`

All spec fields are required (no `+optional` markers exist). Do not add optional fields without updating the CRD validation accordingly.

| Field | Type | Contract |
|---|---|---|
| `clusterDeploymentSelector` | `metav1.LabelSelector` | Matches ClusterDeployment CRs for integration. Empty selector matches all. |
| `targetSecretRef` | `corev1.SecretReference` | Name and namespace on the **target** (managed) cluster where the alerting secret lands. |
| `highEscalationPolicy` | `string` | GoAlert escalation policy ID for high-severity alerts. Passed verbatim to GraphQL. |
| `lowEscalationPolicy` | `string` | GoAlert escalation policy ID for low-severity alerts. Passed verbatim to GraphQL. |
| `servicePrefix` | `string` | Used in resource naming formula; see "Resource Naming" below. |
| `goalertCredsSecretRef` | `corev1.SecretReference` | Points to a Secret containing `USERNAME` and `PASSWORD` keys for GoAlert basic auth. |

The Status subresource is declared (`+kubebuilder:subresource:status`) but currently has no fields. Any new status fields must be added to `GoalertIntegrationStatus` and regenerated with `make generate`.

## Credentials Secret Contract

The Secret referenced by `goalertCredsSecretRef` must contain exactly these keys (defined in `config/config.go`):

| Key | Usage |
|---|---|
| `USERNAME` | HTTP basic auth username for GoAlert login |
| `PASSWORD` | HTTP basic auth password for GoAlert login |

The `LoadSecretData` helper (`pkg/utils/secrets.go`) will error if a key is missing or its value is empty.

## GoAlert Authentication Flow

Authentication is a two-step HTTP process, not GraphQL:

1. **POST** to `{GOALERT_ENDPOINT_URL}/api/v2/identity/providers/basic` with `application/x-www-form-urlencoded` body containing `username` and `password` form fields. A `Referer` header set to `{GOALERT_ENDPOINT_URL}/alerts` is required.
2. Extract the `goalert_session.2` cookie from the response `Set-Cookie` headers. This cookie authenticates all subsequent GraphQL calls.

The endpoint base URL comes from the `GOALERT_ENDPOINT_URL` environment variable. All API paths are relative to this base. Never hardcode the GoAlert host.

## GoAlert GraphQL API Contract

All GoAlert operations use a single endpoint: `{GOALERT_ENDPOINT_URL}/api/graphql` via HTTP POST.

### Request Format

Every request wraps a raw GraphQL query string in a JSON body:

```json
{"Query": "mutation {createService(input:{...}){id}}"}
```

There is no GraphQL client library. Queries are built with `fmt.Sprintf` and string values are escaped with `strconv.Quote`. Tabs are stripped with `strings.ReplaceAll(query, "\t", "")`.

### Required Headers

| Header | Value |
|---|---|
| `Content-Type` | `application/json` |
| `Accept` | `application/json` |
| Cookie | `goalert_session.2={value}` |

### Mutations and Queries

| Operation | GraphQL | Input Fields | Returns |
|---|---|---|---|
| `CreateService` | `mutation {createService(input:{name,description,favorite,escalationPolicyID}){id}}` | All fields quoted except `favorite` (bool) | Service ID |
| `CreateIntegrationKey` | `mutation {createIntegrationKey(input:{serviceID,type,name}){href}}` | `type` is unquoted (enum); `serviceID` and `name` are quoted | Integration key href |
| `CreateHeartbeatMonitor` | `mutation {createHeartbeatMonitor(input:{serviceID,name,timeoutMinutes}){href,id}}` | `timeoutMinutes` is an int (unquoted) | Heartbeat href and ID |
| `DeleteService` | `mutation {deleteAll(input:{id,type:service})}` | `type` is the literal unquoted `service` | Boolean success |
| `IsHeartbeatMonitorInactive` | `query {heartbeatMonitor(id:){lastState}}` | ID is quoted | `lastState` string, compared against `"inactive"` |

**Critical convention:** The `type` field in `CreateIntegrationKey` is a GraphQL enum and must NOT be quoted. The value used in create flow is `prometheusAlertmanager`. The `type` in `DeleteService` is the literal `service` (also unquoted).

### Response Structs

Each GraphQL operation has a dedicated Go response struct (`RespSvcData`, `RespIntKeyData`, `RespHeartBeatData`, `RespDelete`, `RespHeartbeatState`). When adding new operations, follow this pattern: define a struct mirroring the exact GraphQL response JSON shape with `json` tags matching the GraphQL field names.

## Client Interface (`pkg/goalert/`)

The `Client` interface is the sole abstraction for GoAlert API access:

```go
type Client interface {
    CreateService(ctx context.Context, data *Data) (string, error)
    CreateIntegrationKey(ctx context.Context, data *Data) (string, error)
    CreateHeartbeatMonitor(ctx context.Context, data *Data) (string, string, error)
    DeleteService(ctx context.Context, data *Data) error
    NewRequest(ctx context.Context, method string, body interface{}) ([]byte, error)
    IsHeartbeatMonitorInactive(ctx context.Context, data *Data) (bool, error)
}
```

The controller never calls `GraphqlClient` directly -- it receives the client via the `gclient` function field (`func(sessionCookie *http.Cookie) goalert.Client`), set to `goalert.NewClient` in `SetupWithManager`. This enables injecting test doubles.

The `Data` struct is reused across all operations. Fields are populated selectively: `EscalationPolicyID` for service creation, `Id`+`Type` for integration keys, `Id`+`Timeout` for heartbeat monitors. Unused fields zero-value safely.

## Resource Naming Conventions

All secondary Kubernetes resources follow naming rules defined in `config/config.go`.

### Name Formula

```
config.Name(servicePrefix, clusterDeploymentName, suffix) =
    "{servicePrefix}-{clusterDeploymentName}{suffix}"
```

### Resource Name Table

| Resource | Name | Namespace |
|---|---|---|
| ConfigMap | `{servicePrefix}-{cdName}-goalert-config` | ClusterDeployment namespace |
| Secret | `goalert-secret` (constant) | ClusterDeployment namespace |
| SyncSet | `goalert-secret` (constant, same as Secret) | ClusterDeployment namespace |

The Secret and SyncSet share the same constant name `goalert-secret`. The ConfigMap is the only resource with a dynamic name. This means only one GoalertIntegration can create a Secret/SyncSet per ClusterDeployment namespace.

### ConfigMap Data Keys

| Key | Value |
|---|---|
| `HIGH_SERVICE_ID` | GoAlert service ID for high alerts |
| `LOW_SERVICE_ID` | GoAlert service ID for low alerts |
| `HEARTBEATMONITOR_ID` | GoAlert heartbeat monitor ID |

These keys are read back during deletion and heartbeat checking. They are the operator's persistent state for GoAlert resources.

### Secret Data Keys

| Key (from `config/config.go`) | Constant Name | Value |
|---|---|---|
| `GOALERT_URL_HIGH` | `GoalertHighIntKey` | High-alert integration key href |
| `GOALERT_URL_LOW` | `GoalertLowIntKey` | Low-alert integration key href |
| `GOALERT_HEARTBEAT` | `GoalertHeartbeatIntKey` | Heartbeat monitor href |

## GoAlert Service Naming

GoAlert services are named using a cluster identifier derived from the ClusterDeployment namespace, not the CD name:

```go
uid := strings.Split(cd.Namespace, "-")
clusterID := "fedramp-" + uid[len(uid)-1]
```

Two services are created per cluster: `"{clusterID} - High"` and `"{clusterID} - Low"`. The heartbeat monitor is named `"{clusterID}"`. The service `Description` is set to `cd.Spec.ClusterName`.

## Finalizer Contract

Finalizer format: `goalert.managed.openshift.io/goalert-{goalertintegration-name}`

The same finalizer string is added to both the GoalertIntegration CR and each matched ClusterDeployment. During deletion, the finalizer on the ClusterDeployment triggers GoAlert service cleanup, and the finalizer on the GoalertIntegration triggers bulk cleanup of all associated ClusterDeployments.

Finalizers on ClusterDeployments are managed via `client.MergeFrom` patch (not direct update), which avoids conflicts with other controllers.

## SyncSet Contract

The SyncSet uses `ResourceApplyMode: "Sync"` and maps the operator-managed Secret from the hub cluster namespace to the target cluster location specified by `targetSecretRef` on the GoalertIntegration CR. The SyncSet references the ClusterDeployment by name, and the Secret by its source namespace and name.

## Prometheus Metrics Contract

All metrics use the `cgao_` prefix and carry a `name: "configure-goalert-operator"` const label. Metric names and label schemas:

| Metric | Type | Labels |
|---|---|---|
| `cgao_reconcile_duration_seconds` | Histogram | `controller` |
| `cgao_create_failure` | Gauge | `service_name` |
| `cgao_delete_failure` | Gauge | `service_name` |
| `cgao_heartbeat_inactive` | Gauge | `service_name` |

New metrics must follow this naming pattern and be appended to `MetricsList` in `pkg/localmetrics/localmetrics.go` for automatic registration.
