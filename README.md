# Node Maintenance Operator

node-maintenance-operator is an operator generated from the [operator-sdk](https://github.com/operator-framework/operator-sdk).
The purpose of this operator is to watch for new or deleted custom resources called NodeMaintenance which indicate that a node in the cluster should either:
  - NodeMaintenance CR created: move node into maintenance, cordon the node - set it as unschedulable and evict the pods (which can be evicted) from that node.
  - NodeMaintenance CR deleted: remove pod from maintenance and uncordon the node - set it as schedulable

> *Note*:  The current behavior of the  operator is to mimic `kubectl drain <node name>` as performed in [Kubevirt - evict all VMs and Pods on a node ](https://kubevirt.io/user-guide/docs/latest/administration/node-eviction.html#how-to-evict-all-vms-and-pods-on-a-node)

## Build and run the operator

There are three ways to run the operator:

- Deploy the latest version from master branch to a running Openshift/Kubernetes cluster
- Build and deploy from sources to a running or to be created Openshift/Kubernetes cluster
- As Go program outside a cluster

### Deploy the latest version

After every merge to master images were build and pushed to quay.io.
For deployment of NMO using these images you need

- a running Openshift cluster or a Kubernetes cluster with OLM (> v0.15.1) installed
- `oc` or `kubectl` binary installed and configured to access your cluster
- on Openshift, run these commands:
    - run `oc apply -f deploy/deployment-ocp/catalogsource.yaml`
    After this you can install NMO using the OperatorHub UI.
    If you want to install NMO via commandline, go on with:
    - run `oc apply -f deploy/deployment-ocp/namespace.yaml`
    - run `oc apply -f deploy/deployment-ocp/operatorgroup.yaml`
    - run `oc apply -f deploy/deployment-ocp/subscription.yaml`
- on Kubernetes, run these commands:
    - run `kubectl apply -f deploy/deployment-k8s/catalogsource.yaml`
    - run `kubectl apply -f deploy/deployment-k8s/namespace.yaml`
    - run `kubectl apply -f deploy/deployment-k8s/operatorgroup.yaml`
    - run `kubectl apply -f deploy/deployment-k8s/subscription.yaml`

### Build and deploy from sources

For more information on the [Operator Lifecycle
Manager](https://github.com/operator-framework/operator-lifecycle-manager) and
Cluster Service Versions checkout out ["Building a Cluster Service
Version"](https://github.com/operator-framework/operator-lifecycle-manager/blob/master/doc/design/building-your-csv.md)
and the [OLM Book](https://operator-framework.github.io/olm-book/).

Information about the "bundle" and "index" images can be found at [Operator Registry](https://github.com/operator-framework/operator-registry).

This project uses [KubeVirtCI](https://github.com/kubevirt/kubevirtci) for spinning up a cluster for development
and in CI. If you want to use an existing cluster, just run `export KUBEVIRT_PROVIDER=external` and ensure that
the `KUBECONFIG` env var points to your cluster configuration. In case the cluster is an OpenShift cluster the
operator will deployed directly on it. In case of a plain Kubernetes cluster OLM will be installed first.

#### Start the cluster

```shell
make cluster-up
```

> *Note:* This also needs be run for external clusters, it will setup some configuration

#### Build, push and deploy Node Maintenance Operator

```shell
# If you use an external cluster, define the image registry you want to use.
# KubeVirtCI clusters will use a registry running in the cluster itself, so you can skip the next line.
export IMAGE_REGISTRY=quay.io/<username>

make cluster-sync
```

This will execute several steps for you:
- generate manifests (`make make csv-generator`)
- build and push images (`make container-build container-push`)
- customize deployment manifests and place them into `_out/`
- deploy manifest in the cluster and wait until the deployment is ready
- for more details see `hack/sync.sh`

### Run locally outside the cluster

This method is preferred during development cycle to deploy and test faster.

Set the name of the operator in an environment variable:

```sh
export OPERATOR_NAME=node-maintenance-operator
```

Run the operator locally with the default Kubernetes config file present at `$HOME/.kube/config` or
with specificing kubeconfig via the flag `--kubeconfig=<path/to/kubeconfig>`:

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

To set maintenance on a node a `NodeMaintenance` CustomResource should be created.
The `NodeMaintenance` CR spec contains:
- nodeName: The name of the node which will be put into maintenance
- reason: the reason for the node maintenance

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

### Run unit test

`make test`

### Run e2e tests

1. Deploy the operator using OLM and KubeVirtCI as explained above
2. run `make cluster-functest`

## Next Steps
- Handle unremoved pods and daemonsets
- Check where should the operator be deployed (infra pods?)
- Check behavior for storage pods
- Fencing
- Versioning
- Enhance error handling
- Operator integration and packaging
