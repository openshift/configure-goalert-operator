# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

The Configure GoAlert Operator (CGAO) is a Kubernetes operator that automates integrating OpenShift clusters with GoAlert. It runs on Hive clusters and manages the creation/deletion of GoAlert services and integration keys for cluster monitoring.

## Architecture

The operator uses the controller-runtime framework and consists of:

- **GoalertIntegration Controller** (`controllers/goalertintegration/`): Main controller that watches GoalertIntegration CRs and ClusterDeployment CRs
- **API Types** (`api/v1alpha1/`): Kubernetes CRD definitions for GoalertIntegration
- **GoAlert Client** (`pkg/goalert/`): HTTP client for interacting with GoAlert GraphQL API
- **Kube Utils** (`pkg/kube/`): Kubernetes utilities for managing secrets and resources
- **Local Metrics** (`pkg/localmetrics/`): Prometheus metrics collection

### Key Components

- Creates high/low alert services in GoAlert for each installed cluster
- Generates integration keys and heartbeat endpoints
- Manages secrets containing GoAlert credentials in target clusters
- Handles cluster cleanup when ClusterDeployments are deleted

## Development Commands

### Building and Testing
```bash
# Run Go tests
make go-test

# Build operator image and push to registry
IMAGE_REPOSITORY=<your-quay-repo> REGISTRY_USER=<username> REGISTRY_TOKEN=<token> make docker-push

# Run Prow tests locally
make container-coverage    # Coverage test
make container-validate    # Validation test
make container-lint        # Lint test
make container-test        # Full test suite
```

### Testing with Live Cluster
1. Apply Hive CRDs from hive repo: `oc apply -f config/crds`
2. Create operator CRD: `oc create -f deploy/crds/goalert.managed.openshift.io_goalertintegrations.yaml`
3. Create namespace: `oc create ns configure-goalert-operator`
4. Deploy operator: `oc create -f deploy/`
5. Create GoAlert secret with base64-encoded USERNAME/PASSWORD

### Cleanup
```bash
oc delete -f deploy/
oc delete -f deploy/crds/goalert.managed.openshift.io_goalertintegrations.yaml
oc delete ns configure-goalert-operator
```

## Environment Variables

- `GOALERT_ENDPOINT_URL`: GoAlert instance endpoint for GraphQL API
- `KUBECONFIG`: Path to kubeconfig for target cluster access

## Important Notes

- Uses boilerplate framework for standardized OpenShift operator development
- FIPS-enabled builds required (`FIPS_ENABLED=true`)
- Operator only reconciles clusters with `spec.installed: true`
- ClusterDeployments should set `spec.preserveOnDelete: true` for testing to avoid AWS resource deletion