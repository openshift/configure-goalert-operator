# AGENTS.md

This file provides guidance to AI agents when working with code in this repository.

## Docs Index

Detailed domain-specific guidelines live in `docs/`. Consult these before making changes in their respective areas:

- **[Security Guidelines](docs/security-guidelines.md)** -- Credential handling, authentication flow, FIPS compliance, RBAC, injection prevention, container security, logging rules for secrets
- **[Error Handling Guidelines](docs/error-handling-guidelines.md)** -- Reconcile return conventions (`doNotRequeue`/`requeueOnErr`), K8s error patterns (NotFound, AlreadyExists), log-and-continue vs log-and-return, error wrapping, sentinel errors
- **[API Contracts Guidelines](docs/api-contracts-guidelines.md)** -- CRD schema, GraphQL mutations/queries, resource naming formula, ConfigMap/Secret/SyncSet data keys, Prometheus metrics contract
- **[Testing Guidelines](docs/testing-guidelines.md)** -- Table-driven test pattern, httptest mocking, assertion conventions, coverage gaps, envtest setup
- **[Integration Guidelines](docs/integration-guidelines.md)** -- GoAlert client usage, Hive dual-watch pattern, SyncSet propagation, finalizer lifecycle, heartbeat monitoring, resource creation/deletion order

## Review Exclusions

When reviewing code changes, exclude the following paths from findings. These files are maintained externally and reviewed separately:

- `.claude/hooks/` -- Claude Code hook scripts and their tests

## Project Overview

Configure GoAlert Operator (CGAO) automates integrating OpenShift clusters with GoAlert for alerting. It runs on Hive management clusters, watching `GoalertIntegration` CRs and `ClusterDeployment` CRs to create/delete GoAlert services and sync alert credentials to managed clusters via SyncSets.

## Build & Development Commands

All build infrastructure comes from [openshift/boilerplate](https://github.com/openshift/boilerplate). The Makefile is minimal -- it sets `FIPS_ENABLED=true`, exports `SKIP_SAAS_FILE_CHECKS=y`, and includes boilerplate targets.

```bash
# Build
make go-build                  # Build binary (FIPS-enabled)
make                           # Default: go-check + go-test + go-build

# Test
make go-test                   # Run unit tests (installs setup-envtest automatically)
make container-test            # Run tests in boilerplate container (prow-equivalent)
TESTTARGETS=./pkg/goalert/... make go-test  # Run tests for a single package

# Lint
make lint                      # Run golangci-lint + OLM YAML validation
make container-lint            # Lint in boilerplate container (prow-equivalent)

# Code Generation
make generate                  # Run all generators (controller-gen, openapi-gen, go generate)
make manifests                 # Generate CRD manifests only

# Validation & Coverage
make container-validate        # Ensure generated code is current (prow presubmit)
make container-coverage        # Coverage analysis in container

# Boilerplate
make boilerplate-update        # Update boilerplate to latest upstream
```

Use `container-*` variants to match CI behavior exactly. These run inside the boilerplate container image.

### Lint Configuration (Two Configs)

Two golangci-lint configs exist. Know which one applies:

- **`boilerplate/openshift/golang-osd-operator/golangci.yml`** -- Used by `make lint` / `make go-check`. This is the CI-authoritative config. It enables a minimal linter set: `errcheck`, `gosec`, `govet`, `ineffassign`, `misspell`, `staticcheck`, `unused`.
- **`.golangci.yaml`** (project root) -- A broader config enabling additional linters (`gocyclo`, `dupl`, `revive`, `bodyclose`, `gocritic`, etc.). This file is **not** used by `make lint` because the boilerplate Make target passes `-c` to specify its own config. It is only picked up if you run `golangci-lint run` directly without `-c`.

When adding `//nolint` directives, target the linters from the boilerplate config since those are the ones CI enforces.

## Architecture

### Package Layout

| Path | Purpose |
|---|---|
| `api/v1alpha1/` | CRD types (`GoalertIntegration`). See [API Contracts](docs/api-contracts-guidelines.md) for schema details. |
| `controllers/goalertintegration/` | Reconciler split across 5 files: main loop, create handler, delete handler, event handlers, heartbeat check. |
| `pkg/goalert/` | Raw HTTP/GraphQL client. Implements `Client` interface. See [Integration Guidelines](docs/integration-guidelines.md). |
| `pkg/kube/` | Helpers to build ConfigMap, Secret, and SyncSet resources. |
| `pkg/localmetrics/` | Prometheus gauges/histograms prefixed `cgao_`. See [API Contracts](docs/api-contracts-guidelines.md#prometheus-metrics-contract). |
| `pkg/utils/` | `LoadSecretData` helper for reading Secret keys (in `secrets.go`). |
| `config/` | Operator constants (names, secret keys, env vars, finalizer prefix) and the `Name()` function for resource naming. |
| `deploy/` | Kubernetes manifests including RBAC (ClusterRole), Deployment, ServiceMonitor. Used for OLM-based deployment. |
| `deploy_pko/` | Package Kubernetes Operator variant of deploy manifests. Alternative deployment path. |
| `hack/` | OLM artifact templates (including `GOALERT_ENDPOINT_URL` injection), PKO cluster package template, and boilerplate license header. |
| `build/` | Dockerfiles for the operator, OLM registry, and PKO images. |

### Key Dependencies

- Go `1.23.11` (from `go.mod`)
- `sigs.k8s.io/controller-runtime` v0.13.0
- `github.com/openshift/hive/apis` -- ClusterDeployment, SyncSet types
- `github.com/openshift/operator-custom-metrics` -- Custom metrics server (replaces controller-runtime metrics)
- `github.com/pingcap/errors` -- Legacy; used in `clusterdeployment_created.go` and `heartbeatmonitor_check.go` only
- GoAlert API endpoint configured via `GOALERT_ENDPOINT_URL` env var

### Reconciliation Flow (Summary)

1. Fetch GoalertIntegration CR
2. List all ClusterDeployments + those matching the GI's label selector
3. Authenticate to GoAlert (HTTP basic auth -> session cookie)
4. Check heartbeat monitors for matching CDs
5. Handle GI deletion: clean up all CDs with matching finalizer
6. Handle CD deletion / label un-match: delete GoAlert services
7. Handle CD creation: if ConfigMap/Secret/SyncSet don't exist, call `handleCreate`

For detailed create/delete ordering and cleanup logic, see [Integration Guidelines](docs/integration-guidelines.md).

### Two Deployment Paths

The operator supports two deployment mechanisms:

- **OLM** (`deploy/` + `hack/olm-artifacts-template*.yaml`): Traditional Operator Lifecycle Manager. The `deploy/04-operator.yaml` is a template -- `GOALERT_ENDPOINT_URL` and the container image are injected by OLM via the artifact templates in `hack/`.
- **PKO** (`deploy_pko/` + `build/Dockerfile.pko`): Package Kubernetes Operator. Has its own `manifest.yaml` with phase ordering (crds -> namespace -> rbac -> deploy -> cleanup). The PKO image is a `scratch`-based container holding only the manifests.

The `deploy/04-operator.yaml` does **not** include `GOALERT_ENDPOINT_URL` directly. This is intentional -- the env var is added during OLM/PKO template rendering. Do not add it to the base manifest.

## Cross-Cutting Conventions

### Constants over String Literals

All resource names, secret keys, environment variable names, and finalizer prefixes are defined as constants in `config/config.go`. Never use raw string literals for these values -- always reference the constants. This includes:
- `config.SecretName`, `config.ConfigMapSuffix`
- `config.GoalertHighIntKey`, `config.GoalertLowIntKey`, `config.GoalertHeartbeatIntKey`
- `config.GoalertUsernameSecretKey`, `config.GoalertPasswordSecretKey`
- `config.GoalertApiEndpointEnvVar`, `config.GoalertFinalizerPrefix`

**Exception:** ConfigMap data keys (`HIGH_SERVICE_ID`, `LOW_SERVICE_ID`, `HEARTBEATMONITOR_ID`) are raw strings in `pkg/kube/configmap.go` and the controller. These are not defined as constants. Follow the existing pattern, but consider extracting them to constants if adding new keys.

### Import Organization

Follow the existing three-block import style used throughout the codebase:
1. Standard library
2. Third-party / external (including `github.com/openshift/...`, `github.com/pingcap/...`, `github.com/go-logr/...`, `golang.org/x/...`)
3. Kubernetes libraries (`k8s.io/...`, `sigs.k8s.io/...`)

Note: `clusterdeployment_created.go` and `clusterdeployment_deleted.go` include the `//goland:noinspection SpellCheckingInspection` directive. Maintain this in those files but do not add it to new files unless needed.

### License Header

All non-generated Go files must include the Apache 2.0 license header from `hack/boilerplate.go.txt` (Copyright 2023). The `controller-gen` and `make generate` targets use this file automatically for generated code.

### Controller Reference Pattern

All secondary Kubernetes resources (ConfigMap, Secret, SyncSet) created by the operator must have `controllerutil.SetControllerReference(cd, resource, r.Scheme)` set to the ClusterDeployment, not the GoalertIntegration. This enables garbage collection when the CD is deleted.

### Logging Conventions

- Use `r.reqLogger` (scoped per reconcile) in controller methods. Use the package-level `log` variable only in event handlers where no reconciler receiver is available.
- Structured logging: `r.reqLogger.Error(err, "message", "key", value)`. Avoid using `%s` format verbs in messages -- put dynamic values in key-value pairs. Note: some existing log messages contain `%s` placeholders (e.g., `"Checking %s heartbeat monitor"`, `"Cluster %s in deletion"`), which is incorrect since logr does not perform fmt-style formatting. Do not replicate this pattern; use structured key-value pairs instead.
- Never log secret data. Service IDs and resource names are safe to log.

### Context Usage

- Reconciler methods receive a `ctx context.Context` parameter from the framework. Always pass this `ctx` to K8s API calls (`r.Get`, `r.Create`, etc.) and GoAlert client methods.
- Event handlers (`event_handlers.go`) do not receive a context parameter (the `handler.EventHandler` interface does not provide one). They use `context.TODO()` for K8s API calls. This is the expected pattern for this codebase.
- `pkg/goalert/` uses `golang.org/x/net/context`, not stdlib `context`. Maintain this import in that package for consistency. Controller code and new packages should use stdlib `context`.

### Error Import Inconsistency

- `clusterdeployment_created.go` and `heartbeatmonitor_check.go` import `github.com/pingcap/errors` for `IsAlreadyExists`/`IsNotFound`.
- `clusterdeployment_deleted.go` and `goalertintegration_controller.go` import `k8s.io/apimachinery/pkg/api/errors` (the standard K8s package).
- New code should use `k8s.io/apimachinery/pkg/api/errors`. The `pingcap/errors` usage is legacy.

### Generated Files -- Do Not Edit

These files are auto-generated and must not be hand-edited:
- `api/v1alpha1/zz_generated.deepcopy.go` -- regenerated by `make generate`
- `api/v1alpha1/zz_generated.openapi.go` -- regenerated by `make generate`
- `boilerplate/` directory -- managed by `make boilerplate-update`
- `OWNERS_ALIASES` -- managed by boilerplate

After changing CRD types in `api/v1alpha1/goalertintegration_types.go`, always run `make generate && make manifests`.

### Scheme Registration

Three schemes are registered in `main.go`: `clientgoscheme` (core K8s types), `hivev1` (Hive types), and `goalertv1alpha1` (operator CRD). If you add a new external type dependency, register its scheme in the `init()` function in `main.go`.

### Metrics Server

The operator uses `openshift/operator-custom-metrics` for Prometheus metrics, NOT controller-runtime's built-in metrics server. The controller-runtime `MetricsBindAddress` is set to `"0"` (disabled). Metrics are served on port `8080` at `/metrics`. New metrics must be appended to `localmetrics.MetricsList` for automatic registration.

### Dead Code

`event_handlers.go` defines `enqueueRequestForClusterDeploymentOwner` -- a second event handler type that maps owned resources back to ClusterDeployments, then to GoalertIntegrations. This type is **never registered** in `SetupWithManager` and is effectively dead code. Do not extend it or depend on it. If you need owner-reference-based watching, evaluate whether this dead code is the right approach or should be removed.

### Known Bugs

1. **GI deletion bulk cleanup bug:** In `goalertintegration_controller.go`, the GI deletion loop iterates over `matchingClusterDeployments.Items` but indexes into `allClusterDeployments.Items[i]`. This can process the wrong ClusterDeployment. See [Integration Guidelines](docs/integration-guidelines.md#three-deletion-triggers) for details.
2. **Auth failure continuation:** The controller logs auth/cookie errors but continues execution rather than returning early, risking nil-pointer panics on the session cookie. New code that depends on authentication should return early on auth failure.
3. **Inverted finalizer logic:** The `AddFinalizer`/`RemoveFinalizer` calls on the GoalertIntegration check for `!result` before calling `Update`, which means they update when no change was made.

## CI/CD

- **Tekton** pipelines in `.tekton/` for PR and push events (both standard and PKO variants). Pipelines use the Konflux `docker-build-oci-ta` pipeline definition. Images are pushed to `quay.io/redhat-user-workloads/fedramp-srep-tenant/`.
- **Boilerplate** container image (`quay.io/redhat-services-prod/openshift/boilerplate:image-v8.3.4`) provides the CI environment. All `container-*` Make targets run inside this image.
- **Codecov** configured (`.codecov.yml`) -- ignores `**/mocks` and `**/zz_generated*.go`; coverage checks are informational only and will not block PRs.
- **Dependabot** watches Docker image versions in `build/`.
- **Prow presubmits** (via boilerplate): `container-validate` ensures generated code is current, `container-lint` enforces lint, `container-test` runs tests.

## Owners

Maintained by the SREP FedRAMP team. See `OWNERS` / `OWNERS_ALIASES`.
