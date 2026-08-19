# Configure GoAlert Operator

The Configure GoAlert Operator (CGAO) is used to automate integrating OpenShift clusters with GoAlert. This operator is designed to run on Hive clusters.

[![codecov](https://codecov.io/gh/openshift/configure-goalert-operator/branch/main/graph/badge.svg)](https://codecov.io/gh/openshift/configure-goalert-operator)

> **⚠️ Disclaimer:** Portions of this code have been generated using AI.

## Documentation

In addition to this README, the repository includes detailed developer documentation:

- **[AGENTS.md](AGENTS.md)** -- Comprehensive developer reference covering architecture, conventions, known bugs, and CI/CD details
- **[Security Guidelines](docs/security-guidelines.md)** -- Credential handling, authentication flow, FIPS compliance, RBAC, container security
- **[Error Handling Guidelines](docs/error-handling-guidelines.md)** -- Reconcile return conventions, Kubernetes error patterns, error wrapping
- **[API Contracts Guidelines](docs/api-contracts-guidelines.md)** -- CRD schema, GraphQL mutations/queries, resource naming, Prometheus metrics
- **[Testing Guidelines](docs/testing-guidelines.md)** -- Table-driven test pattern, httptest mocking, assertion conventions, envtest setup
- **[Integration Guidelines](docs/integration-guidelines.md)** -- GoAlert client usage, Hive dual-watch pattern, SyncSet propagation, finalizer lifecycle

## Project Structure

| Package | Purpose |
|---|---|
| `api/v1alpha1/` | CRD types for `GoalertIntegration` |
| `controllers/goalertintegration/` | Reconciler (main loop, create/delete handlers, event handlers, heartbeat check) |
| `pkg/goalert/` | HTTP/GraphQL client implementing the `Client` interface (create, read, delete operations) |
| `pkg/kube/` | Helpers to build ConfigMap, Secret, and SyncSet resources |
| `pkg/localmetrics/` | Prometheus gauges/histograms (prefixed `cgao_`) |
| `pkg/utils/` | `LoadSecretData` helper for reading Secret keys |
| `config/` | Operator constants (resource names, secret keys, env vars, finalizer prefix) |
| `deploy/` | Kubernetes/OLM deployment manifests (RBAC, Deployment, ServiceMonitor) |
| `deploy_pko/` | Package Kubernetes Operator deployment manifests |
| `build/` | Dockerfiles for operator, OLM registry, and PKO images |

### Key Dependencies

- **Go 1.24.0** with FIPS-enabled builds (`FIPS_ENABLED=true`)
- **controller-runtime** v0.19.0 -- Kubernetes controller framework
- **openshift/hive** -- ClusterDeployment and SyncSet types
- **openshift/operator-custom-metrics** -- Prometheus metrics server
- **GoAlert** -- Upstream alerting platform, accessed via GraphQL at the URL in `GOALERT_ENDPOINT_URL`

## High level design

To onboard clusters:

* Create a GoalertIntegration CR
* Create a Goalert Integration controller that watches for changes to GoalertIntegration CRs, and also for changes to appropriately labeled ClusterDeployment CRs.
* For each GoalertIntegration CR, it will get a list of matching ClusterDeployments that have the `spec.installed` field set to true.
* For each of these ClusterDeployments, the operator will leverage GraphQL to ensure 2 services exist in Goalert (adopting pre-existing services by exact name match or creating new ones), one for high alerts and one for low alerts, each with the cluster UID as name
* For the high alerts service, the operator will ensure an integration key and a heartbeat api endpoint exist. The service will be attached to the SREP High alerts escalation policy
* For the low alerts service, the operator will ensure an integration key exists
* The operator will then create (or self-heal) a secret in the cluster namespace which contains the integration keys and heartbeat api endpoint required to communicate with Goalert Web application. The operator refuses to persist a secret with empty integration keys, requeuing until all keys are available.

To off-board clusters:

* Controller will check the finalizer on ClusterDeployments when a cluster gets deleted and will remove all services and alerts in Goalert and objects created by the operator.

## Development and Testing

Changes made to the operator should be tested and validated before a PR is submitted and approved.

Your test process should include:
* Validating the operator image builds and has no CVE's introduced
* Validating the operator runs and new features/bug fixes work
* Validating all tests, including Prow tests, pass

Much of this has been simplified through boilerplate, for the rest there are manifests and make commands!

**Build and push the operator image to your Quay repo**

```shell
# It is not recommended to set your password in plain text like this
# Use a password manager like `pass` or even `read -rs REGISTRY_TOKEN` first, the below is just an example to show what is needed
IMAGE_REPOSITORY=<YOUR_QUAY_REPO> REGISTRY_USER=<YOUR_QUAY_USERNAME> REGISTRY_TOKEN=<YOUR_QUAY_PASSWORD> make docker-push
```

**Deploy the operator to a test cluster**

Note: The below commands expect you to have a running cluster and access to it. If using backplane, you will most likely need to add the `--as backplane-cluster-admin` command flag

1. Create hive CRDs. To do so, clone [hive repo](https://github.com/openshift/hive/) and run
```shell
$ oc apply -f config/crds
```
2. Create the operator CRD: `oc create -f deploy/crds/goalert.managed.openshift.io_goalertintegrations.yaml`
3. Update the deployment for the image to run:
    ```shell
    # deploy/04-operator.yaml
    containers:
    - name: configure-goalert-operator
      image: <IMAGE_TAG>
    ```
4. Create the namespace: `oc create ns configure-goalert-operator`
5. Deploy the operator: `oc create -f deploy/`
6. Create test user in Goalert
7. Create secret with Goalert test user credentials
```shell
apiVersion: v1
data:
  USERNAME: bXktYXBpLWtleQ== #echo -n <username> | base64
  PASSWORD: aBCDefgLDBREAn==
kind: Secret
metadata:
  name: goalert-secret
  namespace: configure-goalert-operator
type: Opaque
```

**Create ClusterDeployment**

`configure-goalert-operator` doesn't start reconciling clusters until `spec.installed` is set to `true`.

You can create a dummy ClusterDeployment by copying a real one from an active hive. Warning: deleting the copied CD may trigger the deletion of the AWS objects of the real CD. Set the following spec to keep hive from deprovisioning the resources

```
spec:
  preserveOnDelete: true
```

```terminal
real-hive$ oc get cd -n <namespace> <cdname> -o yaml > /tmp/fake-clusterdeployment.yaml

...

$ oc create namespace fake-cluster-namespace
$ oc apply -f /tmp/fake-clusterdeployment.yaml
```

If present, set `spec.installed` to true.

```terminal
$ oc edit clusterdeployment fake-cluster -n fake-cluster-namespace
```

**Delete ClusterDeployment**

To trigger `configure-goalert-operator` to remove the service in Goalert, delete the clusterdeployment.

```terminal
$ oc delete clusterdeployment fake-cluster -n fake-cluster-namespace
```

You may need to remove dangling finalizers from the `clusterdeployment` object.

```terminal
$ oc edit clusterdeployment fake-cluster -n fake-cluster-namespace
```


**Remove the operator**
1. Delete the deployment and related manifests: `oc delete -f deploy/`
2. Delete the CRD: `oc delete -f deploy/crds/goalert.managed.openshift.io_goalertintegrations.yaml`
3. Delete the namespace: `oc delete ns configure-goalert-operator`


**Run Tests**
1) Go Tests: `make go-test`
2) Prow Coverage test: `make container-coverage`
3) Prow Validate test: `make container-validate`
4) Prow Lint test: `make container-lint`
5) Prow Test: `make container-test`

**Debugging**
If using [VScode](https://code.visualstudio.com/), you can use the following `launch.json` file with [delve](https://github.com/go-delve/delve/tree/master/Documentation/installation) installed for debugging the operator:
```json
{
    "version": "0.2.0",
    "configurations": [
        {
            "name": "configure-goalert-operator",
            "type": "go",
            "request": "launch",
            "mode": "auto",
            "program": "${workspaceFolder}/main.go",
            "env": {
              "GOALERT_ENDPOINT_URL": "<Goalert endpoint>",
              "KUBECONFIG": "<kubeconfig of cluster to run against>"
            },
            "args": []
          }
    ]
}
```
