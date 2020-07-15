package e2e

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"reflect"
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
	testIterations		 =  3
	testDeploymentReplicas = 20
)

func getCurrentOperatorPods() (*corev1.Pod, error) {

	pods, err := KubeClient.CoreV1().Pods("node-maintenance-operator").List(context.Background(), metav1.ListOptions{LabelSelector: "name=node-maintenance-operator"})
	if err != nil {
		return nil, err
	}

	if pods.Size() == 0 {
		return nil, fmt.Errorf("There are no pods deployed in cluster to run the operator")
	}

	return &pods.Items[0], nil
}

func showDeploymentStatus(t *testing.T, callerError error) {

	pod, err := getCurrentOperatorPods()
	if err != nil {
		t.Fatalf("showDeployment: can't get operator deployment error=%v", err)
		return
	}
	podName := pod.ObjectMeta.Name
	podLogOpts := corev1.PodLogOptions{}

	req := KubeClient.CoreV1().Pods("node-maintenance-operator").GetLogs(podName, &podLogOpts)
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
	str := buf.String()

	if callerError != nil {
		t.Fatalf("error: %v operator logs: %s", callerError, str)
	} else {
		t.Logf("operator logs %s", str)
	}
}

func TestNodeMainenance(t *testing.T) {
	// run subtests
	t.Run("NodeMaintenance-group", func(t *testing.T) {
		t.Run("Cluster", ClusterTest)
	})
}

func ClusterTest(t *testing.T) {
	if err := nodeMaintenanceTest(t); err != nil {
		t.Fatal(err)
	}
}

func  checkValidLease(t *testing.T, nodeName string) error {
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

	if lease.Spec.RenewTime == nil  {
		return fmt.Errorf("checkValidLease nil RenewTime")
	}

	timeNow := time.Now()
	if  (*lease.Spec.RenewTime).Time.After(timeNow)  {
		return fmt.Errorf("checkValidLease RenewTime in the future current time %s renew time %s", timeNow.Format(time.UnixDate), (*lease.Spec.RenewTime).Format(time.UnixDate))

	}
	tm := (*lease.Spec.RenewTime).Time
	tm = tm.Add( time.Duration(int64(*lease.Spec.LeaseDurationSeconds) * int64(time.Second))  )
	if  tm.Before(timeNow) {
		return fmt.Errorf("checkValidLease expiration time in the past time Now: %s expiration time %s :: leaseDuration %d renew time %s", timeNow.Format(time.UnixDate), tm.Format(time.UnixDate), *lease.Spec.LeaseDurationSeconds, (*lease.Spec.RenewTime).Time.Format(time.UnixDate))
	}

	return nil
}

func  checkInvalidLease(t *testing.T, nodeName string) error {
	nName := types.NamespacedName{Namespace: nmo.LeaseNamespace, Name: nodeName}
	lease := &coordv1beta1.Lease{}
	err := Client.Get(context.TODO(), nName, lease)
	if err != nil {
		return fmt.Errorf("can't get lease node %s : %v", nodeName, err)
	}

	if lease.Spec.AcquireTime != nil  {
		return fmt.Errorf("AcquireTime not nil %s", nodeName)
	}
	if lease.Spec.LeaseDurationSeconds  != nil {
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

func countNodes(t *testing.T) (int, string, error) {
	nodesList := &corev1.NodeList{}
	err := Client.List(context.TODO(), nodesList, &client.ListOptions{})
	if err != nil {
		showDeploymentStatus(t, fmt.Errorf("Failed to list nodes %v", err))
		return -1, "", err
	}

	computeNodesNumber := 0
	workerNodeName := ""

	for _, node := range nodesList.Items {
		if _, exists := node.Labels["node-role.kubernetes.io/master"]; !exists {
			computeNodesNumber++
			workerNodeName = node.ObjectMeta.Name
		}
	}
	return computeNodesNumber, workerNodeName, nil
}

func enterAndExitMaintenanceMode(t *testing.T) error {
	namespace := os.Getenv("TEST_NAMESPACE")
	if len(namespace) == 0 {
		return fmt.Errorf("could not get namespace")
	}

	computeNodesNumber, workerNodeName, err := countNodes(t)
	if err != nil {
		t.Fatal(err)
	}

	err = createSimpleDeployment(t, namespace, workerNodeName)
	if err != nil {
		t.Fatal(err)
	}

	nodeName, err := getCurrentDeploymentHostName(t)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Putting node %s into maintanance", nodeName)

	// Create node maintenance custom resource
	nodeMaintenance := &operator.NodeMaintenance{
		TypeMeta: metav1.TypeMeta{
			Kind:       "NodeMaintenance",
			APIVersion: "nodemaintenance.kubevirt.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "nodemaintenance-xyz",
		},
		Spec: operator.NodeMaintenanceSpec{
			NodeName: nodeName,
			Reason:   "Set maintenance on node for e2e testing",
		},
	}

	// Create node maintenance CR
	err = Client.Create(context.TODO(), nodeMaintenance)
	if err != nil {
		t.Fatalf("Can't create CRD: %v", err)
	}

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
	err = Client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: nodeName}, node)
	if err != nil {
		showDeploymentStatus(t, fmt.Errorf("Failed to get CRD after entering main. mode : %v", err))
	}

	if node.Spec.Unschedulable == false {
		checkFailureStatus(t)
		showDeploymentStatus(t, fmt.Errorf("Node %s should have been unschedulable ", nodeName))
	}

	if !kubevirtTaintExist(node) {
		checkFailureStatus(t)
		showDeploymentStatus(t, fmt.Errorf("Node %s should have been tainted with kubevirt.io/drain:NoSchedule", nodeName))
	}

	err = checkValidLease(t, nodeName)
	if err != nil {
		showDeploymentStatus(t, fmt.Errorf("no valid lease after nmo completion: %v", err))
	}

	if computeNodesNumber > 2 {
		err = waitForDeployment(t, namespace, testDeployment, 1, retryInterval, timeout)
		if err != nil {
			showDeploymentStatus(t, fmt.Errorf("failed to wait for deployment. error %v", err))
		}

		newNodeName, err := getCurrentDeploymentHostName(t)
		if err != nil {
			showDeploymentStatus(t, err)
		}

		if newNodeName == nodeName {
			showDeploymentStatus(t, fmt.Errorf("Deployment was done on node %s that should be under maintanence", nodeName))
		}
	}

	t.Logf("Setting node %s out of maintanance", nodeName)

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
	err = Client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: nodeName}, node)
	if err != nil {
		showDeploymentStatus(t, fmt.Errorf("can't get CRD. error %v", err))
	}

	if node.Spec.Unschedulable == true {
		showDeploymentStatus(t, fmt.Errorf("Node %s should have been schedulable", nodeName))
	}

	if kubevirtTaintExist(node) {
		showDeploymentStatus(t, fmt.Errorf("Node %s kubevirt.io/drain:NoSchedule taint should have been removed", nodeName) )
	}

	err = checkInvalidLease(t, nodeName)
	if err != nil {
		showDeploymentStatus(t, fmt.Errorf("valid lease after nmo completion %v", err))
	}

	// Check that the deployment has 1 replica running after maintenance is removed.
	t.Logf("%s: wait for deployment.", time.Now().Format("2006-01-02 15:04:05.000000"))
	err = waitForDeployment(t, namespace, testDeployment, 1, retryInterval, timeout)
	if err != nil {
		showDeploymentStatus(t, fmt.Errorf("%s: failed to wait for deployment. error %v.", err, time.Now().Format("2006-01-02 15:04:05.000000")))
	}

	err = deleteSimpleDeployment(t, namespace)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("test deployment deleted")

	return nil
}

func nodeMaintenanceTest(t *testing.T) error {

	for i:=0; i < testIterations; i+=1 {
		if err := enterAndExitMaintenanceMode(t); err != nil {
			t.Fatalf("failed to enter maintenance mode. error %v", err);
		}
	}
	showDeploymentStatus(t, nil)

	return nil
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

func createSimpleDeployment(t *testing.T, namespace string, nodeName string) error {
	replicas := rune(testDeploymentReplicas)
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
			Replicas: &replicas,
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
												Key:      "node-role.kubernetes.io/" + nodeName,
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
					NodeSelector: map[string]string{"kubernetes.io/hostname": nodeName},
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
	err = waitForDeployment(t, namespace, testDeployment, 1, retryInterval, timeout)
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

func getCurrentDeploymentHostName(t *testing.T) (string, error) {
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
			} else  {
				t.Logf("no deployment pods found")
			}
		}
	}
}

func waitForDeployment(t *testing.T, namespace, name string, replicas int, retryInterval, timeout time.Duration) error {
	err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		deployment, err := KubeClient.AppsV1().Deployments(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				t.Logf("Waiting for availability of Deployment: %s in Namespace: %s \n", name, namespace)
				return false, nil
			}
			return false, err
		}

		if int(deployment.Status.AvailableReplicas) >= replicas {
			return true, nil
		}
		t.Logf("Waiting for full availability of %s deployment (%d/%d)\n", name,
			deployment.Status.AvailableReplicas, replicas)
		return false, nil
	})
	if err != nil {
		return err
	}
	t.Logf("Deployment available (%d/%d)\n", replicas, replicas)
	return nil
}

