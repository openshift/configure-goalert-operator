# Testing Guidelines

## Running Tests

```bash
make go-test                                    # Run all unit tests (installs setup-envtest automatically)
TESTTARGETS=./pkg/goalert/... make go-test      # Run tests for a single package
make container-test                             # Run tests in boilerplate container (matches CI)
make container-coverage                         # Coverage analysis in container
```

The `go-test` target automatically installs `setup-envtest` and sets `KUBEBUILDER_ASSETS` for envtest-based controller tests. The envtest K8s version is pinned to `1.28.0` with `setup-envtest@release-0.23` in `boilerplate/openshift/golang-osd-operator/standard.mk`. Do not change these versions without updating boilerplate.

Pass extra flags via `TESTOPTS`: `TESTOPTS="-v -count=1" make go-test`.

## Test File Location

Tests live alongside the code they test in the same package (white-box testing). The only test file currently is `pkg/goalert/service_test.go`. There are no controller tests -- the `controllers/goalertintegration/` package has zero test files.

## Coverage Configuration

Coverage is configured in `.codecov.yml`:
- Status checks (project, patch, changes) are all disabled -- coverage will not block PRs
- Ignored paths: `**/mocks` and `**/zz_generated*.go`
- Coverage runs via `boilerplate/openshift/golang-osd-operator/codecov.sh`, which calls `go test -coverprofile -covermode=atomic -coverpkg=./...` and strips `zz_generated` lines from the profile

## Current Coverage Gaps

The following packages have zero test coverage and are the highest-priority areas for new tests:
- `controllers/goalertintegration/` -- all five files (main reconciler, create handler, delete handler, event handlers, heartbeat check)
- `pkg/kube/` -- `GenerateConfigMap`, `GenerateGoalertSecret`, `GenerateSyncSet`
- `pkg/utils/` -- `LoadSecretData`
- `pkg/localmetrics/` -- metric update and delete helpers

## Table-Driven Test Pattern

All tests use the standard Go table-driven pattern. Follow this structure exactly:

```go
tests := []struct {
    name        string
    data        *Data          // input to the method under test
    expectedID  string         // expected return value(s)
    respData    []byte         // raw JSON the mock server returns
    expectedErr bool           // whether an error is expected
}{
    {
        name:        "Successful createService",
        // ...
    },
}

for _, test := range tests {
    t.Run(test.name, func(t *testing.T) {
        // test body
    })
}
```

Rules:
- Each test case struct must have a `name` field used in `t.Run`
- New tests should include three scenario types per method: success, unsuccessful (valid response but empty/null data), and unmarshal failure (garbage response bytes). Some existing tests do not yet cover all three.
- The `respData` field holds the raw `[]byte` JSON the mock server will return -- this controls all behavior variation between test cases

## httptest Mock Server Pattern

The GoAlert API is mocked using `net/http/httptest`. Each test case creates its own server inside `t.Run`. The mock server URL is injected via `t.Setenv(config.GoalertApiEndpointEnvVar, mockServer.URL)`.

The standard mock setup:

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

Key conventions:
- Always use `t.Setenv` (not `os.Setenv`) so the env var is automatically restored after the test
- Construct a `GraphqlClient` struct directly (not via `NewClient`) to inject the mock server's `httpClient`
- The session cookie uses a dummy `Name: "test_cookie"` value
- Mock servers always return `http.StatusOK` -- error paths are tested via response body content (null JSON fields or garbage bytes), not HTTP status codes
- Use `t.Fatalf` for mock server write failures, not `t.Error` -- a write failure means the client won't receive a valid response, so continuing the test is meaningless

## Assertion Conventions

Tests use `github.com/stretchr/testify/assert` (already in `go.mod`). Follow these patterns:

- `assert.Equal(t, expected, actual)` for value comparison (expected first, actual second)
- `assert.Nil(t, err)` for success paths (not `assert.NoError`)
- `assert.NotNil(t, err)` for error paths (not `assert.Error`)
- The `Test_NewRequest` test uses raw `t.Errorf` and `reflect.DeepEqual` for byte slice comparison, but newer tests should use testify

Do not use `require` (the test file does not import `testify/require`). Tests should continue running after a non-fatal assertion failure.

## The Client Interface for Controller Tests

The `goalert.Client` interface in `pkg/goalert/service.go` exists specifically to enable controller-level testing. The reconciler stores its client factory as a field:

```go
type GoalertIntegrationReconciler struct {
    client.Client
    Scheme    *runtime.Scheme
    reqLogger logr.Logger
    gclient   func(sessionCookie *http.Cookie) goalert.Client
}
```

`gclient` is set to `goalert.NewClient` in `SetupWithManager` but can be overridden in tests to inject a mock `Client` implementation. When writing controller tests, provide a mock that implements the `Client` interface rather than standing up an httptest server.

## envtest Setup for Controller Tests

No controller tests exist yet. When adding them, register all three schemes that `main.go` registers:

```go
utilruntime.Must(clientgoscheme.AddToScheme(scheme))
utilruntime.Must(hivev1.AddToScheme(scheme))
utilruntime.Must(goalertv1alpha1.AddToScheme(scheme))
```

The `KUBEBUILDER_ASSETS` environment variable is set automatically by the `go-test` Makefile target. Do not hardcode asset paths.

## Nolint Directives

The test file uses `//nolint:dupl` on the `for` loop in `Test_CreateService` and `Test_CreateIntegrationKey` because the mock server setup is structurally identical across these tests. Preserve this directive when the duplication is intentional across different method tests.

## Test Naming Convention

Test functions use two styles -- both are acceptable:
- `Test_MethodName` (with underscore): `Test_NewRequest`, `Test_CreateService`, `Test_CreateIntegrationKey`, `Test_CreateHeartbeatMonitor`
- `TestMethodName` (no underscore): `TestDeleteService`

New tests should use `TestMethodName` (no underscore) to match standard Go conventions, but do not rename existing tests.

## Context Usage

All packages use stdlib `context`. Tests should use stdlib `context` as well.
