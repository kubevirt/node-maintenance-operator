package e2e

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	coordv1beta1 "k8s.io/api/coordination/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operator "kubevirt.io/node-maintenance-operator/pkg/apis/nodemaintenance/v1beta1"
	nmo "kubevirt.io/node-maintenance-operator/pkg/controller/nodemaintenance"
)

var (
	retryInterval        = time.Second * 5
	timeout              = time.Second * 120
	cleanupRetryInterval = time.Second * 1
	cleanupTimeout       = time.Second * 5
	testDeployment       = "testdeployment"
	podLabel             = map[string]string{"test": "drain"}
)

func getCurrentOperatorPods() (*corev1.Pod, error) {

	// FIXME get the correct namespace dynamically, on openshift we will be in another one...
	pods, err := KubeClient.CoreV1().Pods("node-maintenance").List(context.Background(), metav1.ListOptions{LabelSelector: "name=node-maintenance-operator"})
	if err != nil {
		return nil, err
	}

	if pods.Size() == 0 {
		return nil, fmt.Errorf("There are no pods deployed in cluster to run the operator")
	}

	return &pods.Items[0], nil
}

func getOperatorLogs(t *testing.T) string {
	pod, err := getCurrentOperatorPods()
	if err != nil {
		t.Fatalf("showDeployment: can't get operator deployment error=%v", err)
		return ""
	}
	podName := pod.ObjectMeta.Name
	podLogOpts := corev1.PodLogOptions{}

	req := KubeClient.CoreV1().Pods(pod.Namespace).GetLogs(podName, &podLogOpts)
	podLogs, err := req.Stream(context.Background())
	if err != nil {
		t.Errorf("showDeployment: can't get log stream error=%v", err)
	}
	defer podLogs.Close()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, podLogs)
	if err != nil {
		t.Errorf("showDeployment: can't copy log stream error=%v", err)
	}
	return buf.String()
}

func showDeploymentStatus(t *testing.T, callerError error) {

	logs := getOperatorLogs(t)

	if callerError != nil {
		t.Fatalf("error: %v operator logs:\n%s", callerError, logs)
	} else {
		t.Logf("operator logs:\n%s", logs)
	}
}

func checkValidLease(t *testing.T, nodeName string) error {

	// FIXME this won't work: nmo.LeaseNamespace is overwritten by the operator during runtime, and we will never see that here...
	nName := types.NamespacedName{Namespace: nmo.LeaseNamespace, Name: nodeName}
	lease := &coordv1beta1.Lease{}
	err := Client.Get(context.TODO(), nName, lease)
	if err != nil {
		return fmt.Errorf("can't get lease node %s : %v", nodeName, err)
	}

	if lease.Spec.LeaseDurationSeconds == nil || *lease.Spec.LeaseDurationSeconds != int32(nmo.LeaseDuration.Seconds()) {
		return fmt.Errorf("checkValidLease wrong LeaseDurationInSeconds")
	}
	if lease.Spec.HolderIdentity == nil || *lease.Spec.HolderIdentity != nmo.LeaseHolderIdentity {
		return fmt.Errorf("checkValidLease wrong Spec.HolderIdentity")
	}

	if lease.Spec.RenewTime == nil {
		return fmt.Errorf("checkValidLease nil RenewTime")
	}

	timeNow := time.Now()
	if (*lease.Spec.RenewTime).Time.After(timeNow) {
		return fmt.Errorf("checkValidLease RenewTime in the future current time %s renew time %s", timeNow.Format(time.UnixDate), (*lease.Spec.RenewTime).Format(time.UnixDate))

	}
	tm := (*lease.Spec.RenewTime).Time
	tm = tm.Add(time.Duration(int64(*lease.Spec.LeaseDurationSeconds) * int64(time.Second)))
	if tm.Before(timeNow) {
		return fmt.Errorf("checkValidLease expiration time in the past time Now: %s expiration time %s :: leaseDuration %d renew time %s", timeNow.Format(time.UnixDate), tm.Format(time.UnixDate), *lease.Spec.LeaseDurationSeconds, (*lease.Spec.RenewTime).Time.Format(time.UnixDate))
	}

	return nil
}

func checkInvalidLease(t *testing.T, nodeName string) error {
	nName := types.NamespacedName{Namespace: nmo.LeaseNamespace, Name: nodeName}
	lease := &coordv1beta1.Lease{}
	err := Client.Get(context.TODO(), nName, lease)
	if err != nil {
		return fmt.Errorf("can't get lease node %s : %v", nodeName, err)
	}

	if lease.Spec.AcquireTime != nil {
		return fmt.Errorf("AcquireTime not nil %s", nodeName)
	}
	if lease.Spec.LeaseDurationSeconds != nil {
		return fmt.Errorf("LeaseDurationSeconds not nil %s", nodeName)
	}
	if lease.Spec.RenewTime != nil {
		return fmt.Errorf("RenewTime not nil %s", nodeName)
	}
	if lease.Spec.LeaseTransitions != nil {
		return fmt.Errorf("LeaseTransitions not nil %s", nodeName)
	}

	return nil
}

func getNodes(t *testing.T) ([]string, []string, error) {
	masters := make([]string, 0)
	workers := make([]string, 0)

	nodesList := &corev1.NodeList{}
	err := Client.List(context.TODO(), nodesList, &client.ListOptions{})
	if err != nil {
		showDeploymentStatus(t, fmt.Errorf("Failed to list nodes %v", err))
		return masters, workers, err
	}

	for _, node := range nodesList.Items {
		if _, exists := node.Labels["node-role.kubernetes.io/master"]; exists {
			masters = append(masters, node.Name)
		} else {
			workers = append(workers, node.Name)
		}
	}
	return masters, workers, nil
}

func enterAndExitMaintenanceMode(t *testing.T) error {
	namespace := os.Getenv("TEST_NAMESPACE")
	if len(namespace) == 0 {
		return fmt.Errorf("could not get namespace")
	}

	masters, workers, err := getNodes(t)
	if err != nil {
		t.Fatal(err)
	}
	if len(masters) == 0 {
		t.Fatal(fmt.Errorf("no master nodes found"))
	}
	if len(workers) == 0 {
		t.Fatal(fmt.Errorf("no worker nodes found"))
	}

	// first check master quorum validation
	nodeMaintenance := &operator.NodeMaintenance{
		TypeMeta: metav1.TypeMeta{
			Kind:       "NodeMaintenance",
			APIVersion: "nodemaintenance.kubevirt.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "nodemaintenance-",
		},
		Spec: operator.NodeMaintenanceSpec{
			Reason: "Set maintenance on node for e2e testing",
		},
	}
	for i, master := range masters {
		nodeMaintenance.Name += master
		nodeMaintenance.Spec.NodeName = master
		if len(masters) == 1 {
			// we have 1 master only
			// on Openshift the etcd-quorum-guard PDB should prevent setting maintenance
			// on k8s the fake etcd-quorum-guard PDB will for sure
			t.Logf("Validation test: create node maintenance CR for single master node")
			err = Client.Create(context.TODO(), nodeMaintenance)
			if err == nil {
				t.Errorf("FAIL: CR creation for single master node should have been rejected")
			} else if !strings.Contains(err.Error(), fmt.Sprintf(operator.ErrorMasterQuorumViolation)) {
				t.Errorf("FAIL: CR creation for single master node has been rejected with unexpected error message: %s", err.Error())
			}
		} else if len(masters) == 3 {
			if i == 0 {
				// first master node should be ok
				t.Logf("Validation test: create node maintenance CR for first master node")
				err = Client.Create(context.TODO(), nodeMaintenance)
				if err != nil {
					t.Errorf("FAIL: CR creation for first master node should have not have been rejected")
				}
			} else {
				t.Logf("Validation test: create node maintenance CR for 2nd and 3rd master node")
				err = Client.Create(context.TODO(), nodeMaintenance)
				if err == nil {
					t.Errorf("FAIL: CR creation for 2nd and 3rd master node should have been rejected")
				} else if !strings.Contains(err.Error(), fmt.Sprintf(operator.ErrorMasterQuorumViolation)) {
					t.Errorf("FAIL: CR creation for 2nd or 3rd master node has been rejected with unexpected error message: %s", err.Error())
				}
			}
		} else {
			t.Fatalf("unexpexted nr of master nodes, can't run master quorum validation test")
		}
	}

	err = createSimpleDeployment(t, namespace)
	if err != nil {
		t.Fatal(err)
	}

	maintenanceNodeName, err := getCurrentDeploymentNodeName(t)
	if err != nil {
		t.Fatal(err)
	}

	// reset CR for next tests
	nodeMaintenance = &operator.NodeMaintenance{
		TypeMeta: metav1.TypeMeta{
			Kind:       "NodeMaintenance",
			APIVersion: "nodemaintenance.kubevirt.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "nodemaintenance-xyz",
		},
		Spec: operator.NodeMaintenanceSpec{
			Reason: "Set maintenance on node for e2e testing",
		},
	}

	t.Logf("Validation test: create node maintenance CR for unexisting node")
	nodeMaintenance.Spec.NodeName = "doesNotExist"
	err = Client.Create(context.TODO(), nodeMaintenance)
	if err == nil {
		t.Errorf("FAIL: CR for unexisting node should have been rejected")
	} else if !strings.Contains(err.Error(), fmt.Sprintf(operator.ErrorNodeNotExists, "doesNotExist")) {
		t.Errorf("FAIL: CR creation for not existing node has been rejected with unexpected error message: %s", err.Error())
	}

	t.Logf("Putting node %s into maintanance", maintenanceNodeName)
	nodeMaintenance.Spec.NodeName = maintenanceNodeName
	err = Client.Create(context.TODO(), nodeMaintenance)
	if err != nil {
		t.Fatalf("Can't create CR: %v", err)
	}

	t.Logf("Validation test: update NodeName")
	nmNew := nodeMaintenance.DeepCopy()
	nmNew.Spec.NodeName = "test"
	err = Client.Patch(context.TODO(), nmNew, client.MergeFrom(nodeMaintenance), &client.PatchOptions{})
	if err == nil {
		t.Errorf("FAIL: Update of NodeName should have been rejected")
	} else if !strings.Contains(err.Error(), fmt.Sprintf(operator.ErrorNodeNameUpdateForbidden)) {
		t.Errorf("FAIL: CR update with new NodeName has been rejected with unexpected error message: %s", err.Error())
	}
	nodeMaintenance.Spec.NodeName = maintenanceNodeName

	t.Logf("Validation test: create NM for same node")
	nmNew = &operator.NodeMaintenance{
		TypeMeta: metav1.TypeMeta{
			Kind:       "NodeMaintenance",
			APIVersion: "nodemaintenance.kubevirt.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "nodemaintenance-new",
		},
		Spec: operator.NodeMaintenanceSpec{
			NodeName: maintenanceNodeName,
			Reason:   "Test duplicate maintenance",
		},
	}
	err = Client.Create(context.TODO(), nmNew)
	if err == nil {
		t.Errorf("FAIL: CR for node already in maintenance should have been rejected: %v", err)
	} else if !strings.Contains(err.Error(), fmt.Sprintf(operator.ErrorNodeMaintenanceExists, maintenanceNodeName)) {
		t.Errorf("FAIL: CR creation for node already in maintenance has been rejected with unexpected error message: %s", err.Error())
	}

	// Go on with maintenance tests
	// Get Running phase first
	if err := wait.PollImmediate(1*time.Second, 20*time.Second, func() (bool, error) {
		nm := &operator.NodeMaintenance{}
		err := Client.Get(context.TODO(), types.NamespacedName{Name: "nodemaintenance-xyz"}, nm)
		if err != nil {
			t.Logf("Failed to get %q nodeMaintenance: %v", nm.Name, err)
			return false, nil
		}

		//t.Logf("%s: operator running state: %s", time.Now().Format("2006-01-02 15:04:05.000000"), nm.Status.Phase)
		if nm.Status.Phase != operator.MaintenanceRunning {
			t.Logf("%s: Running", time.Now().Format("2006-01-02 15:04:05.000000"))
			return false, nil
		}
		return true, nil
	}); err != nil {
		showDeploymentStatus(t, fmt.Errorf("%s: Failed to verify running phase: %v", time.Now().Format("2006-01-02 15:04:05.000000"), err))
	}

	// Wait for operator log showing it reconciles with fixed duration
	// caused by drain, caused by termination graceperiod > drain timeout
	t.Log("Waiting for drain timeout log")
	if err = wait.PollImmediate(5*time.Second, 2*time.Minute, func() (bool, error) {
		logs := getOperatorLogs(t)
		if strings.Contains(logs, nmo.FixedDurationReconcileLog) {
			return true, nil
		}
		return false, nil
	}); err != nil {
		showDeploymentStatus(t, fmt.Errorf("%s: Didn't run into drain timeout: %v", time.Now().Format("2006-01-02 15:04:05.000000"), err))
	}

	// Wait for the maintanance operation to complete successfuly
	if err := wait.PollImmediate(5*time.Second, 120*time.Second, func() (bool, error) {
		nm := &operator.NodeMaintenance{}
		err := Client.Get(context.TODO(), types.NamespacedName{Name: "nodemaintenance-xyz"}, nm)
		if err != nil {
			t.Logf("Failed to get %q nodeMaintenance: %v", nm.Name, err)
			return false, nil
		}
		if nm.Status.Phase != operator.MaintenanceSucceeded {
			return false, nil
		}
		return true, nil
	}); err != nil {
		checkFailureStatus(t)
		showDeploymentStatus(t, fmt.Errorf("Failed to successfuly complete maintanance operation after defined test timeout (120s)"))
	}

	node := &corev1.Node{}
	err = Client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: maintenanceNodeName}, node)
	if err != nil {
		showDeploymentStatus(t, fmt.Errorf("Failed to get CRD after entering main. mode : %v", err))
	}

	if node.Spec.Unschedulable == false {
		checkFailureStatus(t)
		showDeploymentStatus(t, fmt.Errorf("Node %s should have been unschedulable ", maintenanceNodeName))
	}

	if !kubevirtTaintExist(node) {
		checkFailureStatus(t)
		showDeploymentStatus(t, fmt.Errorf("Node %s should have been tainted with kubevirt.io/drain:NoSchedule", maintenanceNodeName))
	}

	err = checkValidLease(t, maintenanceNodeName)
	if err != nil {
		showDeploymentStatus(t, fmt.Errorf("no valid lease after nmo completion: %v", err))
	}

	// if we have 2 workers or more, check the pod was moved to another node
	if len(workers) >= 2 {
		err = waitForDeployment(t, namespace, testDeployment, retryInterval, timeout)
		if err != nil {
			showDeploymentStatus(t, fmt.Errorf("failed to wait for deployment. error %v", err))
		}

		newNodeName, err := getCurrentDeploymentNodeName(t)
		if err != nil {
			showDeploymentStatus(t, err)
		}

		if newNodeName == maintenanceNodeName {
			showDeploymentStatus(t, fmt.Errorf("Deployment was done on node %s that should be under maintanence", maintenanceNodeName))
		}
	}

	t.Logf("Setting node %s out of maintanance", maintenanceNodeName)

	nodeMaintenanceDelete := &operator.NodeMaintenance{}

	err = Client.Get(context.TODO(), types.NamespacedName{Namespace: nodeMaintenance.Namespace, Name: nodeMaintenance.Name}, nodeMaintenanceDelete)
	if err != nil {
		showDeploymentStatus(t, fmt.Errorf("Failed to get CRD. error %v", err))
	}

	// Delete the node maintenance custom resource
	err = Client.Delete(context.TODO(), nodeMaintenanceDelete)
	if err != nil {
		showDeploymentStatus(t, fmt.Errorf("Could not delete node maintenance CR : %v", err))
	}

	time.Sleep(60 * time.Second)

	node = &corev1.Node{}
	err = Client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: maintenanceNodeName}, node)
	if err != nil {
		showDeploymentStatus(t, fmt.Errorf("can't get CRD. error %v", err))
	}

	if node.Spec.Unschedulable == true {
		showDeploymentStatus(t, fmt.Errorf("Node %s should have been schedulable", maintenanceNodeName))
	}

	if kubevirtTaintExist(node) {
		showDeploymentStatus(t, fmt.Errorf("Node %s kubevirt.io/drain:NoSchedule taint should have been removed", maintenanceNodeName))
	}

	err = checkInvalidLease(t, maintenanceNodeName)
	if err != nil {
		showDeploymentStatus(t, fmt.Errorf("valid lease after nmo completion %v", err))
	}

	// Check that the deployment has 1 replica running after maintenance is removed.
	t.Logf("%s: wait for deployment.", time.Now().Format("2006-01-02 15:04:05.000000"))
	err = waitForDeployment(t, namespace, testDeployment, retryInterval, timeout)
	if err != nil {
		showDeploymentStatus(t, fmt.Errorf("failed to wait for deployment: %v", err))
	}

	err = deleteSimpleDeployment(t, namespace)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("test deployment deleted")

	if t.Failed() {
		showDeploymentStatus(t, fmt.Errorf("some test(s) failed"))
	}

	return nil
}

func TestNodeMaintenance(t *testing.T) {

	if err := enterAndExitMaintenanceMode(t); err != nil {
		showDeploymentStatus(t, nil)
		t.Fatalf("failed to enter or exit maintenance mode. error %v", err)
	}

}

func deleteSimpleDeployment(t *testing.T, namespace string) error {

	deploymentToDelete := &appsv1.Deployment{}

	namespaceName := types.NamespacedName{Namespace: namespace, Name: testDeployment}
	err := Client.Get(context.TODO(), namespaceName, deploymentToDelete)
	if err != nil {
		t.Logf("Failed to get deploymten %v error %v", namespaceName, err)
		return err
	}

	err = Client.Delete(context.TODO(), deploymentToDelete)
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete  %v: %v", namespaceName, err)
	}

	return wait.PollImmediate(1*time.Second, 20*time.Second, func() (bool, error) {

		err = Client.Get(context.TODO(), namespaceName, deploymentToDelete)
		if err != nil {
			if errors.IsNotFound(err) {
				return true, nil
			}
			return false, fmt.Errorf("error encountered during deletion of deployment: %v", err)
		}
		return false, nil

	})
}

func createSimpleDeployment(t *testing.T, namespace string) error {
	dep := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      testDeployment,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointer.Int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: podLabel,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Labels:    podLabel,
				},
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "node-role.kubernetes.io/master",
												Operator: corev1.NodeSelectorOpDoesNotExist,
											},
										},
									},
								},
							},
						},
					},
					Containers: []corev1.Container{{
						Image:   "busybox",
						Name:    "testpodbusybox",
						Command: []string{"/bin/sh"},
						Args:    []string{"-c", "while true; do echo hello; sleep 10;done"},
					}},
					// make sure we run into the drain timeout at least once
					TerminationGracePeriodSeconds: pointer.Int64Ptr(int64(nmo.DrainerTimeout.Seconds()) + 10),
				},
			},
		},
	}

	// use TestCtx's create helper to create the object and add a cleanup function for the new object
	err := Client.Create(context.TODO(), dep)
	if err != nil {
		return err
	}
	// wait for testPodDeployment to reach 1 replicas
	err = waitForDeployment(t, namespace, testDeployment, retryInterval, timeout)
	if err != nil {
		return err
	}
	return nil
}

func getCurrentDeploymentPods(t *testing.T) (*corev1.PodList, error) {
	labelSelector := labels.SelectorFromSet(podLabel)
	pods := &corev1.PodList{}
	err := Client.List(context.TODO(), pods, &client.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, fmt.Errorf("Can't get test pods %v", err)
	}

	if pods.Size() == 0 {
		return nil, fmt.Errorf("There are no test pods deployed in cluster")
	}

	return pods, nil
}

func getCurrentDeploymentNodeName(t *testing.T) (string, error) {
	pods, err := getCurrentDeploymentPods(t)
	if err != nil {
		return "", err
	}
	if len(pods.Items) == 0 {
		return "", fmt.Errorf("no test pods running pods for test deployment")
	}
	nodeName := pods.Items[0].Spec.NodeName
	return nodeName, nil
}

func kubevirtTaintExist(node *corev1.Node) bool {
	kubevirtDrainTaint := corev1.Taint{
		Key:    "kubevirt.io/drain",
		Effect: corev1.TaintEffectNoSchedule,
	}
	taints := node.Spec.Taints
	for _, taint := range taints {
		if reflect.DeepEqual(taint, kubevirtDrainTaint) {
			return true
		}
	}
	return false
}

func checkFailureStatus(t *testing.T) {
	nm := &operator.NodeMaintenance{}
	err := Client.Get(context.TODO(), types.NamespacedName{Name: "nodemaintenance-xyz"}, nm)
	if err != nil {
		t.Logf("Failed to get %s nodeMaintenance: %v", nm.Name, err)
		return
	}
	if nm.Status.Phase != operator.MaintenanceRunning {
		t.Logf("Status.Phase on %s nodeMaintenance should have been %s", nm.Name, operator.MaintenanceRunning)
	}
	if len(nm.Status.LastError) == 0 {
		t.Logf("Status.LastError on %s nodeMaintenance should have a value", nm.Name)
	}
	if len(nm.Status.PendingPods) > 0 {
		pods, err := getCurrentDeploymentPods(t)
		if err != nil {
			t.Logf("Failed to get deployment pods %v", err)
		} else {
			if len(pods.Items) != 0 && pods.Items[0].Name != nm.Status.PendingPods[0] {
				t.Logf("Status.PendingPods on %s nodeMaintenance does not contain pod %s", nm.Name, pods.Items[0].Name)
			} else {
				t.Logf("no deployment pods found")
			}
		}
	}
}

func waitForDeployment(t *testing.T, namespace, name string, retryInterval, timeout time.Duration) error {
	err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		deployment, err := KubeClient.AppsV1().Deployments(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				t.Logf("Waiting for availability of Deployment: %s in Namespace: %s \n", name, namespace)
				return false, nil
			}
			return false, err
		}

		if int(deployment.Status.AvailableReplicas) >= 1 {
			return true, nil
		}
		t.Logf("Waiting for full availability of %s deployment (%d/%d)\n", name,
			deployment.Status.AvailableReplicas, 1)
		return false, nil
	})
	if err != nil {
		return err
	}
	t.Logf("Deployment available (%d/%d)\n", 1, 1)
	return nil
}
