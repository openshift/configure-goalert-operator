# Security Guidelines

## Credential Flow

GoAlert credentials follow a specific path through the operator. Understanding this flow is required before modifying authentication or secret-handling code.

1. **Source**: A Kubernetes Secret referenced by `GoalertIntegration.Spec.GoalertCredsSecretRef` contains keys `USERNAME` and `PASSWORD` (constants in `config/config.go`).
2. **Loading**: `pkg/utils/LoadSecretData()` reads individual keys from the Secret. Use this function for loading GoAlert credentials from the Secret referenced by `goalertCredsSecretRef` -- it validates that keys exist and are non-empty.
3. **Authentication**: `authGoalert()` POSTs form-encoded credentials to GoAlert's `/api/v2/identity/providers/basic` endpoint.
4. **Session**: `fetchSessionCookie()` extracts the `goalert_session.2` cookie from the response. This cookie is passed to `GraphqlClient` for all subsequent API calls.
5. **Propagation**: Integration keys returned by GoAlert are stored in a Secret on the Hive cluster, then synced to managed clusters via SyncSet.

## Secret Key Constants

Always reference constants from `config/config.go` when accessing Secret data. Never use string literals for these values:

| Constant | Value | Usage |
|---|---|---|
| `GoalertUsernameSecretKey` | `USERNAME` | Key in GoAlert creds Secret |
| `GoalertPasswordSecretKey` | `PASSWORD` | Key in GoAlert creds Secret |
| `GoalertHighIntKey` | `GOALERT_URL_HIGH` | Key in generated goalert-secret |
| `GoalertLowIntKey` | `GOALERT_URL_LOW` | Key in generated goalert-secret |
| `GoalertHeartbeatIntKey` | `GOALERT_HEARTBEAT` | Key in generated goalert-secret |
| `SecretName` | `goalert-secret` | Fixed Secret name used in every CD namespace (not unique per CD) |

## Logging Rules for Secrets

- **Never log**: Usernames, passwords, session cookies, integration key URLs, or Secret `.Data` contents.
- **Safe to log**: Service IDs (e.g., `goalertHighServiceID`), resource names, namespaces, ConfigMap data keys, heartbeat monitor IDs.
- The existing code correctly logs only the error message on credential load failure, not the credential values. Maintain this pattern.
- Log messages about secrets should reference the resource name or namespace, not data content: `r.reqLogger.Info("creating goalert secret", "ClusterDeployment.Namespace", cd.Namespace)`.

## GraphQL Injection Prevention

All user-controlled values interpolated into GraphQL mutation strings in `pkg/goalert/service.go` must be wrapped with `strconv.Quote()`. The codebase already does this for `Name`, `Description`, `EscalationPolicyID`, and `Id` fields. Note that `data.Type` in `CreateIntegrationKey` is **not** quoted because it is an enum, not a string -- but if its source ever becomes user-controlled, it must be quoted or validated.

When adding new GraphQL queries:
- Use `strconv.Quote()` for all string-type fields interpolated into queries.
- Use `strings.ReplaceAll(query, "\t", "")` to strip tabs from multi-line query strings.
- Wrap the query in a `Q{Query: query}` struct for JSON serialization.

## FIPS Compliance

- `FIPS_ENABLED=true` is set in the `Makefile`, which causes the build to use the `fips_enabled` build tag.
- `fips.go` imports `crypto/tls/fipsonly`, restricting all TLS connections to FIPS-approved cipher suites and TLS 1.2+.
- `fips.go` is boilerplate-generated. Do not edit it directly; run `make ensure-fips` to regenerate.
- Both `authGoalert()` and `GraphqlClient.NewRequest()` use `http.DefaultClient`, which inherits the FIPS TLS restrictions when the build tag is active.
- Never configure a custom `http.Transport` with `InsecureSkipVerify: true` or non-FIPS cipher suites.

## RBAC Configuration

The operator's RBAC is defined in `deploy/01-role.yaml` as a **ClusterRole** (not namespace-scoped). Key observations:

- The operator has `"*"` verbs on core resources (pods, services, endpoints, configmaps, secrets, events, persistentvolumeclaims) and apps resources. This is broader than necessary -- do not widen it further.
- Hive resources (clusterdeployments, syncsets) are scoped to `get`, `list`, `watch`, `update`, `patch`, `create`, `delete` as needed.
- The kubebuilder RBAC markers in the controller file generate a separate RBAC manifest. The deployed RBAC in `deploy/01-role.yaml` is the authoritative source; the markers serve as documentation.
- A separate `Role` in `deploy/07-prometheus_role.yaml` grants Prometheus read-only access to services/endpoints/pods in the operator namespace only.

When adding new resource types the operator must access, add the minimum required verbs to `deploy/01-role.yaml`.

## Container Security

- **Non-root execution**: The Dockerfile sets `USER ${USER_UID}` (1001). The `user_setup` script creates a writable home directory owned by UID 1001 and group 0 (OpenShift's arbitrary UID convention).
- **Minimal base image**: Runtime image uses `ubi9/ubi-minimal`, not a full OS image. Do not add package managers or install packages in the runtime stage.
- **Multi-stage build**: Only the compiled binary and `build/bin` scripts are copied to the runtime image. Source code and build tools are not present.
- **`/etc/passwd` writability**: The entrypoint script writes to `/etc/passwd` for OpenShift UID compatibility. This is expected and required.

## Secret Lifecycle and Ownership

- All Secrets, ConfigMaps, and SyncSets created by the operator must have `controllerutil.SetControllerReference(cd, resource, r.Scheme)` set to the **ClusterDeployment**, not the GoalertIntegration. This ensures garbage collection when the CD is deleted.
- The generated Secret (`goalert-secret`) contains integration key URLs that grant the ability to send alerts to GoAlert. Treat these as sensitive credentials.
- Secret rotation follows a delete-then-recreate pattern (see `clusterdeployment_created.go` lines 153-164). There is no in-place update.

## Environment Variable Security

- `GOALERT_ENDPOINT_URL` is read from the environment via `os.Getenv()` in both `authGoalert()` and `GraphqlClient.NewRequest()`. This value controls which server receives credentials. It is set in the Deployment manifest, not in a Secret.
- The Deployment manifest (`deploy/04-operator.yaml`) does not mount the GoAlert credentials Secret directly -- credentials are loaded dynamically at reconcile time from the Secret referenced in each GoalertIntegration CR.
- Never add `GOALERT_ENDPOINT_URL` to log output if it could contain embedded credentials in the URL.

## Testing Security Patterns

- Tests in `pkg/goalert/service_test.go` use `httptest.NewServer` to mock the GoAlert API. Set the endpoint via `t.Setenv(config.GoalertApiEndpointEnvVar, ts.URL)` -- never hardcode URLs.
- Test session cookies use dummy values (`"test_cookie"`). Never use real credentials in test fixtures.
- The `GraphqlClient` accepts an `httpClient` field, allowing tests to use `mockServer.Client()` instead of `http.DefaultClient`. New GoAlert client methods must preserve this testability.
