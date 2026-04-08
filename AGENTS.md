# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Configure GoAlert Operator (CGAO) automates integrating OpenShift clusters with GoAlert for alerting. It runs on Hive management clusters, watching `GoalertIntegration` CRs and `ClusterDeployment` CRs to create/delete GoAlert services and sync alert credentials to managed clusters via SyncSets.

## Build & Development Commands

All build infrastructure comes from [openshift/boilerplate](https://github.com/openshift/boilerplate). The Makefile is minimal — it sets `FIPS_ENABLED=true` and includes boilerplate targets.

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

## Architecture

### CRD: GoalertIntegration (`api/v1alpha1/`)

Single CRD with spec fields: `clusterDeploymentSelector` (label selector for which ClusterDeployments to integrate), `highEscalationPolicy`/`lowEscalationPolicy` (GoAlert policy IDs), `servicePrefix`, `targetSecretRef`, and `goalertCredsSecretRef`.

### Controller (`controllers/goalertintegration/`)

One reconciler split across multiple files:

- **`goalertintegration_controller.go`** — Main reconcile loop. Watches GoalertIntegration + ClusterDeployment CRs. Authenticates to GoAlert via HTTP basic auth, gets session cookie, then delegates to create/delete handlers.
- **`clusterdeployment_created.go`** — `handleCreate()`: Creates high/low GoAlert services, integration keys, heartbeat monitors, then creates a ConfigMap (service IDs), Secret (integration keys), and SyncSet (to replicate secret to target cluster).
- **`clusterdeployment_deleted.go`** — `handleDelete()`: Deletes GoAlert services using IDs stored in the ConfigMap, cleans up ConfigMap/Secret/SyncSet, removes finalizer.
- **`event_handlers.go`** — Custom event handler mapping ClusterDeployment events to GoalertIntegration reconcile requests.
- **`heartbeatmonitor_check.go`** — Checks heartbeat monitor state, updates Prometheus metrics.

### GoAlert Client (`pkg/goalert/`)

Raw HTTP/GraphQL client (no GraphQL library). Uses session cookie auth. Implements `Client` interface for testability. All mutations are hand-built GraphQL query strings.

### Supporting Packages

- **`pkg/kube/`** — Helpers to build ConfigMap and SyncSet resources.
- **`pkg/localmetrics/`** — Prometheus gauges/histograms prefixed `cgao_`.
- **`pkg/utils/`** — Secret data loading helper.
- **`config/`** — Operator constants (names, secret keys, env vars, finalizer prefix). The `Name()` function generates resource names as `{servicePrefix}-{clusterDeploymentName}{suffix}`.

### Key Dependencies

- `sigs.k8s.io/controller-runtime` v0.13.0
- `github.com/openshift/hive/apis` — ClusterDeployment, SyncSet types
- GoAlert API endpoint configured via `GOALERT_ENDPOINT_URL` env var

### Reconciliation Flow

1. Fetch GoalertIntegration CR
2. List all ClusterDeployments + those matching the GI's label selector
3. Authenticate to GoAlert (HTTP basic auth → session cookie)
4. Check heartbeat monitors for matching CDs
5. Handle GI deletion: clean up all CDs with matching finalizer
6. Handle CD deletion / label un-match: delete GoAlert services
7. Handle CD creation: if ConfigMap/Secret/SyncSet don't exist, call `handleCreate`

Finalizer pattern: `goalert.managed.openshift.io/goalert-{gi-name}` added to both GoalertIntegration and ClusterDeployment resources.

## Testing

Tests exist only for the GoAlert GraphQL client (`pkg/goalert/service_test.go`). They use `net/http/httptest` to mock the GoAlert API and `testify` for assertions.

Running the operator locally requires:
- `KUBECONFIG` pointing to a cluster with Hive CRDs installed
- `GOALERT_ENDPOINT_URL` env var set to a GoAlert instance
- A GoalertIntegration CR with valid credentials secret

## CI/CD

- **Tekton** pipelines in `.tekton/` for PR and push events
- **Boilerplate** container image `v8.3.4` provides the CI environment
- **Codecov** configured (`.codecov.yml`) — ignores `**/mocks` and `**/zz_generated*.go`
- **Dependabot** watches Docker image versions in `build/`

## Owners

Maintained by the SREP FedRAMP team. See `OWNERS` / `OWNERS_ALIASES`.
