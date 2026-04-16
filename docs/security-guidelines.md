# Security Guidelines

## Credential Storage and References

### GoAlert API credentials

The operator authenticates to GoAlert using HTTP basic auth credentials stored in a Kubernetes Secret. The GoalertIntegration CR references this secret via `spec.goalertCredsSecretRef` (name + namespace). The canonical secret name in production is `goalert-creds` in the `configure-goalert-operator` namespace.

Required keys in that secret (defined in `config/config.go`):
- `USERNAME` -- GoAlert username
- `PASSWORD` -- GoAlert password

Rules:
- Load credentials exclusively through `pkg/utils.LoadSecretData()`. This function validates the key exists and is non-empty.
- Never read credentials from environment variables, command-line flags, or ConfigMaps.
- Never add default or fallback values for credentials in code. If the secret is missing, the reconciler must error.

### GoAlert endpoint URL

`GOALERT_ENDPOINT_URL` is the only credential-adjacent value supplied as an environment variable (injected via OLM Subscription config in `hack/olm-artifacts-template.yaml`). It contains a URL, not a secret. Do not add other secret values as env vars -- only non-secret configuration belongs in env vars.

## Authentication Flow

The operator uses session-cookie-based auth against GoAlert's basic auth provider. The flow in `goalertintegration_controller.go`:

1. POST form-encoded `username`/`password` to `{GOALERT_ENDPOINT_URL}/api/v2/identity/providers/basic`
2. Extract `goalert_session.2` cookie from the response `set-cookie` headers
3. Pass the cookie to `goalert.NewClient()` which attaches it to all subsequent GraphQL requests

Rules:
- The session cookie is a bearer credential. Never log its value. Current code does not log it -- maintain this.
- The `http.DefaultClient` is used for auth; the `GraphqlClient` wrapper is used for API calls. Both use the same cookie. Do not introduce a second auth mechanism.
- The `Referer` header is set to `{endpoint}/alerts` on the auth request. This is required by GoAlert's CSRF protection. Do not remove it.
- The auth response body is closed via `defer` in the reconcile loop. If you refactor the auth flow, ensure the response body is always closed.

## Data Classification: Secrets vs ConfigMaps

The operator creates three resources per ClusterDeployment. The split between Secret and ConfigMap is intentional and security-critical:

**ConfigMap** (`{servicePrefix}-{cdName}-goalert-config`):
- `HIGH_SERVICE_ID` -- GoAlert service ID (non-secret, used for deletion lookups)
- `LOW_SERVICE_ID` -- GoAlert service ID
- `HEARTBEATMONITOR_ID` -- heartbeat monitor ID

**Secret** (`goalert-secret`):
- `GOALERT_URL_HIGH` -- integration key/URL for high-severity alerts
- `GOALERT_URL_LOW` -- integration key/URL for low-severity alerts
- `GOALERT_HEARTBEAT` -- heartbeat monitor key

Rules:
- Integration keys and heartbeat keys are bearer tokens that allow sending alerts. They must always be stored in Secrets, never ConfigMaps.
- Service IDs are opaque identifiers, not credentials. They belong in ConfigMaps.
- If you add new GoAlert resources, classify their identifiers: if the value grants any access or action capability, it goes in the Secret.

## Secret Propagation via SyncSets

The `goalert-secret` Secret is propagated to managed clusters using Hive SyncSets. The SyncSet maps the hub-cluster Secret to the target cluster's namespace and name specified by `spec.targetSecretRef` (typically `goalert-secret` in `openshift-monitoring`).

Rules:
- The SyncSet uses `ResourceApplyMode: Sync`, meaning it continuously reconciles. Do not change this to `Apply` or remove it, as that would leave stale credentials on clusters.
- The SyncSet uses `Secrets` field (SecretMapping), not `Resources`. This is important: SecretMapping handles Secrets specially, avoiding the Secret data appearing in the SyncSet spec itself. Never switch to embedding Secret data in `Resources`.
- The target namespace (`openshift-monitoring`) is controlled by the GoalertIntegration CR, not hardcoded. Do not hardcode it in the SyncSet generator.

## Secret Update Pattern

In `clusterdeployment_created.go`, when a Secret already exists but its data has changed, the controller deletes and recreates it rather than patching. This is the established pattern:

```go
if string(sc.Data[config.GoalertHighIntKey]) != highIntKey || ... {
    r.Delete(ctx, secret)
    r.Create(ctx, secret)
}
```

Rules:
- Maintain this delete-then-create pattern for Secrets. Do not use `Update` on Secrets, as the controller reference and ownership metadata must be preserved correctly.
- The comparison uses the actual Secret data keys from `config/config.go` constants. Always use these constants, never string literals.

## RBAC

The deployed ClusterRole (`deploy/01-role.yaml`) grants `*` (all verbs) on core resources including `secrets`. This is broader than the controller-gen annotations in the controller source.

Rules:
- The controller-gen RBAC markers (`+kubebuilder:rbac`) in source only cover the GoalertIntegration CRD. The actual deployed RBAC is in `deploy/01-role.yaml` and `deploy_pko/ClusterRole-configure-goalert-operator.yaml`. Always update both when changing permissions.
- The operator needs `get`, `create`, `delete` on Secrets and `get`, `create`, `update`, `delete` on ConfigMaps. If tightening RBAC, do not go below these.
- SyncSet permissions (`create`, `delete` + `get`, `list`, `watch`, `update`, `patch`) are split across two rule blocks in the ClusterRole. Both are required.

## FIPS Compliance

The operator is built with FIPS-enabled crypto (`fips.go` imports `crypto/tls/fipsonly` under the `fips_enabled` build tag, and `Makefile` sets `FIPS_ENABLED=true`).

Rules:
- Do not import non-FIPS-compliant crypto libraries.
- Do not add TLS configuration that bypasses the default FIPS-only TLS settings (e.g., do not set `MinVersion` to TLS 1.0/1.1 or enable non-FIPS cipher suites).
- The `http.DefaultClient` inherits FIPS-compliant TLS from the runtime. If you create custom `http.Client` instances, do not override `TLSClientConfig` unless necessary, and if you do, ensure FIPS compliance.

## GraphQL Injection Prevention

The GoAlert client (`pkg/goalert/service.go`) constructs GraphQL mutations as raw strings using `fmt.Sprintf`. User-controlled values are escaped with `strconv.Quote()`.

Rules:
- Always wrap string values in GraphQL queries with `strconv.Quote()`. This adds Go string escaping. Example from the codebase:
  ```go
  query := fmt.Sprintf(`mutation {createService(input:{name:%s,...}){id}}`,
      strconv.Quote(data.Name), ...)
  ```
- Enum-type values (like `type: service` in `deleteAll`) are not quoted and must not come from user input.
- If adding new GraphQL operations, follow the existing pattern exactly. Do not use string concatenation without `strconv.Quote`.

## Container Security

The Dockerfile (`build/Dockerfile`) runs the operator as a non-root user (`USER_UID=1001`). The base image is `ubi9/ubi-minimal`.

Rules:
- Do not change `USER ${USER_UID}` or set it to 0 (root).
- Do not add `--privileged` or `SYS_ADMIN` capabilities.
- The OLM CatalogSource uses `securityContextConfig: restricted`. Maintain this setting.

## Logging

Rules:
- Never log Secret data values (passwords, integration keys, heartbeat keys, session cookies).
- Service IDs and resource names are safe to log -- they appear in current log statements.
- Error messages from `utils.LoadSecretData` include the secret name and key name but not the value. Maintain this pattern.
- The `reqLogger` is scoped to each reconcile invocation. Use it rather than the package-level `log` for all controller logging.

## Finalizer Security

Finalizers prevent premature deletion of resources that still have GoAlert services associated with them. The finalizer name is `goalert.managed.openshift.io/goalert-{gi-name}`.

Rules:
- Finalizers are added to both GoalertIntegration and ClusterDeployment resources. Always clean up both.
- Use `client.MergeFrom(cd.DeepCopy())` with `Patch` for finalizer modifications on ClusterDeployments (not `Update`), to avoid conflicts.
- The `handleDelete` function must delete GoAlert services before removing finalizers. Never reverse this order -- doing so risks orphaned GoAlert services.

## Testing with Credentials

Tests in `pkg/goalert/service_test.go` use `httptest.NewServer` to mock GoAlert. They set `GOALERT_ENDPOINT_URL` via `t.Setenv()`, which automatically restores the original value after each test.

Rules:
- Never use real GoAlert credentials in tests.
- Always use `t.Setenv()` (not `os.Setenv()`) so env vars are cleaned up automatically.
- Test cookies use dummy values (`test_cookie`). Never embed real session tokens in test code.
