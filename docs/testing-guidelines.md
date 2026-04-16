# Testing Guidelines

## Running Tests

```bash
# Run all unit tests (default)
make go-test

# Run tests for a single package
TESTTARGETS=./pkg/goalert/... make go-test

# Run tests matching CI exactly (uses boilerplate container)
make container-test

# Coverage analysis
make container-coverage
```

The `go-test` target automatically installs `setup-envtest` and configures `KUBEBUILDER_ASSETS`. You do not need to set these manually. Tests exclude `vendor/` and `test/e2e/` directories automatically via the `TESTTARGETS` variable in `boilerplate/openshift/golang-osd-operator/standard.mk` if these directories are present.

FIPS mode is enabled (`FIPS_ENABLED=true`), which sets `-tags=fips_enabled` and `GOEXPERIMENT=boringcrypto`. Local `go test` invocations outside of `make` will not have these flags. Always use `make go-test` or `container-test`.

## Test File Placement

Place test files adjacent to the code they test, in the same package (not `_test` package). The only existing test file is `pkg/goalert/service_test.go` -- it uses `package goalert`, giving it access to unexported struct fields like `GraphqlClient.sessionCookie` and `GraphqlClient.httpClient`.

## Table-Driven Test Pattern

Most tests for GoAlert client methods use this structure. Follow it for new tests:

```go
func Test_MethodName(t *testing.T) {
    tests := []struct {
        name        string
        data        *Data
        expectedVal string      // expected return value(s)
        respData    []byte      // raw JSON the mock server returns
        expectedErr bool
    }{
        {
            name:        "Successful operation",
            data:        &Data{...},
            expectedVal: "expected-id",
            respData:    []byte(`{"data":{"mutationName":{"id":"expected-id"}}}`),
            expectedErr: false,
        },
        {
            name:        "Null response from API",
            data:        &Data{...},
            expectedVal: "",
            respData:    []byte(`{"data":{"mutationName":null}}`),
            expectedErr: false,
        },
        {
            name:        "Invalid JSON response",
            data:        &Data{...},
            expectedVal: "",
            respData:    []byte(`garbage`),
            expectedErr: true,
        },
    }

    for _, test := range tests {
        t.Run(test.name, func(t *testing.T) {
            // test body
        })
    }
}
```

Every method test should cover at minimum these three cases: success, null/empty API response, and unmarshal failure.

## Mocking the GoAlert HTTP API

Tests mock the GoAlert GraphQL API using `net/http/httptest`. The pattern:

1. Create an `httptest.NewServer` that returns `test.respData`.
2. Set `GOALERT_ENDPOINT_URL` to the test server's URL via `t.Setenv`.
3. Create a `GraphqlClient` with the test server's `*http.Client` and a dummy session cookie.

```go
mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    if _, err := w.Write(test.respData); err != nil {
        t.Fatalf("Unexpected error writing response from httptest server")
    }
}))
defer mockServer.Close()

t.Setenv(config.GoalertApiEndpointEnvVar, mockServer.URL)
mockClient := &GraphqlClient{
    sessionCookie: &http.Cookie{Name: "test_cookie"},
    httpClient:    mockServer.Client(),
}
```

Use `t.Setenv` (not `os.Setenv`) -- it automatically restores the original value after the test. The env var name is `config.GoalertApiEndpointEnvVar` (`GOALERT_ENDPOINT_URL`).

## The `goalert.Client` Interface

The `goalert.Client` interface exists explicitly for testability:

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

The controller holds `gclient func(sessionCookie *http.Cookie) goalert.Client` as a field, set in `SetupWithManager`. For controller-level tests, inject a mock implementation of this interface rather than using httptest. This avoids needing real HTTP auth flows.

## Assertion Library

Use `github.com/stretchr/testify/assert` for assertions. Conventions from existing tests:

- `assert.Equal(t, expected, actual)` for value comparisons.
- `assert.Nil(t, err)` for success paths.
- `assert.NotNil(t, err)` for error paths.

Prefer `testify/assert` for new code. Note that `Test_NewRequest` currently mixes `testify` assertions with raw `t.Errorf` calls; prefer consistent use of `testify` for new tests.

Use `t.Fatalf` (not `t.Errorf`) for errors inside `httptest` handlers -- a failure there means the test infrastructure is broken, not the code under test.

## Coverage Configuration

`.codecov.yml` ignores `**/mocks` and `**/zz_generated*.go`. Coverage status checks (project, patch, changes) are all disabled -- coverage is informational only and will not block PRs. The range is set to `20...100`.

## What Is NOT Tested (Coverage Gaps)

The following packages have zero test coverage and are priority areas for new tests:

| Package | What to test | Suggested approach |
|---|---|---|
| `controllers/goalertintegration/` | Reconcile loop, `handleCreate`, `handleDelete`, heartbeat checks | Use envtest with a fake `goalert.Client` implementation. The `gclient` field on `GoalertIntegrationReconciler` enables dependency injection. |
| `pkg/kube/` | `GenerateConfigMap`, `GenerateSyncSet`, `GenerateGoalertSecret` | Pure functions, straightforward unit tests. Verify returned object metadata and data fields. |
| `pkg/utils/` | `LoadSecretData` | Use a fake `client.Client` from controller-runtime (`fake.NewClientBuilder().WithObjects(...).Build()`). |
| `pkg/localmetrics/` | Prometheus metric updates and deletes | Call update functions, then read metric values using `prometheus.Collector` pattern. |
| `config/` | `Name()` function | Simple string concatenation; table-driven test with various inputs. |

## Controller Tests with envtest

For controller-level integration tests, use `sigs.k8s.io/controller-runtime/pkg/envtest` with the Hive CRDs. The `make go-test` target already sets up envtest assets. A test suite would:

1. Start an `envtest.Environment` with CRD paths pointing to `deploy/crds/goalert.managed.openshift.io_goalertintegrations.yaml` and Hive CRDs.
2. Register the GoalertIntegration and Hive schemes.
3. Create a `GoalertIntegrationReconciler` with a mock `gclient` function.
4. Test reconciliation by creating/deleting GoalertIntegration and ClusterDeployment objects.

## Writing New Tests Checklist

- [ ] Test file is in the same package as the code under test.
- [ ] Uses table-driven tests with `t.Run` for subtests.
- [ ] Covers success, empty/null response, and error/unmarshal-failure cases.
- [ ] Uses `t.Setenv` for environment variables, never `os.Setenv`.
- [ ] Uses `testify/assert` for assertions.
- [ ] Mock servers use `httptest.NewServer` and are cleaned up with `defer mockServer.Close()`.
- [ ] No test depends on external services (GoAlert endpoint, real clusters).
- [ ] Run `make go-test` before pushing; run `make container-test` to match CI.
