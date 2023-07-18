# Configure GoAlert Operator

The Configure GoAlert Operator (CGAO) is used to automate integrating OpenShift clusters with GoAlert. This operator is designed to run on Hive clusters.

[![codecov](https://codecov.io/gh/openshift/configure-goalert-operator/branch/main/graph/badge.svg)](https://codecov.io/gh/openshift/configure-goalert-operator)

## High level design

To onboard clusters:

* Create a GoalertIntegration CR
* Create a Goalert Integration controller that watches for changes to GoalertIntegration CRs, and also for changes to appropriately labeled ClusterDeployment CRs (and ConfigMap/Secret/SyncSet resources owned by such a ClusterDeployment).
* For each GoalertIntegration CR, it will get a list of matching ClusterDeployments that have the `spec.installed` field set to true.
* For each of these ClusterDeployments, the operator will leverage GraphQL to create 2 new services in Goalert, one for high alerts and one for low alerts, each with the cluster UID as name
* For the high alerts service, the operator will make an api call to generate an integration key and a heartbeat api endpoint. The service will be attached to the SREP High alerts escalation policy
* For the low alerts service, the operator will only create a new integration key
* The operator will then create a secret in the cluster which contains the integration keys and heartbeat api endpoint required to communicate with Goalert Web application.

To off-board clusters:

* Controller will put a finalizer on ClusterDeployments when a cluster gets deleted and will delete remove all services and alerts in Goalert before removing the ClusterDeployment

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

**Deploy the operator to a cluster**

Note: The below commands expect you to have a running cluster and access to it. If using backplane, you will most likely need to add the `--as backplane-cluster-admin` command flag

1. Create the CRD: `oc create -f deploy/crds/goalert.managed.openshift.io_goalertintegrations.yaml`
2. Update the deployment for the image to run:
    ```shell
    # deploy/04-operator.yaml
    containers:
    - name: configure-goalert-operator
      image: <IMAGE_TAG>
    ```
3. Create the namespace: `oc create ns configure-goalert-operator`
3. Deploy the operator: `oc create -f deploy/`

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