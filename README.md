# Node Maintenance Operator
node-maintenance-operator is an operator generated from the [operator-sdk](https://github.com/operator-framework/operator-sdk).
The purpose of this operator is to watch for new or deleted custom resources called NodeMaintenance which indicate that a node in the cluster should either:
 - NodeMaintenance CR created: move node into maintenance, cordon the node - set it as unschedulable and evict the pods (which can be evicted) from that node.
  - NodeMaintenance CR deleted: remove pod from maintenance and uncordon the node - set it as schedulable

> *Note*:  The current behavior of the  operator is to mimic `kubectl drain <node name>` as performed in [Kubevirt - evict all VMs and Pods on a node ](https://kubevirt.io/user-guide/docs/latest/administration/node-eviction.html#how-to-evict-all-vms-and-pods-on-a-node)

## Build and run the operator
Before running the operator, the NodeMaintenance CRD and namespace must be registered with the Openshift/Kubernetes apiserver:

```sh
$ kubectl create -f deploy/crds/nodemaintenance_crd.yaml
$ kubectl create -f deploy/namespace.yaml
```

Once this is done, there are two ways to run the operator:

- As a Deployment inside a Openshift/Kubernetes cluster
- As Go program outside a cluster

## Deploy operator using OLM

For more information on the [Operator Lifecycle
Manager](https://github.com/operator-framework/operator-lifecycle-manager) and
Cluster Service Versions checkout out ["Building a Cluster Service
Version"](https://github.com/operator-framework/operator-lifecycle-manager/blob/master/Documentation/design/building-your-csv.md).


1) Build and push operator and operator-registry image.

```shell
./build/make-manifests.sh <VERSION>
make container-build
make container-push
```

2) Create the node-maintenance-operator Namespace.

```shell
oc create -f olm-deploy-manifests/nm-ns.yaml

cat <<EOF | oc create -f -
apiVersion: v1
kind: Namespace
metadata:
  annotations:
  labels:
    kubevirt.io: ""
  name: node-maintenance-operator
EOF
```

3) Create the operator group.

```shell
oc create -f olm-deploy-manifests/nm-op-group.yaml

cat <<EOF | oc create -f -
apiVersion: operators.coreos.com/v1alpha2
kind: OperatorGroup
metadata:
  name: node-maintenance-operator
  namespace: node-maintenance-operator
EOF
```

4)  Using the `node-maintenance-operator-registry` container image built in step 1,
create a CatalogSource. This object tells OLM about the operator.

```shell
oc create -f olm-deploy-manifests/nm-catalog-source.yaml

cat <<EOF | oc create -f -
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: node-maintenance-operator
  namespace: openshift-marketplace
spec:
  sourceType: grpc
  image: quay.io/kubevirt/node-maintenance-operator-registry:<VERSION>
  displayName: node-maintenance-operator
  publisher: Red hat
EOF
```

5) Subscribe to the node-maintenance-operator.

```shell
oc create -f olm-deploy-manifests/nm-sub.yaml

cat <<EOF | oc create -f -
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: node-maintenance-operator-subscription
  namespace: node-maintenance-operator
spec:
  channel: beta
  name: node-maintenance-operator
  source: node-maintenance-operator
  sourceNamespace: openshift-marketplace
  startingCSV: node-maintenance-operator.<VERSION>
EOF
```

### 1. Run as a Deployment inside the cluster

The Deployment manifest is generated at `deploy/operator.yaml`. Be sure to update the deployment image if there are changes as shown [here](https://github.com/operator-framework/operator-sdk/blob/master/doc/user-guide.md#1-run-as-a-deployment-inside-the-cluster).

Setup RBAC and deploy the node-maintenance-operator:

```sh
$ kubectl create -f deploy/service_account.yaml
$ kubectl create -f deploy/role.yaml
$ kubectl create -f deploy/role_binding.yaml
$ kubectl create -f deploy/operator.yaml
```

Verify that the node-maintenance-operator is up and running:

```sh
$ kubectl get deployment
NAME                     DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
node-maintenance-operator      1         1         1            1           1m
```


### 2. Run locally outside the cluster

This method is preferred during development cycle to deploy and test faster.

Set the name of the operator in an environment variable:

```sh
export OPERATOR_NAME=node-maintenance-operator
```

Run the operator locally with the default Kubernetes config file present at `$HOME/.kube/config` or  with specificing kubeconfig via the flag `--kubeconfig=<path/to/kubeconfig>`:

```sh
$ operator-sdk up local --kubeconfig="<path/to/kubeconfig>"

INFO[0000] Running the operator locally.
INFO[0000] Using namespace default.
{"level":"info","ts":1551793839.3308277,"logger":"cmd","msg":"Go Version: go1.11.4"}
{"level":"info","ts":1551793839.3308823,"logger":"cmd","msg":"Go OS/Arch: linux/amd64"}
{"level":"info","ts":1551793839.330899,"logger":"cmd","msg":"Version of operator-sdk: v0.5.0+git"}
...

```

## Setting Node Maintenance
### Set Maintenance on - Create a NodeMaintenance CR
To set maintenance on a node a `NodeMaintenance` CR should be created.
A `NodeMaintenance` CR contains:
- Name: The name of the node which will be put into maintenance
- Reason: the reason for the node maintenance

Create the example `NodeMaintenance` CR found at `deploy/crds/nodemaintenance_cr.yaml`:

```sh
$ cat deploy/crds/nodemaintenance_cr.yaml

apiVersion: nodemaintenance.kubevirt.io/v1beta1
kind: NodeMaintenance
metadata:
  name: nodemaintenance-xyz
spec:
  nodeName: node02
  reason: "Test node maintenance"

$ kubectl apply -f deploy/crds/nodemaintenance_cr.yaml
{"level":"info","ts":1551794418.6742408,"logger":"controller_nodemaintenance","msg":"Reconciling NodeMaintenance","Request.Namespace":"default","Request.Name":"node02"}
{"level":"info","ts":1551794418.674294,"logger":"controller_nodemaintenance","msg":"Applying Maintenance mode on Node: node02 with Reason: Test node maintenance","Request.Namespace":"default","Request.Name":"node02"}
{"level":"info","ts":1551783365.7430992,"logger":"controller_nodemaintenance","msg":"WARNING: ignoring DaemonSet-managed Pods: default/local-volume-provisioner-5xft8, kubevirt/disks-images-provider-bxpc5, kubevirt/virt-handler-52kpr, openshift-monitoring/node-exporter-4c9jt, openshift-node/sync-8w5x8, openshift-sdn/ovs-kvz9w, openshift-sdn/sdn-qnjdz\n"}
{"level":"info","ts":1551783365.7471824,"logger":"controller_nodemaintenance","msg":"evicting pod \"virt-operator-5559b7d86f-2wsnz\"\n"}
{"level":"info","ts":1551783365.7472217,"logger":"controller_nodemaintenance","msg":"evicting pod \"cdi-operator-55b47b74b5-9v25c\"\n"}
{"level":"info","ts":1551783365.747241,"logger":"controller_nodemaintenance","msg":"evicting pod \"virt-api-7fcd86776d-652tv\"\n"}
{"level":"info","ts":1551783365.747243,"logger":"controller_nodemaintenance","msg":"evicting pod \"simple-deployment-1-m5qv9\"\n"}
{"level":"info","ts":1551783365.7472336,"logger":"controller_nodemaintenance","msg":"evicting pod \"virt-controller-8987cffb8-29w26\"\n"}
...
```

### Set Maintenance off - Delete the NodeMaintenance CR
To remove maintenance from a node a `NodeMaintenance` CR with the node's name  should be deleted.

```sh
$ cat deploy/crds/nodemaintenance_cr.yaml

apiVersion: nodemaintenance.kubevirt.io/v1beta1
kind: NodeMaintenance
metadata:
  name: nodemaintenance-xyz
spec:
  nodeName: node02
  reason: "Test node maintenance"

$ kubectl delete -f deploy/crds/nodemaintenance_cr.yaml

{"level":"info","ts":1551794725.0018933,"logger":"controller_nodemaintenance","msg":"Reconciling NodeMaintenance","Request.Namespace":"default","Request.Name":"node02"}
{"level":"info","ts":1551794725.0021605,"logger":"controller_nodemaintenance","msg":"NodeMaintenance Object: default/node02 Deleted ","Request.Namespace":"default","Request.Name":"node02"}
{"level":"info","ts":1551794725.0022023,"logger":"controller_nodemaintenance","msg":"uncordon Node: node02"}

```

## NodeMaintenance Status
The NodeMaintenance CR can contain the following status fields:

```yaml
apiVersion: nodemaintenance.kubevirt.io/v1beta1
kind: NodeMaintenance
metadata:
  name: nodemaintenance-xyz
spec:
  nodeName: node02
  reason: "Test node maintenance"
status:
  phase: "Running"
  lastError: "Last failure message"
  pendingPods: [pod-A,pod-B,pod-C]
  totalPods: 5
  evictionPods: 3

```

`phase` is the representation of the maintenance progress and can hold a string value of: Running|Succeeded.
The phase is updated for each processing attempt on the CR.

`lastError` represents the latest error if any for the latest reconciliation.

`pendingPods` PendingPods is a list of pending pods for eviction.

`totalPods` is the total number of all pods on the node from the start.

`evictionPods` is the total number of pods up for eviction from the start.

## Tests

Running e2e tests:

### Local
Before running the tests, the NodeMaintenance CRD and namespace must be registered with the Openshift/Kubernetes apiserver:

```sh
$ kubectl create -f deploy/crds/nodemaintenance_crd.yaml
$ kubectl create -f deploy/crds/namespace.yaml
```

Chang the `<IMAGE_VERSION>` under `deploy/operator.yaml` image link to the desired version:

```
image: quay.io/kubevirt/node-maintenance-operator:<IMAGE_VERSION>
```


Run the operator tests locally with the default Kubernetes config file present at `$HOME/.kube/config` or  with specificing kubeconfig via the flag `--kubeconfig=<path/to/kubeconfig>` and a namespace `--namespace=<namespace>`:


```sh
operator-sdk test local ./test/e2e --kubeconfig=<path/to/kubeconfig> --namespace="node-maintenance-operator"
```

## Next Steps
- Handle unremoved pods and daemonsets
- Check where should the operator be deployed (infra pods?)
- Check behavior for storage pods
- Fencing
- Versioning
- Enhance error handling
- Operator integration and packaging

