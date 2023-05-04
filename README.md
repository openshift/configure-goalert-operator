# configure-goalert-operator

The configure-goalert-operator is used to automate integrating OpenShift clusters with GoAlert. This operator is designed
to run on Hive clusters.

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

## License

Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

